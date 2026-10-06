// derived.go 派生资产运维端点（B16）：统计与清理。
//
// 权限：与 /admin/settings 同级，由 routes.go 的 authmw 统一保证（仅 owner/admin）。
// 设计与面板的关系：面板只展示 + 触发，判定与执行全在 service 层，
// 避免「面板说清了、磁盘没动」这类判定与实现漂移（B14/B15 连续踩过两次）。
package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// derivedStats GET /api/v1/admin/derived/stats —— 派生资产占用与孤儿数（只读）。
//
// 一并下发 options / ffmpeg / capability_enabled 三个上下文，是为了让面板能自解释：
// 「为什么没有缩略图」可能是能力关了、没装 ffmpeg、或参数配错，面板不该让站长去猜。
func (a *API) derivedStats(w http.ResponseWriter, r *http.Request) {
	if a.files == nil {
		writeErr(w, http.StatusServiceUnavailable, "FILE_STORE_UNAVAILABLE", "文件存储未就绪")
		return
	}
	st := a.files.DerivedStats(r.Context())
	if st.ByExt == nil {
		st.ByExt = map[string]int{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"stats":               st,
		"ffmpeg":              service.FFmpegAvailable(),
		"options":             service.ThumbOptionsFrom(a.cfg),
		"capability_enabled":  a.thumbEnabled(),
		"sweep_interval_min":  a.cfg.GetInt("media.derived_sweep_interval_min"),
	})
}

// derivedPurge POST /api/v1/admin/derived/purge —— 清理派生对象。
//
// body：{"file_id":"..."} 只清该文件；省略/空串 = 清全部。
// 不返回 404：清理是幂等的，没东西可清也是成功（返回 removed=0）。
func (a *API) derivedPurge(w http.ResponseWriter, r *http.Request) {
	if a.files == nil {
		writeErr(w, http.StatusServiceUnavailable, "FILE_STORE_UNAVAILABLE", "文件存储未就绪")
		return
	}
	var req struct {
		FileID string `json:"file_id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // 允许空 body（= 清全部）
	}
	n, freed := a.files.PurgeDerived(r.Context(), strings.TrimSpace(req.FileID))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"removed": n,
		"bytes":   freed,
		// 提示调用方：清掉后**按需懒生成**，不需要额外触发；改尺寸参数后清一次即可生效。
		"note": "派生对象已清理，下次访问缩略图端点会按当前参数重新生成",
	})
}

// derivedSweep POST /api/v1/admin/derived/sweep —— 立即清理孤儿派生对象。
//
// 与定时兜底清理（media.derived_sweep_interval_min）同口径：源文件行已不存在的派生对象删掉。
// 不返回 404：幂等，没孤儿可清也是成功（removed=0）。
func (a *API) derivedSweep(w http.ResponseWriter, r *http.Request) {
	if a.files == nil {
		writeErr(w, http.StatusServiceUnavailable, "FILE_STORE_UNAVAILABLE", "文件存储未就绪")
		return
	}
	n, freed, err := a.files.SweepOrphans(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SWEEP_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"removed": n,
		"bytes":   freed,
		"note":    "已清理源文件不存在的孤儿派生对象；源文件仍在的派生对象与正常缓存不受影响",
	})
}
