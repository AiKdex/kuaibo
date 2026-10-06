// Package handler 的 workflow.go 实现「内容工作流」步骤链引擎（B2 批次）。
//
// 移植自上游 AiKmap.cn 的 handler/workflow.go（P0-2 最小 workflow 执行器），
// 契约同《AgentTools 开发规范》§7：triggers / dependencies / steps / $msg.$prev.$ctx 模板 / on_fail。
//
// 相对上游做了四处适配（本壳架构差异，非偏好）：
//  1. LLM 步骤：上游用 a.ai.Ask；本壳 ai.Gateway 无该方法，改用 ChatJSON。
//     且本壳 gateway 在「未配置模型」时返回降级文案而非 error —— 若照搬会把
//     「AI 服务未配置…」当成文章正文写进草稿，故这里显式判 ActiveProvider 后再调。
//  2. 鉴权：上游 a.isAdminByID(uid)；本壳统一用 blogAdminOnly(w, r)（owner/admin）。
//  3. 步骤注册表：上游内置 tool_im_*（依赖其 im_tools.go，本壳无此组件），
//     本壳改为内容侧步骤：建草稿 / 检索 / 打标签 / 发布。注册表本身是开放的，
//     新增步骤只需往 wfRegistry 加一个函数，不动引擎。
//  4. 站点隔离：本壳是多站点，wfRunCtx 增 SiteID，内容写入按其归属站点，
//     否则矩阵站群下会串数据（SPEC-MS-001 M3 的同一原则）。
//
// 另修正上游一处执行器缺陷：上游 on_fail=retry 时无条件回调 fn()，而 fn 仅在
// 「注册表步骤」分支被赋值 —— tool/llm/condition/delay 步骤配 retry 会 fn=nil panic。
// 这里改为整步重入（wfExecStep），任何步骤类型都能正确重试。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/im"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// ─────────────────────────────────────────────────────────────────────────────
// 定义结构（与上游同构，保证 workflow 定义可跨壳搬运）
// ─────────────────────────────────────────────────────────────────────────────

// wfTrigger 触发定义。
type wfTrigger struct {
	Type   string `json:"type"`   // im_command（IM 指令前缀；定时/事件触发后续扩展）
	Prefix string `json:"prefix"` // 如 /draft
	Intent string `json:"intent"` // 可选意图名
}

// wfRoute 条件路由（Type=condition）：When 匹配 $prev 输出，命中取 Action。
type wfRoute struct {
	When   string `json:"when"`   // $prev.<key> contains "x" | == "x" | != "x" | not empty
	Action string `json:"action"` // abort|skip
}

// wfStep 步骤定义。
// Type：空=注册表步骤函数；tool=内核工具注册表通用工具（reg.Exec）；
//      llm=模型调用；condition=条件路由；delay=延时。
type wfStep struct {
	Tool     string            `json:"tool"`      // 注册表工具 id / 内核工具名（Type=tool 时）
	In       map[string]string `json:"in"`        // 参数模板：$msg.* / $prev.* / $ctx.*
	OnFail   string            `json:"on_fail"`   // abort|skip|retry（retry=整步重入 1 次）
	Type     string            `json:"type"`      // 空|tool|llm|condition|delay
	Model    string            `json:"model"`     // llm 节点分层提示（v1 用当前 llm provider）
	Routes   []wfRoute         `json:"routes"`    // condition 节点路由（顺序匹配首个命中）
	DelaySec int               `json:"delay_sec"` // delay 节点秒数（上限 60）
}

// Workflow 执行定义（持久化于 workflow_defs.nodes）。
type Workflow struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Version      string      `json:"version"`
	Kind         string      `json:"kind"` // workflow
	Author       string      `json:"author"`
	Triggers     []wfTrigger `json:"triggers"`
	Dependencies []string    `json:"dependencies"`
	Steps        []wfStep    `json:"steps"`
	Enabled      bool        `json:"enabled"`
}

// wfRunCtx 工作流执行上下文。
type wfRunCtx struct {
	MsgText string // $msg.text（去除触发前缀后的原文）
	MsgFile string // $msg.file（文件消息 FileID）
	UID     string // $ctx.user（触发者；写入内容的 owner）
	SpaceID string
	SiteID  string // 本壳扩展：内容归属站点（空=默认站）
	MsgKey  string // 幂等键（API 触发显式传入；空=按 MsgFile|MsgText 摘要构造）
	Prev    map[string]any // $prev.*（上一步 out）
}

// wfStepFunc 注册表步骤函数：返回 out 供 $prev 捕获。
type wfStepFunc func(a *API, ctx context.Context, rc *wfRunCtx, args map[string]any) (map[string]any, error)

// wfStepCtl 步骤控制流（delay/condition 这类不产出 $prev 的步骤用）。
type wfStepCtl int

const (
	wfCtlNext     wfStepCtl = iota // 正常：out 写入 $prev，继续
	wfCtlContinue                  // 跳过产出直接下一步（delay 完成 / condition 无命中）
	wfCtlAbort                     // 终止整个工作流（condition 命中 abort）
)

// ─────────────────────────────────────────────────────────────────────────────
// 步骤注册表（本壳内容侧步骤；加步骤=加函数，不动引擎）
// ─────────────────────────────────────────────────────────────────────────────

var wfRegistry = map[string]wfStepFunc{
	"blog_create_draft": wfStepCreateDraft,
	"blog_search":       wfStepSearch,
	"blog_set_tags":     wfStepSetTags,
	"blog_publish":      wfStepPublish,
}

// wfOfficial 内置官方 workflow（开箱可用；市场安装的 workflow 包可覆盖同 id 定义）。
//
// 触发前缀一律带 /wf- 命名空间：上游把"每个 workflow 占一个全局前缀"当默认（如 /publish），
// 而本壳 /publish、/draft 已被既有 IM 指令占用 —— 该假设不成立。除前缀外，
// 还支持统一入口 /wf <id>（见 imWorkflow），自定义与市场安装的工作流走那条，不依赖前缀唯一性。
func wfOfficial() []Workflow {
	return []Workflow{
		{
			ID: "wf-draft-by-command", Name: "指令建草稿", Version: "1.0.0", Kind: "workflow", Author: "official",
			Triggers:     []wfTrigger{{Type: "im_command", Prefix: "/wf-draft", Intent: "wf_draft"}},
			Dependencies: []string{"blog_create_draft"},
			Steps: []wfStep{{
				Tool:   "blog_create_draft",
				In:     map[string]string{"title": "$msg.text", "content": "$msg.text"},
				OnFail: "abort",
			}},
			Enabled: true,
		},
		{
			ID: "wf-polish-to-draft", Name: "AI 整理成稿", Version: "1.0.0", Kind: "workflow", Author: "official",
			Triggers:     []wfTrigger{{Type: "im_command", Prefix: "/wf-polish", Intent: "wf_polish"}},
			Dependencies: []string{"blog_create_draft"},
			Steps: []wfStep{
				{
					Type:   "llm",
					In:     map[string]string{"prompt": "把下面这段素材整理成一篇结构清晰的中文博客草稿。直接输出 Markdown 正文，不要任何额外说明或前后缀：\n$msg.text"},
					OnFail: "abort",
				},
				{
					Tool:   "blog_create_draft",
					In:     map[string]string{"title": "$msg.text", "content": "$prev.text"},
					OnFail: "abort",
				},
			},
			Enabled: true,
		},
		{
			ID: "wf-find-content", Name: "检索内容", Version: "1.0.0", Kind: "workflow", Author: "official",
			Triggers:     []wfTrigger{{Type: "im_command", Prefix: "/wf-find", Intent: "wf_find"}},
			Dependencies: []string{"blog_search"},
			Steps: []wfStep{{
				Tool:   "blog_search",
				In:     map[string]string{"query": "$msg.text", "limit": "5"},
				OnFail: "abort",
			}},
			Enabled: true,
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 内容侧步骤实现
// ─────────────────────────────────────────────────────────────────────────────

// wfStepCreateDraft 建文章草稿（写入博客目录，归属触发站点），产出 file_id/title/url。
func wfStepCreateDraft(a *API, ctx context.Context, rc *wfRunCtx, args map[string]any) (map[string]any, error) {
	title := strings.TrimSpace(wfStr(args["title"]))
	content := wfStr(args["content"])
	if title == "" {
		title = wfFirstLine(content)
	}
	if title == "" {
		return nil, fmt.Errorf("缺少标题与正文（指令后请跟标题或正文）")
	}
	if r := []rune(title); len(r) > 60 {
		title = string(r[:60])
	}
	if strings.TrimSpace(content) == "" {
		content = title
	}
	name := title
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		name += ".md"
	}
	owner := wfOwner(rc)
	siteID := wfSite(rc)
	f, err := a.files.CreateDoc(ctx, owner, service.SystemHomeSpaceID, service.BlogDirID, name, content, siteID)
	if err != nil {
		// 重名兜底：自动化场景下同标题重复触发很常见（重复指令 / 重放），
		// 追加时间戳再试一次，避免整条流水线因文件名冲突而 abort。
		alt := strings.TrimSuffix(name, ".md") + "-" + time.Now().Format("20060102-150405") + ".md"
		f, err = a.files.CreateDoc(ctx, owner, service.SystemHomeSpaceID, service.BlogDirID, alt, content, siteID)
		if err != nil {
			return nil, fmt.Errorf("建草稿失败: %v", err)
		}
	}
	return map[string]any{
		"file_id": f.ID,
		"title":   f.Name,
		"url":     "/blog/" + f.ID,
	}, nil
}

// wfStepSearch 内容检索（文件名/正文），产出 count/items/first_id。
func wfStepSearch(a *API, ctx context.Context, rc *wfRunCtx, args map[string]any) (map[string]any, error) {
	q := strings.TrimSpace(wfStr(args["query"]))
	if q == "" {
		return nil, fmt.Errorf("缺少检索关键词")
	}
	limit := 5
	if n, err := strconv.Atoi(strings.TrimSpace(wfStr(args["limit"]))); err == nil && n > 0 {
		limit = n
	}
	if limit > 50 {
		limit = 50
	}
	hits, err := a.files.Search(ctx, service.SearchOpts{
		SpaceID: wfSpace(rc),
		Query:   q,
		Limit:   limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		if h.File == nil {
			continue
		}
		items = append(items, map[string]any{"file_id": h.File.ID, "title": h.File.Name})
	}
	out := map[string]any{"count": len(items), "query": q, "items": items}
	if len(items) > 0 {
		out["first_id"] = items[0]["file_id"]
		out["first_title"] = items[0]["title"]
	}
	return out, nil
}

// wfStepSetTags 为文章追加标签（逗号分隔；标签不存在则按路径创建）。
// file_id 缺省取上一步产出的 file_id —— 这样「建草稿 → 打标签」可零参数串联。
func wfStepSetTags(a *API, ctx context.Context, rc *wfRunCtx, args map[string]any) (map[string]any, error) {
	fid := strings.TrimSpace(wfStr(args["file_id"]))
	if fid == "" {
		fid = wfPrevID(rc)
	}
	if fid == "" {
		return nil, fmt.Errorf("缺少 file_id（需前置步骤产出或显式传入）")
	}
	raw := strings.TrimSpace(wfStr(args["tags"]))
	if raw == "" {
		return nil, fmt.Errorf("缺少 tags（逗号分隔）")
	}
	owner := wfOwner(rc)
	var ids, names []string
	for _, part := range strings.Split(raw, ",") {
		n := strings.TrimSpace(part)
		if n == "" {
			continue
		}
		tg, err := a.tags.EnsureTagPath(ctx, owner, n)
		if err != nil {
			return nil, fmt.Errorf("标签 %q 创建失败: %v", n, err)
		}
		ids = append(ids, tg.ID)
		names = append(names, n)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("tags 为空")
	}
	if err := a.tags.MergeFileTags(ctx, owner, fid, ids); err != nil {
		return nil, err
	}
	return map[string]any{"file_id": fid, "tags": names, "tag_count": len(ids)}, nil
}

// wfStepPublish 将草稿置为已发布（content_state.status=published）。
func wfStepPublish(a *API, ctx context.Context, rc *wfRunCtx, args map[string]any) (map[string]any, error) {
	fid := strings.TrimSpace(wfStr(args["file_id"]))
	if fid == "" {
		fid = wfPrevID(rc)
	}
	if fid == "" {
		return nil, fmt.Errorf("缺少 file_id（需前置步骤产出或显式传入）")
	}
	f, err := a.files.SetStatus(ctx, wfOwner(rc), fid, "published")
	if err != nil {
		return nil, err
	}
	return map[string]any{"file_id": f.ID, "status": "published"}, nil
}

// wfOwner 写入者：优先触发者，缺失时回落系统账号（定时/系统触发场景）。
func wfOwner(rc *wfRunCtx) string {
	if rc.UID != "" {
		return rc.UID
	}
	return service.SystemOwnerID
}

// wfSpace 工作空间：默认系统 home 空间（博客目录所在空间）。
func wfSpace(rc *wfRunCtx) string {
	if rc.SpaceID != "" {
		return rc.SpaceID
	}
	return service.SystemHomeSpaceID
}

// wfSite 内容归属站点（空=默认站）。
func wfSite(rc *wfRunCtx) string {
	if rc.SiteID != "" {
		return rc.SiteID
	}
	return service.DefaultSiteID
}

// wfPrevID 取 $prev.file_id。
func wfPrevID(rc *wfRunCtx) string {
	if v, ok := rc.Prev["file_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// wfStr 把模板求值结果（any）统一成字符串（$prev 里的值可能是数字/布尔）。
func wfStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}

// wfFirstLine 取首个非空行作为标题兜底。
func wfFirstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#>-* \t"))
		if line != "" {
			return line
		}
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// 模板求值
// ─────────────────────────────────────────────────────────────────────────────

// wfEvalTpl 求值步骤参数模板：$msg.text / $msg.file / $prev.<key> / $ctx.user。
func wfEvalTpl(tpl string, rc *wfRunCtx) string {
	s := tpl
	s = strings.ReplaceAll(s, "$msg.text", rc.MsgText)
	s = strings.ReplaceAll(s, "$msg.file", rc.MsgFile)
	s = strings.ReplaceAll(s, "$ctx.user", rc.UID)
	s = strings.ReplaceAll(s, "$ctx.site", rc.SiteID)
	for k, v := range rc.Prev {
		s = strings.ReplaceAll(s, "$prev."+k, wfStr(v))
	}
	return s
}

// wfEvalArgs 把步骤 In 模板展开为参数 map。
func wfEvalArgs(in map[string]string, rc *wfRunCtx) map[string]any {
	args := map[string]any{}
	for k, v := range in {
		args[k] = wfEvalTpl(v, rc)
	}
	return args
}

// ─────────────────────────────────────────────────────────────────────────────
// 条件路由
// ─────────────────────────────────────────────────────────────────────────────

// wfEvalRoute 顺序求值条件路由：首个命中返回其 Action（abort|skip），无命中返回 ""。
func wfEvalRoute(routes []wfRoute, rc *wfRunCtx) (string, error) {
	for _, r := range routes {
		hit, err := wfEvalCond(r.When, rc)
		if err != nil {
			return "", err
		}
		if hit {
			return r.Action, nil
		}
	}
	return "", nil
}

// wfEvalCond 条件表达式求值：contains / == / != / not empty。
func wfEvalCond(expr string, rc *wfRunCtx) (bool, error) {
	expr = strings.TrimSpace(expr)
	if parts := strings.SplitN(expr, " contains ", 2); len(parts) == 2 {
		val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		return strings.Contains(wfStr(wfPrevVal(parts[0], rc)), val), nil
	}
	if parts := strings.SplitN(expr, " == ", 2); len(parts) == 2 {
		return wfStr(wfPrevVal(parts[0], rc)) == strings.Trim(strings.TrimSpace(parts[1]), `"`), nil
	}
	if parts := strings.SplitN(expr, " != ", 2); len(parts) == 2 {
		return wfStr(wfPrevVal(parts[0], rc)) != strings.Trim(strings.TrimSpace(parts[1]), `"`), nil
	}
	if strings.HasSuffix(expr, " not empty") {
		k := strings.TrimSpace(strings.TrimSuffix(expr, " not empty"))
		v := wfStr(wfPrevVal(k, rc))
		return v != "" && v != "<nil>", nil
	}
	return false, fmt.Errorf("不支持的条件表达式: %s", expr)
}

// wfPrevVal 解析 $prev.<key> 引用值（容错：非法引用原样返回）。
func wfPrevVal(ref string, rc *wfRunCtx) any {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "$prev.") {
		return rc.Prev[strings.TrimPrefix(ref, "$prev.")]
	}
	return ref
}

// ─────────────────────────────────────────────────────────────────────────────
// 执行器
// ─────────────────────────────────────────────────────────────────────────────

// wfAll 返回全部 workflow：内置官方 + DB 中 published 的自定义定义。
func (a *API) wfAll() []Workflow {
	out := wfOfficial()
	rows, err := a.db.Query(`SELECT id, nodes FROM workflow_defs WHERE status='published'`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, nodes string
		if err := rows.Scan(&id, &nodes); err != nil {
			continue
		}
		var wf Workflow
		if err := json.Unmarshal([]byte(nodes), &wf); err != nil || wf.ID == "" {
			wf.ID = id
		}
		out = append(out, wf)
	}
	return out
}

// wfMatchPrefix 按消息文本匹配已启用 workflow，返回命中的定义与其触发前缀
// （前缀需完整匹配或后跟空格；前缀用于剥离后得到 $msg.text）。
func (a *API) wfMatchPrefix(text string) (*Workflow, string) {
	text = strings.TrimSpace(text)
	for _, wf := range a.wfAll() {
		if !wf.Enabled {
			continue
		}
		for _, tg := range wf.Triggers {
			if tg.Type != "im_command" || tg.Prefix == "" {
				continue
			}
			if text == tg.Prefix || strings.HasPrefix(text, tg.Prefix+" ") {
				w := wf
				return &w, tg.Prefix
			}
		}
	}
	return nil, ""
}

// wfStripTrigger 去掉指令前缀，返回其后的正文（作为 $msg.text）。
func wfStripTrigger(text, prefix string) string {
	text = strings.TrimSpace(text)
	if prefix == "" {
		return text
	}
	if text == prefix {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(text, prefix))
}

// wfExecStep 执行单个步骤。
// 返回 (out, err, ctl)；ctl 见 wfStepCtl（delay/condition 不产出 $prev）。
// 设计说明：抽成独立函数是为了修正上游缺陷 —— 上游 on_fail=retry 直接回调 fn()，
// 而 fn 仅在注册表分支赋值，tool/llm/condition/delay 配 retry 会 nil panic；
// 整步重入后任何步骤类型都能正确重试。
func wfExecStep(a *API, ctx context.Context, rc *wfRunCtx, st wfStep, idx int, lines *[]string) (map[string]any, error, wfStepCtl) {
	switch st.Type {
	case "tool":
		if a.reg == nil {
			return nil, fmt.Errorf("内核工具注册表未注入（Type=tool 步骤不可用）"), wfCtlNext
		}
		b, jerr := json.Marshal(wfCoerceArgs(a.reg, st.Tool, wfEvalArgs(st.In, rc)))
		if jerr != nil {
			return nil, fmt.Errorf("参数序列化失败: %v", jerr), wfCtlNext
		}
		raw, terr := a.reg.Exec(ctx, st.Tool, b)
		if terr != nil {
			return nil, terr, wfCtlNext
		}
		out := map[string]any{"output": raw}
		if mm := map[string]any{}; json.Unmarshal([]byte(raw), &mm) == nil {
			out = mm
		}
		return out, nil, wfCtlNext

	case "llm":
		if a.ai == nil {
			return nil, fmt.Errorf("AI 网关未注入"), wfCtlNext
		}
		// 本壳 gateway 在未配置模型时返回降级文案（而非 error），必须显式拦截，
		// 否则会把「AI 服务未配置…」当成正文写进草稿。
		if a.ai.ActiveProvider("ai.llm") == "" {
			return nil, fmt.Errorf("AI 模型未配置（设置 → AI 模型与路由）"), wfCtlNext
		}
		res, aerr := a.ai.ChatJSON(ctx, []ai.Msg{{Role: "user", Content: wfEvalTpl(st.In["prompt"], rc)}}, nil)
		if aerr != nil {
			return nil, aerr, wfCtlNext
		}
		if res == nil || strings.TrimSpace(res.Content) == "" {
			return nil, fmt.Errorf("模型返回为空"), wfCtlNext
		}
		if res.Error != "" {
			return nil, fmt.Errorf("模型调用失败: %s", res.Error), wfCtlNext
		}
		return map[string]any{"text": res.Content}, nil, wfCtlNext

	case "condition":
		action, cerr := wfEvalRoute(st.Routes, rc)
		if cerr != nil {
			return nil, cerr, wfCtlNext
		}
		*lines = append(*lines, fmt.Sprintf("· 第 %d 步 条件：%s", idx, wfCondLabel(action)))
		if action == "abort" {
			return nil, nil, wfCtlAbort
		}
		// skip 与「无命中」语义一致：跳过本步（不产出 $prev），继续下一步。
		return nil, nil, wfCtlContinue

	case "delay":
		d := st.DelaySec
		if d <= 0 {
			d = 1
		}
		if d > 60 {
			d = 60
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err(), wfCtlNext
		case <-time.After(time.Duration(d) * time.Second):
			*lines = append(*lines, fmt.Sprintf("· 第 %d 步 延时 %d 秒 ✓", idx, d))
			return nil, nil, wfCtlContinue
		}

	default: // 注册表步骤
		fn, ok := wfRegistry[st.Tool]
		if !ok {
			return nil, fmt.Errorf("未注册的步骤 %q", st.Tool), wfCtlNext
		}
		out, err := fn(a, ctx, rc, wfEvalArgs(st.In, rc))
		return out, err, wfCtlNext
	}
}

// wfCondLabel 条件路由结果的中文标注。
func wfCondLabel(action string) string {
	switch action {
	case "abort":
		return "命中 abort，终止"
	case "skip":
		return "命中 skip，跳过"
	default:
		return "无命中，继续"
	}
}

// wfRun 执行一个 workflow：顺序调度步骤，on_fail abort|skip|retry(整步重入 1 次)。
// 返回 (执行详情, 是否成功)；详情写入 workflow_runs 供审计，幂等键冲突时静默忽略。
func (a *API) wfRun(ctx context.Context, wf *Workflow, rc *wfRunCtx) (string, bool) {
	if rc.Prev == nil {
		rc.Prev = map[string]any{}
	}
	lines := []string{fmt.Sprintf("【%s】开始执行 %d 步", wf.Name, len(wf.Steps))}
	ok := true

	for i, st := range wf.Steps {
		out, err, ctl := wfExecStep(a, ctx, rc, st, i+1, &lines)
		if ctl == wfCtlAbort {
			ok = false
			break
		}
		if err != nil {
			action := st.OnFail
			if action == "" {
				action = "abort"
			}
			if action == "retry" {
				out, err, ctl = wfExecStep(a, ctx, rc, st, i+1, &lines)
				if ctl == wfCtlAbort {
					ok = false
					break
				}
				if err == nil {
					action = "retried"
				}
			}
			if err != nil {
				lines = append(lines, fmt.Sprintf("· 第 %d 步 %s：%v → %s", i+1, wfStepLabel(st), err, action))
				if action == "skip" {
					continue
				}
				ok = false
				break
			}
		}
		if ctl == wfCtlContinue {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		rc.Prev = out
		lines = append(lines, fmt.Sprintf("· 第 %d 步 %s ✓", i+1, wfStepLabel(st)))
	}
	if ok {
		lines = append(lines, "全部完成")
	}

	key := rc.MsgKey
	if key == "" {
		key = rc.MsgFile + "|" + wfTruncate(rc.MsgText, 32)
	}
	status := "failed"
	if ok {
		status = "ok"
	}
	_, _ = a.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO workflow_runs (id, wf_id, msg_key, status, detail, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		newID(), wf.ID, key, status, strings.Join(lines, "\n"), time.Now().Unix())
	return strings.Join(lines, "\n"), ok
}

// wfStepLabel 步骤在详情里的展示名。
func wfStepLabel(st wfStep) string {
	if st.Tool != "" {
		return st.Tool
	}
	if st.Type != "" {
		return st.Type
	}
	return "step"
}

// wfTruncate 按 rune 截断（避免多字节字符被切半）。
func wfTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP handlers
// ─────────────────────────────────────────────────────────────────────────────

// wfList GET /api/v1/workflows：列出全部 workflow 定义（含内置官方）。
func (a *API) wfList(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": a.wfAll()})
}

// wfSave POST /api/v1/workflows：发布/更新自定义 workflow（写入 workflow_defs，status=published）。
func (a *API) wfSave(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	uid := a.curUserID(r)
	var wf Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		writeErr(w, http.StatusBadRequest, "WF_BAD_JSON", err.Error())
		return
	}
	if wf.ID == "" || len(wf.Steps) == 0 {
		writeErr(w, http.StatusBadRequest, "WF_BAD_DEF", "需要 id 与至少一个 steps")
		return
	}
	if wf.Kind == "" {
		wf.Kind = "workflow"
	}
	if err := a.wfPersist(r.Context(), &wf, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "WF_SAVE_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": wf.ID})
}

// wfPersist 落库一条已发布的工作流定义（幂等 upsert：同 id 覆盖，节点以最新为准）。
func (a *API) wfPersist(ctx context.Context, wf *Workflow, authorID string) error {
	nodes, err := json.Marshal(wf)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	_, err = a.db.ExecContext(ctx,
		`INSERT INTO workflow_defs (id, name, description, nodes, status, version, author_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'published', 1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, nodes=excluded.nodes, status='published', updated_at=excluded.updated_at`,
		wf.ID, wf.Name, wf.Name, string(nodes), authorID, now, now)
	return err
}

// wfDelete DELETE /api/v1/workflows/{id}：删除自定义 workflow（内置官方不可删）。
func (a *API) wfDelete(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	for _, wf := range wfOfficial() {
		if wf.ID == id {
			writeErr(w, http.StatusBadRequest, "WF_BUILTIN", "内置官方工作流不可删除")
			return
		}
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM workflow_defs WHERE id=?`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "WF_DEL_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// wfRunAPI POST /api/v1/workflows/{id}/run：API/外部触发执行工作流。
// body: {"msg_key":"幂等键(必填)", "input":{"text":"","file_id":"","uid":"","space_id":""}}
func (a *API) wfRunAPI(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	uid := a.curUserID(r)
	id := r.PathValue("id")
	var req struct {
		MsgKey string         `json:"msg_key"`
		Input  map[string]any `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "WF_BAD_JSON", err.Error())
		return
	}
	if strings.TrimSpace(req.MsgKey) == "" {
		writeErr(w, http.StatusBadRequest, "WF_BAD_KEY", "msg_key 必填（幂等键）")
		return
	}
	wf := a.wfFind(id)
	if wf == nil {
		writeErr(w, http.StatusNotFound, "WF_NOT_FOUND", "工作流不存在或未发布")
		return
	}
	// 幂等闸门：同 (wf_id, msg_key) 已执行过则直接返回 duplicate，不重复产生副作用。
	var dup int
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM workflow_runs WHERE wf_id=? AND msg_key=?`, id, req.MsgKey).Scan(&dup); err == nil && dup > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"duplicate": true, "ok": true})
		return
	}
	rc := &wfRunCtx{
		MsgKey:  req.MsgKey,
		Prev:    map[string]any{},
		UID:     uid,
		SpaceID: service.SystemHomeSpaceID,
		SiteID:  currentSiteID(r), // 站点隔离：内容写当前请求所属站点
	}
	if v, _ := req.Input["text"].(string); v != "" {
		rc.MsgText = v
	}
	if v, _ := req.Input["file_id"].(string); v != "" {
		rc.MsgFile = v
	}
	if v, _ := req.Input["uid"].(string); v != "" {
		rc.UID = v
	}
	if v, _ := req.Input["space_id"].(string); v != "" {
		rc.SpaceID = v
	}
	detail, ok := a.wfRun(r.Context(), wf, rc)
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "detail": detail})
}

// wfFind 按 id 取工作流定义（先内置官方，再 DB published）。
func (a *API) wfFind(id string) *Workflow {
	for _, wf := range wfOfficial() {
		if wf.ID == id {
			w := wf
			return &w
		}
	}
	var nodes string
	if err := a.db.QueryRow(`SELECT nodes FROM workflow_defs WHERE id=? AND status='published'`, id).Scan(&nodes); err != nil {
		return nil
	}
	var w Workflow
	if err := json.Unmarshal([]byte(nodes), &w); err != nil || w.ID == "" {
		w.ID = id
	}
	return &w
}

// ─────────────────────────────────────────────────────────────────────────────
// IM 入口（B2）：把工作流接到既有 IM 指令链路上
// ─────────────────────────────────────────────────────────────────────────────

// wfSplitFirst 把 "id 其余正文" 拆成 (id, 正文)。
func wfSplitFirst(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// wfByShortName 按短名解析工作流：/wf-<name> ⇄ id=wf-<name>（也接受写全 id）。
func (a *API) wfByShortName(name string) *Workflow {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	for _, wf := range a.wfAll() {
		if wf.ID == name || wf.ID == "wf-"+name || strings.TrimPrefix(wf.ID, "wf-") == name {
			w := wf
			return &w
		}
	}
	return nil
}

// imWorkflow IM 触发内容工作流（B2）。三种写法：
//
//	/wf                  —— 列出可用工作流
//	/wf <id> [正文]      —— 统一入口：按 id 触发（自定义 / 应用中心安装的工作流走这条）
//	/wf-<name> [正文]    —— 短前缀：等价于 /wf wf-<name> [正文]（内置工作流快捷写法）
func (a *API) imWorkflow(ctx context.Context, r *http.Request, msg *im.Msg, intent im.IntentResult) *im.Reply {
	q := strings.TrimSpace(intent.Params["query"])
	var wf *Workflow
	var text string
	if strings.HasPrefix(q, "-") {
		name, rest := wfSplitFirst(strings.TrimSpace(q[1:]))
		wf, text = a.wfByShortName(name), rest
	} else {
		id, rest := wfSplitFirst(q)
		text = rest
		if id == "" || id == "list" {
			return &im.Reply{Text: a.wfHelpText(), OK: true}
		}
		wf = a.wfFind(id)
	}
	if wf == nil {
		return &im.Reply{Text: "没有这个工作流。\n" + a.wfHelpText(), OK: false}
	}
	if !wf.Enabled {
		return &im.Reply{Text: fmt.Sprintf("工作流「%s」已停用。", wf.Name), OK: false}
	}
	return a.imRunWorkflow(ctx, r, msg, wf, text)
}

// imRunWorkflow 执行工作流并把执行详情回给 IM。
// 幂等键取 IM 的 platform:msg_id —— 与 imWebhook 的 im_ingest 幂等同一粒度，
// 平台重发/用户连发同一消息只会真正执行一次。
func (a *API) imRunWorkflow(ctx context.Context, r *http.Request, msg *im.Msg, wf *Workflow, text string) *im.Reply {
	key := ""
	if msg != nil && msg.MsgID != "" {
		key = "im:" + msg.Platform + ":" + msg.MsgID
	}
	rc := &wfRunCtx{
		UID:     a.homeOwnerID(),
		SpaceID: service.SystemHomeSpaceID,
		SiteID:  currentSiteID(r),
		MsgKey:  key,
		Prev:    map[string]any{},
	}
	if msg != nil {
		rc.MsgText = text
		rc.MsgFile = msg.FileURL
	}
	detail, ok := a.wfRun(ctx, wf, rc)
	if !ok {
		return &im.Reply{Text: detail, OK: false}
	}
	out := &im.Reply{Text: detail, OK: true}
	if v, has := rc.Prev["file_id"]; has {
		out.FileID = wfStr(v)
	}
	return out
}

// wfHelpText IM 工作流帮助（列出可用工作流与两种触发写法）。
func (a *API) wfHelpText() string {
	var b strings.Builder
	b.WriteString("可用工作流：\n")
	for _, wf := range a.wfAll() {
		if !wf.Enabled {
			continue
		}
		pfx := ""
		for _, tg := range wf.Triggers {
			if tg.Type == "im_command" && tg.Prefix != "" {
				pfx = "（" + tg.Prefix + " / "
				break
			}
		}
		if pfx != "" {
			b.WriteString(fmt.Sprintf("· %s %s/wf %s）\n", wf.ID, pfx, wf.ID))
		} else {
			b.WriteString(fmt.Sprintf("· %s（/wf %s）\n", wf.ID, wf.ID))
		}
	}
	b.WriteString("用法：/wf <id> 正文   或   /wf-<短名> 正文")
	return b.String()
}

// SetRegistry 注入内核工具注册表（main 装配时调用；供 Type=tool 步骤执行通用工具）。
// SetConceptSummarizer 注入概念页 AI 摘要器（A09+A5；nil=能力未启用，端点返回明确错误）。
func (a *API) SetConceptSummarizer(c *ai.ConceptSummarizer) { a.conceptSum = c }

func (a *API) SetRegistry(reg *ai.Registry) { a.reg = reg }

// wfCoerceArgs 按工具 JSON Schema 把模板参数（全 string）转换为目标类型（integer/number/boolean/array），
// 兼容 JSON schema 期望类型，避免 search_files.limit=3 之类 string→int 解码失败。
func wfCoerceArgs(reg *ai.Registry, toolName string, args map[string]any) map[string]any {
	t, ok := reg.Get(toolName)
	if !ok {
		return args
	}
	props, ok := t.Parameters["properties"].(map[string]any)
	if !ok {
		return args
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		str, isStr := v.(string)
		if !isStr {
			out[k] = v
			continue
		}
		ps, _ := props[k].(map[string]any)
		if ps == nil {
			out[k] = v
			continue
		}
		switch ps["type"] {
		case "integer":
			if n, err := strconv.ParseInt(strings.TrimSpace(str), 10, 64); err == nil {
				out[k] = n
			} else {
				out[k] = v
			}
		case "number":
			if f, err := strconv.ParseFloat(strings.TrimSpace(str), 64); err == nil {
				out[k] = f
			} else {
				out[k] = v
			}
		case "boolean":
			switch strings.TrimSpace(str) {
			case "true":
				out[k] = true
			case "false":
				out[k] = false
			default:
				out[k] = v
			}
		case "array":
			if arr := []any{}; json.Unmarshal([]byte(str), &arr) == nil {
				out[k] = arr
			} else {
				out[k] = v
			}
		default:
			out[k] = v
		}
	}
	return out
}
