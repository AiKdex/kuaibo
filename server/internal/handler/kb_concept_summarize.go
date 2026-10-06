// kb_concept_summarize.go 概念页 AI 独立摘要端点（A09+A5，B27 自上游 2d12a42/b0fc988 移植）：
// POST /api/v1/kb/concepts/{id}/summarize —— 综合该概念（标签）下已解读文件生成概念级概述，
// 写 kb_concept_summaries 缓存（含来源文档引用链）并返回。前端"重新生成"按钮幂等：每次调用覆盖旧缓存。
package handler

import (
	"net/http"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
)

// kbConceptSummarize POST /api/v1/kb/concepts/{id}/summarize
func (a *API) kbConceptSummarize(w http.ResponseWriter, r *http.Request) {
	if a.conceptSum == nil {
		writeErr(w, http.StatusBadRequest, "CONCEPT_SUMMARIZER_OFF", "概念摘要能力未启用")
		return
	}
	id := r.PathValue("id")
	spaceID := a.curHomeSpaceID(r)
	// 取概念（标签）基本信息
	var tagName string
	if err := a.db.QueryRowContext(r.Context(), `SELECT name FROM tags WHERE id=?`, id).Scan(&tagName); err != nil {
		writeErr(w, http.StatusNotFound, "KB_CONCEPT_FAILED", "概念不存在")
		return
	}
	// 取该标签下已解读文件的摘要（id + title + summary），带 file_id 供 A5 来源回链
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT f.id, f.name, s.summary FROM file_ai_summaries s
		 JOIN file_tags ft ON ft.file_id = s.file_id
		 JOIN files f ON f.id = s.file_id
		 WHERE ft.tag_id=? AND s.status='done' AND s.summary<>'' AND f.deleted_at IS NULL
		   AND f.space_id=? ORDER BY length(s.summary) DESC LIMIT 20`, id, spaceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "KB_CONCEPT_SUMMARIZE_FAILED", err.Error())
		return
	}
	items := []ai.ConceptSummaryItem{}
	for rows.Next() {
		var it ai.ConceptSummaryItem
		if err := rows.Scan(&it.FileID, &it.Name, &it.Summary); err != nil {
			rows.Close()
			writeErr(w, http.StatusInternalServerError, "KB_CONCEPT_SUMMARIZE_FAILED", err.Error())
			return
		}
		items = append(items, it)
	}
	rows.Close()
	summary, updatedAt, err := a.conceptSum.SummarizeConcept(r.Context(), id, tagName, items)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "KB_CONCEPT_SUMMARIZE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "updated_at": updatedAt, "source": "ai"})
}
