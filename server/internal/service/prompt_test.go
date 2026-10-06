package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/repo"
)

func newTestPromptStore(t *testing.T) *PromptStore {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewPromptStore(db)
}

// 变量声明校验：合法 JSON、中文变量名、重复名、非法名、非 JSON。
func TestNormalizeVariables(t *testing.T) {
	if vs, err := normalizeVariables(""); err != nil || len(vs) != 0 {
		t.Fatalf("空串应得空数组，得到 %+v err=%v", vs, err)
	}
	vs, err := normalizeVariables(`[{"name":"标题","label":"文章标题","default":""},{"name":"tone","default":"专业"}]`)
	if err != nil {
		t.Fatalf("合法声明不应报错: %v", err)
	}
	if len(vs) != 2 || vs[0].Name != "标题" || vs[1].Default != "专业" {
		t.Fatalf("解析结果不对: %+v", vs)
	}
	if _, err := normalizeVariables(`[{"name":"a"},{"name":"a"}]`); !errors.Is(err, ErrPromptInvalid) {
		t.Fatalf("重复变量名应报 ErrPromptInvalid，得到 %v", err)
	}
	if _, err := normalizeVariables(`[{"name":"a b"}]`); !errors.Is(err, ErrPromptInvalid) {
		t.Fatalf("含空格的变量名应非法，得到 %v", err)
	}
	if _, err := normalizeVariables(`not-json`); !errors.Is(err, ErrPromptInvalid) {
		t.Fatalf("非 JSON 应报 ErrPromptInvalid，得到 %v", err)
	}
}

// 渲染纯函数：值优先 → 默认值 → 原样保留并报缺失。
func TestRenderPromptText(t *testing.T) {
	declared := []PromptVariable{
		{Name: "标题", Default: ""},
		{Name: "tone", Default: "专业"},
	}
	content := "请以{{tone}}口吻，为《{{标题}}》写简介。字数{{n}}。"

	// ① 全部提供
	r := renderPromptText(content, declared, map[string]string{"标题": "爱库录", "n": "200"})
	if r.Text != "请以专业口吻，为《爱库录》写简介。字数200。" {
		t.Fatalf("渲染结果不对: %q", r.Text)
	}
	if len(r.Missing) != 0 || len(r.Undeclared) != 0 {
		t.Fatalf("不应有缺失/未声明: %+v", r)
	}

	// ② 标题未提供（声明了但无默认值）→ 替换为空串，不算 missing（作者意图明确）
	r = renderPromptText(content, declared, map[string]string{"n": "200"})
	if r.Text != "请以专业口吻，为《》写简介。字数200。" {
		t.Fatalf("已声明无默认值应替换为空串: %q", r.Text)
	}
	if len(r.Missing) != 0 {
		t.Fatalf("已声明变量不应计入 missing: %+v", r.Missing)
	}

	// ③ n 未声明、未提供 → 原样保留 + 同时进 missing 与 undeclared
	r = renderPromptText(content, declared, map[string]string{"标题": "X"})
	if r.Text != "请以专业口吻，为《X》写简介。字数{{n}}。" {
		t.Fatalf("未声明占位符应原样保留: %q", r.Text)
	}
	if len(r.Missing) != 1 || r.Missing[0] != "n" {
		t.Fatalf("missing 应为 [n]: %+v", r.Missing)
	}
	if len(r.Undeclared) != 1 || r.Undeclared[0] != "n" {
		t.Fatalf("undeclared 应为 [n]: %+v", r.Undeclared)
	}

	// ④ 同一变量多次出现只报一次
	r = renderPromptText("{{x}} 与 {{x}} 与 {{ x }}", nil, nil)
	if len(r.Missing) != 1 {
		t.Fatalf("重复占位符应去重，得到 %+v", r.Missing)
	}
}

// 仓储 CRUD + 可见性：私有模板他人不可见、不可改。
func TestPromptStoreCRUDAndVisibility(t *testing.T) {
	ps := newTestPromptStore(t)
	ctx := context.Background()
	alice := PromptActor{UID: "u-alice"}
	bob := PromptActor{UID: "u-bob"}
	admin := PromptActor{UID: "u-admin", IsAdmin: true}

	// 私有模板
	priv, err := ps.Create(ctx, alice, PromptInput{
		Name: "我的私货", Content: "写点{{主题}}", Variables: `[{"name":"主题"}]`, IsPublic: false,
	})
	if err != nil {
		t.Fatalf("create private: %v", err)
	}
	if priv.IsPublic || !priv.Editable {
		t.Fatalf("私有模板标记不对: %+v", priv)
	}
	// 公开模板
	pub, err := ps.Create(ctx, alice, PromptInput{
		Name: "通用润色", Category: "写作", Content: "润色：{{正文}}", Variables: `[{"name":"正文"}]`, IsPublic: true,
	})
	if err != nil {
		t.Fatalf("create public: %v", err)
	}

	// bob 只看到公开的那条
	bobList, err := ps.List(ctx, bob, "")
	if err != nil {
		t.Fatalf("bob list: %v", err)
	}
	if len(bobList) != 1 || bobList[0].ID != pub.ID {
		t.Fatalf("bob 应只看到 1 条公开模板，得到 %+v", bobList)
	}
	if bobList[0].Editable {
		t.Fatal("bob 不应可编辑 alice 的模板")
	}
	// bob 读 alice 的私有模板 → NotFound（不泄露存在性）
	if _, err := ps.Get(ctx, bob, priv.ID); !errors.Is(err, ErrPromptNotFound) {
		t.Fatalf("bob 读私有模板应 NotFound，得到 %v", err)
	}
	// bob 改 alice 的公开模板 → Forbidden
	if _, err := ps.Update(ctx, bob, pub.ID, PromptInput{Name: "x", Content: "y"}); !errors.Is(err, ErrPromptForbidden) {
		t.Fatalf("bob 改他人模板应 Forbidden，得到 %v", err)
	}
	// alice 自己列表：2 条
	aliceList, _ := ps.List(ctx, alice, "")
	if len(aliceList) != 2 {
		t.Fatalf("alice 应看到自己的 2 条，得到 %d", len(aliceList))
	}
	// 分类过滤
	if got, _ := ps.List(ctx, alice, "写作"); len(got) != 1 {
		t.Fatalf("按分类过滤应得 1 条，得到 %d", len(got))
	}
	// 管理员可见全部 + 可改
	admList, _ := ps.List(ctx, admin, "")
	if len(admList) != 2 {
		t.Fatalf("管理员应看到全部 2 条，得到 %d", len(admList))
	}
	if _, err := ps.Get(ctx, admin, priv.ID); err != nil {
		t.Fatalf("管理员应能读私有模板: %v", err)
	}
	upd, err := ps.Update(ctx, admin, pub.ID, PromptInput{Name: "通用润色 v2", Content: "润色2：{{正文}}", IsPublic: true})
	if err != nil {
		t.Fatalf("管理员应能改: %v", err)
	}
	if upd.Name != "通用润色 v2" {
		t.Fatalf("改名未生效: %+v", upd)
	}

	// 校验：名称为空 / 正文为空
	if _, err := ps.Create(ctx, alice, PromptInput{Content: "x"}); !errors.Is(err, ErrPromptInvalid) {
		t.Fatalf("空名称应报 ErrPromptInvalid，得到 %v", err)
	}
	if _, err := ps.Create(ctx, alice, PromptInput{Name: "n", Content: "   "}); !errors.Is(err, ErrPromptInvalid) {
		t.Fatalf("空正文应报 ErrPromptInvalid，得到 %v", err)
	}

	// 渲染走仓储
	r, err := ps.Render(ctx, alice, pub.ID, map[string]string{"正文": "今天很好"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if r.Text != "润色2：今天很好" {
		t.Fatalf("渲染结果不对: %q", r.Text)
	}

	// 删除：bob 不行、alice 可以
	if err := ps.Delete(ctx, bob, priv.ID); !errors.Is(err, ErrPromptForbidden) {
		t.Fatalf("bob 删他人模板应 Forbidden，得到 %v", err)
	}
	if err := ps.Delete(ctx, alice, priv.ID); err != nil {
		t.Fatalf("alice 删自己的模板应成功: %v", err)
	}
	if _, err := ps.Get(ctx, alice, priv.ID); !errors.Is(err, ErrPromptNotFound) {
		t.Fatalf("删除后应 NotFound，得到 %v", err)
	}
}
