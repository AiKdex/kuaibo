// Package handler 提供 HTTP/WS 入口。
// kb.go 实现知识库聚合层 API（设计需求文档 A05/A06/A09）：
// 总览统计 / 标签聚合 / 概念页 / 图谱 / 集合 CRUD。
package handler

import (
	"encoding/json"
	"net/http"
)

// ---- 知识库聚合 API ----

// kbOverview GET /api/v1/kb/overview
func (a *API) kbOverview(w http.ResponseWriter, r *http.Request) {
	o, err := a.kb.Overview(r.Context(), a.homeOwnerID(), a.homeSpaceID())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "KB_OVERVIEW_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// kbSummariesRetry POST /api/v1/kb/summaries/retry —— 重试失败解读（error → pending 重新入队）
func (a *API) kbSummariesRetry(w http.ResponseWriter, r *http.Request) {
	if a.summarizer == nil {
		writeErr(w, http.StatusBadRequest, "SUMMARIZER_OFF", "解读队列未启用")
		return
	}
	n, retried, err := a.summarizer.RetryErrors(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "KB_SUMMARY_RETRY_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleaned": n, "retried": retried})
}

// kbSummariesRequeue POST /api/v1/kb/summaries/requeue —— 精读策略：批量把未解读/失败文件重排进解读队列，
// 支持按扩展名筛选（body: {"exts":["md","pdf","doc"]}，空=全部类型）。已解读文件不重跑（不浪费额度）。
func (a *API) kbSummariesRequeue(w http.ResponseWriter, r *http.Request) {
	if a.summarizer == nil {
		writeErr(w, http.StatusBadRequest, "SUMMARIZER_OFF", "解读队列未启用")
		return
	}
	var body struct {
		Exts []string `json:"exts"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	n, err := a.summarizer.RequeueSpaceByExts(r.Context(), a.homeSpaceID(), body.Exts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "KB_SUMMARY_REQUEUE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queued": n, "exts": body.Exts})
}

// kbTags GET /api/v1/kb/tags —— 标签聚合（标签 + 文件 + 摘要）
func (a *API) kbTags(w http.ResponseWriter, r *http.Request) {
	items, err := a.kb.TagsAggregate(r.Context(), a.homeSpaceID())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "KB_TAGS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// kbConcept GET /api/v1/kb/concepts/{id} —— 概念页
func (a *API) kbConcept(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := a.kb.Concept(r.Context(), a.homeSpaceID(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "KB_CONCEPT_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// kbLint GET /api/v1/kb/lint —— 知识质量 lint（A08：孤儿/空标签/未解读/重复/断链/解读失败）
func (a *API) kbLint(w http.ResponseWriter, r *http.Request) {
	rep, err := a.kb.Lint(r.Context(), a.homeOwnerID(), a.homeSpaceID())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "KB_LINT_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// kbDedupResolve POST /api/v1/kb/dedup/resolve —— 重复文件处理（软删，可恢复）
// body: {"sha256":"...", "keep_file_id":"...", "delete_file_ids":["..."]}
// 安全：delete_file_ids 必须与 keep 同 sha256（后端再验），防止误删非重复文件。
func (a *API) kbDedupResolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sha256       string   `json:"sha256"`
		KeepFileID   string   `json:"keep_file_id"`
		DeleteFileIDs []string `json:"delete_file_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Sha256 == "" || req.KeepFileID == "" || len(req.DeleteFileIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "BAD_REQUEST", "sha256/keep_file_id/delete_file_ids required")
		return
	}
	ctx := r.Context()
	ownerID := a.homeOwnerID()
	deleted := 0
	skipped := []string{}
	for _, id := range req.DeleteFileIDs {
		if id == req.KeepFileID {
			continue
		}
		f, err := a.files.Get(ctx, id)
		if err != nil || f.SHA256 != req.Sha256 {
			skipped = append(skipped, id)
			continue
		}
		if err := a.files.Delete(ctx, ownerID, id); err != nil {
			skipped = append(skipped, id)
			continue
		}
		deleted++
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "skipped": skipped})
}

// kbGraph GET /api/v1/kb/graph —— 图谱（文件-标签二部图 + 知识边）
func (a *API) kbGraph(w http.ResponseWriter, r *http.Request) {
	g, err := a.kb.Graph(r.Context(), a.homeSpaceID())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "KB_GRAPH_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, g)
}

// ---- 集合 API ----

// collectionsList GET /api/v1/collections
func (a *API) collectionsList(w http.ResponseWriter, r *http.Request) {
	items, err := a.kb.ListCollections(r.Context(), a.homeSpaceID())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "COLLECTIONS_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// collectionsCreate POST /api/v1/collections {name, kind, query}
func (a *API) collectionsCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Kind  string `json:"kind"`
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	c, err := a.kb.CreateCollection(r.Context(), a.homeOwnerID(), a.homeSpaceID(), req.Name, req.Kind, req.Query)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "COLLECTION_CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// collectionsDelete DELETE /api/v1/collections/{id}
func (a *API) collectionsDelete(w http.ResponseWriter, r *http.Request) {
	if err := a.kb.DeleteCollection(r.Context(), a.homeSpaceID(), r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, "COLLECTION_DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// collectionsFiles GET /api/v1/collections/{id}/files （smart 集合动态匹配 / manual 关联表）
func (a *API) collectionsFiles(w http.ResponseWriter, r *http.Request) {
	items, err := a.kb.CollectionFilesByKind(r.Context(), a.homeSpaceID(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "COLLECTION_FILES_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// collectionsAddFile POST /api/v1/collections/{id}/files {file_id}
func (a *API) collectionsAddFile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileID string `json:"file_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQUEST", "file_id required")
		return
	}
	if err := a.kb.AddToCollection(r.Context(), a.homeSpaceID(), r.PathValue("id"), req.FileID); err != nil {
		writeErr(w, http.StatusBadRequest, "COLLECTION_ADD_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// collectionsRemoveFile DELETE /api/v1/collections/{id}/files/{fileId}
func (a *API) collectionsRemoveFile(w http.ResponseWriter, r *http.Request) {
	if err := a.kb.RemoveFromCollection(r.Context(), a.homeSpaceID(), r.PathValue("id"), r.PathValue("fileId")); err != nil {
		writeErr(w, http.StatusBadRequest, "COLLECTION_REMOVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
