// Package service 的 source.go 实现采集源配置存储与采集运行记录（频道化 P10）。
// 设计要点：
//   - 源配置化：一个 Source = 一个官办站点（城市 + 域名 + 站点模板），
//     "探路一次·模板复用"——新城市 = 新增源配置而非写代码（见 docs/设计需求文档.md P10）。
//   - 模板 JSON 由探路流程（AI 辅助识别 → 人工半确认）生成，采集器按模板抓取/解析/入库。
//   - 运行记录可审计：每次采集的 fetched/created/skipped/failed 落库，供频道运营与排查。
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Source 采集源配置。
type Source struct {
	ID          string          `json:"id"`
	City        string          `json:"city"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	ChannelType string          `json:"channel_type"`         // job_position|job_notice|generic（把关 L0：栏目类型声明）
	FetchMode   string          `json:"fetch_mode"`           // http|tls|trawl|skip（分级 fetch）
	TargetDir   string          `json:"target_dir,omitempty"` // 入库目录（可设置；空=按 kind 默认 采集/就业情报|采集/培训补贴）
	URL         string          `json:"url"`
	Template    json.RawMessage `json:"template"`
	Enabled     bool            `json:"enabled"`
	LastRunAt   int64           `json:"last_run_at,omitempty"`
	LastStatus  string          `json:"last_status,omitempty"`
	LastCount   int             `json:"last_count,omitempty"`
	PackID      string          `json:"pack_id,omitempty"` // 来源源包 id（SPEC-SP-001：覆盖更新与卸载按此定位；自建源为空）
	CreatedAt   int64           `json:"created_at"`
	UpdatedAt   int64           `json:"updated_at"`
}

// NormalizeChannelType 把关 L0：栏目类型归一化（空默认 job_position——岗位栏目）。
func NormalizeChannelType(v string) string {
	switch v {
	case "job_notice", "generic":
		return v
	default:
		return "job_position"
	}
}

// NormalizeFetchMode 分级 fetch 归一化（空/非法默认 http——普通请求）。
func NormalizeFetchMode(v string) string {
	switch v {
	case "tls", "trawl", "skip":
		return v
	default:
		return "http"
	}
}

// CollectRun 一次采集运行记录。
type CollectRun struct {
	ID             int64    `json:"id"`
	City           string   `json:"city"`
	Trigger        string   `json:"trigger"`
	Status         string   `json:"status"`
	Fetched        int      `json:"fetched"`
	Created        int      `json:"created"`
	Skipped        int      `json:"skipped"`
	Rejected       int      `json:"rejected"`                  // 把关剔除（无效/空正文/命中排除词）
	RejectedDetail []string `json:"rejected_detail,omitempty"` // 把关剔除明细（标题+原因，L3 可观测）
	Failed         int      `json:"failed"`
	Error          string   `json:"error,omitempty"`
	StartedAt      int64    `json:"started_at"`
	FinishedAt     int64    `json:"finished_at,omitempty"`
}

// SourceStore 采集源配置存储。
type SourceStore struct {
	db *sql.DB
}

// NewSourceStore 创建 SourceStore。
func NewSourceStore(db *sql.DB) *SourceStore {
	return &SourceStore{db: db}
}

// CreateSource 新增源配置。
func (s *SourceStore) CreateSource(ctx context.Context, src *Source) (*Source, error) {
	if src.ID == "" {
		src.ID = uuid.NewString()
	}
	if src.Kind == "" {
		src.Kind = "job"
	}
	if src.City == "" || src.Name == "" || src.URL == "" {
		return nil, errors.New("service: city/name/url are required")
	}
	tpl := src.Template
	if len(tpl) == 0 {
		tpl = json.RawMessage("{}")
	}
	enabled := 1
	if !src.Enabled {
		enabled = 0
	}
	src.ChannelType = NormalizeChannelType(src.ChannelType)
	src.FetchMode = NormalizeFetchMode(src.FetchMode)
	ts := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sources (id, city, name, kind, channel_type, fetch_mode, target_dir, url, template, enabled, pack_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		src.ID, src.City, src.Name, src.Kind, src.ChannelType, src.FetchMode, src.TargetDir, src.URL, string(tpl), enabled, src.PackID, ts, ts); err != nil {
		return nil, err
	}
	src.Enabled = enabled == 1
	src.CreatedAt = ts
	src.UpdatedAt = ts
	return src, nil
}

// ListSources 列出源配置（可按城市过滤；kind 可选过滤）。
func (s *SourceStore) ListSources(ctx context.Context, city, kind string) ([]*Source, error) {
	q := `SELECT id, city, name, kind, channel_type, fetch_mode, target_dir, url, template, enabled, last_run_at, last_status, last_count, COALESCE(pack_id,''), created_at, updated_at FROM sources WHERE 1=1`
	var args []any
	if city != "" {
		q += ` AND city = ?`
		args = append(args, city)
	}
	if kind != "" {
		q += ` AND kind = ?`
		args = append(args, kind)
	}
	q += ` ORDER BY city, name`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Source
	for rows.Next() {
		var (
			sc       Source
			tpl      string
			enabled  int
			lastRun  int64
			lastStat sql.NullString
			lastCnt  int
			created  int64
			updated  int64
		)
		var tgt, packID sql.NullString
		if err := rows.Scan(&sc.ID, &sc.City, &sc.Name, &sc.Kind, &sc.ChannelType, &sc.FetchMode, &tgt, &sc.URL, &tpl, &enabled,
			&lastRun, &lastStat, &lastCnt, &packID, &created, &updated); err != nil {
			return nil, err
		}
		sc.PackID = packID.String
		sc.ChannelType = NormalizeChannelType(sc.ChannelType)
		sc.FetchMode = NormalizeFetchMode(sc.FetchMode)
		sc.TargetDir = tgt.String
		sc.Template = json.RawMessage(tpl)
		sc.Enabled = enabled == 1
		sc.LastRunAt = lastRun
		sc.LastStatus = lastStat.String
		sc.LastCount = lastCnt
		sc.CreatedAt = created
		sc.UpdatedAt = updated
		out = append(out, &sc)
	}
	return out, rows.Err()
}

// GetSource 取单个源配置。
func (s *SourceStore) GetSource(ctx context.Context, id string) (*Source, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, city, name, kind, channel_type, fetch_mode, target_dir, url, template, enabled, last_run_at, last_status, last_count, COALESCE(pack_id,''), created_at, updated_at
		 FROM sources WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	var (
		sc       Source
		tpl      string
		enabled  int
		lastRun  int64
		lastStat sql.NullString
		lastCnt  int
		created  int64
		updated  int64
	)
	var tgt, packID sql.NullString
	if err := rows.Scan(&sc.ID, &sc.City, &sc.Name, &sc.Kind, &sc.ChannelType, &sc.FetchMode, &tgt, &sc.URL, &tpl, &enabled,
		&lastRun, &lastStat, &lastCnt, &packID, &created, &updated); err != nil {
		return nil, err
	}
	sc.PackID = packID.String
	sc.ChannelType = NormalizeChannelType(sc.ChannelType)
	sc.FetchMode = NormalizeFetchMode(sc.FetchMode)
	sc.TargetDir = tgt.String
	sc.Template = json.RawMessage(tpl)
	sc.Enabled = enabled == 1
	sc.LastRunAt = lastRun
	sc.LastStatus = lastStat.String
	sc.LastCount = lastCnt
	sc.CreatedAt = created
	sc.UpdatedAt = updated
	return &sc, nil
}

// UpdateSource 更新源配置（name/url/template/kind/enabled 白名单）。
func (s *SourceStore) UpdateSource(ctx context.Context, id string, patch map[string]any) (*Source, error) {
	cur, err := s.GetSource(ctx, id)
	if err != nil {
		return nil, err
	}
	if v, ok := patch["name"].(string); ok {
		cur.Name = v
	}
	if v, ok := patch["url"].(string); ok {
		cur.URL = v
	}
	if v, ok := patch["kind"].(string); ok {
		cur.Kind = v
	}
	if v, ok := patch["channel_type"].(string); ok {
		cur.ChannelType = NormalizeChannelType(v)
	}
	if v, ok := patch["fetch_mode"].(string); ok {
		cur.FetchMode = NormalizeFetchMode(v)
	}
	if v, ok := patch["target_dir"].(string); ok {
		cur.TargetDir = v
	}
	if v, ok := patch["template"]; ok {
		raw, _ := json.Marshal(v)
		cur.Template = raw
	}
	if v, ok := patch["enabled"].(bool); ok {
		cur.Enabled = v
	}
	enabled := 1
	if !cur.Enabled {
		enabled = 0
	}
	ts := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sources SET name=?, url=?, kind=?, channel_type=?, fetch_mode=?, target_dir=?, template=?, enabled=?, updated_at=? WHERE id=?`,
		cur.Name, cur.URL, cur.Kind, cur.ChannelType, cur.FetchMode, cur.TargetDir, string(cur.Template), enabled, ts, id); err != nil {
		return nil, err
	}
	cur.UpdatedAt = ts
	return cur, nil
}

// TombstoneSource 记录采集删除记忆：采集产物被删（软删）时记下 source_url，
// 后续采集命中该条目不重建（防"清理→重采→又回来"死循环）；恢复文件后幂等命中不受影响。
func (s *SourceStore) TombstoneSource(ctx context.Context, sourceURL, reason string) error {
	if sourceURL == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO collect_tombstones(source_url, reason, note, created_at) VALUES(?,?,?,?)`,
		sourceURL, reason, "", time.Now().Unix())
	return err
}

// IsTombstoned 采集条目是否已被删除/人工剔除（命中墓碑）？
func (s *SourceStore) IsTombstoned(ctx context.Context, sourceURL string) bool {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collect_tombstones WHERE source_url=?`, sourceURL).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// DeleteSource 删除源配置。
func (s *SourceStore) DeleteSource(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sources WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MarkSourceRun 采集结束后回写源的最近运行状态。
func (s *SourceStore) MarkSourceRun(ctx context.Context, id, status string, count int) error {
	ts := time.Now().Unix()
	_, err := s.db.ExecContext(ctx,
		`UPDATE sources SET last_run_at=?, last_status=?, last_count=?, updated_at=? WHERE id=?`,
		ts, status, count, ts, id)
	return err
}

// StartRun 记录采集运行开始，返回 run id。
func (s *SourceStore) StartRun(ctx context.Context, city, trigger string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO collect_runs (city, trigger, status, started_at) VALUES (?, ?, 'running', ?)`,
		city, trigger, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishRun 记录采集运行结束（rejected 为把关剔除数）。
func (s *SourceStore) FinishRun(ctx context.Context, id int64, status string, fetched, created, skipped, rejected, failed int, errMsg string) error {
	return s.FinishRunDetail(ctx, id, status, fetched, created, skipped, rejected, failed, errMsg, "")
}

// FinishRunDetail 记录采集运行结束（含把关拒绝明细 JSON；L3 可观测："拒了多少、为什么拒、拒了哪些"）。
func (s *SourceStore) FinishRunDetail(ctx context.Context, id int64, status string, fetched, created, skipped, rejected, failed int, errMsg, rejectDetail string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE collect_runs SET status=?, fetched=?, created=?, skipped=?, rejected=?, failed=?, error=?, rejected_detail=?, finished_at=? WHERE id=?`,
		status, fetched, created, skipped, rejected, failed, errMsg, rejectDetail, time.Now().Unix(), id)
	return err
}

// OrphanRuns 启动收尾：把超过 staleAfter 仍 running 的采集记录落终态（进程中断兜底，
// 防止"卡 running 不收尾"——第三方反馈问题 1）。返回收尾条数。
func (s *SourceStore) OrphanRuns(ctx context.Context, staleAfter time.Duration) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE collect_runs SET status='failed', error='进程中断（服务重启/崩溃，run 未收尾）',
		        finished_at=? WHERE status='running' AND started_at < ?`,
		time.Now().Unix(), time.Now().Add(-staleAfter).Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RunStatus 读取单条采集运行终态（供终态回调/通知使用）。
func (s *SourceStore) RunStatus(ctx context.Context, id int64) (*CollectRun, bool) {
	var r CollectRun
	err := s.db.QueryRowContext(ctx,
		`SELECT id, city, trigger, status, fetched, created, skipped, rejected, failed, COALESCE(error,''), started_at, COALESCE(finished_at,0)
		 FROM collect_runs WHERE id=?`, id).
		Scan(&r.ID, &r.City, &r.Trigger, &r.Status, &r.Fetched, &r.Created, &r.Skipped, &r.Rejected, &r.Failed, &r.Error, &r.StartedAt, &r.FinishedAt)
	if err != nil {
		return nil, false
	}
	return &r, true
}

// ListRuns 列出采集运行记录（按城市过滤，倒序）。
func (s *SourceStore) ListRuns(ctx context.Context, city string, limit int) ([]*CollectRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, city, trigger, status, fetched, created, skipped, rejected, failed, error, rejected_detail, started_at, finished_at
	      FROM collect_runs`
	var args []any
	if city != "" {
		q += ` WHERE city = ?`
		args = append(args, city)
	}
	q += ` ORDER BY started_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*CollectRun
	for rows.Next() {
		var (
			r   CollectRun
			er  sql.NullString
			rd  sql.NullString
			fin sql.NullInt64
		)
		if err := rows.Scan(&r.ID, &r.City, &r.Trigger, &r.Status, &r.Fetched, &r.Created,
			&r.Skipped, &r.Rejected, &r.Failed, &er, &rd, &r.StartedAt, &fin); err != nil {
			return nil, err
		}
		r.Error = er.String
		if rd.Valid && rd.String != "" {
			_ = json.Unmarshal([]byte(rd.String), &r.RejectedDetail)
		}
		r.FinishedAt = fin.Int64
		out = append(out, &r)
	}
	return out, rows.Err()
}
