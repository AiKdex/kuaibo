// Package im 实现 IM 网关（平台无关核心）：统一消息结构 + 三层意图识别。
//
// 移植自 Kmap 的 services/im（Python/FastAPI）→ AiKmap Go 后端。
// 设计要点：
//   - 意图识别三层策略：前缀命令（确定性 100%）→ 正则模式（80%+）→ Agent 兜底；
//   - 业务分发不在本包（由 handler 注入回调，避免包循环依赖）；
//   - 身份映射：站内用户 ↔ IM 身份由 handler 侧 im_binding 服务解析（B7）；
//     本包只负责"意图"，不认识用户。未绑定时 handler 回退到系统 owner（零破坏单用户部署）。
package im

import (
	"regexp"
	"strings"
)

// IntentType 意图类型。
type IntentType string

const (
	IntentText     IntentType = "ingest_text"  // 保存文本碎片
	IntentURL      IntentType = "ingest_url"   // 剪藏链接
	IntentSearch   IntentType = "search"       // 检索
	IntentList     IntentType = "list_docs"    // 最近文档
	IntentRetrieve IntentType = "retrieve_doc" // 取回文档内容
	IntentSummary  IntentType = "summary"      // 总结（规则摘要，AI 摘要走 Agent）
	IntentHelp     IntentType = "help"
	IntentStatus   IntentType = "status"
	IntentStart    IntentType = "start"
	IntentAgent    IntentType = "agent" // 开放请求 → Agent 层
	IntentPost     IntentType = "blog_post"  // 发博客（发布）
	IntentDraft    IntentType = "blog_draft" // 存为博客草稿
	IntentWorkflow IntentType = "workflow"   // 内容工作流（/wf 命名空间）
	IntentBind     IntentType = "im_bind"    // B7：/bind <码> 消费绑定码，把 IM 身份绑到站内账号
	IntentUnbind   IntentType = "im_unbind"  // B7：/unbind 解除当前 IM 身份的绑定
	IntentUnknown  IntentType = "unknown"
)

// IntentResult 意图识别结果。
type IntentResult struct {
	Type   IntentType
	Params map[string]string
	Raw    string
}

var urlRe = regexp.MustCompile(`(?i)https?://\S+`)

// IntentRouter 三层意图路由（无状态，可并发安全使用）。
type IntentRouter struct {
	prefix  map[string]IntentType
	patterns []pattern
}

type pattern struct {
	re *regexp.Regexp
	t  IntentType
}

// NewIntentRouter 构造路由表。
func NewIntentRouter() *IntentRouter {
	r := &IntentRouter{
		prefix: map[string]IntentType{
			"/help":     IntentHelp,
			"/search":   IntentSearch,
			"/post":     IntentPost,    // 发博客（立即发布）
			"/publish":  IntentPost,    // 发博客别名
			"/draft":    IntentDraft,   // 存为博客草稿
			"/save":     IntentDraft,   // 保存/存 → 博客草稿
			"/list":     IntentList,
			"/get":      IntentRetrieve,
			"/summary":  IntentSummary,
			"/status":   IntentStatus,
			"/start":    IntentStart,
			"/bind":     IntentBind,   // B7：/bind <绑定码>（码由网页端「IM 绑定」页生成）
			"/unbind":   IntentUnbind, // B7：解除当前 IM 身份与本账号的绑定
			"/regist":   IntentHelp,   // 单用户阶段占位：/regist 提示
			"/register": IntentHelp,
		},
		patterns: []pattern{
			{regexp.MustCompile(`^(搜索|找|查|查一下|搜一下|检索)\s*(.+)`), IntentSearch},
			{regexp.MustCompile(`^(保存|存|收藏|记录|记下|备注|记)\s*(.+)`), IntentDraft},
			{regexp.MustCompile(`^(总结|摘要|提炼)\s*(.+)`), IntentSummary},
			{regexp.MustCompile(`^(发给我|给我|下载|看看|打开)\s*(.+)`), IntentRetrieve},
			{regexp.MustCompile(`^(最近|今天|本周)\s*(文档|更新|动态)?$`), IntentList},
			{regexp.MustCompile(`^(列表|全部|我的文档|文档列表)$`), IntentList},
			{regexp.MustCompile(`^(帮助|怎么用|帮助文档|说明)$`), IntentHelp},
			{regexp.MustCompile(`^(状态|status)$`), IntentStatus},
		},
	}
	return r
}

// Route 三层识别。text 为已 Trim 的原始消息。
func (r *IntentRouter) Route(text string) IntentResult {
	text = strings.TrimSpace(text)
	if text == "" {
		return IntentResult{Type: IntentUnknown, Params: map[string]string{}, Raw: text}
	}
	// Layer 0：工作流命名空间 —— /wf <id> [正文] 或 /wf-<name> [正文]。
	// 独立于下面的固定前缀表：workflow id 是开放的（站长自定义 / 应用中心安装），
	// 塞进 prefix 表会与既有指令抢键（/post /draft 等已被占用），且市场装多个包必然互相撞车。
	low := strings.ToLower(text)
	if low == "/wf" || strings.HasPrefix(low, "/wf ") || strings.HasPrefix(low, "/wf-") {
		return IntentResult{Type: IntentWorkflow, Params: map[string]string{"query": strings.TrimSpace(text[3:])}, Raw: text}
	}
	// Layer 1：前缀命令（精确/前缀匹配）
	for prefix, t := range r.prefix {
		if low == prefix || strings.HasPrefix(low, prefix+" ") {
			rest := strings.TrimSpace(text[len(prefix):])
			return IntentResult{Type: t, Params: map[string]string{"query": rest}, Raw: text}
		}
	}
	// Layer 2：正则模式
	for _, p := range r.patterns {
		if m := p.re.FindStringSubmatch(text); m != nil {
			q := ""
			if len(m) > 1 {
				q = strings.TrimSpace(m[len(m)-1])
			}
			return IntentResult{Type: p.t, Params: map[string]string{"query": q}, Raw: text}
		}
	}
	// Layer 2.5：纯 URL → 剪藏
	if urlRe.MatchString(text) && len(text) < 2000 {
		return IntentResult{Type: IntentURL, Params: map[string]string{"url": urlRe.FindString(text)}, Raw: text}
	}
	// Layer 3：开放请求 → Agent
	return IntentResult{Type: IntentAgent, Params: map[string]string{"query": text}, Raw: text}
}
