package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/repo"
)

func newBilling(t *testing.T) (*BillingService, context.Context) {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := NewBilling(db)
	ctx := context.Background()
	if err := s.SeedDefaultPlans(ctx); err != nil {
		t.Fatal(err)
	}
	return s, ctx
}

// 1) 套餐目录幂等 seed：重复 seed 不报错、条数稳定。
func TestBillingSeedIdempotent(t *testing.T) {
	s, ctx := newBilling(t)
	if err := s.SeedDefaultPlans(ctx); err != nil {
		t.Fatal(err)
	}
	plans, err := s.ListPlans(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 {
		t.Fatalf("内置套餐应为 3 档（free/pro/team），得 %d", len(plans))
	}
}

// 2) AI 额度：免费档 20 次/日，第 21 次应被拒。
func TestBillingAIQuotaEnforced(t *testing.T) {
	s, ctx := newBilling(t)
	for i := 0; i < 20; i++ {
		if err := s.AddAIUsage(ctx, "default"); err != nil {
			t.Fatalf("第 %d 次应在额度内：%v", i+1, err)
		}
	}
	err := s.AddAIUsage(ctx, "default")
	if err == nil {
		t.Fatal("超过日额度应被拒")
	}
	// 升级到 pro（500/日）后应恢复
	if _, err := s.SubscribeSite(ctx, "default", "pro", "yearly", time.Now().AddDate(1, 0, 0).UnixMilli(), ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAIUsage(ctx, "default"); err != nil {
		t.Fatalf("升级 pro 后应可用：%v", err)
	}
}

// 3) 存储超限：超过 free 档 512MB 应拒。
func TestBillingStorageQuota(t *testing.T) {
	s, ctx := newBilling(t)
	if err := s.SetStorageUsage(ctx, "default", 100<<20); err != nil { // 100MB
		t.Fatalf("100MB 应在 free 档内：%v", err)
	}
	if err := s.SetStorageUsage(ctx, "default", 900<<20); err == nil { // 900MB > 512MB
		t.Fatal("超过 512MB 应被拒")
	}
}

// 4) 订阅到期自动降档回 free。
func TestBillingExpiryDowngrade(t *testing.T) {
	s, ctx := newBilling(t)
	// 订阅 pro，但期限已过（1 小时前）
	past := time.Now().Add(-time.Hour).UnixMilli()
	if _, err := s.SubscribeSite(ctx, "default", "pro", "yearly", past, ""); err != nil {
		t.Fatal(err)
	}
	sub, err := s.GetSiteSubscription(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	if sub.PlanID != "free" {
		t.Fatalf("到期应自动降档 free，得 %s", sub.PlanID)
	}
}

// 5) 未订阅站点按 free 兜底（不报错、可用）。
func TestBillingNoSubscriptionFallsBackFree(t *testing.T) {
	s, ctx := newBilling(t)
	sub, err := s.GetSiteSubscription(ctx, "never-subscribed")
	if err != nil {
		t.Fatal(err)
	}
	if sub.PlanID != "free" {
		t.Fatalf("未订阅应兜底 free，得 %s", sub.PlanID)
	}
}

// 6) 特性按档位：free 无 knowledge_graph，team 有。
func TestBillingFeatureByTier(t *testing.T) {
	s, ctx := newBilling(t)
	if s.CheckFeature(ctx, "site-a", "knowledge_graph") {
		t.Fatal("free 档不应含 knowledge_graph")
	}
	if _, err := s.SubscribeSite(ctx, "site-b", "team", "yearly", time.Now().AddDate(1, 0, 0).UnixMilli(), ""); err != nil {
		t.Fatal(err)
	}
	if !s.CheckFeature(ctx, "site-b", "knowledge_graph") {
		t.Fatal("team 档应含 knowledge_graph")
	}
}

// 7) 免费档不可停用（handler 层校验依赖此不变式）。
func TestBillingFreePlanIsSeededEnabled(t *testing.T) {
	s, ctx := newBilling(t)
	p, err := s.GetPlan(ctx, "free")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Enabled {
		t.Fatal("免费档必须启用（到期降档兜底）")
	}
}
