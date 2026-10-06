// ingest_test.go 评论收录（B3）回归。
//
// 覆盖点：
//  1. 纯函数契约：文本判定、归因区块模板三要素（署名/日期/回指链接）、锚点区块追加-摘除**可字节复原**
//     （这是"只 append 不 mutate、撤销只摘一个年轮"的硬边界，用断言钉住比靠人记可靠）。
//  2. 迁移落地：ingest_groups / ingest_items 两表 + file.ingested 事件 topic 在册。
//  3. 真机链路（本地 FileStore + 临时库）：引用式收录写正文并可撤销、重复收录拒绝、
//     生长策略 frozen 拦截、AI 未配置时融合式被守卫拦、跨站点待办隔离。
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/service"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// newIngestAPI 建一个「能真正读写正文」的测试环境，返回 API、文章 id、文章原始正文。
func newIngestAPI(t *testing.T) (*API, string, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "ingest.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	b := bus.New()
	aud := service.New(db)
	ctx := context.Background()
	if err := service.EnsureSystem(ctx, db, aud); err != nil {
		t.Fatalf("ensure system: %v", err)
	}
	if err := service.EnsureBlogSpace(ctx, db); err != nil {
		t.Fatalf("ensure blog space: %v", err)
	}
	st, err := storage.NewLocal(filepath.Join(dir, "files"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	a := &API{db: db, b: b, aud: aud, files: service.NewFileStore(db, st, b, aud), notify: service.NewNotifyStore(db)}
	body := "# 标题\n\n正文段落。"
	f, err := a.files.CreateDoc(ctx, service.SystemOwnerID, service.SystemHomeSpaceID,
		service.BlogDirID, "收录测试.md", body, service.DefaultSiteID)
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	return a, f.ID, body
}

// addComment 直接落一条「已通过」评论（绕过 HTTP 层，聚焦收录逻辑）。
func addComment(t *testing.T, a *API, fileID, body, guest string) string {
	t.Helper()
	id := newID()
	if _, err := a.db.Exec(
		`INSERT INTO comments (id, file_id, user_id, body, status, guest_name, created_at) VALUES (?,?,NULL,?,?,?,?)`,
		id, fileID, body, "approved", guest, 1); err != nil {
		t.Fatalf("insert comment: %v", err)
	}
	return id
}

// readDoc 读回文章正文（测试断言用）。
func readDoc(t *testing.T, a *API, fileID string) string {
	t.Helper()
	rc, _, err := a.files.Content(context.Background(), fileID)
	if err != nil {
		t.Fatalf("读正文: %v", err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("读正文: %v", err)
	}
	return string(b)
}

func postJSON(t *testing.T, a *API, fn http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	// M5 后收录/引用等写端点走 blogAuthorOnly（只认 authMiddleware 注入的显式登录身份），
	// 本测试是直调 handler 形态，需自己补上站长身份。
	req = asOwner(a, req)
	w := httptest.NewRecorder()
	fn(w, req)
	return w
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析响应失败: %v（body=%s）", err, w.Body.String())
	}
	return out.Error.Code
}

// ---------- 1. 纯函数契约 ----------

func TestIsTextContent(t *testing.T) {
	cases := map[string]bool{
		"":                          true, // 未知 mime 放行（历史数据 mime 常为空）
		"text/markdown":             true,
		"text/plain; charset=utf-8": true,
		"application/json":          true,
		"application/xml":           true,
		"image/png":                 false,
		"application/octet-stream":  false,
		"application/pdf":           false,
		// 上游子串匹配的误判现场：mime 里含 "xml"（open**xml**formats）但实为二进制文档。
		// 收录侧必须判 false，否则会把 docx 当正文读进来照录 → 写回即损坏。
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": false,
		// 正向：结构化语法后缀仍须放行
		"application/atom+xml":     true,
		"application/ld+json":      true,
		"text/html; charset=utf-8": true,
	}
	for mime, want := range cases {
		if got := isTextContent(mime); got != want {
			t.Errorf("isTextContent(%q) = %v, want %v", mime, got, want)
		}
	}
}

// TestIngestQuoteBodyTemplate 归因三要素写死、不可关闭（署名 + 日期 + 回指链接）。
func TestIngestQuoteBodyTemplate(t *testing.T) {
	blk := ingestQuoteBody("g1", "读者甲", "https://aiklog.cn/hello#comment-c1", "这个做法我试过，有效。", "2026-09-21")
	for _, want := range []string{
		`<span id="ingest-g1"></span>`, // 锚点起点
		"### ✦ 读者补充",                   // 固定小节标题
		"@读者甲",                         // 署名
		"2026-09-21",                   // 日期
		"[原评论](https://aiklog.cn/hello#comment-c1)", // 回指原评论
		"这个做法我试过，有效。",                               // 原文照录
		"<!-- ingest-end:g1 -->",                    // 锚点终点
	} {
		if !strings.Contains(blk, want) {
			t.Errorf("引用式区块缺少 %q\n---\n%s", want, blk)
		}
	}
}

// TestIngestFuseBodyTemplate 融合式：AI 文本 + 脚注归因，每个人名链回原评论。
func TestIngestFuseBodyTemplate(t *testing.T) {
	blk := ingestFuseBody("g2", "读者甲认为 A 更好，读者乙补充了 B 的细节。", "读者补充",
		[]ingestItemRow{{CommentID: "c1", AuthorName: "读者甲"}, {CommentID: "c2", AuthorName: "读者乙"}})
	for _, want := range []string{
		`<span id="ingest-g2"></span>`,
		"### ✦ 读者补充",
		"读者甲认为 A 更好，读者乙补充了 B 的细节。",
		"[@读者甲](#comment-c1)",
		"[@读者乙](#comment-c2)",
		"<!-- ingest-end:g2 -->",
	} {
		if !strings.Contains(blk, want) {
			t.Errorf("融合式区块缺少 %q\n---\n%s", want, blk)
		}
	}
	// 空标题回落默认值（AI 不给标题时不能出现 "### ✦ " 空标题）
	if s := ingestFuseBody("g3", "x", "", nil); !strings.Contains(s, "### ✦ 读者补充") {
		t.Errorf("空标题未回落默认值：%s", s)
	}
}

// TestIngestBlockRoundTrip 追加-摘除严格对称：撤销后**字节复原**，且不影响其它年轮。
func TestIngestBlockRoundTrip(t *testing.T) {
	orig := "# 标题\n\n正文段落。"
	b1 := ingestQuoteBody("g1", "甲", "u1", "第一条", "2026-09-21")
	b2 := ingestQuoteBody("g2", "乙", "u2", "第二条", "2026-09-22")

	// 两次收录
	body := ingestAppendBlock(ingestAppendBlock(orig, b1), b2)
	if !strings.Contains(body, "第一条") || !strings.Contains(body, "第二条") {
		t.Fatal("两次追加后两条都在正文里")
	}
	// 撤销第一条（早期年轮）→ 第二条必须原样保留
	got, ok := ingestRemoveBlock(body, "g1")
	if !ok {
		t.Fatal("g1 锚点在册，应能摘除")
	}
	if !strings.Contains(got, "第二条") {
		t.Fatalf("撤销 g1 不应动到 g2：\n%s", got)
	}
	if strings.Contains(got, "第一条") {
		t.Fatalf("撤销 g1 后不应残留 g1 内容：\n%s", got)
	}
	// 再撤销第二条 → 严格回到原文（字节级相等）
	got2, ok := ingestRemoveBlock(got, "g2")
	if !ok {
		t.Fatal("g2 锚点在册，应能摘除")
	}
	if got2 != orig {
		t.Fatalf("两次撤销后未字节复原：\ngot  = %q\nwant = %q", got2, orig)
	}
}

// TestIngestRemoveBlockAnchorLost 锚点被人工编辑破坏时必须拒绝盲删。
func TestIngestRemoveBlockAnchorLost(t *testing.T) {
	if _, ok := ingestRemoveBlock("# 标题\n\n没有任何锚点", "g9"); ok {
		t.Fatal("锚点缺失时应返回 ok=false")
	}
	if _, ok := ingestRemoveBlock("<!-- ingest-end:g9 -->\n<span id=\"ingest-g9\">", "g9"); ok {
		t.Fatal("终哨在起点之前（顺序颠倒）时应返回 ok=false")
	}
}

// ---------- 2. 迁移 / 契约落地 ----------

func TestIngestTablesMigrated(t *testing.T) {
	a, _, _ := newIngestAPI(t)
	for _, tbl := range []string{"ingest_groups", "ingest_items"} {
		var name string
		if err := a.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name); err != nil {
			t.Fatalf("表 %s 未随迁移创建: %v", tbl, err)
		}
	}
}

// TestIngestTopicRegistered file.ingested 必须在内核事件契约清单内，否则 webhook 无法订阅。
func TestIngestTopicRegistered(t *testing.T) {
	if !bus.KernelTopics["file.ingested"] {
		t.Fatal("file.ingested 未登记进 bus.KernelTopics（webhook/插件将无法订阅）")
	}
}

// ---------- 3. 真机链路 ----------

// TestIngestQuoteEndToEnd 引用式：写正文 + 登记 + 版本自增 + 重复收录拒绝 + 撤销复原。
func TestIngestQuoteEndToEnd(t *testing.T) {
	a, fileID, orig := newIngestAPI(t)
	cid := addComment(t, a, fileID, "补一个反例：并发下会重复计数。", "读者甲")

	w := postJSON(t, a, a.ingestQuote, "/api/v1/ingests/quote",
		`{"file_id":"`+fileID+`","comment_id":"`+cid+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("引用式收录应 200，got %d %s", w.Code, w.Body.String())
	}
	var ok struct {
		OK     bool   `json:"ok"`
		ID     string `json:"id"`
		Anchor string `json:"anchor"`
		Ver    int    `json:"version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ok); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if !ok.OK || ok.ID == "" || ok.Anchor != "ingest-"+ok.ID || ok.Ver < 2 {
		t.Fatalf("响应不正确：%s", w.Body.String())
	}

	// 正文确实被追加，且原有正文一字未动（只 append 不 mutate）
	body := readDoc(t, a, fileID)
	if !strings.HasPrefix(body, orig) {
		t.Fatalf("既有正文被改动：\n%s", body)
	}
	if !strings.Contains(body, "补一个反例：并发下会重复计数。") || !strings.Contains(body, "@读者甲") {
		t.Fatalf("归因区块未写入正文：\n%s", body)
	}
	if !strings.Contains(body, "#comment-"+cid) {
		t.Fatalf("归因区块缺少回指原评论链接：\n%s", body)
	}

	// 组/项落库
	var status, mode string
	if err := a.db.QueryRow(`SELECT status, mode FROM ingest_groups WHERE id=?`, ok.ID).Scan(&status, &mode); err != nil {
		t.Fatalf("组未落库: %v", err)
	}
	if status != ingestStatusAccepted || mode != ingestModeQuote {
		t.Fatalf("组状态/模式不正确: %s/%s", status, mode)
	}
	var items int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM ingest_items WHERE group_id=?`, ok.ID).Scan(&items); err != nil || items != 1 {
		t.Fatalf("收录项应 1 条，got %d err=%v", items, err)
	}

	// 重复收录同一评论 → 409（幂等）
	w2 := postJSON(t, a, a.ingestQuote, "/api/v1/ingests/quote",
		`{"file_id":"`+fileID+`","comment_id":"`+cid+`"}`)
	if w2.Code != http.StatusConflict || errCode(t, w2) != "INGEST_DUPLICATE" {
		t.Fatalf("重复收录应 409 INGEST_DUPLICATE，got %d %s", w2.Code, w2.Body.String())
	}

	// 撤销 → 正文字节复原 + 状态 reverted
	req := httptest.NewRequest("DELETE", "/api/v1/ingests/"+ok.ID, nil)
	req.SetPathValue("id", ok.ID)
	w3 := httptest.NewRecorder()
	a.ingestRevert(w3, asOwner(a, req))
	if w3.Code != http.StatusOK {
		t.Fatalf("撤销应 200，got %d %s", w3.Code, w3.Body.String())
	}
	if got := readDoc(t, a, fileID); got != orig {
		t.Fatalf("撤销后正文未复原：\ngot  = %q\nwant = %q", got, orig)
	}
	if err := a.db.QueryRow(`SELECT status FROM ingest_groups WHERE id=?`, ok.ID).Scan(&status); err != nil || status != ingestStatusReverted {
		t.Fatalf("撤销后状态应为 reverted，got %s err=%v", status, err)
	}
	// 撤销后再撤销 → 400（只有 accepted 可撤销）
	req2 := httptest.NewRequest("DELETE", "/api/v1/ingests/"+ok.ID, nil)
	req2.SetPathValue("id", ok.ID)
	w4 := httptest.NewRecorder()
	a.ingestRevert(w4, asOwner(a, req2))
	if w4.Code != http.StatusBadRequest || errCode(t, w4) != "INGEST_NOT_ACCEPTED" {
		t.Fatalf("二次撤销应 400 INGEST_NOT_ACCEPTED，got %d %s", w4.Code, w4.Body.String())
	}
	// 撤销后同一评论可再次收录（去重只看 accepted）
	w5 := postJSON(t, a, a.ingestQuote, "/api/v1/ingests/quote",
		`{"file_id":"`+fileID+`","comment_id":"`+cid+`"}`)
	if w5.Code != http.StatusOK {
		t.Fatalf("撤销后应可重新收录，got %d %s", w5.Code, w5.Body.String())
	}
}

// TestIngestGuardRejectsNonBlogFile 非博客目录的文件一律 404（防止把任意文档当文章收录）。
func TestIngestGuardRejectsNonBlogFile(t *testing.T) {
	a, _, _ := newIngestAPI(t)
	f, err := a.files.CreateDoc(context.Background(), service.SystemOwnerID, service.SystemHomeSpaceID,
		"", "随手记.md", "正文", service.DefaultSiteID)
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	cid := addComment(t, a, f.ID, "评论", "读者")
	w := postJSON(t, a, a.ingestQuote, "/api/v1/ingests/quote",
		`{"file_id":"`+f.ID+`","comment_id":"`+cid+`"}`)
	if w.Code != http.StatusNotFound || errCode(t, w) != "POST_NOT_FOUND" {
		t.Fatalf("非博客文件应 404 POST_NOT_FOUND，got %d %s", w.Code, w.Body.String())
	}
}

// TestIngestGrowthFrozenBlocks growth=frozen 的文章拒绝一切收录。
func TestIngestGrowthFrozenBlocks(t *testing.T) {
	a, fileID, _ := newIngestAPI(t)
	cid := addComment(t, a, fileID, "评论", "读者")
	if _, err := a.db.Exec(
		`UPDATE files SET content_state=json_set(content_state,'$.growth','frozen') WHERE id=?`, fileID); err != nil {
		t.Fatalf("设 growth: %v", err)
	}
	w := postJSON(t, a, a.ingestQuote, "/api/v1/ingests/quote",
		`{"file_id":"`+fileID+`","comment_id":"`+cid+`"}`)
	if w.Code != http.StatusBadRequest || errCode(t, w) != "INGEST_FROZEN" {
		t.Fatalf("frozen 文章应 400 INGEST_FROZEN，got %d %s", w.Code, w.Body.String())
	}
	if a.growthAllowed(context.Background(), fileID) {
		t.Fatal("growth=frozen 时 growthAllowed 应为 false（拦住一切收录）")
	}
	// growth 缺省（旧数据）应放行 —— 向后兼容
	if _, err := a.db.Exec(`UPDATE files SET content_state=json_remove(content_state,'$.growth') WHERE id=?`, fileID); err != nil {
		t.Fatalf("清 growth: %v", err)
	}
	if !a.growthAllowed(context.Background(), fileID) {
		t.Fatal("growth 缺省（旧数据）应视为 allow")
	}
}

// TestIngestFuseDraftAIOff AI 未配置时融合式必须被守卫拦下（不得产出降级文案草稿）。
func TestIngestFuseDraftAIOff(t *testing.T) {
	a, fileID, _ := newIngestAPI(t)
	cid := addComment(t, a, fileID, "评论", "读者")
	w := postJSON(t, a, a.ingestFuseDraft, "/api/v1/ingests/fuse-draft",
		`{"file_id":"`+fileID+`","comment_ids":["`+cid+`"]}`)
	if w.Code != http.StatusBadRequest || errCode(t, w) != "INGEST_AI_OFF" {
		t.Fatalf("AI 未配置应 400 INGEST_AI_OFF，got %d %s", w.Code, w.Body.String())
	}
	var n int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM ingest_groups`).Scan(&n)
	if n != 0 {
		t.Fatalf("被守卫拦下时不应落任何收录组，got %d", n)
	}
	// 参数校验先于守卫：超过 8 条直接 400
	ids := make([]string, 0, 9)
	for i := 0; i < 9; i++ {
		ids = append(ids, `"c`+string(rune('a'+i))+`"`)
	}
	w2 := postJSON(t, a, a.ingestFuseDraft, "/api/v1/ingests/fuse-draft",
		`{"file_id":"`+fileID+`","comment_ids":[`+strings.Join(ids, ",")+`]}`)
	if w2.Code != http.StatusBadRequest || errCode(t, w2) != "INGEST_TOO_MANY" {
		t.Fatalf("超 8 条应 400 INGEST_TOO_MANY，got %d %s", w2.Code, w2.Body.String())
	}
}

// TestIngestPendingSiteIsolated 跨文件待办必须按站点隔离（多站点下不得串数据）。
func TestIngestPendingSiteIsolated(t *testing.T) {
	a, fileDefault, _ := newIngestAPI(t)
	ctx := context.Background()
	fB, err := a.files.CreateDoc(ctx, service.SystemOwnerID, service.SystemHomeSpaceID,
		service.BlogDirID, "B站文章.md", "正文", "site-b")
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	for _, fid := range []string{fileDefault, fB.ID} {
		if _, err := a.db.Exec(
			`INSERT INTO ingest_groups (id, file_id, mode, source, status, draft, actor_id, created_at, updated_at)
			 VALUES (?,?,'fuse','blog','draft','草稿内容',?,?,?)`,
			newID(), fid, service.SystemOwnerID, 1, 1); err != nil {
			t.Fatalf("insert draft group: %v", err)
		}
	}
	list := func(site string) int {
		req := httptest.NewRequest("GET", "/api/v1/ingests/pending", nil)
		req = req.WithContext(context.WithValue(req.Context(), ctxSiteID, site))
		w := httptest.NewRecorder()
		a.ingestPendings(w, asOwner(a, req))
		if w.Code != http.StatusOK {
			t.Fatalf("pending(%s) 应 200，got %d %s", site, w.Code, w.Body.String())
		}
		var out struct {
			Total int `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("解析: %v", err)
		}
		return out.Total
	}
	if n := list("default"); n != 1 {
		t.Fatalf("default 站待办应 1 条，got %d", n)
	}
	if n := list("site-b"); n != 1 {
		t.Fatalf("site-b 站待办应 1 条，got %d", n)
	}
	if n := list("site-c"); n != 0 {
		t.Fatalf("无待办的站点应 0 条（不得跨站串数据），got %d", n)
	}
}

// TestIngestMyCount 个人被收录次数（只算 accepted）。
func TestIngestMyCount(t *testing.T) {
	a, fileID, _ := newIngestAPI(t)
	// 一条登录用户评论（作者=系统 owner），一条访客评论
	uid := service.SystemOwnerID
	cid := newID()
	if _, err := a.db.Exec(
		`INSERT INTO comments (id, file_id, user_id, body, status, guest_name, created_at) VALUES (?,?,?,?,?,?,?)`,
		cid, fileID, uid, "我的评论", "approved", "", 1); err != nil {
		t.Fatalf("insert: %v", err)
	}
	w := postJSON(t, a, a.ingestQuote, "/api/v1/ingests/quote",
		`{"file_id":"`+fileID+`","comment_id":"`+cid+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("收录应 200，got %d %s", w.Code, w.Body.String())
	}
	req := httptest.NewRequest("GET", "/api/v1/ingests/my-count", nil)
	rec := httptest.NewRecorder()
	a.ingestMyCount(rec, asOwner(a, req))
	if rec.Code != http.StatusOK {
		t.Fatalf("my-count 应 200，got %d", rec.Code)
	}
	var out struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析: %v", err)
	}
	if out.Count != 1 {
		t.Fatalf("被收录次数应 1，got %d（body=%s）", out.Count, rec.Body.String())
	}
}

// TestIngestListHidesAcceptedBody 回查接口对 accepted 组不回吐大段区块（正文里已有）。
func TestIngestListHidesAcceptedBody(t *testing.T) {
	a, fileID, _ := newIngestAPI(t)
	cid := addComment(t, a, fileID, "评论内容", "读者")
	if w := postJSON(t, a, a.ingestQuote, "/api/v1/ingests/quote",
		`{"file_id":"`+fileID+`","comment_id":"`+cid+`"}`); w.Code != http.StatusOK {
		t.Fatalf("收录应 200，got %d %s", w.Code, w.Body.String())
	}
	req := httptest.NewRequest("GET", "/api/v1/ingests?file_id="+fileID, nil)
	rec := httptest.NewRecorder()
	a.ingestList(rec, asOwner(a, req))
	if rec.Code != http.StatusOK {
		t.Fatalf("回查应 200，got %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Total int `json:"total"`
		Items []struct {
			Status   string `json:"status"`
			Body     string `json:"body"`
			Comments []struct {
				AuthorName string `json:"author_name"`
			} `json:"comments"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析: %v", err)
	}
	if out.Total != 1 || len(out.Items) != 1 {
		t.Fatalf("应有 1 组，got %d", out.Total)
	}
	if out.Items[0].Body != "" {
		t.Fatalf("accepted 组的 body 应在接口层隐藏，got %q", out.Items[0].Body)
	}
	if len(out.Items[0].Comments) != 1 || out.Items[0].Comments[0].AuthorName != "读者" {
		t.Fatalf("组内评论归因信息缺失：%+v", out.Items[0].Comments)
	}
}

// ---- B11 收录入口 UI 配套：草稿放弃 + 跨文章列表 ----

// pendingTotal 走真实端点取「待采纳」总数（带站点上下文，避免跨站串数据干扰）。
func pendingTotal(t *testing.T, a *API, site string) int {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/v1/ingests/pending", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxSiteID, site))
	w := httptest.NewRecorder()
	a.ingestPendings(w, asOwner(a, req))
	if w.Code != http.StatusOK {
		t.Fatalf("pending(%s) 应 200，got %d %s", site, w.Code, w.Body.String())
	}
	var out struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析: %v", err)
	}
	return out.Total
}

// TestIngestDiscardDraft 草稿放弃：DELETE /ingests/{id} 对 draft 置 discarded。
//
// 与「撤销」的区别是本测试的核心断言：
//   - 放弃 = 从未写入正文，故**正文必须一字未动**，且从「待采纳」列表消失；
//   - 撤销只对 accepted 生效（草稿走放弃），二次操作一律 400，不存在 id 404。
func TestIngestDiscardDraft(t *testing.T) {
	a, fileID, orig := newIngestAPI(t)
	gid := newID()
	if _, err := a.db.Exec(
		`INSERT INTO ingest_groups (id, file_id, mode, source, status, draft, actor_id, created_at, updated_at)
		 VALUES (?,?,'fuse','blog','draft','草稿正文',?,?,?)`,
		gid, fileID, service.SystemOwnerID, 1, 1); err != nil {
		t.Fatalf("insert draft: %v", err)
	}
	if n := pendingTotal(t, a, service.DefaultSiteID); n != 1 {
		t.Fatalf("放弃前待采纳应 1 条，got %d", n)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/ingests/"+gid, nil)
	req.SetPathValue("id", gid)
	w := httptest.NewRecorder()
	a.ingestRevert(w, asOwner(a, req))
	if w.Code != http.StatusOK {
		t.Fatalf("放弃草稿应 200，got %d %s", w.Code, w.Body.String())
	}
	var out struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析: %v", err)
	}
	if !out.OK || out.Status != ingestStatusDiscarded {
		t.Fatalf("响应应为 discarded，got %s", w.Body.String())
	}

	var status string
	if err := a.db.QueryRow(`SELECT status FROM ingest_groups WHERE id=?`, gid).Scan(&status); err != nil {
		t.Fatalf("查库: %v", err)
	}
	if status != ingestStatusDiscarded {
		t.Fatalf("库内状态应为 discarded，got %s", status)
	}
	if got := readDoc(t, a, fileID); got != orig {
		t.Fatalf("放弃草稿不得改动正文：\ngot  = %q\nwant = %q", got, orig)
	}
	if n := pendingTotal(t, a, service.DefaultSiteID); n != 0 {
		t.Fatalf("放弃后待采纳应 0 条，got %d", n)
	}

	// 二次放弃 → 400（非 accepted 不可撤销；discarded 亦然）
	req2 := httptest.NewRequest("DELETE", "/api/v1/ingests/"+gid, nil)
	req2.SetPathValue("id", gid)
	w2 := httptest.NewRecorder()
	a.ingestRevert(w2, asOwner(a, req2))
	if w2.Code != http.StatusBadRequest || errCode(t, w2) != "INGEST_NOT_ACCEPTED" {
		t.Fatalf("二次放弃应 400 INGEST_NOT_ACCEPTED，got %d %s", w2.Code, w2.Body.String())
	}

	// 不存在的 id → 404
	req3 := httptest.NewRequest("DELETE", "/api/v1/ingests/not-exist", nil)
	req3.SetPathValue("id", "not-exist")
	w3 := httptest.NewRecorder()
	a.ingestRevert(w3, asOwner(a, req3))
	if w3.Code != http.StatusNotFound || errCode(t, w3) != "INGEST_NOT_FOUND" {
		t.Fatalf("不存在的收录应 404 INGEST_NOT_FOUND，got %d %s", w3.Code, w3.Body.String())
	}
}

// TestIngestListWithoutFileIDScopesSite 不带 file_id 时按站点出跨文章列表，并补出文章名。
// 多站点下若漏了 site_id 过滤会跨站串数据（前端「收录记录」面板直接暴露到别的站）。
func TestIngestListWithoutFileIDScopesSite(t *testing.T) {
	a, fileDefault, _ := newIngestAPI(t)
	ctx := context.Background()
	fB, err := a.files.CreateDoc(ctx, service.SystemOwnerID, service.SystemHomeSpaceID,
		service.BlogDirID, "B站收录.md", "正文", "site-b")
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	// 默认站：真实走一次引用式收录（accepted）；B 站：直接落一条融合草稿
	cid := addComment(t, a, fileDefault, "跨文章列表用评论", "读者乙")
	if w := postJSON(t, a, a.ingestQuote, "/api/v1/ingests/quote",
		`{"file_id":"`+fileDefault+`","comment_id":"`+cid+`"}`); w.Code != http.StatusOK {
		t.Fatalf("收录应 200，got %d %s", w.Code, w.Body.String())
	}
	if _, err := a.db.Exec(
		`INSERT INTO ingest_groups (id, file_id, mode, source, status, draft, actor_id, created_at, updated_at)
		 VALUES (?,?,'fuse','blog','draft','B站草稿',?,?,?)`,
		newID(), fB.ID, service.SystemOwnerID, 2, 2); err != nil {
		t.Fatalf("insert draft: %v", err)
	}

	type row struct {
		Status string `json:"status"`
		Mode   string `json:"mode"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		Draft  string `json:"draft"`
	}
	list := func(site string) []row {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/ingests", nil)
		req = req.WithContext(context.WithValue(req.Context(), ctxSiteID, site))
		w := httptest.NewRecorder()
		a.ingestList(w, asOwner(a, req))
		if w.Code != http.StatusOK {
			t.Fatalf("list(%s) 应 200，got %d %s", site, w.Code, w.Body.String())
		}
		var out struct {
			Total int   `json:"total"`
			Items []row `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("解析: %v", err)
		}
		if out.Total != len(out.Items) {
			t.Fatalf("total(%d) 与 items(%d) 不一致", out.Total, len(out.Items))
		}
		return out.Items
	}

	items := list(service.DefaultSiteID)
	if len(items) != 1 {
		t.Fatalf("默认站跨文章列表应 1 条（不得含 B 站记录），got %d %+v", len(items), items)
	}
	if items[0].Title != "收录测试.md" {
		t.Fatalf("应补出文章名，got %q", items[0].Title)
	}
	if items[0].Status != ingestStatusAccepted || items[0].Body != "" {
		t.Fatalf("accepted 组应隐藏 body，got status=%s body=%q", items[0].Status, items[0].Body)
	}

	itemsB := list("site-b")
	if len(itemsB) != 1 {
		t.Fatalf("B 站应 1 条，got %d %+v", len(itemsB), itemsB)
	}
	if itemsB[0].Title != "B站收录.md" {
		t.Fatalf("B 站应补出文章名，got %q", itemsB[0].Title)
	}
	if itemsB[0].Draft != "B站草稿" {
		t.Fatalf("草稿原文应可见（前端「待采纳」要展示），got %q", itemsB[0].Draft)
	}

	if itemsC := list("site-c"); len(itemsC) != 0 {
		t.Fatalf("无记录的站点应 0 条（不得跨站串数据），got %d %+v", len(itemsC), itemsC)
	}
}
