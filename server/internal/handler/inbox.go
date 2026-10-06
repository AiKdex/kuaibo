package handler

import (
	"net/http"
	"strconv"
)

// ---- P1-3 收件箱：采集内容聚合（未读/归档） ----

// inboxList GET /api/v1/inbox?filter=unread|all&limit=N
func (a *API) inboxList(w http.ResponseWriter, r *http.Request) {
	spaceID := a.homeSpaceID()
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	filter := r.URL.Query().Get("filter")
	if filter != "unread" && filter != "all" {
		filter = "all"
	}
	items, err := a.inbox.List(r.Context(), spaceID, filter, limit, a.cfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INBOX_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// inboxUnread GET /api/v1/inbox/unread-count
func (a *API) inboxUnread(w http.ResponseWriter, r *http.Request) {
	n, err := a.inbox.UnreadCount(r.Context(), a.homeSpaceID(), a.cfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INBOX_UNREAD_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": n})
}

// inboxArchive POST /api/v1/inbox/{id}/archive
func (a *API) inboxArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.inbox.Archive(r.Context(), id, a.homeSpaceID()); err != nil {
		writeErr(w, http.StatusBadRequest, "INBOX_ARCHIVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// inboxArchiveAll POST /api/v1/inbox/archive-all
func (a *API) inboxArchiveAll(w http.ResponseWriter, r *http.Request) {
	n, err := a.inbox.ArchiveAll(r.Context(), a.homeSpaceID(), a.cfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INBOX_ARCHIVE_ALL_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "archived": n})
}
