// Package handler 提供 HTTP/WS 入口。
//
// ingest.go 评论收录（B3：内容引用与归因）。
// 移植自上游 AiKmap.cn（`handler/ingest.go`，内容树《AiK 内容树-收录机制与旧文自生长》§一），
// 按本壳数据模型与守卫体系改造。语义：把评论区高价值内容**带归因**追加进 .md 正文——
// 交互沉淀为内容，旧文自生长。
//
//   - 引用式（quote）：原文照录 + 归因区块（署名/日期/回指原评论锚链接），零风险，默认；
//   - 融合式（fuse）：AI 把多条碎片评论重组为与正文连贯的段落，归因脚注式
//     （"本节整理自 @A、@B"，人名链回原评论）。两段式落地：
//     draft（AI 只产草稿，不碰正文）→ accept（站长确认才写入）。
//
// 硬边界：收录只 append 不 mutate；撤销按锚点区块精确删除（不动后来追加的年轮）。
// 归因区块模板写死、不可关闭（署名 + 日期 + 回指链接三要素）；被收录者收到站内通知（荣耀激励）。
//
// 本壳相对上游的改造点（理由见 docs/07-协作/上游内容线与工作流集成方案-20260921.md §5.4）：
//  1. 评论表用本壳 `comments`（上游另起 `blog_comments`）：正文列 `body`、归属列 `file_id`；
//     作者名 = 登录用户 `users.display_name`，访客回落 `comments.guest_name`
//     （上游用冗余列 `author_name`）。
//  2. 守卫用本壳 `blogAuthorOnly`（owner/admin/作者白名单），并校验目标确为博客文章
//     （上游按 `files.owner_id == uid` 判，在本壳「文章统一挂博客目录」的形态下过窄）。
//  3. 回指链接用本壳公开文章地址 `{origin}/{slug}#comment-{id}`（上游是 SPA 深链
//     `#/p/blog?slug=...`）；SSR 列表已由 sitemap 以同一地址公布，保持一致。
//  4. AI 步骤先判 `ActiveProvider("ai.llm")`——本壳 ChatJSON 未配置时**返回降级文案而非 error**，
//     不判会把「AI 服务未配置…」当成文章正文写进草稿（与 B2 的 llm 步骤同一坑）。
//  5. 通知走 `service.NotifyStore.AddUser`（统一 payload 契约），不直接 INSERT notifications。
//  6. 跨文件聚合端点（pending）带站点隔离（`files.site_id = currentSiteID(r)`），
//     否则多站点下会跨站串数据。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

const (
	ingestStatusDraft     = "draft"
	ingestStatusAccepted  = "accepted"
	ingestStatusReverted  = "reverted"
	ingestStatusDiscarded = "discarded"

	ingestModeQuote = "quote"
	ingestModeFuse  = "fuse"

	ingestSourceBlog = "blog"

	ingestMaxComments = 8    // 融合式单组最多聚合评论数（保成本、保可读）
	ingestMaxBodyRune = 6000 // 融合输入材料总长上限（rune）
	ingestMaxHeadRune = 2000 // 正文开头作为上下文的截断长度（rune）
)

// ingestGroupRow 收录组输出项。
type ingestGroupRow struct {
	ID            string          `json:"id"`
	FileID        string          `json:"file_id"`
	Mode          string          `json:"mode"`
	Source        string          `json:"source"`
	Status        string          `json:"status"`
	Title         string          `json:"title"`
	Body          string          `json:"body,omitempty"`
	Draft         string          `json:"draft,omitempty"`
	VersionBefore int             `json:"version_before"`
	VersionAfter  int             `json:"version_after"`
	CreatedAt     int64           `json:"created_at"`
	Comments      []ingestItemRow `json:"comments,omitempty"`
}

// ingestItemRow 收录项（组内一条评论；按组返回）。
type ingestItemRow struct {
	CommentID  string `json:"comment_id"`
	AuthorID   string `json:"-"` // 服务端用（重投通知），不外露
	AuthorName string `json:"author_name"`
	Content    string `json:"content"`
}

// ingestCommentRow 评论素材（含归因所需的作者展示名与作者 id）。
type ingestCommentRow struct {
	CommentID  string
	AuthorID   string
	AuthorName string
	Content    string
}

// ingestGuard 收录守卫：博客作者/管理员（本壳统一走 blogAuthorOnly），
// 且目标必须是博客目录下的文章。返回操作者 id；失败时已写出响应。
func (a *API) ingestGuard(w http.ResponseWriter, r *http.Request, fileID string) (string, bool) {
	if fileID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQUEST", "file_id required")
		return "", false
	}
	if !a.blogAuthorOnly(w, r) {
		return "", false
	}
	if !a.isBlogPostFile(a.db, fileID, service.BlogDirID) {
		writeErr(w, http.StatusNotFound, "POST_NOT_FOUND", "文章不存在或不属于博客："+fileID)
		return "", false
	}
	return a.curUserID(r), true
}

// ingestReadContent 取正文当前内容（仅文本文件可收录）。
func (a *API) ingestReadContent(ctx context.Context, fileID string) (string, error) {
	rc, f, err := a.files.Content(ctx, fileID)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	if f.Kind != "file" || !isTextContent(f.Mime) {
		return "", errors.New("仅文本文件可收录")
	}
	return string(b), nil
}

// isTextContent 收录侧文本判定：**结构化文本白名单**，比 service.isTextMime 更严。
//
// 为什么不照抄上游的子串匹配：`strings.Contains(m, "xml")` 会命中
// `application/vnd.openxmlformats-officedocument.wordprocessingml.document`
// （openxmlformats 含 "xml"）→ 会把 .docx 当正文读进来照录，写回即损坏二进制。
// 收录是「把内容 append 进正文」的写操作，判据必须精确：
//   - `text/*` 一律放行；
//   - RFC 6839 结构化语法后缀 `+json` / `+xml`（application/atom+xml、application/ld+json …）；
//   - 少数无后缀的 application/* 文本类型显式列举；
//   - 其余（含二进制文档、图片、压缩包）一律拒绝。
//
// 白名单与 service.isTextMime 同构，唯一差异：空 mime 在收录侧按文本放行
// （未知类型不应一律拒绝），而 service.isTextMime 对空 mime 返回 false。
// 其余判据一致，保证「读得过 → 也写得回」，不会出现读到一半写失败的半状态。
func isTextContent(mime string) bool {
	if mime == "" {
		return true
	}
	m := strings.ToLower(strings.TrimSpace(mime))
	if i := strings.IndexByte(m, ';'); i >= 0 { // 去掉 charset 等参数
		m = strings.TrimSpace(m[:i])
	}
	if strings.HasPrefix(m, "text/") {
		return true
	}
	if strings.HasSuffix(m, "+json") || strings.HasSuffix(m, "+xml") {
		return true
	}
	switch m {
	case "application/json", "application/xml",
		"application/yaml", "application/x-yaml",
		"application/javascript", "application/x-javascript",
		"application/typescript":
		return true
	}
	return false
}

// growthAllowed 文章生长策略：files.content_state.growth 非 frozen 才允许收录。
// growth: allow（允许评论收录，默认）| collect（+采集线索）| auto（全自动）| frozen（冻结）。
// 由 POST /api/v1/blog/posts/meta 的 growth 字段写入；缺省即 allow（向后兼容旧数据）。
func (a *API) growthAllowed(ctx context.Context, fileID string) bool {
	var state string
	_ = a.db.QueryRowContext(ctx,
		`SELECT COALESCE(json_extract(content_state, '$.growth'), 'allow') FROM files WHERE id=?`, fileID).Scan(&state)
	return state != "frozen"
}

// ingestFetchComment 读取单条「已通过」评论（登录用户取 display_name，访客回落 guest_name）。
// 返回评论素材与它所属的文章 id。
func (a *API) ingestFetchComment(ctx context.Context, commentID string) (ingestCommentRow, string, error) {
	var c ingestCommentRow
	var fileID string
	err := a.db.QueryRowContext(ctx, `
		SELECT c.id,
		       COALESCE(c.user_id,''),
		       COALESCE(NULLIF(COALESCE(u.display_name,''),''), NULLIF(COALESCE(c.guest_name,''),''), '匿名读者'),
		       c.body,
		       c.file_id
		  FROM comments c LEFT JOIN users u ON u.id = c.user_id
		 WHERE c.id=? AND c.status='approved'`, commentID).
		Scan(&c.CommentID, &c.AuthorID, &c.AuthorName, &c.Content, &fileID)
	if err != nil {
		return c, "", err
	}
	return c, fileID, nil
}

// ingestPermalink 评论回指链接：本壳公开文章为 {origin}/{slug}，锚点 #comment-{id}。
// slug 缺失（未发布）时回落文件名去扩展名，与 SSR 列表 slugOf 口径一致。
func (a *API) ingestPermalink(r *http.Request, f *service.File, commentID string) string {
	slug := ""
	if f != nil {
		slug = strings.TrimSpace(f.Slug)
		if slug == "" {
			slug = strings.TrimSuffix(strings.TrimSuffix(f.Name, ".md"), ".markdown")
		}
	}
	return a.publicBaseURL(r) + "/" + urlQueryEscape(slug) + "#comment-" + commentID
}

// ingestNotify 收录成功 → 通知被收录评论作者（荣耀激励闭环）。
// 自己收录自己、或访客评论（无账号）不通知。
func (a *API) ingestNotify(ctx context.Context, postID, actorID, commentID, authorID, postName, permalink string) {
	if authorID == "" || authorID == actorID || a.notify == nil {
		return
	}
	_ = a.notify.AddUser(ctx, authorID, "ingest", map[string]any{
		"title":   "你的评论已被收录进正文",
		"message": "《" + postName + "》补录了你的评论，已保留署名并链接回原文。",
		"link":    permalink,
		"extra": map[string]any{
			"post_id":    postID,
			"comment_id": commentID,
		},
	})
}

// ingestAppendBlock 收录唯一写入方式：尾部追加（只 append 不 mutate，既有人工内容一字不动）。
func ingestAppendBlock(body, block string) string {
	return strings.TrimRight(body, "\n") + "\n\n" + block
}

// ingestRemoveBlock 按锚点精确摘除收录区块——撤销的唯一删除口径。
//
// 只摘本组锚点区间，**后来追加的年轮（其它 ingest 区块 / 人工新增内容）原样保留**：
//   - 区块前按 TrimRight 回收 append 时补的分隔空行（与 ingestAppendBlock 严格对称 → 可字节复原）；
//   - 区块后仍有内容时补回一处分隔，避免把两块粘在一行；
//   - 锚点缺失（可能被人工编辑破坏）返回 ok=false，调用方转人工处理而不是盲删。
func ingestRemoveBlock(body, gid string) (string, bool) {
	startMark := "<span id=\"ingest-" + gid + "\">"
	endMark := "<!-- ingest-end:" + gid + " -->"
	si := strings.Index(body, startMark)
	ei := strings.Index(body, endMark)
	if si < 0 || ei < 0 || ei < si {
		return body, false
	}
	end := ei + len(endMark)
	for end < len(body) && (body[end] == '\n' || body[end] == '\r') {
		end++
	}
	head := strings.TrimRight(body[:si], "\n")
	tail := body[end:]
	if tail == "" {
		return head, true
	}
	return head + "\n\n" + tail, true
}

// ingestQuoteBody 引用式归因区块模板（写死、不可关闭：署名 + 日期 + 回指锚链接三要素）。
func ingestQuoteBody(gid, authorName, permalink, content, date string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "<span id=\"ingest-%s\"></span>\n\n", gid)
	sb.WriteString("### ✦ 读者补充\n\n")
	fmt.Fprintf(&sb, "> 补录自读者 @%s 的评论（%s）\n", authorName, date)
	fmt.Fprintf(&sb, "> 评论原文见 [原评论](%s) ｜ 已获站内收录条款授权\n\n", permalink)
	sb.WriteString(content)
	sb.WriteString("\n\n<!-- ingest-end:")
	sb.WriteString(gid)
	sb.WriteString(" -->\n")
	return sb.String()
}

// ingestFuseBody 融合式区块：AI 融合正文 + 脚注归因（每个人名链回原评论）。
func ingestFuseBody(gid, fused, title string, items []ingestItemRow) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "<span id=\"ingest-%s\"></span>\n\n", gid)
	if title == "" {
		title = "读者补充"
	}
	fmt.Fprintf(&sb, "### ✦ %s\n\n", title)
	sb.WriteString(strings.TrimSpace(fused))
	sb.WriteString("\n\n本节整理自 ")
	refs := []string{}
	for _, it := range items {
		refs = append(refs, fmt.Sprintf("[@%s](#comment-%s)", it.AuthorName, it.CommentID))
	}
	sb.WriteString(strings.Join(refs, "、"))
	sb.WriteString(" 的评论\n\n<!-- ingest-end:")
	sb.WriteString(gid)
	sb.WriteString(" -->\n")
	return sb.String()
}

// ingestFuseSystem 融合式 AI 指令：读者出观点、AI 只代笔排版（不代笔观点）。
const ingestFuseSystem = `你是文章编辑。下面是一篇文章正文开头，以及评论区若干条高价值回复。
请把回复中相互补充的观点合并成一段与正文语气连贯的「读者补充」内容（中文，150-400 字）：
1. 只整合回复中已有的观点与事实，不添加回复中没有的新信息，不评价回复质量；
2. 保留有价值的细节（数据、做法、反例），把口语与重复整理成书面表达；
3. 输出正文段落本身，不要标题、不要编号、不要"以下是我整理的内容"这类说明；
4. 如果回复之间观点互相矛盾，保留并说明分歧（"有读者认为…也有读者提出…"）。`

// ingestAIReady AI 可用性守卫：本壳 ChatJSON 在未配置模型时返回降级文案而非 error，
// 必须显式判 ActiveProvider，否则降级文案会被当成正文写出去。
func (a *API) ingestAIReady(w http.ResponseWriter) bool {
	if a.ai == nil || a.ai.ActiveProvider("ai.llm") == "" {
		writeErr(w, http.StatusBadRequest, "INGEST_AI_OFF", "AI 模型未配置，融合式收录不可用（可改用引用式）")
		return false
	}
	return true
}

// ingestQuote 引用式收录：POST /api/v1/ingests/quote
// body: {"file_id":"...", "comment_id":"..."}
// 原文照录 + 归因区块追加进正文；版本快照 + 登记 + 审计 + 通知。
func (a *API) ingestQuote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileID    string `json:"file_id"`
		CommentID string `json:"comment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID == "" || req.CommentID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQUEST", "file_id/comment_id required")
		return
	}
	ctx := r.Context()
	uid, ok := a.ingestGuard(w, r, req.FileID)
	if !ok {
		return
	}
	if !a.growthAllowed(ctx, req.FileID) {
		writeErr(w, http.StatusBadRequest, "INGEST_FROZEN", "该文章已冻结收录（生长策略=frozen）")
		return
	}
	// 评论必须存在、已通过审核、且属于该文章
	c, cFileID, err := a.ingestFetchComment(ctx, req.CommentID)
	if err != nil || cFileID != req.FileID {
		writeErr(w, http.StatusBadRequest, "COMMENT_NOT_FOUND", "评论不存在或不属于该文章")
		return
	}
	// 防重复：同一评论只能在 accepted 组内收录一次
	var dup string
	_ = a.db.QueryRowContext(ctx,
		`SELECT g.id FROM ingest_items it JOIN ingest_groups g ON g.id=it.group_id
		 WHERE it.comment_id=? AND g.status='accepted' AND g.file_id=?`, req.CommentID, req.FileID).Scan(&dup)
	if dup != "" {
		writeErr(w, http.StatusConflict, "INGEST_DUPLICATE", "该评论已被收录")
		return
	}
	// 读正文 → 追加归因区块
	body, err := a.ingestReadContent(ctx, req.FileID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INGEST_NOT_TEXT", err.Error())
		return
	}
	f, _ := a.files.Get(ctx, req.FileID)
	if f == nil {
		writeErr(w, http.StatusNotFound, "POST_NOT_FOUND", "文章不存在")
		return
	}
	versionBefore := f.Version
	date := time.Now().Format("2006-01-02")
	permalink := a.ingestPermalink(r, f, req.CommentID)
	gid := newID()
	block := ingestQuoteBody(gid, c.AuthorName, permalink, c.Content, date)
	nf, err := a.files.UpdateContent(ctx, uid, req.FileID, ingestAppendBlock(body, block))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INGEST_WRITE_FAILED", err.Error())
		return
	}
	now := time.Now().UnixMilli()
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO ingest_groups (id, file_id, mode, source, status, body, version_before, version_after, actor_id, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		gid, req.FileID, ingestModeQuote, ingestSourceBlog, ingestStatusAccepted, block,
		versionBefore, nf.Version, uid, now, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "INGEST_REGISTER_FAILED", err.Error())
		return
	}
	_, _ = a.db.ExecContext(ctx,
		`INSERT INTO ingest_items (group_id, comment_id, author_id, author_name, content, created_at) VALUES (?,?,?,?,?,?)`,
		gid, req.CommentID, c.AuthorID, c.AuthorName, c.Content, now)
	a.ingestNotify(ctx, req.FileID, uid, req.CommentID, c.AuthorID, f.Name, permalink)
	a.b.Publish(ctx, bus.Event{Topic: "file.ingested", Key: req.FileID, Data: map[string]any{
		"mode": ingestModeQuote, "group_id": gid, "version": nf.Version}})
	_, _ = a.aud.Append(ctx, uid, "ingest.quote", req.FileID, map[string]any{
		"group_id": gid, "comment_id": req.CommentID, "version": nf.Version})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": gid, "anchor": "ingest-" + gid, "version": nf.Version})
}

// ingestFuseDraft 融合式草稿：POST /api/v1/ingests/fuse-draft
// body: {"file_id":"...", "comment_ids":["...", ...]}
// AI 只产出融合草稿（status=draft，不碰正文）；确认后走 /accept 才写入。
func (a *API) ingestFuseDraft(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileID     string   `json:"file_id"`
		CommentIDs []string `json:"comment_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID == "" || len(req.CommentIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "BAD_REQUEST", "file_id/comment_ids required")
		return
	}
	if len(req.CommentIDs) > ingestMaxComments {
		writeErr(w, http.StatusBadRequest, "INGEST_TOO_MANY",
			"融合式最多聚合 "+strconv.Itoa(ingestMaxComments)+" 条评论")
		return
	}
	ctx := r.Context()
	uid, ok := a.ingestGuard(w, r, req.FileID)
	if !ok {
		return
	}
	if !a.ingestAIReady(w) {
		return
	}
	if !a.growthAllowed(ctx, req.FileID) {
		writeErr(w, http.StatusBadRequest, "INGEST_FROZEN", "该文章已冻结收录")
		return
	}
	// 取评论素材（去重、按传入顺序保序）
	seen := map[string]bool{}
	items := []ingestItemRow{}
	for _, cid := range req.CommentIDs {
		if seen[cid] {
			continue
		}
		seen[cid] = true
		c, cfid, err := a.ingestFetchComment(ctx, cid)
		if err != nil || cfid != req.FileID {
			continue
		}
		items = append(items, ingestItemRow{
			CommentID: c.CommentID, AuthorID: c.AuthorID, AuthorName: c.AuthorName, Content: c.Content})
	}
	if len(items) == 0 {
		writeErr(w, http.StatusBadRequest, "COMMENT_NOT_FOUND", "没有可用的已通过评论")
		return
	}
	// 文章正文开头作上下文（截断保成本）
	cur, err := a.ingestReadContent(ctx, req.FileID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INGEST_NOT_TEXT", err.Error())
		return
	}
	head := cur
	if rs := []rune(head); len(rs) > ingestMaxHeadRune {
		head = string(rs[:ingestMaxHeadRune])
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "文章正文开头：\n%s\n\n评论区高价值回复：\n", head)
	total := 0
	for i, it := range items {
		line := fmt.Sprintf("%d. @%s：%s", i+1, it.AuthorName, strings.TrimSpace(it.Content))
		total += len([]rune(line))
		if total > ingestMaxBodyRune {
			break
		}
		sb.WriteString(line + "\n")
	}
	res, err := a.ai.ChatJSON(ctx, []ai.Msg{
		{Role: "system", Content: ingestFuseSystem},
		{Role: "user", Content: sb.String()},
	}, nil)
	fuseErr := ""
	switch {
	case err != nil:
		fuseErr = err.Error()
	case res == nil:
		fuseErr = "空响应"
	case res.Error != "":
		fuseErr = res.Error
	case strings.TrimSpace(res.Content) == "":
		fuseErr = "空内容"
	}
	if fuseErr != "" {
		writeErr(w, http.StatusBadGateway, "INGEST_FUSE_FAILED", "AI 服务不可用："+fuseErr)
		return
	}
	gid := newID()
	now := time.Now().UnixMilli()
	draft := strings.TrimSpace(res.Content)
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO ingest_groups (id, file_id, mode, source, status, draft, actor_id, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		gid, req.FileID, ingestModeFuse, ingestSourceBlog, ingestStatusDraft, draft, uid, now, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "INGEST_REGISTER_FAILED", err.Error())
		return
	}
	for _, it := range items {
		_, _ = a.db.ExecContext(ctx,
			`INSERT INTO ingest_items (group_id, comment_id, author_id, author_name, content, created_at) VALUES (?,?,?,?,?,?)`,
			gid, it.CommentID, it.AuthorID, it.AuthorName, it.Content, now)
	}
	_, _ = a.aud.Append(ctx, uid, "ingest.fuse_draft", req.FileID, map[string]any{
		"group_id": gid, "comments": len(items)})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": gid, "draft": draft, "status": ingestStatusDraft, "comments": len(items)})
}

// ingestAccept 采纳融合草稿：POST /api/v1/ingests/{id}/accept
// 站长确认后写入正文（版本快照 + 登记 + 事件 + 通知 + 审计）。草稿原文保留可回查。
func (a *API) ingestAccept(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("id")
	ctx := r.Context()
	var fileID, mode, status, draft string
	err := a.db.QueryRowContext(ctx,
		`SELECT file_id, mode, status, draft FROM ingest_groups WHERE id=?`, gid).Scan(&fileID, &mode, &status, &draft)
	if err != nil {
		writeErr(w, http.StatusNotFound, "INGEST_NOT_FOUND", "收录记录不存在")
		return
	}
	if mode != ingestModeFuse || status != ingestStatusDraft {
		writeErr(w, http.StatusBadRequest, "INGEST_NOT_DRAFT", "仅融合式草稿可采纳")
		return
	}
	uid, ok := a.ingestGuard(w, r, fileID)
	if !ok {
		return
	}
	if !a.growthAllowed(ctx, fileID) {
		writeErr(w, http.StatusBadRequest, "INGEST_FROZEN", "该文章已冻结收录")
		return
	}
	// 取组内评论项（脚注归因）
	rows, err := a.db.QueryContext(ctx,
		`SELECT comment_id, COALESCE(author_id,''), COALESCE(author_name,''), content
		   FROM ingest_items WHERE group_id=? ORDER BY created_at ASC`, gid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INGEST_ITEMS_FAILED", err.Error())
		return
	}
	items := []ingestItemRow{}
	for rows.Next() {
		var it ingestItemRow
		if rows.Scan(&it.CommentID, &it.AuthorID, &it.AuthorName, &it.Content) == nil {
			items = append(items, it)
		}
	}
	rows.Close()
	body, err := a.ingestReadContent(ctx, fileID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INGEST_NOT_TEXT", err.Error())
		return
	}
	f, _ := a.files.Get(ctx, fileID)
	if f == nil {
		writeErr(w, http.StatusNotFound, "POST_NOT_FOUND", "文章不存在")
		return
	}
	versionBefore := f.Version
	block := ingestFuseBody(gid, draft, "", items)
	nf, err := a.files.UpdateContent(ctx, uid, fileID, ingestAppendBlock(body, block))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INGEST_WRITE_FAILED", err.Error())
		return
	}
	now := time.Now().UnixMilli()
	if _, err := a.db.ExecContext(ctx,
		`UPDATE ingest_groups SET status=?, body=?, version_before=?, version_after=?, updated_at=? WHERE id=?`,
		ingestStatusAccepted, block, versionBefore, nf.Version, now, gid); err != nil {
		writeErr(w, http.StatusInternalServerError, "INGEST_REGISTER_FAILED", err.Error())
		return
	}
	// 通知：按原评论作者回投（author_id 在 fuse-draft 时已冗余落库，无需反查）
	for _, it := range items {
		a.ingestNotify(ctx, fileID, uid, it.CommentID, it.AuthorID, f.Name,
			a.ingestPermalink(r, f, it.CommentID))
	}
	a.b.Publish(ctx, bus.Event{Topic: "file.ingested", Key: fileID, Data: map[string]any{
		"mode": ingestModeFuse, "group_id": gid, "version": nf.Version}})
	_, _ = a.aud.Append(ctx, uid, "ingest.fuse_accept", fileID, map[string]any{
		"group_id": gid, "version": nf.Version})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": gid, "anchor": "ingest-" + gid, "version": nf.Version})
}

// ingestRevert 撤销收录：DELETE /api/v1/ingests/{id}
// 按锚点区块精确删除（不动其他年轮），登记置 reverted 留痕。
func (a *API) ingestRevert(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("id")
	ctx := r.Context()
	var fileID, status string
	err := a.db.QueryRowContext(ctx,
		`SELECT file_id, status FROM ingest_groups WHERE id=?`, gid).Scan(&fileID, &status)
	if err != nil {
		writeErr(w, http.StatusNotFound, "INGEST_NOT_FOUND", "收录记录不存在")
		return
	}
	// 草稿放弃：从未写入正文，天然无需动 body，仅登记置 discarded 退出待采纳列表。
	// （撤销=回滚已生效的写入；放弃=丢弃未生效的草稿，两者语义不同故分列状态。）
	if status == ingestStatusDraft {
		uid, ok := a.ingestGuard(w, r, fileID)
		if !ok {
			return
		}
		now := time.Now().UnixMilli()
		if _, err := a.db.ExecContext(ctx,
			`UPDATE ingest_groups SET status=?, updated_at=? WHERE id=?`,
			ingestStatusDiscarded, now, gid); err != nil {
			writeErr(w, http.StatusInternalServerError, "INGEST_REGISTER_FAILED", err.Error())
			return
		}
		_, _ = a.aud.Append(ctx, uid, "ingest.discard", fileID, map[string]any{"group_id": gid})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": ingestStatusDiscarded})
		return
	}
	if status != ingestStatusAccepted {
		writeErr(w, http.StatusBadRequest, "INGEST_NOT_ACCEPTED", "仅已写入正文的收录可撤销（草稿可放弃）")
		return
	}
	uid, ok := a.ingestGuard(w, r, fileID)
	if !ok {
		return
	}
	body, err := a.ingestReadContent(ctx, fileID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INGEST_NOT_TEXT", err.Error())
		return
	}
	newContent, ok := ingestRemoveBlock(body, gid)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "INGEST_ANCHOR_LOST",
			"正文锚点缺失（可能已被手动编辑），请人工处理")
		return
	}
	nf, err := a.files.UpdateContent(ctx, uid, fileID, newContent)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INGEST_WRITE_FAILED", err.Error())
		return
	}
	now := time.Now().UnixMilli()
	if _, err := a.db.ExecContext(ctx,
		`UPDATE ingest_groups SET status=?, version_after=?, updated_at=? WHERE id=?`,
		ingestStatusReverted, nf.Version, now, gid); err != nil {
		writeErr(w, http.StatusInternalServerError, "INGEST_REGISTER_FAILED", err.Error())
		return
	}
	a.b.Publish(ctx, bus.Event{Topic: "file.ingested", Key: fileID, Data: map[string]any{
		"mode": "revert", "group_id": gid, "version": nf.Version}})
	_, _ = a.aud.Append(ctx, uid, "ingest.revert", fileID, map[string]any{
		"group_id": gid, "version": nf.Version})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": nf.Version})
}

// ingestList 收录回查：GET /api/v1/ingests?file_id=...
func (a *API) ingestList(w http.ResponseWriter, r *http.Request) {
	fileID := strings.TrimSpace(r.URL.Query().Get("file_id"))
	ctx := r.Context()
	// 给了 file_id → 单篇回查（走完整文章守卫）；没给 → 当前站点跨文章列表（作者可见范围）。
	if fileID != "" {
		if _, ok := a.ingestGuard(w, r, fileID); !ok {
			return
		}
	} else if !a.blogAuthorOnly(w, r) {
		return
	}
	q := `SELECT g.id, g.file_id, g.mode, g.source, g.status, g.title, g.body, g.draft,
	             g.version_before, g.version_after, g.created_at, COALESCE(f.name,'')
	      FROM ingest_groups g LEFT JOIN files f ON f.id=g.file_id`
	args := []any{}
	if fileID != "" {
		q += ` WHERE g.file_id=?`
		args = append(args, fileID)
	} else {
		// 跨文章：站点隔离 + 非管理员只看自己的文章（与 /ingests/pending 同口径），
		// 否则多站点下会跨站串数据、越权看到他人收录记录。
		sel := `SELECT id FROM files WHERE deleted_at IS NULL AND site_id=?`
		args = append(args, currentSiteID(r))
		if !a.isAdmin(r) {
			sel += ` AND owner_id=?`
			args = append(args, a.curUserID(r))
		}
		q += ` WHERE g.file_id IN (` + sel + `)`
	}
	q += ` ORDER BY g.created_at DESC LIMIT 100`
	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INGEST_LIST_FAILED", err.Error())
		return
	}
	defer rows.Close()
	out := []ingestGroupRow{}
	for rows.Next() {
		var g ingestGroupRow
		var postName string
		if err := rows.Scan(&g.ID, &g.FileID, &g.Mode, &g.Source, &g.Status, &g.Title, &g.Body, &g.Draft,
			&g.VersionBefore, &g.VersionAfter, &g.CreatedAt, &postName); err != nil {
			continue
		}
		if g.Title == "" {
			g.Title = postName // 复用 Title 承载文章名（前端列表展示用，与 pending 一致）
		}
		if g.Status == ingestStatusAccepted {
			g.Body = "" // accepted 内容已在正文里，回查接口不再重复吐大段区块
		}
		if irows, err := a.db.QueryContext(ctx,
			`SELECT comment_id, COALESCE(author_id,''), COALESCE(author_name,''), content
			   FROM ingest_items WHERE group_id=?`, g.ID); err == nil {
			for irows.Next() {
				var it ingestItemRow
				if irows.Scan(&it.CommentID, &it.AuthorID, &it.AuthorName, &it.Content) == nil {
					g.Comments = append(g.Comments, it)
				}
			}
			irows.Close()
		}
		out = append(out, g)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

// ingestMyCount 我的被收录次数：GET /api/v1/ingests/my-count
// 个人页声誉（reputation）用；跨站点全局计数（作者维度，与站点归属无关）。
func (a *API) ingestMyCount(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	var n int
	_ = a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM ingest_items it
		 JOIN ingest_groups g ON g.id=it.group_id
		 WHERE it.author_id=? AND g.status='accepted'`, uid).Scan(&n)
	writeJSON(w, http.StatusOK, map[string]any{"count": n})
}

// ingestPendings 待采纳融合草稿：GET /api/v1/ingests/pending（跨文章）
// 站点隔离：只出当前站点文章的草稿；非管理员只看自己的文章。
func (a *API) ingestPendings(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	q := `SELECT g.id, g.file_id, g.mode, g.status, g.draft, g.created_at, COALESCE(f.name,'')
	      FROM ingest_groups g JOIN files f ON f.id=g.file_id
	      WHERE g.status='draft' AND f.deleted_at IS NULL AND f.site_id=?`
	args := []any{currentSiteID(r)}
	if !a.isAdmin(r) {
		q += ` AND f.owner_id=?`
		args = append(args, uid)
	}
	q += ` ORDER BY g.created_at DESC LIMIT 50`
	rows, err := a.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INGEST_PENDING_FAILED", err.Error())
		return
	}
	defer rows.Close()
	out := []ingestGroupRow{}
	for rows.Next() {
		var g ingestGroupRow
		var postName string
		if err := rows.Scan(&g.ID, &g.FileID, &g.Mode, &g.Status, &g.Draft, &g.CreatedAt, &postName); err != nil {
			continue
		}
		g.Title = postName // 复用 Title 字段承载文章名（前端列表展示用）
		out = append(out, g)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}
