// workflow_test.go 内容工作流引擎（B2）回归。
//
// 覆盖点：
//  1. 内置官方定义的契约自检 —— 前缀命名空间纪律、步骤/依赖均已注册、id 唯一；
//     这几条是"以后加步骤时容易忘"的地方，用断言钉住比靠人记可靠。
//  2. 模板求值 / 条件路由 / IM 参数拆分的纯函数行为。
//  3. 执行器语义：condition 命中 abort 终止、未注册步骤失败、幂等键只落一次记录。
//  4. 迁移落地：workflow_defs / workflow_runs 两表就位。
package handler

import (
	"context"
	"strings"
	"testing"
)

// TestWfOfficialDefinitionsSanity 内置工作流定义的契约自检。
func TestWfOfficialDefinitionsSanity(t *testing.T) {
	seenID := map[string]bool{}
	seenPrefix := map[string]bool{}
	for _, wf := range wfOfficial() {
		if wf.ID == "" {
			t.Fatal("内置工作流缺 id")
		}
		if seenID[wf.ID] {
			t.Fatalf("内置工作流 id 重复: %s", wf.ID)
		}
		seenID[wf.ID] = true
		if len(wf.Steps) == 0 {
			t.Fatalf("%s 没有 steps", wf.ID)
		}
		// 触发前缀必须落在 /wf 命名空间：本壳 /post、/draft、/publish 等已被既有 IM 指令占用，
		// 裸前缀会与之抢键（上游按"前缀全局唯一"设计，在本壳不成立）。
		for _, tg := range wf.Triggers {
			if tg.Type != "im_command" {
				continue
			}
			if !strings.HasPrefix(tg.Prefix, "/wf") {
				t.Fatalf("%s 的触发前缀 %q 不在 /wf 命名空间内（会与既有 IM 指令抢键）", wf.ID, tg.Prefix)
			}
			if seenPrefix[tg.Prefix] {
				t.Fatalf("触发前缀重复: %s", tg.Prefix)
			}
			seenPrefix[tg.Prefix] = true
		}
		// 注册表步骤必须已注册；显式声明的 dependencies 同样必须在册。
		for _, st := range wf.Steps {
			if st.Type != "" {
				continue
			}
			if _, ok := wfRegistry[st.Tool]; !ok {
				t.Fatalf("%s 引用未注册步骤 %q", wf.ID, st.Tool)
			}
		}
		for _, dep := range wf.Dependencies {
			if _, ok := wfRegistry[dep]; !ok {
				t.Fatalf("%s 声明依赖 %q 但未注册", wf.ID, dep)
			}
		}
	}
}

func TestWfEvalTpl(t *testing.T) {
	rc := &wfRunCtx{
		MsgText: "原始正文",
		MsgFile: "file-1",
		UID:     "u-1",
		SiteID:  "site-b",
		Prev:    map[string]any{"text": "上一步产出", "n": 7},
	}
	got := wfEvalTpl("$msg.text | $msg.file | $ctx.user | $ctx.site | $prev.text | $prev.n", rc)
	want := "原始正文 | file-1 | u-1 | site-b | 上一步产出 | 7"
	if got != want {
		t.Fatalf("wfEvalTpl = %q, want %q", got, want)
	}
	// $prev 里的非字符串值也要能落到模板里（JSON 解出的可能是数字/布尔）。
	if s := wfEvalTpl("$prev.n", rc); s != "7" {
		t.Fatalf("数值 $prev 求值 = %q, want 7", s)
	}
}

func TestWfEvalArgs(t *testing.T) {
	rc := &wfRunCtx{MsgText: "abc", Prev: map[string]any{}}
	args := wfEvalArgs(map[string]string{"query": "$msg.text", "limit": "5"}, rc)
	if args["query"] != "abc" || args["limit"] != "5" {
		t.Fatalf("wfEvalArgs = %#v", args)
	}
}

func TestWfEvalCond(t *testing.T) {
	rc := &wfRunCtx{Prev: map[string]any{"text": "hello world", "empty": ""}}
	cases := []struct {
		expr string
		want bool
	}{
		{`$prev.text contains "world"`, true},
		{`$prev.text contains "nope"`, false},
		{`$prev.text == "hello world"`, true},
		{`$prev.text != "other"`, true},
		{`$prev.text not empty`, true},
		{`$prev.empty not empty`, false},
	}
	for _, c := range cases {
		got, err := wfEvalCond(c.expr, rc)
		if err != nil {
			t.Fatalf("wfEvalCond(%q) 报错: %v", c.expr, err)
		}
		if got != c.want {
			t.Fatalf("wfEvalCond(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
	// 不支持的表达式必须显式报错，而不是静默 false（否则工作流会"看起来跑了但没生效"）。
	if _, err := wfEvalCond("$prev.text 随便写点啥", rc); err == nil {
		t.Fatal("非法条件表达式应当返回错误")
	}
}

func TestWfEvalRouteFirstHitWins(t *testing.T) {
	rc := &wfRunCtx{Prev: map[string]any{"a": "x"}}
	action, err := wfEvalRoute([]wfRoute{
		{When: `$prev.a == "nope"`, Action: "skip"},
		{When: `$prev.a == "x"`, Action: "abort"},
		{When: `$prev.a == "x"`, Action: "skip"},
	}, rc)
	if err != nil {
		t.Fatalf("wfEvalRoute: %v", err)
	}
	if action != "abort" {
		t.Fatalf("应当命中第一条匹配的路由，got %q", action)
	}
}

func TestWfSplitFirst(t *testing.T) {
	cases := []struct{ in, id, rest string }{
		{"wf-polish 素材文字", "wf-polish", "素材文字"},
		{"wf-polish", "wf-polish", ""},
		{"  spaced  text  ", "spaced", "text"},
		{"", "", ""},
	}
	for _, c := range cases {
		id, rest := wfSplitFirst(c.in)
		if id != c.id || rest != c.rest {
			t.Fatalf("wfSplitFirst(%q) = (%q, %q), want (%q, %q)", c.in, id, rest, c.id, c.rest)
		}
	}
}

func TestWfMatchPrefix(t *testing.T) {
	a := newTestAPI(t)
	wf, prefix := a.wfMatchPrefix("/wf-polish 一段素材")
	if wf == nil || wf.ID != "wf-polish-to-draft" {
		t.Fatalf("未命中内置工作流: %#v", wf)
	}
	if prefix != "/wf-polish" {
		t.Fatalf("prefix = %q, want /wf-polish", prefix)
	}
	if got := wfStripTrigger("/wf-polish 一段素材", prefix); got != "一段素材" {
		t.Fatalf("wfStripTrigger = %q", got)
	}
	// 前缀必须完整匹配或以空格分隔，避免 /wf-polishX 误命中 /wf-polish。
	if wf, _ := a.wfMatchPrefix("/wf-polishing 素材"); wf != nil {
		t.Fatalf("前缀粘连不应命中，got %s", wf.ID)
	}
}

// TestWfRunConditionAbort condition 命中 abort 时终止整个工作流。
func TestWfRunConditionAbort(t *testing.T) {
	a := newTestAPI(t)
	wf := &Workflow{
		ID: "wf-test-abort", Name: "条件终止", Enabled: true,
		Steps: []wfStep{{Type: "condition", Routes: []wfRoute{{When: `$prev.flag == "yes"`, Action: "abort"}}}},
	}
	rc := &wfRunCtx{MsgKey: "k-abort", Prev: map[string]any{"flag": "yes"}}
	detail, ok := a.wfRun(context.Background(), wf, rc)
	if ok {
		t.Fatal("命中 abort 应当返回失败")
	}
	if !strings.Contains(detail, "命中 abort") {
		t.Fatalf("执行详情应记录 abort：%s", detail)
	}
}

// TestWfRunUnregisteredStep 未注册步骤必须失败且给出可读原因（而非 panic）。
func TestWfRunUnregisteredStep(t *testing.T) {
	a := newTestAPI(t)
	wf := &Workflow{
		ID: "wf-test-missing", Name: "未注册步骤", Enabled: true,
		Steps: []wfStep{{Tool: "not_registered_step", OnFail: "abort"}},
	}
	rc := &wfRunCtx{MsgKey: "k-missing", Prev: map[string]any{}}
	detail, ok := a.wfRun(context.Background(), wf, rc)
	if ok {
		t.Fatal("未注册步骤应当失败")
	}
	if !strings.Contains(detail, "未注册") {
		t.Fatalf("详情应说明未注册：%s", detail)
	}
}

// TestWfRunOnFailSkip on_fail=skip 时跳过该步并继续，整体仍算成功。
func TestWfRunOnFailSkip(t *testing.T) {
	a := newTestAPI(t)
	wf := &Workflow{
		ID: "wf-test-skip", Name: "跳过失败步骤", Enabled: true,
		Steps: []wfStep{
			{Tool: "not_registered_step", OnFail: "skip"},
			{Type: "condition", Routes: []wfRoute{{When: `$prev.x == "nope"`, Action: "abort"}}},
		},
	}
	rc := &wfRunCtx{MsgKey: "k-skip", Prev: map[string]any{}}
	_, ok := a.wfRun(context.Background(), wf, rc)
	if !ok {
		t.Fatal("on_fail=skip 应使整体继续并成功")
	}
}

// TestWfRunIdempotent 同一 (wf_id, msg_key) 重复执行只落一条记录 —— 幂等闸门本体。
func TestWfRunIdempotent(t *testing.T) {
	a := newTestAPI(t)
	wf := &Workflow{
		ID: "wf-test-idem", Name: "幂等", Enabled: true,
		Steps: []wfStep{{Type: "condition", Routes: []wfRoute{{When: `$prev.x == "nope"`, Action: "abort"}}}},
	}
	for i := 0; i < 2; i++ {
		rc := &wfRunCtx{MsgKey: "same-key", Prev: map[string]any{}}
		if _, ok := a.wfRun(context.Background(), wf, rc); !ok {
			t.Fatalf("第 %d 次执行应当成功", i+1)
		}
	}
	var n int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM workflow_runs WHERE wf_id=? AND msg_key=?`, wf.ID, "same-key").Scan(&n); err != nil {
		t.Fatalf("查询 runs: %v", err)
	}
	if n != 1 {
		t.Fatalf("幂等键应只落 1 条记录，got %d", n)
	}
}

// TestWorkflowTablesMigrated 两表随迁移就位（本壳曾把 workflow_defs 标为"已裁剪"）。
func TestWorkflowTablesMigrated(t *testing.T) {
	a := newTestAPI(t)
	for _, tbl := range []string{"workflow_defs", "workflow_runs"} {
		var name string
		if err := a.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name); err != nil {
			t.Fatalf("表 %s 未随迁移创建: %v", tbl, err)
		}
	}
}

// TestWfSaveAndFind 落库 + 取回闭环（发布的自定义工作流可被 wfFind 解析）。
func TestWfSaveAndFind(t *testing.T) {
	a := newTestAPI(t)
	wf := &Workflow{
		ID: "wf-test-saved", Name: "自定义工作流", Enabled: true,
		Triggers: []wfTrigger{{Type: "im_command", Prefix: "/wf-saved"}},
		Steps:    []wfStep{{Tool: "blog_search", In: map[string]string{"query": "$msg.text"}}},
	}
	if err := a.wfPersist(context.Background(), wf, ""); err != nil {
		t.Fatalf("wfPersist: %v", err)
	}
	got := a.wfFind("wf-test-saved")
	if got == nil || got.Name != "自定义工作流" {
		t.Fatalf("wfFind 未取回已发布工作流: %#v", got)
	}
	if len(got.Steps) != 1 || got.Steps[0].Tool != "blog_search" {
		t.Fatalf("取回的 steps 不正确: %#v", got.Steps)
	}
	// 内置官方同样可被 wfFind 解析（不必落库）。
	if got := a.wfFind("wf-find-content"); got == nil || got.Name != "检索内容" {
		t.Fatalf("wfFind 未解析内置工作流: %#v", got)
	}
}
