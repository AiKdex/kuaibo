// kb_memory.go 空间级长期记忆端点（WeKnora 借鉴 A3，B27 自上游 55683ab 移植）：
//   GET    /api/v1/kb/memory?space=&kind=   列表
//   POST   /api/v1/kb/memory               写入/覆盖 {space_id, kind, key, content, source}
//   DELETE /api/v1/kb/memory?space=&key=   删除
//   GET    /api/v1/kb/memory/context?space= 汇总为注入上下文（Agent/云雇工开场合用）
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// kbMemoryList GET /api/v1/kb/memory
func (a *API) kbMemoryList(w http.ResponseWriter, r *http.Request) {
	space := r.URL.Query().Get("space")
	if space == "" {
		writeErr(w, http.StatusBadRequest, "SPACE_REQUIRED", "space 参数必填")
		return
	}
	items, err := a.kb.MemoryList(r.Context(), space, r.URL.Query().Get("kind"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "MEMORY_LIST_FAILED", err.Error())
		return
	}
	if items == nil {
		items = []*service.Memory{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": items})
}

// kbMemorySet POST /api/v1/kb/memory（幂等：同空间同 key 覆盖）
func (a *API) kbMemorySet(w http.ResponseWriter, r *http.Request) {
	var m service.Memory
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	m.CreatedBy = a.curUserID(r)
	if m.Source == "" {
		m.Source = "user"
	}
	if err := a.kb.MemorySet(r.Context(), &m); err != nil {
		writeErr(w, http.StatusBadRequest, "MEMORY_SET_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": m.Key, "space_id": m.SpaceID})
}

// kbMemoryDelete DELETE /api/v1/kb/memory?space=&key=
func (a *API) kbMemoryDelete(w http.ResponseWriter, r *http.Request) {
	space, key := r.URL.Query().Get("space"), r.URL.Query().Get("key")
	if space == "" || key == "" {
		writeErr(w, http.StatusBadRequest, "PARAM_REQUIRED", "space 与 key 必填")
		return
	}
	if err := a.kb.MemoryDelete(r.Context(), space, key); err != nil {
		writeErr(w, http.StatusBadRequest, "MEMORY_DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": key})
}

// kbMemoryContext GET /api/v1/kb/memory/context?space=：汇总为注入上下文文本
func (a *API) kbMemoryContext(w http.ResponseWriter, r *http.Request) {
	space := r.URL.Query().Get("space")
	if space == "" {
		writeErr(w, http.StatusBadRequest, "SPACE_REQUIRED", "space 参数必填")
		return
	}
	ctx, err := a.kb.MemoryContext(r.Context(), space, 40)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "MEMORY_CTX_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"context": ctx, "empty": ctx == ""})
}
