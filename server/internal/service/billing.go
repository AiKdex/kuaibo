package service

// billing.go 多租户 SaaS 计费（SPEC-BILLING）。
//
// 分层（与 site_licenses 正交）：
//   site_licenses —— 「能开几个站」（站点数授权，M2 已落地）
//   billing      —— 「这个站是什么档、额度多少、到什么时候」（本文件）
//
// 计费三件事：
//  1. 套餐目录（billing_plans）—— 档位定义与额度上限，管理员可调；
//  2. 站点订阅（site_subscriptions）—— 每站一档，到期自动降档到 free；
//  3. 额度计量与超限拒绝（site_usage_daily）—— 按站×日聚合，超限直接拒绝（不是事后统计）。
//
// 口径：额度检查是**准入判断**（能不能用），用量记录是**事实**（用了多少）；
// 二者不互相推算，避免「先放行后扣减」在并发下超发。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrQuotaExceeded 额度超限（调用方据此返回 402/429）。
	ErrQuotaExceeded = errors.New("billing: 额度已用尽，请升级套餐")
	// ErrPlanNotFound 套餐不存在。
	ErrPlanNotFound = errors.New("billing: 套餐不存在")
)

// ---- 套餐 ----

// BillingPlan 套餐定义。
type BillingPlan struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Edition         string   `json:"edition"`
	PriceCents      int      `json:"price_cents"`
	BillingPeriod   string   `json:"billing_period"`
	MaxSites        int      `json:"max_sites"`
	MaxMembers      int      `json:"max_members"`
	MaxStorageMB    int      `json:"max_storage_mb"`
	MaxAICallsDaily int      `json:"max_ai_calls_daily"`
	Features        []string `json:"features"`
	Sort            int      `json:"sort"`
	Enabled         bool     `json:"enabled"`
}

const planCols = `id, name, description, edition, price_cents, billing_period,
	max_sites, max_members, max_storage_mb, max_ai_calls_daily, features, sort, enabled`

func scanPlan(s scanner) (*BillingPlan, error) {
	p := &BillingPlan{}
	var feat string
	if err := s.Scan(&p.ID, &p.Name, &p.Description, &p.Edition, &p.PriceCents, &p.BillingPeriod,
		&p.MaxSites, &p.MaxMembers, &p.MaxStorageMB, &p.MaxAICallsDaily, &feat, &p.Sort, &p.Enabled); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(feat), &p.Features)
	return p, nil
}

// ---- 订阅 ----

// SiteSubscription 站点订阅。
type SiteSubscription struct {
	ID                 string `json:"id"`
	SiteID             string `json:"site_id"`
	PlanID             string `json:"plan_id"`
	Status             string `json:"status"` // active|past_due|canceled|expired
	BillingPeriod      string `json:"billing_period"`
	PriceCents         int    `json:"price_cents"`
	StartedAt          int64  `json:"started_at"`
	CurrentPeriodEnd   int64  `json:"current_period_end"`
	CancelAtPeriodEnd  bool   `json:"cancel_at_period_end"`
	OrderID            string `json:"order_id"`
}

const subCols = `id, site_id, plan_id, status, billing_period, price_cents,
	started_at, current_period_end, cancel_at_period_end, order_id`

func scanSub(s scanner) (*SiteSubscription, error) {
	x := &SiteSubscription{}
	if err := s.Scan(&x.ID, &x.SiteID, &x.PlanID, &x.Status, &x.BillingPeriod, &x.PriceCents,
		&x.StartedAt, &x.CurrentPeriodEnd, &x.CancelAtPeriodEnd, &x.OrderID); err != nil {
		return nil, err
	}
	return x, nil
}

// ---- 用量 ----

// SiteUsage 某站某日用量。
type SiteUsage struct {
	SiteID       string `json:"site_id"`
	Day          string `json:"day"`
	AICalls      int    `json:"ai_calls"`
	StorageBytes int64  `json:"storage_bytes"`
	Files        int    `json:"files"`
}

// BillingService 计费服务。
type BillingService struct{ db *sql.DB }

// NewBilling 构造计费服务。
func NewBilling(db *sql.DB) *BillingService { return &BillingService{db: db} }

// Today 返回 YYYY-MM-DD（按本地时区）。
func Today() string { return time.Now().Format("2006-01-02") }

// SeedDefaultPlans 幂等写入内置套餐目录。
// 免费档永远存在且不可删（超限降档的兜底目标），付费档管理员可改价/停用。
func (s *BillingService) SeedDefaultPlans(ctx context.Context) error {
	now := time.Now().UnixMilli()
	seed := []BillingPlan{
		{ID: "free", Name: "免费版", Description: "单人使用，基础博客能力", Edition: "community",
			PriceCents: 0, BillingPeriod: "free", MaxSites: 1, MaxMembers: 1, MaxStorageMB: 512, MaxAICallsDaily: 20,
			Features: []string{"blog", "themes", "ai_ask"}, Sort: 10, Enabled: true},
		{ID: "pro", Name: "专业版", Description: "单站更高额度 + 全部 AI 能力", Edition: "pro",
			PriceCents: 9900, BillingPeriod: "yearly", MaxSites: 1, MaxMembers: 5, MaxStorageMB: 10240, MaxAICallsDaily: 500,
			Features: []string{"blog", "themes", "ai_ask", "ai_summary", "knowledge_graph", "cs", "market"}, Sort: 20, Enabled: true},
		{ID: "team", Name: "团队版", Description: "多成员协作 + 组织架构 + 客服", Edition: "pro",
			PriceCents: 29900, BillingPeriod: "yearly", MaxSites: 3, MaxMembers: 50, MaxStorageMB: 102400, MaxAICallsDaily: 5000,
			Features: []string{"blog", "themes", "ai_ask", "ai_summary", "knowledge_graph", "cs", "market", "org", "sso"}, Sort: 30, Enabled: true},
	}
	for _, p := range seed {
		feat, _ := json.Marshal(p.Features)
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO billing_plans(`+planCols+`, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
			  name=excluded.name, description=excluded.description, edition=excluded.edition,
			  price_cents=excluded.price_cents, billing_period=excluded.billing_period,
			  max_sites=excluded.max_sites, max_members=excluded.max_members,
			  max_storage_mb=excluded.max_storage_mb, max_ai_calls_daily=excluded.max_ai_calls_daily,
			  features=excluded.features, sort=excluded.sort, enabled=excluded.enabled, updated_at=excluded.updated_at`,
			p.ID, p.Name, p.Description, p.Edition, p.PriceCents, p.BillingPeriod,
			p.MaxSites, p.MaxMembers, p.MaxStorageMB, p.MaxAICallsDaily, string(feat), p.Sort, boolInt(p.Enabled), now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ListPlans 套餐目录（仅 enabled，按 sort）。
func (s *BillingService) ListPlans(ctx context.Context, includeDisabled bool) ([]BillingPlan, error) {
	q := `SELECT ` + planCols + ` FROM billing_plans`
	var args []any
	if !includeDisabled {
		q += ` WHERE enabled=1`
	}
	q += ` ORDER BY sort ASC, price_cents ASC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BillingPlan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPlan 取套餐。
func (s *BillingService) GetPlan(ctx context.Context, id string) (*BillingPlan, error) {
	p, err := scanPlan(s.db.QueryRowContext(ctx, `SELECT `+planCols+` FROM billing_plans WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPlanNotFound
	}
	return p, err
}

// SubscribeSite 为站点设置订阅档位（幂等：同站覆盖）。
// periodEnd=0 表示永久（免费档）。
func (s *BillingService) SubscribeSite(ctx context.Context, siteID, planID, period string, periodEnd int64, orderID string) (*SiteSubscription, error) {
	plan, err := s.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if period == "" {
		period = plan.BillingPeriod
	}
	var id string
	_ = s.db.QueryRowContext(ctx, `SELECT id FROM site_subscriptions WHERE site_id=?`, siteID).Scan(&id)
	if id == "" {
		id = "sub-" + siteID
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO site_subscriptions(id, site_id, plan_id, status, billing_period, price_cents,
		  started_at, current_period_end, cancel_at_period_end, order_id, created_at, updated_at)
		VALUES(?,?,?,'active',?,?,?,?,0,?,?,?)
		ON CONFLICT(site_id) DO UPDATE SET
		  plan_id=excluded.plan_id, status='active', billing_period=excluded.billing_period,
		  price_cents=excluded.price_cents, current_period_end=excluded.current_period_end,
		  order_id=excluded.order_id, updated_at=excluded.updated_at`,
		id, siteID, planID, period, plan.PriceCents, now, periodEnd, orderID, now, now); err != nil {
		return nil, err
	}
	return s.GetSiteSubscription(ctx, siteID)
}

// GetSiteSubscription 取某站订阅；无订阅按 free 档兜底（未订阅 ≠ 不能用）。
func (s *BillingService) GetSiteSubscription(ctx context.Context, siteID string) (*SiteSubscription, error) {
	x, err := scanSub(s.db.QueryRowContext(ctx, `SELECT `+subCols+` FROM site_subscriptions WHERE site_id=?`, siteID))
	if errors.Is(err, sql.ErrNoRows) {
		return &SiteSubscription{
			ID: "sub-" + siteID, SiteID: siteID, PlanID: "free", Status: "active",
			BillingPeriod: "free", StartedAt: 0, CurrentPeriodEnd: 0,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	// 到期降档：订阅期已过且未标记「到期后取消」→ 落回 free，避免过期档位继续放行
	if x.CurrentPeriodEnd > 0 && time.Now().UnixMilli() > x.CurrentPeriodEnd && x.PlanID != "free" {
		if !x.CancelAtPeriodEnd {
			_ = s.downgradeToFree(ctx, siteID)
			return s.GetSiteSubscription(ctx, siteID)
		}
	}
	return x, nil
}

func (s *BillingService) downgradeToFree(ctx context.Context, siteID string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`UPDATE site_subscriptions SET plan_id='free', status='expired', billing_period='free',
		   price_cents=0, current_period_end=0, updated_at=? WHERE site_id=?`, now, siteID)
	return err
}

// CancelAtPeriodEnd 标记到期后取消（保留当前档直到期满）。
func (s *BillingService) CancelAtPeriodEnd(ctx context.Context, siteID string, cancel bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE site_subscriptions SET cancel_at_period_end=?, updated_at=? WHERE site_id=?`,
		boolInt(cancel), time.Now().UnixMilli(), siteID)
	return err
}

// ---- 额度计量与准入 ----

// UsageOf 取某站今日用量。
func (s *BillingService) UsageOf(ctx context.Context, siteID, day string) (*SiteUsage, error) {
	if day == "" {
		day = Today()
	}
	u := &SiteUsage{SiteID: siteID, Day: day}
	err := s.db.QueryRowContext(ctx,
		`SELECT ai_calls, storage_bytes, files FROM site_usage_daily WHERE site_id=? AND day=?`, siteID, day).
		Scan(&u.AICalls, &u.StorageBytes, &u.Files)
	if errors.Is(err, sql.ErrNoRows) {
		return u, nil // 今天还没用量
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// AddAIUsage 记一次 AI 调用并做**前置准入**。
// 先查额度再自增：并发下最多轻微超发（读到同值），不会漏记；返回 ErrQuotaExceeded 表示已满。
func (s *BillingService) AddAIUsage(ctx context.Context, siteID string) error {
	plan, err := s.effectivePlan(ctx, siteID)
	if err != nil {
		return err
	}
	day := Today()
	if plan.MaxAICallsDaily > 0 {
		u, err := s.UsageOf(ctx, siteID, day)
		if err != nil {
			return err
		}
		if u.AICalls >= plan.MaxAICallsDaily {
			return fmt.Errorf("%w（今日 %d/%d）", ErrQuotaExceeded, u.AICalls, plan.MaxAICallsDaily)
		}
	}
	now := time.Now().UnixMilli()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO site_usage_daily(site_id, day, ai_calls, storage_bytes, files, updated_at)
		VALUES(?,?,1,0,0,?)
		ON CONFLICT(site_id, day) DO UPDATE SET ai_calls = ai_calls + 1, updated_at=excluded.updated_at`,
		siteID, day, now)
	return err
}

// SetStorageUsage 记录某站存储占用（字节）；超限返回 ErrQuotaExceeded。
func (s *BillingService) SetStorageUsage(ctx context.Context, siteID string, bytes int64) error {
	plan, err := s.effectivePlan(ctx, siteID)
	if err != nil {
		return err
	}
	limitMB := plan.MaxStorageMB
	if limitMB > 0 {
		limit := int64(limitMB) << 20
		if bytes > limit {
			return fmt.Errorf("%w（存储 %.1fMB / 上限 %dMB）", ErrQuotaExceeded, float64(bytes)/(1<<20), limitMB)
		}
	}
	day := Today()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO site_usage_daily(site_id, day, ai_calls, storage_bytes, files, updated_at)
		VALUES(?,?,0,?,0,?)
		ON CONFLICT(site_id, day) DO UPDATE SET storage_bytes=excluded.storage_bytes, updated_at=excluded.updated_at`,
		siteID, day, bytes, time.Now().UnixMilli())
	return err
}

// effectivePlan 取站点生效套餐（订阅 → 套餐；订阅不存在/失效 → free）。
func (s *BillingService) effectivePlan(ctx context.Context, siteID string) (*BillingPlan, error) {
	sub, err := s.GetSiteSubscription(ctx, siteID)
	if err != nil {
		return nil, err
	}
	p, err := s.GetPlan(ctx, sub.PlanID)
	if errors.Is(err, ErrPlanNotFound) {
		return s.GetPlan(ctx, "free")
	}
	return p, err
}

// CheckFeature 校验站点是否拥有某特性（按当前档 features 清单）。
func (s *BillingService) CheckFeature(ctx context.Context, siteID, feature string) bool {
	p, err := s.effectivePlan(ctx, siteID)
	if err != nil {
		return false
	}
	for _, f := range p.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// SetPlan 管理员改套餐定义（价格/额度/开关）。
func (s *BillingService) SetPlan(ctx context.Context, p *BillingPlan) error {
	if strings.TrimSpace(p.ID) == "" {
		return errors.New("billing: plan id 必填")
	}
	now := time.Now().UnixMilli()
	feat, _ := json.Marshal(p.Features)
	res, err := s.db.ExecContext(ctx, `
		UPDATE billing_plans SET name=?, description=?, price_cents=?, billing_period=?,
		  max_members=?, max_storage_mb=?, max_ai_calls_daily=?, features=?, sort=?, enabled=?, updated_at=?
		WHERE id=?`,
		p.Name, p.Description, p.PriceCents, p.BillingPeriod,
		p.MaxMembers, p.MaxStorageMB, p.MaxAICallsDaily, string(feat), p.Sort, boolInt(p.Enabled), now, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPlanNotFound
	}
	return nil
}
