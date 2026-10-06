// conversations.go 实现 AI 对话会话管理：GET/POST/DELETE 会话 + GET 消息历史。
// 多话题模式：每个会话独立上下文；chat 请求带 conversation_id 续接，无则自动新建。
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/google/uuid"
)

// convInfo 会话列表项。
type convInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	MsgCount  int    `json:"msg_count"`
	UpdatedAt int64  `json:"updated_at"`
}

// convList GET /api/v1/ai/conversations：当前用户会话列表（按更新时间倒序）。
func (a *API) convList(w http.ResponseWriter, r *http.Request) {
	// M11 修复：归属改显式登录身份（服务令牌/IM/定时任务等内部场景由 authMiddleware
	// 注入 SystemOwnerID，curUserID 仍回落站长）——登录成员只看到自己的会话。
	owner := a.curUserID(r)
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT c.id, c.title, c.updated_at, COUNT(m.id)
		FROM ai_conversations c
		LEFT JOIN ai_messages m ON m.conversation_id = c.id
		WHERE c.owner_id = ?
		GROUP BY c.id
		ORDER BY c.updated_at DESC`, owner)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CONV_LIST_FAILED", err.Error())
		return
	}
	defer rows.Close()
	out := []convInfo{}
	for rows.Next() {
		var c convInfo
		if err := rows.Scan(&c.ID, &c.Title, &c.UpdatedAt, &c.MsgCount); err != nil {
			writeErr(w, http.StatusInternalServerError, "CONV_LIST_FAILED", err.Error())
			return
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

// convCreate POST /api/v1/ai/conversations：新建空会话。
func (a *API) convCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	title := req.Title
	if title == "" {
		title = "新对话"
	}
	id := uuid.NewString()
	now := time.Now().Unix()
	// M11 修复：归属记到显式登录身份（不再统一挂到站长 homeOwnerID）
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO ai_conversations (id, owner_id, title, created_at, updated_at) VALUES (?,?,?,?,?)`,
		id, a.curUserID(r), title, now, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "CONV_CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "title": title})
}

// convDelete DELETE /api/v1/ai/conversations/{id}：删除会话及其消息（应用层级联）。
func (a *API) convDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "CONV_NO_ID", "conversation id required")
		return
	}
	// M11 修复：归属校验改「显式登录身份本人或管理员」，不再与常量 homeOwnerID 比对
	var owner string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT owner_id FROM ai_conversations WHERE id=?`, id).Scan(&owner); err != nil {
		writeErr(w, http.StatusNotFound, "CONV_NOT_FOUND", "conversation not found")
		return
	}
	uid, ok := ctxUID(r)
	if !ok || (owner != uid && !a.isAdmin(r)) {
		writeErr(w, http.StatusForbidden, "CONV_FORBIDDEN", "not your conversation")
		return
	}
	// 事务级联删除：消息与会话要么都删、要么都不删
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CONV_DEL_FAILED", err.Error())
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM ai_messages WHERE conversation_id=?`, id); err != nil {
		_ = tx.Rollback()
		writeErr(w, http.StatusInternalServerError, "CONV_DEL_FAILED", err.Error())
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM ai_conversations WHERE id=?`, id); err != nil {
		_ = tx.Rollback()
		writeErr(w, http.StatusInternalServerError, "CONV_DEL_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, "CONV_DEL_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// convMessages GET /api/v1/ai/conversations/{id}/messages?limit=50&before=ts：会话历史消息（分页，最新在前）。
func (a *API) convMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// M11 修复：补归属校验——会话 owner 必须为当前显式登录用户，或当前用户为管理员
	//（此前按 id 直取消息，任意登录用户可越权读他人会话全文）。
	var owner string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT owner_id FROM ai_conversations WHERE id=?`, id).Scan(&owner); err != nil {
		writeErr(w, http.StatusNotFound, "CONV_NOT_FOUND", "conversation not found")
		return
	}
	uid, ok := ctxUID(r)
	if !ok || (owner != uid && !a.isAdmin(r)) {
		writeErr(w, http.StatusForbidden, "CONV_FORBIDDEN", "not your conversation")
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := atoiSafe(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	// before=游标（created_at），缺省取最新 limit 条；子查询取最新 N 条后反转回正序
	inner := `SELECT role, content, created_at, rowid FROM ai_messages WHERE conversation_id=? `
	args := []any{id}
	if before := r.URL.Query().Get("before"); before != "" {
		if ts, err := atoiSafe(before); err == nil {
			inner += `AND created_at < ? `
			args = append(args, ts)
		}
	}
	inner += `ORDER BY created_at DESC, rowid DESC LIMIT ?`
	args = append(args, limit)
	q := `SELECT role, content, created_at FROM (` + inner + `) ORDER BY created_at ASC, rowid ASC`
	rows, err := a.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CONV_MSGS_FAILED", err.Error())
		return
	}
	defer rows.Close()
	out := []ai.Msg{}
	for rows.Next() {
		var m ai.Msg
		var ts int64
		if err := rows.Scan(&m.Role, &m.Content, &ts); err != nil {
			writeErr(w, http.StatusInternalServerError, "CONV_MSGS_FAILED", err.Error())
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": out})
}
