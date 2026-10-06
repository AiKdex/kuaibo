// source.go 实现采集源配置与采集运行的 API（频道化 P10）。
// 对外接口即"对接协议"：第三方（采集适配器/城市模板生产）按本组端点接入，
// 完整协议见 docs/采集对接API协议.md。
package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// sourcesList GET /api/v1/sources?city=&kind=
func (a *API) sourcesList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := a.sources.ListSources(r.Context(), q.Get("city"), q.Get("kind"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SOURCES_READ_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// sourcesCreate POST /api/v1/sources {city,name,kind,url,template,enabled}
func (a *API) sourcesCreate(w http.ResponseWriter, r *http.Request) {
	// M10 修复：采集源配置（含抓取 URL/解析模板）仅 owner/admin 可写
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	var in service.Source
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	src, err := a.sources.CreateSource(r.Context(), &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "SOURCE_CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, src)
}

// sourcesUpdate PUT /api/v1/sources/{id} {name?,url?,kind?,template?,enabled?}
func (a *API) sourcesUpdate(w http.ResponseWriter, r *http.Request) {
	// M10 修复：采集源修改仅 owner/admin
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	id := r.PathValue("id")
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	src, err := a.sources.UpdateSource(r.Context(), id, patch)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "SOURCE_UPDATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, src)
}

// sourcesDelete DELETE /api/v1/sources/{id}
func (a *API) sourcesDelete(w http.ResponseWriter, r *http.Request) {
	// M10 修复：采集源删除仅 owner/admin
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	if err := a.sources.DeleteSource(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, "SOURCE_DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// collectRun POST /api/v1/collect/run {city, source_ids?, limit?, force?}
// 同步执行：逐源抓取解析入库，返回汇总统计（含 collect_runs 记录 id）。
func (a *API) collectRun(w http.ResponseWriter, r *http.Request) {
	// 报告 §8.4 修复：采集执行是站点级重操作（外网抓取+入库），仅 owner/admin 可触发
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	var in struct {
		City      string   `json:"city"`
		SourceIDs []string `json:"source_ids"`
		Limit     int      `json:"limit"`
		Force     bool     `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	if in.City == "" {
		writeErr(w, http.StatusBadRequest, "CITY_REQUIRED", "city 不能为空")
		return
	}
	if a.collector == nil {
		a.collector = service.NewCollector(a.sources, a.files, a.tags)
	}
	// 异步执行：立即返回 run_id，后台逐源抓取入库（慢源不阻塞 API；状态经 GET /collect/runs 查询）
	runID, err := a.collector.StartRunAsync(r.Context(), in.City, in.SourceIDs, in.Limit, in.Force, "api")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "COLLECT_RUN_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run_id": runID, "status": "started", "city": in.City})
}

// collectRuns GET /api/v1/collect/runs?city=&limit=
func (a *API) collectRuns(w http.ResponseWriter, r *http.Request) {
	// 报告 §8.4 修复：采集运行记录含源配置摘要，仅 owner/admin 可读
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		_ = json.Unmarshal([]byte(v), &limit)
	}
	items, err := a.sources.ListRuns(r.Context(), q.Get("city"), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "RUNS_READ_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ensureSources 供 routes 依赖注入校验（handler.New 已注入，此处保留编译期检查）。
func ensureSources(db *sql.DB) *service.SourceStore { return service.NewSourceStore(db) }
