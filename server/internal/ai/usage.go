// 用量记账（WeKnora 借鉴 A1：LLM 调用可观测，B27 自上游 9ddd6e7 移植）。
//
// 设计要点：
//   - UsageRecorder 接口挂在 ai.Gateway / OpenAICompat 上，未注入（nil）时零开销、不记账；
//   - UsageDB 是默认 SQLite 实现：异步落库、失败静默（绝不阻塞主流程）；
//   - llm_usage 表由本模块自建（CREATE TABLE IF NOT EXISTS），缺模块/未注入即完全无痕迹；
//   - 账本口径：一次模型调用一条记录（provider/cap/model/tokens/耗时/成败），是后续
//     token 成本核算与计费的前置数据源。
package ai

import (
	"context"
	"database/sql"
	"log"
	"strconv"
	"time"
)

// Usage OpenAI 兼容响应中的用量（chat/completions 与 embeddings 均带）。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// UsageRecord 一次模型调用的记账记录。
type UsageRecord struct {
	Provider         string // ai.llm / ai.embedding / ...
	Cap              string // llm / embedding / rerank / ...
	Model            string
	PromptTokens     int
	CompletionTokens int
	LatencyMs        int64
	OK               bool
	Err              string
	Caller           string // 可选调用来源（chat/summarize/agent…），默认空
}

// UsageRecorder 用量落库接口。nil 表示不记账。
type UsageRecorder interface {
	Record(ctx context.Context, u UsageRecord)
}

// UsageDB 默认 SQLite 实现：异步写 llm_usage，失败仅打日志不阻塞。
type UsageDB struct {
	db *sql.DB
}

// NewUsageDB 创建记账器并确保表存在（幂等）。
func NewUsageDB(db *sql.DB) *UsageDB {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS llm_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts TEXT NOT NULL,
		provider TEXT NOT NULL DEFAULT '',
		cap TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		prompt_tokens INTEGER NOT NULL DEFAULT 0,
		completion_tokens INTEGER NOT NULL DEFAULT 0,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		ok INTEGER NOT NULL DEFAULT 1,
		err TEXT NOT NULL DEFAULT '',
		caller TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		log.Printf("usage: create table failed: %v", err)
		return nil
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_llm_usage_ts ON llm_usage(ts)`)
	if err != nil {
		log.Printf("usage: create index failed: %v", err)
	}
	return &UsageDB{db: db}
}

// Record 异步落库（带 5s 超时保护，失败静默）。
func (u *UsageDB) Record(ctx context.Context, rec UsageRecord) {
	if u == nil || u.db == nil {
		return
	}
	go func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ok := 1
		if !rec.OK {
			ok = 0
		}
		_, err := u.db.ExecContext(c,
			`INSERT INTO llm_usage(ts, provider, cap, model, prompt_tokens, completion_tokens, latency_ms, ok, err, caller)
			 VALUES(?,?,?,?,?,?,?,?,?,?)`,
			time.Now().UTC().Format(time.RFC3339), rec.Provider, rec.Cap, rec.Model,
			rec.PromptTokens, rec.CompletionTokens, rec.LatencyMs, ok, rec.Err, rec.Caller)
		if err != nil {
			log.Printf("usage: record failed: %v", err)
		}
	}()
}

// UsageSummary 管理端聚合查询结果行。
type UsageSummary struct {
	Provider         string `json:"provider"`
	Cap              string `json:"cap"`
	Model            string `json:"model"`
	Calls            int    `json:"calls"`
	Failed           int    `json:"failed"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	AvgLatencyMs     int64  `json:"avg_latency_ms"`
	LastTs           string `json:"last_ts"`
}

// SummarizeUsage 按 provider/cap/model 聚合近 days 天用量（管理端账本）。
func (u *UsageDB) SummarizeUsage(ctx context.Context, days int) ([]UsageSummary, error) {
	if u == nil || u.db == nil {
		return nil, nil
	}
	if days <= 0 {
		days = 7
	}
	rows, err := u.db.QueryContext(ctx, `
		SELECT provider, cap, model,
		       COUNT(*) AS calls,
		       SUM(CASE WHEN ok=0 THEN 1 ELSE 0 END) AS failed,
		       SUM(prompt_tokens) AS pt,
		       SUM(completion_tokens) AS ct,
		       AVG(latency_ms) AS avg_ms,
		       MAX(ts) AS last_ts
		FROM llm_usage
		WHERE ts >= datetime('now', ?)
		GROUP BY provider, cap, model
		ORDER BY calls DESC`, "-"+strconv.Itoa(days)+" days")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UsageSummary, 0, 16)
	for rows.Next() {
		var s UsageSummary
		var avg float64
		if err := rows.Scan(&s.Provider, &s.Cap, &s.Model, &s.Calls, &s.Failed,
			&s.PromptTokens, &s.CompletionTokens, &avg, &s.LastTs); err != nil {
			return nil, err
		}
		s.AvgLatencyMs = int64(avg)
		out = append(out, s)
	}
	return out, rows.Err()
}
