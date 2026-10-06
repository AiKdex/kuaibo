// settings_org_test.go 设置页运维层门控（B14）回归。
//
// 背景：B4 落下了 org.* 四个运行期子开关，但一直没进 settableKeys 白名单 → 只能直改数据库，
// 线上「组织模块是活的、四个开关永远关着且无开关可点」。本文件守住补上的白名单与联动语义。
//
// 覆盖点：
//  1. 白名单：org.* 四键可写、非法值 400、未知键仍 403；capability.* 支持空串（跟随默认）；
//  2. 联动语义：org.enabled=false 时 tree/department/transfer 一律视同关闭；
//  3. 持久化：default_value 取自 Defaults（不是 '<nil>'，证明已登记为正式配置键）；
//  4. 只读端点：/admin/capabilities 的生效态与判定来源，且能反映被停用的应用中心记录。
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/config"
	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// newGateAPI 建一个「cfg + org 都在位」的最小环境（org 子开关需 OrgStore 求值）。
func newGateAPI(t *testing.T) *API {
	t.Helper()
	db, err := repo.Open(filepath.Join(t.TempDir(), "gate.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	b := bus.New()
	cfg := config.New(db, b)
	aud := service.New(db)
	return &API{db: db, cfg: cfg, aud: aud, b: b, org: service.NewOrgStore(db, b, aud, cfg)}
}

// seedOwnerUser 确保 users 表存在 owner 行。
// C1 修复后 updateSettings 要求管理员身份（isAdmin 查 users.role），
// 本测试是"直调 handler"形态、不过中间件，故需自己把 owner 行与登录身份补齐。
func seedOwnerUser(t *testing.T, a *API) {
	t.Helper()
	if _, err := a.db.Exec(
		`INSERT OR IGNORE INTO users (id, username, pass_hash, role, status, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?)`,
		a.homeOwnerID(), "owner", "x", "owner", "active", 0, 0); err != nil {
		t.Fatalf("seed owner user: %v", err)
	}
}

// putSetting 调真实写接口，返回状态码与响应体。
func putSetting(t *testing.T, a *API, key, val string) (int, string) {
	t.Helper()
	seedOwnerUser(t, a)
	payload, err := json.Marshal(map[string]string{"key": key, "value": val})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", strings.NewReader(string(payload)))
	// 模拟 authMiddleware 注入的登录身份（updateSettings 的管理员守卫依赖它）
	req = req.WithContext(context.WithValue(req.Context(), ctxUserID, a.homeOwnerID()))
	rec := httptest.NewRecorder()
	a.updateSettings(rec, req)
	return rec.Code, rec.Body.String()
}

// orgGates 读 /org/settings 的四道闸门。
func orgGates(t *testing.T, a *API) map[string]bool {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/settings", nil)
	rec := httptest.NewRecorder()
	a.orgSettings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("orgSettings status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Org map[string]bool `json:"org"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal gates: %v", err)
	}
	return out.Org
}

type capItem struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Note       string `json:"note"`
	Enabled    bool   `json:"enabled"`
	Source     string `json:"source"`
	Configured bool   `json:"configured"`
}

func capStates(t *testing.T, a *API) ([]capItem, bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/capabilities", nil)
	rec := httptest.NewRecorder()
	a.capabilityStates(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("capabilityStates status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items           []capItem `json:"items"`
		RestartRequired bool      `json:"restart_required"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal capabilities: %v", err)
	}
	return out.Items, out.RestartRequired
}

// 1) 白名单登记：org.* 四键在位，且接受三态（""/true/false）、拒绝乱值。
func TestSettableKeysOrgGatesRegistered(t *testing.T) {
	for _, k := range []string{"org.enabled", "org.tree", "org.department", "org.transfer"} {
		spec, ok := settableKeys[k]
		if !ok {
			t.Fatalf("%s 未登记进 settableKeys（只能直改数据库）", k)
		}
		if spec.valid == nil {
			t.Fatalf("%s 缺少校验器", k)
		}
		for _, v := range []string{"", "true", "false"} {
			if _, ok := spec.valid(v); !ok {
				t.Fatalf("%s 应接受 %q", k, v)
			}
		}
		if _, ok := spec.valid("maybe"); ok {
			t.Fatalf("%s 不应接受 maybe", k)
		}
	}
	// capability.* 也应支持空串（清除显式配置 → 跟随应用中心/内置默认）
	if _, ok := settableKeys["capability.org"].valid(""); !ok {
		t.Fatal("capability.org 应支持空串（清除显式配置）")
	}
}

// 2) 主流程：总开关开启 → 子开关可用；总开关关闭 → 子开关一律视同关闭。
func TestUpdateSettingsOrgGateFlow(t *testing.T) {
	a := newGateAPI(t)

	if g := orgGates(t, a); g["enabled"] || g["tree"] || g["department"] || g["transfer"] {
		t.Fatalf("初始应四闸全关，得 %v", g)
	}
	// 子开关先开（此时总开关还没开）→ 必须仍为关
	if code, body := putSetting(t, a, "org.tree", "true"); code != http.StatusOK {
		t.Fatalf("写 org.tree 失败 code=%d body=%s", code, body)
	}
	if g := orgGates(t, a); g["tree"] {
		t.Fatal("总开关未开时 tree 不得为 true（子开关必须被总开关压住）")
	}
	// 开总开关 → tree 生效
	if code, body := putSetting(t, a, "org.enabled", "true"); code != http.StatusOK {
		t.Fatalf("写 org.enabled 失败 code=%d body=%s", code, body)
	}
	if g := orgGates(t, a); !g["enabled"] || !g["tree"] {
		t.Fatalf("总开关开启后 enabled/tree 应为 true，得 %v", g)
	}
	// 关总开关 → 子开关全部回落为关（值保留、语义关闭）
	if code, _ := putSetting(t, a, "org.enabled", "false"); code != http.StatusOK {
		t.Fatalf("关总开关失败 code=%d", code)
	}
	if g := orgGates(t, a); g["enabled"] || g["tree"] {
		t.Fatalf("关总开关后应全关，得 %v", g)
	}
	// 空串 = 清除显式配置（回到未配置态）
	if code, _ := putSetting(t, a, "org.tree", ""); code != http.StatusOK {
		t.Fatal("写空串（清除）应成功")
	}
	if v := a.cfg.GetString("org.tree"); v != "" {
		t.Fatalf("清除后 cfg 应为空串，得 %q", v)
	}
}

// 3) 非法值与未知键：非法 400 SETTING_INVALID；未登记键仍 403 SETTING_READONLY。
func TestUpdateSettingsOrgInvalidAndUnknown(t *testing.T) {
	a := newGateAPI(t)
	if code, body := putSetting(t, a, "org.enabled", "maybe"); code != http.StatusBadRequest ||
		!strings.Contains(body, "SETTING_INVALID") {
		t.Fatalf("非法值应 400 SETTING_INVALID，得 code=%d body=%s", code, body)
	}
	for _, k := range []string{"org.whatever", "capability.unknown", "org.enabled.extra"} {
		if code, body := putSetting(t, a, k, "true"); code != http.StatusForbidden ||
			!strings.Contains(body, "SETTING_READONLY") {
			t.Fatalf("%s 应 403 SETTING_READONLY，得 code=%d body=%s", k, code, body)
		}
	}
}

// 4) 持久化：default_value 取自 Defaults（证明 org.* 已登记为正式配置键，而非动态键的 '<nil>'）。
func TestUpdateSettingsOrgPersistsWithDefault(t *testing.T) {
	a := newGateAPI(t)
	if code, body := putSetting(t, a, "org.enabled", "true"); code != http.StatusOK {
		t.Fatalf("写入失败 code=%d body=%s", code, body)
	}
	var val, def string
	if err := a.db.QueryRow(
		`SELECT value, COALESCE(default_value,'') FROM settings WHERE key='org.enabled' AND scope='instance'`).
		Scan(&val, &def); err != nil {
		t.Fatalf("查 settings 行: %v", err)
	}
	if val != "true" {
		t.Fatalf("落库值应为 true，得 %q", val)
	}
	if def != "false" {
		t.Fatalf("default_value 应为 false（Defaults 已登记），得 %q", def)
	}
	// 重启后仍能生效：LoadFromDB 只对 Defaults 中的键走 parseEnv，bool 默认值必须解析回 bool
	if v, ok := a.cfg.Get("org.enabled"); !ok || v != "true" {
		t.Fatalf("写入后内存值应为字符串 true，得 %#v ok=%v", v, ok)
	}
}

// 5) 只读端点：六项生效态 + 来源；默认全开且来源 default。
func TestCapabilityStatesEndpointDefaults(t *testing.T) {
	a := newGateAPI(t)
	items, restart := capStates(t, a)
	if len(items) != len(service.CapabilityNames) {
		t.Fatalf("应返回 %d 项，得 %d", len(service.CapabilityNames), len(items))
	}
	if !restart {
		t.Fatal("能力装配在启动阶段完成，端点应声明 restart_required=true")
	}
	for _, it := range items {
		if it.Name == "" || it.Label == "" {
			t.Fatalf("条目缺少 name/label：%+v", it)
		}
		// 默认值随能力类型而异：核心能力默认开；org/family 是应用中心可安装能力，
		// 空库默认关（未安装=不装配）。两者都是 CapSourceDefault 且未被显式配置。
		wantOn := it.Name != service.CapOrg && it.Name != service.CapFamily &&
			it.Name != service.CapCSInbox && it.Name != service.CapCSContacts &&
			it.Name != service.CapCSChannels && it.Name != service.CapCSAIDraft
		if it.Enabled != wantOn || it.Source != service.CapSourceDefault || it.Configured {
			t.Fatalf("空库默认态不符（%s 期望 enabled=%v），得 %+v", it.Name, wantOn, it)
		}
	}
}

// 6) 只读端点反映站长配置与被停用的应用中心记录。
func TestCapabilityStatesReflectsConfigAndPlugin(t *testing.T) {
	a := newGateAPI(t)
	if code, body := putSetting(t, a, "capability.org", "false"); code != http.StatusOK {
		t.Fatalf("写 capability.org 失败 code=%d body=%s", code, body)
	}
	// 停用一条声明 digest 的 capacity 记录
	if _, err := a.db.Exec(
		`INSERT INTO blog_plugins(id, name, capabilities, capability_mode, enabled, created_at, updated_at)
		 VALUES('pack-x', '能力包 X', '["digest"]', 'capacity', 0, 0, 0)`); err != nil {
		t.Fatalf("insert plugin: %v", err)
	}
	items, _ := capStates(t, a)
	byName := map[string]capItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	if it := byName["org"]; it.Enabled || it.Source != service.CapSourceConfig || !it.Configured {
		t.Fatalf("org 应为 关闭/config/已配置，得 %+v", it)
	}
	if it := byName["digest"]; it.Enabled || it.Source != service.CapSourceDeclaration {
		t.Fatalf("digest 应被停用记录熄掉且来源 plugin-capability，得 %+v", it)
	}
	if it := byName["webdav"]; !it.Enabled || it.Source != service.CapSourceDefault {
		t.Fatalf("webdav 未受影响，应仍为默认开启，得 %+v", it)
	}
}

// callOrgEndpoint 直接调 org 端点 handler（httptest，不带会话）。
// 门控判定位于 isAdmin 之前，故「总开关关闭 → ORG_DISABLED」可在无会话下断言；
// 反过来「未出现门控错误码」即证明门控已放行（其后才会撞上 ORG_ADMIN_REQUIRED）。
func callOrgEndpoint(a *API, fn, method, path string) (int, string) {
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	switch fn {
	case "orgTreeList":
		a.orgTreeList(rec, req)
	case "orgTreeCreate":
		a.orgTreeCreate(rec, req)
	case "orgTreeUpdate":
		a.orgTreeUpdate(rec, req)
	case "orgTreeDelete":
		a.orgTreeDelete(rec, req)
	case "orgTreePaths":
		a.orgTreePaths(rec, req)
	case "orgMembershipsSet":
		a.orgMembershipsSet(rec, req)
	case "orgNodeMembers":
		a.orgNodeMembers(rec, req)
	case "orgMembershipsHistory":
		a.orgMembershipsHistory(rec, req)
	case "orgCustodianCount":
		a.orgCustodianCount(rec, req)
	case "orgDepartmentsCreate":
		a.orgDepartmentsCreate(rec, req)
	case "orgDepartmentsList":
		a.orgDepartmentsList(rec, req)
	case "orgDepartmentsMembers":
		a.orgDepartmentsMembers(rec, req)
	case "orgDepartmentsFiles":
		a.orgDepartmentsFiles(rec, req)
	case "orgTransfersCreate":
		a.orgTransfersCreate(rec, req)
	case "orgTransfersList":
		a.orgTransfersList(rec, req)
	case "orgTransferItems":
		a.orgTransferItems(rec, req)
	case "orgTransferPreview":
		a.orgTransferPreview(rec, req)
	case "orgTransferItemSkip":
		a.orgTransferItemSkip(rec, req)
	case "orgTransferExecute":
		a.orgTransferExecute(rec, req)
	case "orgTransferAccept":
		a.orgTransferAccept(rec, req)
	case "orgTransferCancel":
		a.orgTransferCancel(rec, req)
	default:
		panic("未登记的 org 端点: " + fn)
	}
	return rec.Code, rec.Body.String()
}

// orgEndpoints 全部 org 读写端点（不含 GET /org/settings —— 它是门控查询端点本身，永不被拦）。
var orgEndpoints = []struct{ fn, method, path string }{
	{"orgTreeList", "GET", "/api/v1/org/tree"},
	{"orgTreeCreate", "POST", "/api/v1/org/tree"},
	{"orgTreeUpdate", "PUT", "/api/v1/org/tree/x"},
	{"orgTreeDelete", "DELETE", "/api/v1/org/tree/x"},
	{"orgTreePaths", "GET", "/api/v1/org/tree/paths"},
	{"orgMembershipsSet", "POST", "/api/v1/org/memberships"},
	{"orgNodeMembers", "GET", "/api/v1/org/members?node_id=x"},
	{"orgMembershipsHistory", "GET", "/api/v1/org/memberships"},
	{"orgCustodianCount", "GET", "/api/v1/org/custodian-count"},
	{"orgDepartmentsCreate", "POST", "/api/v1/org/departments"},
	{"orgDepartmentsList", "GET", "/api/v1/org/departments"},
	{"orgDepartmentsMembers", "POST", "/api/v1/org/departments/x/members"},
	{"orgDepartmentsFiles", "GET", "/api/v1/org/departments/x/files"},
	{"orgTransfersCreate", "POST", "/api/v1/org/transfers"},
	{"orgTransfersList", "GET", "/api/v1/org/transfers"},
	{"orgTransferItems", "GET", "/api/v1/org/transfers/x/items"},
	{"orgTransferPreview", "POST", "/api/v1/org/transfers/x/preview"},
	{"orgTransferItemSkip", "POST", "/api/v1/org/transfers/x/items/y"},
	{"orgTransferExecute", "POST", "/api/v1/org/transfers/x/execute"},
	{"orgTransferAccept", "PUT", "/api/v1/org/transfers/x/accept"},
	{"orgTransferCancel", "POST", "/api/v1/org/transfers/x/cancel"},
}

// 6) 总开关必须真实覆盖全部 org 端点。
// B14 实测暴露：org.enabled 此前在 handler 层零使用点 —— 组织树建/改/删在总开关关闭时
// 仍能写库（e2e 真建出了一个节点）。此测试把「总开关 = 摆设」钉死。
func TestOrgMasterGateCoversAllEndpoints(t *testing.T) {
	a := newGateAPI(t)
	for _, e := range orgEndpoints {
		code, body := callOrgEndpoint(a, e.fn, e.method, e.path)
		if code != http.StatusForbidden || !strings.Contains(body, "ORG_DISABLED") {
			t.Fatalf("总开关关闭时 %s 应 403 ORG_DISABLED，实得 code=%d body=%s", e.fn, code, body)
		}
	}
	// 四道闸门全开（总开关 + 组织树 + 部门 + 移交流）后，任何端点都不应再被门控拦截。
	// 注意：只开总开关是不够的 —— 组织树写端点还会被 org.tree 正确拦住，那是预期行为。
	for _, k := range []string{"org.enabled", "org.tree", "org.department", "org.transfer"} {
		if code, body := putSetting(t, a, k, "true"); code != http.StatusOK {
			t.Fatalf("开启 %s 失败 code=%d body=%s", k, code, body)
		}
	}
	for _, e := range orgEndpoints {
		code, body := callOrgEndpoint(a, e.fn, e.method, e.path)
		if strings.Contains(body, "ORG_DISABLED") || strings.Contains(body, "ORG_TREE_DISABLED") {
			t.Fatalf("四道闸门全开，%s 不应再被门控拦截：code=%d body=%s", e.fn, code, body)
		}
	}
}

// 7) 组织树子开关只锁「维护」动作，不锁读取（与 settings 描述一致）。
func TestOrgTreeGateBlocksWritesOnly(t *testing.T) {
	a := newGateAPI(t)
	if code, body := putSetting(t, a, "org.enabled", "true"); code != http.StatusOK {
		t.Fatalf("开启总开关失败 code=%d body=%s", code, body)
	}
	for _, e := range orgEndpoints {
		switch e.fn {
		case "orgTreeCreate", "orgTreeUpdate", "orgTreeDelete":
			code, body := callOrgEndpoint(a, e.fn, e.method, e.path)
			if code != http.StatusForbidden || !strings.Contains(body, "ORG_TREE_DISABLED") {
				t.Fatalf("组织树子开关关闭时 %s 应 403 ORG_TREE_DISABLED，实得 code=%d body=%s", e.fn, code, body)
			}
		}
	}
	for _, e := range orgEndpoints {
		switch e.fn {
		case "orgTreeList", "orgTreePaths", "orgNodeMembers", "orgMembershipsHistory", "orgCustodianCount":
			code, body := callOrgEndpoint(a, e.fn, e.method, e.path)
			if strings.Contains(body, "ORG_TREE_DISABLED") {
				t.Fatalf("%s 是读取类端点，不应被组织树子开关拦截：code=%d body=%s", e.fn, code, body)
			}
		}
	}
	if code, body := putSetting(t, a, "org.tree", "true"); code != http.StatusOK {
		t.Fatalf("开启组织树子开关失败 code=%d body=%s", code, body)
	}
	for _, e := range orgEndpoints {
		switch e.fn {
		case "orgTreeCreate", "orgTreeUpdate", "orgTreeDelete":
			code, body := callOrgEndpoint(a, e.fn, e.method, e.path)
			if strings.Contains(body, "ORG_TREE_DISABLED") || strings.Contains(body, "ORG_DISABLED") {
				t.Fatalf("子开关已开，%s 不应再被门控拦截：code=%d body=%s", e.fn, code, body)
			}
		}
	}
}

// 8) 部门/移交流在总开关关闭时报 ORG_DISABLED（而非误报子开关未开启），避免误导。
func TestOrgSubGatesReportMasterFirst(t *testing.T) {
	a := newGateAPI(t)
	for _, k := range []string{"org.tree", "org.department", "org.transfer"} {
		if code, body := putSetting(t, a, k, "true"); code != http.StatusOK {
			t.Fatalf("开启 %s 失败 code=%d body=%s", k, code, body)
		}
	}
	for _, fn := range []string{"orgDepartmentsList", "orgTransfersList", "orgTreeCreate"} {
		code, body := callOrgEndpoint(a, fn, "GET", "/api/v1/org/x")
		if code != http.StatusForbidden || !strings.Contains(body, "ORG_DISABLED") {
			t.Fatalf("总开关关闭时 %s 应 403 ORG_DISABLED（而非子开关错误码），实得 code=%d body=%s", fn, code, body)
		}
	}
}

// 9) orgPaidGate（R1 / REQ-008）：M1/M2 付费门禁以「站点级 license」为粒度，
// 不再依赖不存在的 users.tier 列。
//   - community 站点、无 feature:org 授权 => 403（拦截）
//   - 注入含 feature:org（scope=all 命中）/pro 的合法 license => 放行
func TestOrgPaidGateSiteLicense(t *testing.T) {
	a := newGateAPI(t)
	uid := "org-paid-test-user"
	if _, err := a.db.Exec(
		`INSERT OR IGNORE INTO users (id, username, pass_hash, role, status, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?)`,
		uid, "u", "x", "member", "active", 0, 0); err != nil {
		t.Fatalf("seed member user: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/departments", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxUserID, uid))

	// (a) 无 license、非 pro => 拦截
	rec := httptest.NewRecorder()
	if a.orgPaidGate(rec, req) {
		t.Fatalf("community 站点、无 feature:org 应通过付费门禁")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("期望 403，得到 %d (%s)", rec.Code, rec.Body.String())
	}

	// (b) 注入上游测试 license（scope=all，命中 feature:org 与 pro）=> 放行
	const lic = "eyJlZGl0aW9uIjoicHJvIiwiZXhwIjoxNzkyMzE4MzcyLCJpYXQiOjE3ODk3MjYzNzIsImlzcyI6ImFpa21hcC1saWNlbnNlIiwianRpIjoidGVzdC1hbGwtc2hlbGxzLTAwMSIsInNjb3BlcyI6WyJhbGwiXX0.95GCzxcHouUVGojwUPltWnXmO-Tum_sYTtq92SXfd9kH0Lcp3Jx-gECm7PhI1MgBDRPHuQGvcI9y0FUSGE14DQ"
	if _, err := a.cfg.Set(context.Background(), licenseKeyCfg, lic, "secret", "test-org-gate", "test"); err != nil {
		t.Fatalf("set license: %v", err)
	}
	rec2 := httptest.NewRecorder()
	if !a.orgPaidGate(rec2, req) {
		t.Fatalf("含 feature:org 授权应通过付费门禁")
	}
}
