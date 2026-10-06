// blog_plugins_v2_test.go 能力插件协议 v2 落地回归（docs/能力插件协议-v2草案.md §1、§4）。
//
// 覆盖点（REQ-001 / REQ-004）：
//  1. repo.Migrate 后 blog_plugins 具备 v2 六列（capabilities/backend_entry/routes/min_schema/max_schema/capability_mode）；
//  2. normalizePluginV2Kind 把上游生包 kind=ui|capacity 迁到 capability_mode，kind 归一为本壳语义；
//      ——本壳 kind 承载应用中心安装类型（plugin|theme），与 v2 的 ui|capacity 取值域冲突，不可混用；
//  3. validatePluginV2 五重拒绝：capability_mode 非法域、capability 白名单、capacity 缺 backend_entry、
//     routes 越界、schema 区间；
//  4. blogPluginUpsert → upsertPluginManifest 真实落库，且 list 输出 v2 字段。
package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/config"
	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// newTestAPI 构造最小 API（db + 空 cfg/aud 即可覆盖插件登记链路）。
func newTestAPI(t *testing.T) *API {
	t.Helper()
	db, err := repo.Open(filepath.Join(t.TempDir(), "v2test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	b := bus.New()
	return &API{db: db, cfg: config.New(db, b), aud: service.New(db), b: b}
}

func TestBlogPluginsV2MigrationColumns(t *testing.T) {
	a := newTestAPI(t)
	want := []string{"capabilities", "backend_entry", "routes", "min_schema", "max_schema", "capability_mode"}
	for _, col := range want {
		var n int
		if err := a.db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('blog_plugins') WHERE name=?`, col).Scan(&n); err != nil {
			t.Fatalf("pragma %s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("迁移缺列 %s：pragma_table_info 命中 %d", col, n)
		}
	}
}

func TestNormalizePluginV2Kind(t *testing.T) {
	cases := []struct{ kind, mode, wantKind, wantMode string }{
		{"ui", "", "plugin", "ui"},        // 上游生包 ui → 归一 plugin + capability_mode=ui
		{"capacity", "", "plugin", "capacity"}, // 上游 capacity 包 → 能力分类迁移
		{"plugin", "", "plugin", "ui"},
		{"theme", "", "theme", "ui"},
		{"", "capacity", "plugin", "capacity"},
		{"capacity", "ui", "plugin", "capacity"}, // capability_mode 显式优先，不被覆盖
	}
	for _, c := range cases {
		m := pluginManifest{Kind: c.kind, CapabilityMode: c.mode}
		normalizePluginV2Kind(&m)
		if m.Kind != c.wantKind || m.CapabilityMode != c.wantMode {
			t.Errorf("kind=%q mode=%q → (%q,%q)，期望 (%q,%q)",
				c.kind, c.mode, m.Kind, m.CapabilityMode, c.wantKind, c.wantMode)
		}
	}
}

func TestValidatePluginV2(t *testing.T) {
	ok := pluginManifest{ID: "p1", CapabilityMode: "capacity", Capabilities: []string{"collector"}, BackendEntry: "collector/engine", Routes: []string{"/api/v1/plugins/p1/run"}}
	if code, msg := validatePluginV2(ok, 1); code != "" {
		t.Fatalf("合法 capacity 被拒：%s %s", code, msg)
	}

	cases := []struct {
		name, wantCode string
		m              pluginManifest
	}{
		{"capability_mode 非法域", "PLUGIN_CAPABILITY_MODE_INVALID", pluginManifest{ID: "p", CapabilityMode: "plugin"}},
		{"capability 越白名单", "PLUGIN_CAPABILITY_INVALID", pluginManifest{ID: "p", CapabilityMode: "capacity", Capabilities: []string{"mine-bitcoin"}, BackendEntry: "x"}},
		{"capacity 缺 capabilities", "PLUGIN_CAPABILITY_MISSING", pluginManifest{ID: "p", CapabilityMode: "capacity", BackendEntry: "x"}},
		{"capacity 缺 backend_entry", "PLUGIN_BACKEND_MISSING", pluginManifest{ID: "p", CapabilityMode: "capacity", Capabilities: []string{"collector"}}},
		{"routes 越界", "PLUGIN_ROUTE_FORBIDDEN", pluginManifest{ID: "p", CapabilityMode: "capacity", Capabilities: []string{"collector"}, BackendEntry: "x", Routes: []string{"/api/v1/admin/users"}}},
	}
	for _, c := range cases {
		code, _ := validatePluginV2(c.m, 1)
		if code != c.wantCode {
			t.Errorf("%s：返回 %q，期望 %q", c.name, code, c.wantCode)
		}
	}
	// schema 区间
	maxOK := 1
	if code, _ := validatePluginV2(pluginManifest{ID: "p", MinSchema: 2}, 1); code != "PLUGIN_CORE_TOO_OLD" {
		t.Errorf("min_schema 超界应拒，返回 %q", code)
	}
	if code, _ := validatePluginV2(pluginManifest{ID: "p", MaxSchema: &maxOK}, 2); code != "PLUGIN_CORE_TOO_OLD" {
		t.Errorf("max_schema 超界应拒，返回 %q", code)
	}
}

func postManifest(t *testing.T, a *API, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/blog/plugins", bytes.NewReader(raw))
	// H1 修复后插件 upsert 走 blogAdminOnly（只认显式登录身份），直调 handler 需自补身份
	a.blogPluginUpsert(httptest.NewRecorder(), asOwner(a, req))
	rec := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/blog/plugins", bytes.NewReader(raw))
	a.blogPluginUpsert(rec, asOwner(a, req))
	return rec
}

func TestBlogPluginUpsertV2EndToEnd(t *testing.T) {
	a := newTestAPI(t)

	// 1) 上游语义 capacity 包（kind=capacity）应被接受，且 capability_mode 落库为 capacity
	rec := postManifest(t, a, map[string]any{
		"id": "cap-collector", "name": "采集引擎", "kind": "capacity",
		"capabilities": []string{"collector"}, "backend_entry": "collector/engine",
		"routes": []string{"/api/v1/plugins/cap-collector/run"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("capacity 包注册应 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	var kindOut, modeOut, capsOut, beOut string
	err := a.db.QueryRow(
		`SELECT kind, capability_mode, capabilities, backend_entry FROM blog_plugins WHERE id='cap-collector'`).
		Scan(&kindOut, &modeOut, &capsOut, &beOut)
	if err != nil {
		t.Fatalf("查 capacity 记录：%v", err)
	}
	if kindOut != "plugin" {
		t.Errorf("kind 应归一为 plugin（应用中心类型列），实际 %q", kindOut)
	}
	if modeOut != "capacity" {
		t.Errorf("capability_mode 应为 capacity，实际 %q", modeOut)
	}
	if capsOut != `["collector"]` || beOut != "collector/engine" {
		t.Errorf("capabilities/backend_entry 落库不符：%q / %q", capsOut, beOut)
	}

	// 2) kind 语义不被污染：theme 仍是 theme
	if rec := postManifest(t, a, map[string]any{"id": "theme-x", "name": "某主题", "kind": "theme"}); rec.Code != http.StatusOK {
		t.Fatalf("theme 注册应 200，实际 %d", rec.Code)
	}
	var k string
	_ = a.db.QueryRow(`SELECT kind FROM blog_plugins WHERE id='theme-x'`).Scan(&k)
	if k != "theme" {
		t.Errorf("theme 应保持 theme，实际 %q", k)
	}

	// 3) 非法 capacity 包被拒（缺 backend_entry）
	rec = postManifest(t, a, map[string]any{
		"id": "bad-cap", "name": "缺后端入口", "kind": "capacity", "capabilities": []string{"collector"},
	})
	if rec.Code != http.StatusUnprocessableEntity || !bytes.Contains(rec.Body.Bytes(), []byte("PLUGIN_BACKEND_MISSING")) {
		t.Errorf("缺 backend_entry 应 422 PLUGIN_BACKEND_MISSING，实际 %d %s", rec.Code, rec.Body.String())
	}
	var cnt int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM blog_plugins WHERE id='bad-cap'`).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("校验失败的包不应落库，实际 %d 行", cnt)
	}

	// 4) list 输出 v2 字段
	lrec := httptest.NewRecorder()
	a.blogPluginList(lrec, httptest.NewRequest(http.MethodGet, "/api/v1/blog/plugins", nil))
	if lrec.Code != http.StatusOK {
		t.Fatalf("list 应 200，实际 %d %s", lrec.Code, lrec.Body.String())
	}
	var out struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(lrec.Body.Bytes(), &out); err != nil {
		t.Fatalf("list 解析：%v", err)
	}
	var hit map[string]any
	for _, it := range out.Items {
		if it["id"] == "cap-collector" {
			hit = it
		}
	}
	if hit == nil {
		t.Fatal("list 未返回 cap-collector")
	}
	if hit["capability_mode"] != "capacity" || hit["backend_entry"] != "collector/engine" {
		t.Errorf("list v2 字段不符：mode=%v backend_entry=%v", hit["capability_mode"], hit["backend_entry"])
	}
	if hit["kind"] != "plugin" {
		t.Errorf("list kind 应为 plugin，实际 %v", hit["kind"])
	}
}

// TestCapabilityGateQueryReady 确保能力门控的 capacity 判据能命中（不再依赖 kind='capacity'）。
func TestCapabilityGateQueryReady(t *testing.T) {
	a := newTestAPI(t)
	// 应用 id 与 name 都不等于能力名（第三方包的常态），仅 capabilities 数组能命中
	postManifest(t, a, map[string]any{
		"id": "collector-app", "name": "采集引擎", "kind": "capacity",
		"capabilities": []string{"collector"}, "backend_entry": "collector/engine",
	})
	var enabled int
	err := a.db.QueryRow(
		`SELECT enabled FROM blog_plugins
		  WHERE (capability_mode='capacity' OR kind='capacity') AND (id=? OR name=?)
		  ORDER BY enabled DESC LIMIT 1`, "collector", "collector").Scan(&enabled)
	if err == nil {
		t.Fatalf("id/name 不应命中能力名（应用 id=collector-app, name=采集引擎），但命中了 enabled=%d", enabled)
	}
	// capabilities 数组命中（第三方包名≠能力名的常规情形）
	var jsonEnabled int
	if err := queryCapabilityEnabled(a.db, "collector", &jsonEnabled); err != nil {
		t.Fatalf("capabilities 数组判据应命中 collector：%v", err)
	}
	if jsonEnabled != 1 {
		t.Errorf("capabilities 命中的记录应 enabled=1，实际 %d", jsonEnabled)
	}

	// 停用必须能熄灭能力：判据里**不能**用 enabled=1 过滤，否则查不到记录会退回内置默认 true
	// （曾出现的真实缺陷：应用停用后能力仍开着）。
	if _, err := a.db.Exec(`UPDATE blog_plugins SET enabled=0 WHERE id='collector-app'`); err != nil {
		t.Fatalf("停用：%v", err)
	}
	var off int
	if err := queryCapabilityEnabled(a.db, "collector", &off); err != nil {
		t.Fatalf("停用后仍应命中记录（以其 enabled 为准）：%v", err)
	}
	if off != 0 {
		t.Errorf("停用后 enabled 应为 0，实际 %d", off)
	}
}

// queryCapabilityEnabled 复刻 cmd/aikmap/capabilities.go 的能力判据（capabilities 数组分支）。
// 放在此处是为了让「应用停用 → 能力熄灭」的口径有回归保护：两处必须保持一致。
func queryCapabilityEnabled(db *sql.DB, name string, out *int) error {
	return db.QueryRow(
		`SELECT enabled FROM blog_plugins
		  WHERE capability_mode='capacity'
		    AND EXISTS (SELECT 1 FROM json_each(blog_plugins.capabilities) WHERE json_each.value=?)
		  ORDER BY enabled DESC LIMIT 1`, name).Scan(out)
}

var _ = sql.ErrNoRows
