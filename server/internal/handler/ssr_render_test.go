// ssr_render_test.go SSR 渲染层回归（B12）：正文 @提及 渲染 + 文章页评论锚点闭合。
//
// 覆盖点：
//  1. @提及 渲染：显式写法不再渲染成「指向 user_id 的死链」；纯写法的前界检查（邮箱不误伤）；
//     代码块/行内代码里的 @ 天然不渲染（goldmark 代码段不进行内联解析，故这条是"钉住别退化"）。
//  2. 文章页评论：只取 approved、按时间正序、访客昵称 > display_name > 角色兜底、跨文章隔离。
//  3. 内置 SSR 模板：评论带 id="comment-{id}"（归因回指链接 {origin}/{slug}#comment-{id} 的落点），
//     且评论正文被转义（评论是访客可写内容，SSR 侧必须转义）。
package handler

import (
	"bytes"
	"context"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// mdRender 用与生产同一个 mdGold 渲染（已挂 @提及 扩展），避免测试与线上两套渲染器。
func mdRender(t *testing.T, src string) string {
	t.Helper()
	var b strings.Builder
	if err := mdGold.Convert([]byte(src), &b); err != nil {
		t.Fatalf("md convert: %v", err)
	}
	return b.String()
}

func TestMDMentionExplicitRendersSpanNotDeadLink(t *testing.T) {
	out := mdRender(t, "你好 @[张三](abcd1234-1234-1234-1234-1234567890ab) 请查收。")
	want := `<span class="mention" data-mention-id="abcd1234-1234-1234-1234-1234567890ab">@张三</span>`
	if !strings.Contains(out, want) {
		t.Fatalf("显式提及未渲染为 span：\n%s", out)
	}
	// 回归：扩展接入前，`@[名](id)` 是合法链接语法 → 渲染成 <a href="user_id">，即一条死链。
	if strings.Contains(out, `<a href="abcd1234`) {
		t.Fatalf("显式提及仍被当成链接（死链回归）：\n%s", out)
	}
}

func TestMDMentionPlainRendersSpan(t *testing.T) {
	out := mdRender(t, "谢谢 @lisi 和 @张三。")
	for _, want := range []string{`<span class="mention">@lisi</span>`, `<span class="mention">@张三</span>`} {
		if !strings.Contains(out, want) {
			t.Fatalf("纯写法提及未渲染：want %s\n实际：%s", want, out)
		}
	}
}

func TestMDMentionPlainSkipsEmailAndCode(t *testing.T) {
	out := mdRender(t, "联系 someone@example.com 或看 `@lisi` 这一段。")
	if strings.Contains(out, `class="mention"`) {
		t.Fatalf("邮箱/行内代码里的 @ 被误渲染成提及：\n%s", out)
	}
	if !strings.Contains(out, "someone@example.com") {
		t.Fatalf("邮箱原文被破坏：\n%s", out)
	}
}

func TestMDMentionInCodeFenceUntouched(t *testing.T) {
	out := mdRender(t, "```\n@media (max-width:600px){color:red}\n```\n")
	if strings.Contains(out, `class="mention"`) {
		t.Fatalf("代码块里的 @ 被误渲染：\n%s", out)
	}
}

// addCommentRow 落一条指定状态/时间/作者的评论（自带 created_at，便于断言排序）。
// userID 传空 = 访客评论（guest_name 兜底展示）。
func addCommentRow(t *testing.T, a *API, fileID, body, guest, userID, status string, createdAt int64) string {
	t.Helper()
	id := newID()
	var uid any
	if userID != "" {
		uid = userID
	}
	if _, err := a.db.Exec(
		`INSERT INTO comments (id, file_id, user_id, body, status, guest_name, created_at) VALUES (?,?,?,?,?,?,?)`,
		id, fileID, uid, body, status, guest, createdAt); err != nil {
		t.Fatalf("insert comment: %v", err)
	}
	return id
}

func TestBlogPostCommentsOnlyApprovedInOrder(t *testing.T) {
	a, fileID, _ := newIngestAPI(t)
	ctx := context.Background()

	c1 := addCommentRow(t, a, fileID, "第一条，\n带换行", "读者甲", "", "approved", 1000)
	c2 := addCommentRow(t, a, fileID, "第二条", "", "", "approved", 2000)
	_ = addCommentRow(t, a, fileID, "待审的", "待审乙", "", "pending", 1500)

	// 登录用户评论：作者名读库取期望值（避免把种子数据的 display_name 写死在断言里）。
	owner := service.SystemOwnerID
	var ownerName string
	if err := a.db.QueryRow(`SELECT COALESCE(display_name,'') FROM users WHERE id=?`, owner).Scan(&ownerName); err != nil {
		t.Fatalf("读 owner: %v", err)
	}
	if ownerName == "" {
		ownerName = "管理员" // 与 blogPostComments 的角色兜底一致
	}
	c3 := addCommentRow(t, a, fileID, "登录者说", "", owner, "approved", 3000)

	// 同库另一篇文章：用于验证按 file_id 隔离（不能串评论）。
	other, err := a.files.CreateDoc(ctx, service.SystemOwnerID, service.SystemHomeSpaceID,
		service.BlogDirID, "另一篇.md", "# 另一篇\n\n正文", service.DefaultSiteID)
	if err != nil {
		t.Fatalf("create other doc: %v", err)
	}
	_ = addCommentRow(t, a, other.ID, "另一篇的评论", "路人", "", "approved", 4000)

	req := httptest.NewRequest("GET", "/x", nil)
	got := a.blogPostComments(req, fileID)
	if len(got) != 3 {
		t.Fatalf("应只返回 3 条已通过评论，得到 %d", len(got))
	}
	if got[0].ID != c1 || got[1].ID != c2 || got[2].ID != c3 {
		t.Fatalf("评论未按时间正序：%s / %s / %s", got[0].ID, got[1].ID, got[2].ID)
	}
	if got[0].Author != "读者甲" {
		t.Fatalf("访客昵称未优先采用：%q", got[0].Author)
	}
	if got[1].Author != "读者" {
		t.Fatalf("无昵称、无用户行的评论作者兜底应为「读者」：%q", got[1].Author)
	}
	if got[2].Author != ownerName {
		t.Fatalf("登录用户评论作者应为 %q，得到 %q", ownerName, got[2].Author)
	}
	if !strings.Contains(got[0].Body, "\n") {
		t.Fatalf("评论换行未保留：%q", got[0].Body)
	}
	if got[2].Date == "" {
		t.Fatalf("评论日期为空")
	}
	if n := len(a.blogPostComments(req, other.ID)); n != 1 {
		t.Fatalf("按文章隔离失效：另一篇应 1 条，得到 %d", n)
	}
	if n := len(a.blogPostComments(req, "")); n != 0 {
		t.Fatalf("file_id 为空应返回空，得到 %d", n)
	}
}

func TestSSRPostTemplateRendersCommentsWithAnchor(t *testing.T) {
	data := blogHTMLData{
		SiteName: "测试站", Title: "文章标题", IsList: false,
		BodyHTML: template.HTML("<p>正文</p>"),
		Comments: []blogHTMLComment{
			{ID: "c1", Author: "读者甲", Date: "2026-09-22", Body: "第一行\n第二行"},
			{ID: "c2", Author: "读者乙", Date: "2026-09-23", Body: "<script>alert(1)</script>"},
		},
	}
	var buf bytes.Buffer
	if err := blogHTMLTmpl.Execute(&buf, data); err != nil {
		t.Fatalf("模板渲染失败：%v", err)
	}
	out := buf.String()
	for _, want := range []string{`id="comment-c1"`, `id="comment-c2"`, `id="comments"`, `<span class="cmts-n">2</span>`} {
		if !strings.Contains(out, want) {
			t.Fatalf("SSR 评论锚点/计数缺失：want %s\n%s", want, out)
		}
	}
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Fatalf("评论正文未转义（XSS）：\n%s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("评论正文应以转义形式展示：\n%s", out)
	}

	// 无评论：不渲染列表，给引导文案（保证空评论区不出现空壳标题）
	buf.Reset()
	data.Comments = nil
	if err := blogHTMLTmpl.Execute(&buf, data); err != nil {
		t.Fatalf("模板渲染失败：%v", err)
	}
	if !strings.Contains(buf.String(), `class="cmts-e"`) {
		t.Fatalf("空评论区未给引导文案：\n%s", buf.String())
	}
}
