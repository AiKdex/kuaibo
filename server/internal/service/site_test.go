package service

import (
	"context"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/repo"
)

func openSiteTestDB(t *testing.T) *SiteStore {
	t.Helper()
	conn, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return NewSiteStore(conn)
}

func TestSiteStoreEnsureDefault(t *testing.T) {
	st := openSiteTestDB(t)
	ctx := context.Background()
	if err := st.EnsureDefaultSite(ctx); err != nil {
		t.Fatalf("ensure default: %v", err)
	}
	// 重复调用幂等
	if err := st.EnsureDefaultSite(ctx); err != nil {
		t.Fatalf("ensure default 2nd: %v", err)
	}
	def, err := st.GetSite(ctx, defaultSiteID)
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if def.Domain != "*" || def.Status != "active" {
		t.Fatalf("default site 异常: %+v", def)
	}
	n, err := st.CountActiveSites(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("活跃站数应为 1，实际 %d", n)
	}
	// 默认站配置已 seed
	ss, err := st.GetSiteSettings(ctx, defaultSiteID)
	if err != nil {
		t.Fatalf("default settings: %v", err)
	}
	if ss.DefaultTheme != "aiklog" || !ss.AllowVisitorThemeSwitch {
		t.Fatalf("default settings 异常: %+v", ss)
	}
	// 解析兜底到默认站
	got, err := st.ResolveSite(ctx, "", "/", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.ID != defaultSiteID {
		t.Fatalf("解析应兜底默认站，实际 %q", got.ID)
	}
}

func TestSiteStoreResolvePriority(t *testing.T) {
	st := openSiteTestDB(t)
	ctx := context.Background()
	if err := st.EnsureDefaultSite(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSite(ctx, &Site{Slug: "news", Domain: "news.example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSite(ctx, &Site{Slug: "shop", Subdomain: "shop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSite(ctx, &Site{Slug: "docs", PathPrefix: "/docs"}); err != nil {
		t.Fatal(err)
	}

	// 1) 自定义域名精确匹配优先
	got, err := st.ResolveSite(ctx, "news.example.com", "/", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "news" {
		t.Fatalf("自定义域名解析应命中 news，实际 %q", got.Slug)
	}

	// 2) 子域名（剥离 base domain）
	got, err = st.ResolveSite(ctx, "shop.aiklog.cn", "/", "aiklog.cn")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "shop" {
		t.Fatalf("子域名解析应命中 shop，实际 %q", got.Slug)
	}

	// 3) 子目录
	got, err = st.ResolveSite(ctx, "aiklog.cn", "/docs/page", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "docs" {
		t.Fatalf("子目录解析应命中 docs，实际 %q", got.Slug)
	}

	// 4) 未匹配 → 默认站
	got, err = st.ResolveSite(ctx, "unknown.example.com", "/", "aiklog.cn")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != defaultSiteID {
		t.Fatalf("未匹配应兜底默认站，实际 %q", got.ID)
	}

	// 解析优先于子目录：自定义域名存在时应命中域名而非子目录同名段
	if _, err := st.CreateSite(ctx, &Site{Slug: "news2", Domain: "docs.example.com"}); err != nil {
		t.Fatal(err)
	}
	got, err = st.ResolveSite(ctx, "docs.example.com", "/docs/page", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "news2" {
		t.Fatalf("自定义域名应优先于子目录解析，实际 %q", got.Slug)
	}
}

func TestSiteStoreCRUD(t *testing.T) {
	st := openSiteTestDB(t)
	ctx := context.Background()
	if err := st.EnsureDefaultSite(ctx); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateSite(ctx, &Site{Slug: "alpha", Domain: "alpha.example.com", OwnerID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("创建后应有 id")
	}
	// 取回
	got, err := st.GetSite(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Domain != "alpha.example.com" || got.OwnerID != "u1" {
		t.Fatalf("取回异常: %+v", got)
	}
	// 更新
	got.Domain = "alpha2.example.com"
	if err := st.UpdateSite(ctx, got); err != nil {
		t.Fatal(err)
	}
	got2, err := st.GetSite(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Domain != "alpha2.example.com" {
		t.Fatalf("更新未生效: %+v", got2)
	}
	// 软删
	if err := st.DeleteSite(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	n, err := st.CountActiveSites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 { // 仅默认站
		t.Fatalf("软删后活跃站应为 1，实际 %d", n)
	}
	// 列表不含已删
	list, err := st.ListSites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == created.ID {
			t.Fatal("已删站不应出现在列表")
		}
	}
}

func TestSiteStoreSettingsUpsert(t *testing.T) {
	st := openSiteTestDB(t)
	ctx := context.Background()
	if err := st.EnsureDefaultSite(ctx); err != nil {
		t.Fatal(err)
	}
	ss := &SiteSettings{SiteID: defaultSiteID, DefaultTheme: "aurora", AllowVisitorThemeSwitch: false, Title: "我的站", SEODescription: "描述"}
	if err := st.UpsertSiteSettings(ctx, ss); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSiteSettings(ctx, defaultSiteID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultTheme != "aurora" {
		t.Fatalf("theme = %q", got.DefaultTheme)
	}
	if got.AllowVisitorThemeSwitch {
		t.Fatal("switch 应为 false")
	}
	if got.Title != "我的站" || got.SEODescription != "描述" {
		t.Fatalf("字段异常: %+v", got)
	}
	// 覆盖更新
	if err := st.UpsertSiteSettings(ctx, &SiteSettings{SiteID: defaultSiteID, DefaultTheme: "brutal"}); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetSiteSettings(ctx, defaultSiteID)
	if got.DefaultTheme != "brutal" {
		t.Fatalf("覆盖更新失败: %q", got.DefaultTheme)
	}
}
