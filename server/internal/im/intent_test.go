// intent_test.go IM 意图路由回归（B2 相关工作流命名空间部分）。
//
// 重点：/wf 命名空间必须与既有固定指令互不干扰 —— 这是把上游
// "每个 workflow 占一个全局前缀" 改成本壳命名空间方案后的第一风险点。
package im

import "testing"

func TestRouteWorkflowNamespace(t *testing.T) {
	r := NewIntentRouter()
	cases := []struct {
		text  string
		want  IntentType
		query string
	}{
		{"/wf", IntentWorkflow, ""},
		{"/wf list", IntentWorkflow, "list"},
		{"/wf wf-polish-to-draft 一段素材", IntentWorkflow, "wf-polish-to-draft 一段素材"},
		{"/wf-polish 一段素材", IntentWorkflow, "-polish 一段素材"},
		{"/wf-draft 标题", IntentWorkflow, "-draft 标题"},
		{"/WF-POLISH 大小写不敏感", IntentWorkflow, "-POLISH 大小写不敏感"},
	}
	for _, c := range cases {
		got := r.Route(c.text)
		if got.Type != c.want {
			t.Fatalf("Route(%q).Type = %q, want %q", c.text, got.Type, c.want)
		}
		if got.Params["query"] != c.query {
			t.Fatalf("Route(%q).query = %q, want %q", c.text, got.Params["query"], c.query)
		}
	}
}

// TestRouteWorkflowDoesNotStealExistingCommands /wf 命名空间不得影响既有指令语义。
func TestRouteWorkflowDoesNotStealExistingCommands(t *testing.T) {
	r := NewIntentRouter()
	cases := []struct {
		text string
		want IntentType
	}{
		{"/draft 正文", IntentDraft},
		{"/save 正文", IntentDraft},
		{"/post 正文", IntentPost},
		{"/publish 正文", IntentPost},
		{"/search 关键词", IntentSearch},
		{"/help", IntentHelp},
	}
	for _, c := range cases {
		if got := r.Route(c.text); got.Type != c.want {
			t.Fatalf("Route(%q).Type = %q, want %q（既有指令被 /wf 命名空间抢走）", c.text, got.Type, c.want)
		}
	}
}

// TestRouteNonWorkflowSlashNotHijacked 相似但非法的写法不得被误判为工作流。
func TestRouteNonWorkflowSlashNotHijacked(t *testing.T) {
	r := NewIntentRouter()
	for _, text := range []string{"/wfx", "/wfoo", "/wf_polish", "/waterfall"} {
		if got := r.Route(text); got.Type == IntentWorkflow {
			t.Fatalf("Route(%q) 被误判为工作流", text)
		}
	}
}
