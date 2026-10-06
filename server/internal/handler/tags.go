package handler

import (
	"encoding/json"
	"net/http"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// tagsList: GET /api/v1/tags 标签树（带文件计数）。
func (a *API) tagsList(w http.ResponseWriter, r *http.Request) {
	items, err := a.tags.List(r.Context(), a.homeOwnerID())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "TAGS_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// tagsCreate: POST /api/v1/tags {name, parent_id?}
func (a *API) tagsCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	t, err := a.tags.Create(r.Context(), a.homeOwnerID(), req.Name, req.ParentID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "TAG_CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// tagsRename: PUT /api/v1/tags/{id} {name}
func (a *API) tagsRename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	t, err := a.tags.Rename(r.Context(), a.homeOwnerID(), id, req.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "TAG_RENAME_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// tagsDelete: DELETE /api/v1/tags/{id}
func (a *API) tagsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.tags.Delete(r.Context(), a.homeOwnerID(), id); err != nil {
		writeErr(w, http.StatusBadRequest, "TAG_DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// fileTagsSet: PUT /api/v1/files/{id}/tags {tag_ids: []string}
func (a *API) fileTagsSet(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		TagIDs []string `json:"tag_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	if err := a.tags.SetFileTags(r.Context(), a.homeOwnerID(), id, req.TagIDs); err != nil {
		writeErr(w, http.StatusBadRequest, "FILE_TAGS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// fileTagsGet: GET /api/v1/files/{id}/tags
func (a *API) fileTagsGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, err := a.tags.FileTags(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FILE_TAGS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// listFilesWithTags 组装文件列表（附加每文件的标签，供列表徽章/筛选）。
func (a *API) listFilesWithTags(w http.ResponseWriter, r *http.Request, items []*service.File) {
	tagMap, err := a.tags.TagsForFiles(r.Context(), fileIDs(items))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "TAGS_FOR_FILES_FAILED", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, f := range items {
		m := map[string]any{
			"id":            f.ID,
			"space_id":      f.SpaceID,
			"owner_id":      f.OwnerID,
			"parent_id":     f.ParentID,
			"name":          f.Name,
			"kind":          f.Kind,
			"mime":          f.Mime,
			"size":          f.Size,
			"sha256":        f.SHA256,
			"storage_ref":   f.StorageRef,
			"version":       f.Version,
			"content_state": f.ContentState,
			"created_at":    f.CreatedAt,
			"updated_at":    f.UpdatedAt,
			"tags":          tagMap[f.ID],
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func fileIDs(items []*service.File) []string {
	ids := make([]string, 0, len(items))
	for _, f := range items {
		ids = append(ids, f.ID)
	}
	return ids
}

// ruleTagsApply: POST /api/v1/admin/tags/apply-rules
// 对 home 空间存量文件批量补打规则标签（扩展名/文件名关键词，L0 层）。
// 幂等：已挂的标签合并保留；新上传的文件由事件订阅即时打标，此接口仅用于存量补打。
func (a *API) ruleTagsApply(w http.ResponseWriter, r *http.Request) {
	spaceID := a.homeSpaceID()
	owner := a.homeOwnerID()
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, name FROM files WHERE space_id=? AND kind='file' AND deleted_at IS NULL`, spaceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "RULES_SCAN_FAILED", err.Error())
		return
	}
	defer rows.Close()
	scanned, applied := 0, 0
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			break
		}
		scanned++
		if paths := a.tags.ApplyRuleTags(r.Context(), owner, id, name); len(paths) > 0 {
			applied++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"scanned": scanned, "applied": applied})
}
