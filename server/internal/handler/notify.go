// notify.go 消息通知中心 HTTP 入口。
//
//	GET  /api/v1/notifications?limit=50&unread=1   通知列表 + 未读数（新→旧）
//	POST /api/v1/notifications/read                标记已读：{"id": "..."} 或 {"all": true}
//	GET  /api/v1/notifications/unread-count        轻量未读数（前端轮询）
//
// 写入侧：bus 订阅（impex.*）+ Collector.OnFinish（采集终态）+ 订阅/提及 统一落 NotifyStore。
// 读侧多用户隔离：只返回当前用户的通知；管理员（owner/admin）额外可见站点级通知。
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// notifications 通知列表。
func (a *API) notifications(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	unreadOnly := r.URL.Query().Get("unread") == "1"
	items, unread, err := a.notify.List(r.Context(), a.curUserID(r), a.isAdmin(r), limit, unreadOnly)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "NOTIFY_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "unread_count": unread})
}

// notificationsRead 标记已读（单条或全部）。
func (a *API) notificationsRead(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID  string `json:"id"`
		All bool   `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "请求体格式错误")
		return
	}
	if body.All {
		if err := a.notify.MarkAllRead(r.Context(), a.curUserID(r), a.isAdmin(r)); err != nil {
			writeErr(w, http.StatusInternalServerError, "NOTIFY_READ_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if body.ID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "需要 id 或 all=true")
		return
	}
	if err := a.notify.MarkRead(r.Context(), a.curUserID(r), a.isAdmin(r), body.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "NOTIFY_READ_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// notificationsUnread 未读数。
func (a *API) notificationsUnread(w http.ResponseWriter, r *http.Request) {
	n, err := a.notify.UnreadCount(r.Context(), a.curUserID(r), a.isAdmin(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "NOTIFY_COUNT_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": n})
}
