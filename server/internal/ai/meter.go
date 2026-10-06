// 用量管控与增值计费（meter.go）。
//
// 解决的问题：站长不设自备模型（custom）时，读者/用户消耗的是平台侧
// provider 的 key —— 需要两道闸：
//  1. 配额（quota）：按主体（登录用户 / 匿名公开访客）设每日调用次数与
//     token 上限，settings 热生效（ai.quota.*）；
//  2. 增值 token 余额（billing）：开启后平台模型调用按 token 扣余额，
//     余额不足即拦截（ai.billing.enabled）；站长后台可充值/赠送，
//     流水落 ai_token_ledger（仅人工增减，用量消耗不逐条记流水，
//     消耗明细走 llm_usage + ai_usage_daily 聚合）。
//
// 口径：
//   - 主体 subject：登录用户 = "user:<uid>"（authmw 注入 ctx）；匿名 = "public"。
//   - 平台模型判定：ai.<cap>.provider != "custom" 即平台模型（自备不计量不限制）。
//   - 计量 token = prompt+completion；TTS/ASR 等无 usage 的调用计次数、0 token。
//   - 余额扣减下限 0（并发下的超扣允许少量穿透，按 min(余额, tokens) 收口）。
//
// 收口点：newReq 前查（checkGate），recordUsage 后扣（charge）—— handler 零侵入。
package ai

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// ---- 主体（subject）----

type subjectKey struct{}

// WithSubject 把计量主体注入 ctx（authmw 对登录用户调用）。
func WithSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, subjectKey{}, subject)
}

// SubjectFrom 取计量主体；匿名/未注入时为 "public"（公开访客）。
func SubjectFrom(ctx context.Context) string {
	if v, ok := ctx.Value(subjectKey{}).(string); ok && v != "" {
		return v
	}
	return "public"
}

// SiteSubject 站点池主体（B38）：应用中心「平台 AI 供给包」的充值落点，全站共享额度。
// 扣费语义：调用主体余额优先，不足部分由站点池兜底（见 charge）；两处余额均 ≤0 才拦截
// （见 checkGate）。没有站点池记录时行为与旧口径完全一致。
const SiteSubject = "site"

// ---- 错误 ----

// QuotaError 配额/余额拦截（调用方按需映射 402/403，当前默认 502 透传文案）。
type QuotaError struct{ Msg string }

func (e *QuotaError) Error() string { return e.Msg }

// ---- Meter ----

// Meter 用量管控器：nil 时全链路零开销（不查、不记、不扣）。
type Meter struct {
	db *sql.DB
}

// NewMeter 创建管控器并确保表存在（幂等；失败返回 nil = 管控整体停用）。
func NewMeter(db *sql.DB) *Meter {
	if db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ai_usage_daily (
			subject     TEXT NOT NULL,
			day         TEXT NOT NULL,               -- UTC YYYY-MM-DD
			calls       INTEGER NOT NULL DEFAULT 0,
			tokens      INTEGER NOT NULL DEFAULT 0,
			updated_at  INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (subject, day)
		)`,
		`CREATE TABLE IF NOT EXISTS ai_token_balance (
			subject     TEXT PRIMARY KEY,
			balance     INTEGER NOT NULL DEFAULT 0,  -- 剩余 token
			updated_at  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS ai_token_ledger (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			ts            INTEGER NOT NULL,
			subject       TEXT NOT NULL,
			delta         INTEGER NOT NULL,          -- 正=充值/赠送 负=人工扣减
			balance_after INTEGER NOT NULL,
			reason        TEXT NOT NULL DEFAULT '',
			operator      TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_token_ledger_sub ON ai_token_ledger(subject, id DESC)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			log.Printf("meter: ensure table failed: %v", err)
			return nil
		}
	}
	return &Meter{db: db}
}

// ---- 配额策略（settings 热生效）----

// QuotaPolicy 管控策略视图（0 = 不限）。
type QuotaPolicy struct {
	UserDailyCalls    int  `json:"user_daily_calls"`
	UserDailyTokens   int  `json:"user_daily_tokens"`
	PublicDailyCalls  int  `json:"public_daily_calls"`
	PublicDailyTokens int  `json:"public_daily_tokens"`
	BillingEnabled    bool `json:"billing_enabled"`
}

// QuotaPolicy 从配置读当前策略（cfg 为内存缓存，读无锁开销可忽略）。
func (g *Gateway) QuotaPolicy() QuotaPolicy {
	return QuotaPolicy{
		UserDailyCalls:    g.cfg.GetInt("ai.quota.user_daily_calls"),
		UserDailyTokens:   g.cfg.GetInt("ai.quota.user_daily_tokens"),
		PublicDailyCalls:  g.cfg.GetInt("ai.quota.public_daily_calls"),
		PublicDailyTokens: g.cfg.GetInt("ai.quota.public_daily_tokens"),
		BillingEnabled:    g.cfg.GetString("ai.billing.enabled") == "true",
	}
}

// IsPlatform 判定该能力当前是否走平台模型（自备 custom = false）。
func (g *Gateway) IsPlatform(cap string) bool {
	return g.cfg.GetString("ai."+cap+".provider") != "custom"
}

// ---- 前置闸（newReq 调用）----

// checkGate 调用前校验：平台模型才受配额与余额约束。
// 每 upstream 请求查一次（TTS 分块流式时逐块查——分块即真实 upstream 调用）。
func (g *Gateway) checkGate(ctx context.Context, cap string) error {
	if g.meter == nil || !g.IsPlatform(cap) {
		return nil
	}
	pol := g.QuotaPolicy()
	subj := SubjectFrom(ctx)
	isPublic := subj == "public"

	calls, tokens, err := g.meter.dailyUsage(subj, utcDay())
	if err != nil {
		return nil // 记账故障不阻断业务（fail-open）
	}
	dailyCalls := pol.UserDailyCalls
	dailyTokens := pol.UserDailyTokens
	who := "用户"
	if isPublic {
		dailyCalls = pol.PublicDailyCalls
		dailyTokens = pol.PublicDailyTokens
		who = "公开访客"
	}
	if dailyCalls > 0 && calls >= dailyCalls {
		return &QuotaError{Msg: fmt.Sprintf("%s AI 调用次数已达今日上限（%d 次），明日恢复或由站长调整配额", who, dailyCalls)}
	}
	if dailyTokens > 0 && tokens >= dailyTokens {
		return &QuotaError{Msg: fmt.Sprintf("%s AI token 已达今日上限（%d），明日恢复或由站长调整配额", who, dailyTokens)}
	}
	if pol.BillingEnabled {
		// 站点池兜底（B38）：主体无余额但站点池有额度（应用中心平台供给包）→ 放行
		if g.meter.Balance(subj) <= 0 && g.meter.Balance(SiteSubject) <= 0 {
			return &QuotaError{Msg: "平台模型 token 余额不足，请联系站长充值，或在设置中改用自备模型"}
		}
	}
	return nil
}

// ---- 后置扣费（recordUsage 调用）----

// charge 一次 upstream 调用后的记账：日聚合 +（计费开启时）扣余额。
// billing 由 Gateway 判定（QuotaPolicy().BillingEnabled && IsPlatform(cap)）后透传。
func (m *Meter) charge(subject string, tokens int, billing bool) {
	if m == nil || m.db == nil {
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	day := utcDay()
	now := time.Now().Unix()
	if _, err := m.db.ExecContext(c, `
		INSERT INTO ai_usage_daily(subject, day, calls, tokens, updated_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(subject, day) DO UPDATE SET
			calls = calls + 1,
			tokens = tokens + excluded.tokens,
			updated_at = excluded.updated_at`,
		subject, day, 1, tokens, now); err != nil {
		log.Printf("meter: daily aggregate failed: %v", err)
	}
	// 余额扣减：计费开启且确有 token 消耗才动账
	if tokens <= 0 || !billing {
		return
	}
	var bal int
	err := m.db.QueryRowContext(c, `SELECT balance FROM ai_token_balance WHERE subject=?`, subject).Scan(&bal)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("meter: balance read failed: %v", err)
		return
	}
	if err == sql.ErrNoRows {
		bal = 0
	}
	deduct := tokens
	if deduct > bal { // 下限 0：允许少量并发穿透，不产生负余额
		deduct = bal
	}
	if deduct > 0 {
		if _, err := m.db.ExecContext(c,
			`UPDATE ai_token_balance SET balance = balance - ?, updated_at = ? WHERE subject = ?`,
			deduct, now, subject); err != nil {
			log.Printf("meter: balance deduct failed: %v", err)
		}
	}
	// 站点池兜底（B38）：主体余额不足的差额从站点池扣（应用中心平台供给包，全站共享）。
	if rest := tokens - deduct; rest > 0 {
		var sbal int
		err := m.db.QueryRowContext(c, `SELECT balance FROM ai_token_balance WHERE subject=?`, SiteSubject).Scan(&sbal)
		if err != nil && err != sql.ErrNoRows {
			log.Printf("meter: site balance read failed: %v", err)
			return
		}
		if err == sql.ErrNoRows {
			sbal = 0
		}
		d2 := rest
		if d2 > sbal {
			d2 = sbal
		}
		if d2 > 0 {
			if _, err := m.db.ExecContext(c,
				`UPDATE ai_token_balance SET balance = balance - ?, updated_at = ? WHERE subject = ?`,
				d2, now, SiteSubject); err != nil {
				log.Printf("meter: site balance deduct failed: %v", err)
			}
		}
	}
}

// ---- 查询 / 管理操作（handler 用）----

// dailyUsage 某主体当日（UTC）调用次数与 token。
func (m *Meter) dailyUsage(subject, day string) (calls, tokens int, err error) {
	if m == nil || m.db == nil {
		return 0, 0, nil
	}
	err = m.db.QueryRow(
		`SELECT calls, tokens FROM ai_usage_daily WHERE subject=? AND day=?`,
		subject, day).Scan(&calls, &tokens)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	return
}

// Balance 主体当前余额（无记录 = 0）。
func (m *Meter) Balance(subject string) int {
	if m == nil || m.db == nil {
		return 0
	}
	var bal int
	err := m.db.QueryRow(`SELECT balance FROM ai_token_balance WHERE subject=?`, subject).Scan(&bal)
	if err != nil {
		return 0
	}
	return bal
}

// Grant 人工充值/扣减（delta 可负），返回新余额。记流水。
func (m *Meter) Grant(subject string, delta int, reason, operator string) (int, error) {
	if m == nil || m.db == nil {
		return 0, fmt.Errorf("meter disabled")
	}
	now := time.Now().Unix()
	if _, err := m.db.Exec(`
		INSERT INTO ai_token_balance(subject, balance, updated_at) VALUES(?, ?, ?)
		ON CONFLICT(subject) DO UPDATE SET balance = balance + ?, updated_at = ?`,
		subject, delta, now, delta, now); err != nil {
		return 0, err
	}
	bal := m.Balance(subject)
	if _, err := m.db.Exec(
		`INSERT INTO ai_token_ledger(ts, subject, delta, balance_after, reason, operator) VALUES(?,?,?,?,?,?)`,
		now, subject, delta, bal, reason, operator); err != nil {
		return bal, err
	}
	return bal, nil
}

// BalanceRec 余额列表行。
type BalanceRec struct {
	Subject   string `json:"subject"`
	Balance   int    `json:"balance"`
	UpdatedAt int64  `json:"updated_at"`
}

// Balances 余额列表（按更新时间倒序）。
func (m *Meter) Balances(limit int) ([]BalanceRec, error) {
	if m == nil || m.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := m.db.Query(
		`SELECT subject, balance, updated_at FROM ai_token_balance ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BalanceRec{}
	for rows.Next() {
		var r BalanceRec
		if err := rows.Scan(&r.Subject, &r.Balance, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LedgerRec 流水行。
type LedgerRec struct {
	ID          int64  `json:"id"`
	TS          int64  `json:"ts"`
	Subject     string `json:"subject"`
	Delta       int    `json:"delta"`
	BalanceAfter int   `json:"balance_after"`
	Reason      string `json:"reason"`
	Operator    string `json:"operator"`
}

// Ledger 主体流水（id 倒序）。
func (m *Meter) Ledger(subject string, limit int) ([]LedgerRec, error) {
	if m == nil || m.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := m.db.Query(
		`SELECT id, ts, subject, delta, balance_after, reason, operator
		 FROM ai_token_ledger WHERE subject=? ORDER BY id DESC LIMIT ?`, subject, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LedgerRec{}
	for rows.Next() {
		var r LedgerRec
		if err := rows.Scan(&r.ID, &r.TS, &r.Subject, &r.Delta, &r.BalanceAfter, &r.Reason, &r.Operator); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UsageDailyRec 日用量行（管理面板展示）。
type UsageDailyRec struct {
	Subject string `json:"subject"`
	Day     string `json:"day"`
	Calls   int    `json:"calls"`
	Tokens  int    `json:"tokens"`
}

// TodayUsage 近 n 天按主体聚合（n=1 即今日）。
func (m *Meter) TodayUsage(days int) ([]UsageDailyRec, error) {
	if m == nil || m.db == nil {
		return nil, nil
	}
	if days <= 0 {
		days = 1
	}
	since := utcDay()
	if days > 1 {
		since = time.Now().In(dayZone).AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	}
	rows, err := m.db.Query(
		`SELECT subject, day, calls, tokens FROM ai_usage_daily WHERE day >= ? ORDER BY day DESC, tokens DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageDailyRec{}
	for rows.Next() {
		var r UsageDailyRec
		if err := rows.Scan(&r.Subject, &r.Day, &r.Calls, &r.Tokens); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// dayZone 计量自然日时区：北京时间（与 ai_quota.go / 运营报表口径一致）。
var dayZone = time.FixedZone("CST", 8*3600)

func utcDay() string { return time.Now().In(dayZone).Format("2006-01-02") }
