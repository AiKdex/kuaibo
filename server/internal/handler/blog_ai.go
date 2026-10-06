// blog_ai.go AI 写作工作台后端（登录态）。
//
// 能力：素材检索（博客公开文章关键词混合匹配）/ 大纲生成 / 续写 / 润色（含扩写缩写）/
// 标签建议。RAG 上下文 = 博客目录已发布文章的标题+摘要（collectDirFiles 同源），
// 可选指定 paths 聚焦素材。LLM 走 ai.Gateway.ChatJSON（与读者问答/摘要同一出口）。
//
// 限流：按用户 10 分钟 30 次（AI 成本保护）；输入上限：正文 16000 字符。
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

const (
	blogAIMaxInput    = 16000 // 单次送入正文上限（rune）
	blogAIMaxMaterial = 12    // 素材检索默认返回条数
)

// AI 限流（进程内，按用户）：10 分钟 30 次
var (
	aiWbMu    sync.Mutex
	aiWbHits  = map[string][]time.Time{}
	aiWbWin   = 10 * time.Minute
	aiWbMax   = 30
)

func blogAIAllowed(userID string) bool {
	aiWbMu.Lock()
	defer aiWbMu.Unlock()
	now := time.Now()
	arr := aiWbHits[userID][:0]
	for _, t := range aiWbHits[userID] {
		if now.Sub(t) < aiWbWin {
			arr = append(arr, t)
		}
	}
	if len(arr) >= aiWbMax {
		aiWbHits[userID] = arr
		return false
	}
	aiWbHits[userID] = append(arr, now)
	return true
}

// clipRunes 按 rune 截断。
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// requireAIPermit 登录 + 限流 + 博客开启校验（各端点公共前置）。
func (a *API) requireAIPermit(w http.ResponseWriter, r *http.Request) (string, bool) {
	if v, ok := a.cfg.Get("blog.open"); ok {
		if s, _ := v.(string); s == "false" {
			writeErr(w, http.StatusNotFound, "BLOG_CLOSED", "博客已关闭")
			return "", false
		}
	}
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return "", false
	}
	if !blogAIAllowed(uid) {
		writeErr(w, http.StatusTooManyRequests, "AI_RATE", "AI 调用过于频繁，请稍后再试")
		return "", false
	}
	return uid, true
}

// blogMaterial 素材检索实现：优先精确聚焦 paths，否则按关键词匹配标题/摘要。
// 返回 [{path, title, preview}]。
func (a *API) blogMaterial(r *http.Request, q string, paths []string, limit int) []map[string]any {
	if limit <= 0 || limit > blogAIMaxMaterial {
		limit = blogAIMaxMaterial
	}
	var blogDirID string
	_ = a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(dir_id,'') FROM shares
		 WHERE owner_id=? AND token=? AND (revoked_at IS NULL OR revoked_at=0)
		   AND (expires_at IS NULL OR expires_at=0 OR expires_at>?)`,
		a.homeOwnerID(), service.BlogToken, time.Now().UnixMilli()).Scan(&blogDirID)
	if blogDirID == "" {
		return []map[string]any{}
	}
	files, err := a.collectBlogArticles(r.Context(), blogDirID, "")
	if err != nil {
		return []map[string]any{}
	}
	bypath := map[string]dirFile{}
	for _, it := range files {
		bypath[it.path] = it
	}
	out := []map[string]any{}
	seen := map[string]bool{}
	add := func(it dirFile) {
		if seen[it.path] {
			return
		}
		seen[it.path] = true
		title := strings.TrimSuffix(it.f.Name, ".md")
		title = strings.TrimSuffix(title, ".markdown")
		out = append(out, map[string]any{"path": it.path, "title": title, "preview": clipRunes(it.preview, 200)})
	}
	// 1) 指定路径优先
	for _, p := range paths {
		if it, ok := bypath[p]; ok {
			add(it)
		}
	}
	// 2) 关键词匹配（标题/摘要；空格分词，全部词命中才算）
	if len(out) < limit {
		terms := strings.Fields(strings.ToLower(strings.TrimSpace(q)))
		for _, it := range files {
			if len(out) >= limit {
				break
			}
			if seen[it.path] || len(terms) == 0 {
				continue
			}
			title := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(it.f.Name, ".md"), ".markdown"))
			preview := strings.ToLower(it.preview)
			hit := true
			for _, t := range terms {
				if !strings.Contains(title, t) && !strings.Contains(preview, t) {
					hit = false
					break
				}
			}
			if hit {
				add(it)
			}
		}
	}
	return out
}

// materialContext 把素材列表拼成 LLM 上下文段落。
func materialContext(mats []map[string]any) string {
	if len(mats) == 0 {
		return "（知识库暂无相关素材，请按通用知识作答并注明）\n"
	}
	var b strings.Builder
	b.WriteString("可用素材（来自本站知识库，引用时保持事实一致）：\n")
	for _, m := range mats {
		fmt.Fprintf(&b, "- 《%s》（路径 %s）：%s\n", m["title"], m["path"], m["preview"])
	}
	return b.String()
}

// ---- 端点 ----

// blogAISearch POST /api/v1/blog/ai/search {q, paths?, limit?} → {items:[{path,title,preview}]}
// 素材检索（不调 LLM，纯检索，不占 AI 限流）。
func (a *API) blogAISearch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Q     string   `json:"q"`
		Paths []string `json:"paths"`
		Limit int      `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "AI_BAD_BODY", "请求体解析失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": a.blogMaterial(r, in.Q, in.Paths, in.Limit),
	})
}

// blogAIChatReq 公共请求形状（各端点按需取字段）。
type blogAIChatReq struct {
	Topic       string   `json:"topic"`
	Content     string   `json:"content"`
	Instruction string   `json:"instruction"`
	Paths       []string `json:"paths"`
	Title       string   `json:"title"`
	Mode        string   `json:"mode"`
}

func (a *API) decodeAIReq(w http.ResponseWriter, r *http.Request) (*blogAIChatReq, bool) {
	var in blogAIChatReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "AI_BAD_BODY", "请求体解析失败")
		return nil, false
	}
	in.Content = clipRunes(strings.TrimSpace(in.Content), blogAIMaxInput)
	in.Topic = clipRunes(strings.TrimSpace(in.Topic), 200)
	in.Instruction = clipRunes(strings.TrimSpace(in.Instruction), 1000)
	in.Title = clipRunes(strings.TrimSpace(in.Title), 200)
	return &in, true
}

// blogAIChat 调 LLM 并输出 {text}（system=指令与素材，user=具体请求）。
func (a *API) blogAIChat(w http.ResponseWriter, r *http.Request, system, user string) {
	res, err := a.ai.ChatJSON(r.Context(), []ai.Msg{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AI_UNAVAILABLE", "AI 服务暂不可用："+err.Error())
		return
	}
	if res.Error != "" {
		writeErr(w, http.StatusBadGateway, "AI_ERROR", res.Error)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(res.Content)})
}

// blogAIOutline POST /api/v1/blog/ai/outline {topic, paths?} → {text: markdown 大纲}
func (a *API) blogAIOutline(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAIPermit(w, r); !ok {
		return
	}
	in, ok := a.decodeAIReq(w, r)
	if !ok {
		return
	}
	if in.Topic == "" {
		writeErr(w, http.StatusBadRequest, "AI_TOPIC_REQUIRED", "topic 必填")
		return
	}
	mats := a.blogMaterial(r, in.Topic, in.Paths, blogAIMaxMaterial)
	sys := "你是资深博客作者的写作助手。基于主题与素材生成文章大纲（Markdown 输出）：" +
		"含 3-6 个层级化章节、每章 1 句要点说明、结尾给出标题建议 3 个。只输出大纲本身。\n\n" +
		materialContext(mats)
	a.blogAIChat(w, r, sys, "写作主题："+in.Topic)
}

// blogAIContinue POST /api/v1/blog/ai/continue {content, instruction?, paths?} → {text}
func (a *API) blogAIContinue(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAIPermit(w, r); !ok {
		return
	}
	in, ok := a.decodeAIReq(w, r)
	if !ok {
		return
	}
	if in.Content == "" {
		writeErr(w, http.StatusBadRequest, "AI_CONTENT_REQUIRED", "content 必填")
		return
	}
	mats := a.blogMaterial(r, in.Topic, in.Paths, 8)
	extra := ""
	if in.Instruction != "" {
		extra = "；续写要求：" + in.Instruction
	}
	sys := "你是与作者风格一致的写作助手：从给定正文的结尾自然续写（Markdown，300-600 字），" +
		"不重复已有内容、不写总结性结束语（除非明确要求）、不要输出正文原有部分。" +
		"用户消息里会给出续写要求与已有正文。\n\n" + materialContext(mats)
	a.blogAIChat(w, r, sys, extra+"\n已有正文：\n"+in.Content)
}

// blogAIPolish POST /api/v1/blog/ai/polish {content, mode: polish|expand|shorten} → {text}
func (a *API) blogAIPolish(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAIPermit(w, r); !ok {
		return
	}
	in, ok := a.decodeAIReq(w, r)
	if !ok {
		return
	}
	if in.Content == "" {
		writeErr(w, http.StatusBadRequest, "AI_CONTENT_REQUIRED", "content 必填")
		return
	}
	mode := in.Mode
	if mode == "" {
		mode = "polish"
	}
	var task string
	switch mode {
	case "expand":
		task = "扩写：在保持原意与结构的前提下补充细节、示例与过渡，篇幅约为原文 1.5-2 倍。"
	case "shorten":
		task = "缩写：保留核心观点与关键细节，篇幅约为原文一半，语句精炼。"
	default:
		task = "润色：优化表达、修正错别字与语病、统一语气，不改变原意与篇幅。"
	}
	sys := "你是专业中文编辑。" + task + "（Markdown 输出，只输出改写后的正文，不要任何解释）。"
	a.blogAIChat(w, r, sys, "原文：\n"+in.Content)
}

// blogAITags POST /api/v1/blog/ai/tags {title, content, paths?} → {tags: []}
func (a *API) blogAITags(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAIPermit(w, r); !ok {
		return
	}
	in, ok := a.decodeAIReq(w, r)
	if !ok {
		return
	}
	if in.Content == "" && in.Title == "" {
		writeErr(w, http.StatusBadRequest, "AI_CONTENT_REQUIRED", "title 或 content 至少一项")
		return
	}
	mats := a.blogMaterial(r, in.Title+" "+firstLineOf(in.Content), in.Paths, 6)
	sys := "你是博客标签编辑：根据标题与正文推荐 3-6 个中文标签（每个不超过 8 字、不带 #）。\n" +
		"只输出 JSON 数组，格式如 [\"效率工具\",\"工作流\"]，不要任何其他文字。\n\n" +
		materialContext(mats)
	user := "标题：" + in.Title + "\n正文：\n" + clipRunes(in.Content, 3000)
	res, err := a.ai.ChatJSON(r.Context(), []ai.Msg{
		{Role: "system", Content: sys},
		{Role: "user", Content: user},
	}, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AI_UNAVAILABLE", "AI 服务暂不可用："+err.Error())
		return
	}
	if res.Error != "" {
		writeErr(w, http.StatusBadGateway, "AI_ERROR", res.Error)
		return
	}
	tags := parseTagsJSON(res.Content)
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

// parseTagsJSON 宽松解析模型输出的标签 JSON 数组（容忍 ```json 包裹与尾部说明）。
func parseTagsJSON(s string) []string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			s = s[i : j+1]
		}
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return []string{}
	}
	clean := []string{}
	for _, t := range out {
		t = strings.TrimSpace(t)
		if t != "" && len([]rune(t)) <= 16 {
			clean = append(clean, t)
		}
	}
	if len(clean) > 8 {
		clean = clean[:8]
	}
	return clean
}

// firstLineOf 取文本第一行（素材关键词用）。
func firstLineOf(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}
