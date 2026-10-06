package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// enableCSCapability 按生产真实路径开启能力：在 blog_plugins 装一条 capacity 应用。
// （刻意走应用中心这条路径而非 cfg.Set("capability.*")：断言落在真实的生产判据上，
//  避免只测到 cfg 第 1 层就以为覆盖了能力链。）
func enableCSCapability(t *testing.T, a *API, name string, enabled bool) {
	t.Helper()
	flag := 0
	if enabled {
		flag = 1
	}
	_, err := a.db.ExecContext(context.Background(),
		`INSERT OR REPLACE INTO blog_plugins
		   (id,name,version,enabled,capability_mode,capabilities,created_at,updated_at)
		 VALUES(?,?,?,?,'capacity',?,?,?)`,
		"cap-"+name, name, "0.1.0", flag, `["`+name+`"]`, time.Now().UnixMilli(), time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
}

// TestCSGateCapabilityOffBlocks 锁定「能力关 = 端点全关」。
//
// 回归背景：csBlocked 此前只查 cs.enabled（默认未设 = 放行），导致
// capability.cs.inbox 关闭（内置默认关）时侧栏虽不亮，匿名挂件轨仍可读写——
// 与 org/family「关闭态逐端点 403」的既有语义不一致。
func TestCSGateCapabilityOffBlocks(t *testing.T) {
	a := newTestAPI(t)

	// 显式装一个「已装但停用」的 capacity 应用，覆盖"应用中心装了但关掉"这条路径
	enableCSCapability(t, a, service.CapCSInbox, false)

	on, src := service.CapabilityEnabled(a.db, nil, service.CapCSInbox)
	if on {
		t.Fatalf("停用的 capacity 应用应判为关，得 on=true src=%s", src)
	}

	rec := httptest.NewRecorder()
	if !a.csBlocked(rec) {
		t.Fatal("能力关闭时 csBlocked 应拦截")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("应 403，得 %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error.Code != "CS_DISABLED" {
		t.Fatalf("错误码应 CS_DISABLED，得 %q", body.Error.Code)
	}
}

// TestCSGateCapabilityOnAllows 能力启用后总闸放行。
func TestCSGateCapabilityOnAllows(t *testing.T) {
	a := newTestAPI(t)
	enableCSCapability(t, a, service.CapCSInbox, true)

	rec := httptest.NewRecorder()
	if a.csBlocked(rec) {
		t.Fatalf("能力开启后不应拦截，却写了 %d %s", rec.Code, rec.Body.String())
	}
}

// TestCSGateKillSwitch 总闸 cs.enabled=false 时即使能力开着也全关（出问题时快速止血）。
// 走 a.cfg 判据，因此这里给 API 挂一个最小 cfg 实现。
func TestCSGateKillSwitch(t *testing.T) {
	a := newTestAPI(t)
	if _, err := a.cfg.Set(context.Background(), "cs.enabled", "false", "bool", "", "test"); err != nil {
		t.Fatal(err)
	}
	enableCSCapability(t, a, service.CapCSInbox, true)

	rec := httptest.NewRecorder()
	if !a.csBlocked(rec) {
		t.Fatal("cs.enabled=false 时应拦截")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("应 403，得 %d", rec.Code)
	}
}

// TestCSGateKillSwitchAllowsWhenTrue cs.enabled=true 不应误拦。
func TestCSGateKillSwitchAllowsWhenTrue(t *testing.T) {
	a := newTestAPI(t)
	if _, err := a.cfg.Set(context.Background(), "cs.enabled", "true", "bool", "", "test"); err != nil {
		t.Fatal(err)
	}
	enableCSCapability(t, a, service.CapCSInbox, true)

	rec := httptest.NewRecorder()
	if a.csBlocked(rec) {
		t.Fatalf("不应拦截，却写了 %d", rec.Code)
	}
}

// TestExtractCSAnswer 起草结果解析：容忍 JSON 包装与纯文本两种模型返回。
func TestExtractCSAnswer(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"answer":"建议回复A"}`, "建议回复A"},
		{`{"reply":"建议回复B"}`, "建议回复B"},
		{`{"content":"建议回复C"}`, "建议回复C"},
		{`直接纯文本回复`, "直接纯文本回复"},
		{`  {"answer":"  带空格  "}  `, "带空格"},
		{`{"other":"x"}`, `{"other":"x"}`}, // 无可识别键 → 原样返回，不吞内容
		{``, ``},
	}
	for _, c := range cases {
		if got := extractCSAnswer(c.in); got != c.want {
			t.Fatalf("extractCSAnswer(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}
