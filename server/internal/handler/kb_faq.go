// kb_faq.go FAQ 知识库类型端点（WeKnora 借鉴 A2，B27 自上游 bbd6764 移植）：
//   GET    /api/v1/kb/faqs?space=&q=      列表（可选过滤）
//   POST   /api/v1/kb/faqs                新建 {space_id, standard_q, similar_qs[], counter_qs[], answer, tags[]}
//   PUT    /api/v1/kb/faqs/{id}           更新（同空间）
//   DELETE /api/v1/kb/faqs/{id}           删除（同空间）
//   GET    /api/v1/kb/faqs/search?space=&q= 问题检索（标准问优先）
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

type faqReq struct {
	SpaceID   string   `json:"space_id"`
	StandardQ string   `json:"standard_q"`
	SimilarQs []string `json:"similar_qs"`
	CounterQs []string `json:"counter_qs"`
	Answer    string   `json:"answer"`
	Tags      []string `json:"tags"`
}

func (a *API) faqFromReq(r *http.Request) (*service.FAQ, error) {
	var q faqReq
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		return nil, err
	}
	return &service.FAQ{
		SpaceID: q.SpaceID, StandardQ: q.StandardQ, SimilarQs: q.SimilarQs,
		CounterQs: q.CounterQs, Answer: q.Answer, Tags: q.Tags,
	}, nil
}

// kbFAQList GET /api/v1/kb/faqs
func (a *API) kbFAQList(w http.ResponseWriter, r *http.Request) {
	space := r.URL.Query().Get("space")
	if space == "" {
		writeErr(w, http.StatusBadRequest, "SPACE_REQUIRED", "space 参数必填")
		return
	}
	items, err := a.kb.ListFAQ(r.Context(), space, r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FAQ_LIST_FAILED", err.Error())
		return
	}
	if items == nil {
		items = []*service.FAQ{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"faqs": items})
}

// kbFAQCreate POST /api/v1/kb/faqs
func (a *API) kbFAQCreate(w http.ResponseWriter, r *http.Request) {
	f, err := a.faqFromReq(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	f.CreatedBy = a.curUserID(r)
	if err := a.kb.CreateFAQ(r.Context(), f); err != nil {
		writeErr(w, http.StatusBadRequest, "FAQ_CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "faq": f})
}

// kbFAQUpdate PUT /api/v1/kb/faqs/{id}
func (a *API) kbFAQUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f, err := a.faqFromReq(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	if err := a.kb.UpdateFAQ(r.Context(), id, f.SpaceID, f); err != nil {
		writeErr(w, http.StatusBadRequest, "FAQ_UPDATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// kbFAQDelete DELETE /api/v1/kb/faqs/{id}
func (a *API) kbFAQDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	space := r.URL.Query().Get("space")
	if space == "" {
		writeErr(w, http.StatusBadRequest, "SPACE_REQUIRED", "space 参数必填")
		return
	}
	if err := a.kb.DeleteFAQ(r.Context(), id, space); err != nil {
		writeErr(w, http.StatusBadRequest, "FAQ_DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// kbFAQSearch GET /api/v1/kb/faqs/search?space=&q=
func (a *API) kbFAQSearch(w http.ResponseWriter, r *http.Request) {
	space := r.URL.Query().Get("space")
	q := r.URL.Query().Get("q")
	if space == "" || q == "" {
		writeErr(w, http.StatusBadRequest, "PARAM_REQUIRED", "space 与 q 必填")
		return
	}
	items, err := a.kb.SearchFAQ(r.Context(), space, q, 8)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FAQ_SEARCH_FAILED", err.Error())
		return
	}
	if items == nil {
		items = []*service.FAQ{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"faqs": items})
}
