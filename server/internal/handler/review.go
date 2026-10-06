package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// ---- A3 SRS 主动复习：队列 / 评分 / 统计 / 入队 / 暂停恢复 ----

// reviewQueue GET /api/v1/review/queue?limit=N
// 返回今日待复习条目（due_at<=now 或首次未排期），按到期时间升序。
func (a *API) reviewQueue(w http.ResponseWriter, r *http.Request) {
	if a.review == nil {
		writeErr(w, http.StatusNotFound, "REVIEW_DISABLED", "复习模块未启用")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := a.review.Queue(r.Context(), a.curUserID(r), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "REVIEW_QUEUE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// reviewRate POST /api/v1/review/rate {"file_id":"...","rating":0-3}
// 提交复习反馈：0=完全忘了 1=有点难 2=刚好 3=太简单。推进间隔并写轨迹。
func (a *API) reviewRate(w http.ResponseWriter, r *http.Request) {
	if a.review == nil {
		writeErr(w, http.StatusNotFound, "REVIEW_DISABLED", "复习模块未启用")
		return
	}
	var req struct {
		FileID string `json:"file_id"`
		Rating int    `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "参数缺失：file_id 必填")
		return
	}
	item, err := a.review.Rate(r.Context(), a.curUserID(r), req.FileID, req.Rating)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "REVIEW_RATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "item": item})
}

// reviewStats GET /api/v1/review/stats
func (a *API) reviewStats(w http.ResponseWriter, r *http.Request) {
	if a.review == nil {
		writeErr(w, http.StatusNotFound, "REVIEW_DISABLED", "复习模块未启用")
		return
	}
	s, err := a.review.Stats(r.Context(), a.curUserID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "REVIEW_STATS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": s})
}

// reviewEnroll POST /api/v1/review/enroll {"file_id":"..."}
// 手动将文件加入复习队列（幂等）。
func (a *API) reviewEnroll(w http.ResponseWriter, r *http.Request) {
	if a.review == nil {
		writeErr(w, http.StatusNotFound, "REVIEW_DISABLED", "复习模块未启用")
		return
	}
	var req struct {
		FileID string `json:"file_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "参数缺失：file_id 必填")
		return
	}
	if err := a.review.Enroll(r.Context(), a.curUserID(r), req.FileID); err != nil {
		writeErr(w, http.StatusInternalServerError, "REVIEW_ENROLL_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// reviewStatus POST /api/v1/review/status {"file_id":"...","status":"active|paused"}
func (a *API) reviewStatus(w http.ResponseWriter, r *http.Request) {
	if a.review == nil {
		writeErr(w, http.StatusNotFound, "REVIEW_DISABLED", "复习模块未启用")
		return
	}
	var req struct {
		FileID string `json:"file_id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "参数缺失：file_id 必填")
		return
	}
	if err := a.review.SetStatus(r.Context(), a.curUserID(r), req.FileID, req.Status); err != nil {
		writeErr(w, http.StatusInternalServerError, "REVIEW_STATUS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
