// subscription.go 内容订阅 HTTP 入口（B6 订阅与通知）。
//
//	GET    /api/v1/subscriptions                                  我订阅的全部目标
//	GET    /api/v1/subscriptions/status?target_type=&target_id=    是否已订阅（含订阅总数）
//	POST   /api/v1/subscriptions                                  订阅 {"target_type":"file|dir","target_id":"..."}
//	DELETE /api/v1/subscriptions/{target_type}/{target_id}         退订
//
// 鉴权：user_id **一律取会话**，不接受请求体传入 —— 否则可替他人订阅/退订。
package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// subItem 订阅项（附带目标名称与 kind，前端可直接渲染，免二次查询）。
type subItem struct {
	service.DocSubscription
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// subscriptionsList 我订阅的全部目标（新→旧），批量解析目标名称。
func (a *API) subscriptionsList(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	rows, err := a.subs.ListByUser(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SUB_LIST_FAILED", err.Error())
		return
	}
	items := make([]subItem, 0, len(rows))
	for _, d := range rows {
		items = append(items, subItem{DocSubscription: d})
	}
	if len(items) > 0 {
		names := a.subTargetMeta(r, items)
		for i := range items {
			if m, ok := names[items[i].TargetID]; ok {
				items[i].Name, items[i].Kind = m[0], m[1]
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

// subTargetMeta 批量取目标 id → [name, kind]（一次查询，避免 N+1）。
func (a *API) subTargetMeta(r *http.Request, items []subItem) map[string][2]string {
	out := map[string][2]string{}
	ids := make([]any, 0, len(items))
	ph := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.TargetID)
		ph = append(ph, "?")
	}
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, COALESCE(name,''), COALESCE(kind,'') FROM files WHERE id IN (`+strings.Join(ph, ",")+`)`,
		ids...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, kind string
		if err := rows.Scan(&id, &name, &kind); err != nil {
			return out
		}
		out[id] = [2]string{name, kind}
	}
	return out
}

// subscriptionsStatus 是否已订阅某目标；顺带返回订阅总数（前端角标用）。
func (a *API) subscriptionsStatus(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	ok, err := a.subs.IsSubscribed(r.Context(), uid,
		r.URL.Query().Get("target_type"), r.URL.Query().Get("target_id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "SUB_BAD_TARGET", err.Error())
		return
	}
	cnt, _ := a.subs.CountByUser(r.Context(), uid)
	writeJSON(w, http.StatusOK, map[string]any{"subscribed": ok, "count": cnt})
}

// subscriptionsCreate 订阅（幂等：重复订阅不报错，返回 created=false）。
func (a *API) subscriptionsCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "请求体格式错误")
		return
	}
	created, err := a.subs.Subscribe(r.Context(), a.curUserID(r), body.TargetType, body.TargetID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "SUB_BAD_TARGET", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "created": created, "subscribed": true})
}

// subscriptionsDelete 退订（幂等：未订阅过也返回 200，removed=false）。
func (a *API) subscriptionsDelete(w http.ResponseWriter, r *http.Request) {
	removed, err := a.subs.Unsubscribe(r.Context(), a.curUserID(r),
		r.PathValue("target_type"), r.PathValue("target_id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "SUB_BAD_TARGET", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": removed, "subscribed": false})
}

// afterContentChange 内容保存后的增值动作（B6）：投订阅通知 + 落 @提及。
//
// 契约：**绝不返回错误、绝不阻断保存** —— 订阅通知与提及都是增值能力，
// 任何一步失败只少一条记录，不能让"保存文章"这件事受它们影响。
func (a *API) afterContentChange(r *http.Request, fileID, editorID, content string, f *service.File) {
	ctx := r.Context()
	title := "订阅的内容有更新"
	link := "/read/" + fileID
	if f != nil {
		if f.Name != "" {
			title = "《" + f.Name + "》已更新"
		}
		if f.Slug != "" {
			link = "/post/" + f.Slug
		}
	}
	// 1) 订阅通知：同一 (订阅者,文件) 在 10 分钟窗口内只发一次，防自动保存刷屏。
	_ = a.subs.DispatchFileChange(ctx, a.notify, service.FileChangeNotice{
		FileID:      fileID,
		EditorID:    editorID,
		Title:       title,
		Link:        link,
		DebounceSec: 600,
	})
	// 2) @提及：落库幂等（同一条内容重复保存不会重复通知）。
	_ = a.mentions.Record(ctx, a.notify, service.MentionInput{
		SourceType:  service.MentionSourcePost,
		SourceID:    fileID,
		MentionerID: editorID,
		Text:        content,
		Title:       title,
		Link:        link,
	})
}

// afterCommentCreated 评论落库后的增值动作（B6）：@提及落库 + 回复通知父楼作者。
// 与 afterContentChange 同样的**不阻断**契约。访客评论走待审，审核通过时才会调到这里。
func (a *API) afterCommentCreated(r *http.Request, commentID, fileID, authorID, body, parentID string) {
	ctx := r.Context()
	link := commentLink(fileID, commentID)
	// 1) @提及：落库幂等（同一条评论重复处理不会重复通知）
	_ = a.mentions.Record(ctx, a.notify, service.MentionInput{
		SourceType:  service.MentionSourceComment,
		SourceID:    commentID,
		MentionerID: authorID,
		Text:        body,
		Title:       "有人在评论里提到了你",
		Link:        link,
	})
	// 2) 回复通知父楼作者（自己回复自己不通知）
	if strings.TrimSpace(parentID) == "" {
		return
	}
	var puid string
	if err := a.db.QueryRowContext(ctx,
		`SELECT COALESCE(user_id,'') FROM comments WHERE id=?`, parentID).Scan(&puid); err != nil || puid == "" {
		return
	}
	if puid == authorID {
		return
	}
	_ = a.notify.AddUser(ctx, puid, "comment", map[string]any{
		"title":   "有人回复了你的评论",
		"message": truncateRunes(body, 80),
		"link":    link,
		"extra":   map[string]any{"file_id": fileID, "comment_id": commentID},
	})
}

// commentLink 生成评论的站内跳转（SPA 阅读路由 + 评论锚点，与主题侧 id="comment-{id}" 契约一致）。
func commentLink(fileID, commentID string) string {
	link := "/read/" + fileID
	if commentID != "" {
		link += "#comment-" + commentID
	}
	return link
}

// truncateRunes 按「字符」截断（不是字节），避免中文被切成半个。
func truncateRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}
