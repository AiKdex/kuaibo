// mention.go @提及 HTTP 入口（B6）。
//
//	GET  /api/v1/mentions               我被提及的（新→旧，附来源名称与可点击跳转）
//	GET  /api/v1/mentions/unread-count  未读数（铃铛轮询用）
//	POST /api/v1/mentions/read          全部标记已读
//
// 写入侧不在此处：评论创建/审核（commentsCreate / blogCommentApprove）与文章内容更新
// 统一调 service.MentionStore.Record，避免多个入口各写一份解析逻辑。
package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// mentionItem 提及项：在库记录之外补上「谁提的 / 在哪提的 / 点哪儿」，
// 由后端一次批量解析（3 条 IN 查询），前端拿到即可渲染，不必自己再查。
type mentionItem struct {
	service.Mention
	Link       string `json:"link"`
	SourceName string `json:"source_name"`
	FromName   string `json:"from_name"`
}

// mentionsList 我被提及的记录。
func (a *API) mentionsList(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := a.mentions.ListByUser(r.Context(), uid, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "MENTION_LIST_FAILED", err.Error())
		return
	}
	items := make([]mentionItem, 0, len(rows))
	for _, m := range rows {
		items = append(items, mentionItem{Mention: m})
	}
	if len(items) > 0 {
		a.fillMentionMeta(r, items)
	}
	unread, _ := a.mentions.UnreadCount(r.Context(), uid)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "unread_count": unread})
}

// fillMentionMeta 批量补齐 from_name / source_name / link（避免 N+1）。
func (a *API) fillMentionMeta(r *http.Request, items []mentionItem) {
	ctx := r.Context()
	userIDs := map[string]bool{}
	commentIDs := []string{}
	fileIDs := map[string]bool{}
	for _, it := range items {
		if it.MentionerID != "" {
			userIDs[it.MentionerID] = true
		}
		if it.SourceType == service.MentionSourceComment {
			commentIDs = append(commentIDs, it.SourceID)
		} else if it.SourceID != "" {
			fileIDs[it.SourceID] = true
		}
	}

	// 评论 → 所属文件（评论锚点链接需要 file_id）
	commentFile := map[string]string{}
	if len(commentIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(commentIDs)), ",")
		args := make([]any, len(commentIDs))
		for i, v := range commentIDs {
			args[i] = v
		}
		if rows, err := a.db.QueryContext(ctx,
			`SELECT id, file_id FROM comments WHERE id IN (`+ph+`)`, args...); err == nil {
			for rows.Next() {
				var cid, fid string
				if rows.Scan(&cid, &fid) == nil {
					commentFile[cid] = fid
					fileIDs[fid] = true
				}
			}
			rows.Close()
		}
	}

	// 文件 → 名称
	fileNames := map[string]string{}
	if len(fileIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(fileIDs)), ",")
		args := make([]any, 0, len(fileIDs))
		for id := range fileIDs {
			args = append(args, id)
		}
		if rows, err := a.db.QueryContext(ctx,
			`SELECT id, COALESCE(name,'') FROM files WHERE id IN (`+ph+`)`, args...); err == nil {
			for rows.Next() {
				var id, name string
				if rows.Scan(&id, &name) == nil {
					fileNames[id] = name
				}
			}
			rows.Close()
		}
	}

	// 用户 → 显示名（回落 username）
	userNames := map[string]string{}
	if len(userIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
		args := make([]any, 0, len(userIDs))
		for id := range userIDs {
			args = append(args, id)
		}
		if rows, err := a.db.QueryContext(ctx,
			`SELECT id, COALESCE(NULLIF(display_name,''), username) FROM users WHERE id IN (`+ph+`)`, args...); err == nil {
			for rows.Next() {
				var id, name string
				if rows.Scan(&id, &name) == nil {
					userNames[id] = name
				}
			}
			rows.Close()
		}
	}

	for i := range items {
		fid := items[i].SourceID
		if items[i].SourceType == service.MentionSourceComment {
			fid = commentFile[items[i].SourceID]
		}
		items[i].SourceName = fileNames[fid]
		items[i].FromName = userNames[items[i].MentionerID]
		if fid != "" {
			items[i].Link = commentLink(fid, "")
			if items[i].SourceType == service.MentionSourceComment {
				items[i].Link = commentLink(fid, items[i].SourceID)
			}
		}
	}
}

// mentionsUnread 未读数。
func (a *API) mentionsUnread(w http.ResponseWriter, r *http.Request) {
	n, err := a.mentions.UnreadCount(r.Context(), a.curUserID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "MENTION_COUNT_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": n})
}

// mentionsRead 全部标记已读（提及粒度就是「我的全部」；单条已读由通知中心承担）。
func (a *API) mentionsRead(w http.ResponseWriter, r *http.Request) {
	if err := a.mentions.MarkAllRead(r.Context(), a.curUserID(r)); err != nil {
		writeErr(w, http.StatusInternalServerError, "MENTION_READ_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
