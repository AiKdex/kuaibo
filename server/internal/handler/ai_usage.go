// ai_usage.go 用量账本：GET /api/v1/admin/ai/llm-usage（A1 LLM 调用可观测，B27 自上游 9ddd6e7 移植；上游同路径已被本仓 B9 ai_ask_usage 端点占用，故改挂 llm-usage）。
// 按 provider/cap/model 聚合近 N 天调用量/token/失败/耗时，是 token 成本计费的前置数据源。
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
)

// aiUsageList GET /api/v1/admin/ai/llm-usage?days=7：用量聚合账本（owner/admin 可见）。
func (a *API) aiUsageList(w http.ResponseWriter, r *http.Request) {
	rec := a.ai.UsageRecorder()
	udb, _ := rec.(*ai.UsageDB)
	if udb == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "rows": []ai.UsageSummary{}, "since": ""})
		return
	}
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	rows, err := udb.SummarizeUsage(r.Context(), days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "USAGE_QUERY_FAILED", err.Error())
		return
	}
	if rows == nil {
		rows = []ai.UsageSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true,
		"days":    days,
		"rows":    rows,
		"since":   time.Now().AddDate(0, 0, -days).Format(time.RFC3339),
	})
}
