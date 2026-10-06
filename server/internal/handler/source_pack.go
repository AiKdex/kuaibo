// source_pack.go 采集源包安装/卸载（《采集源包格式规范 v1》SPEC-SP-001 宿主侧实现）。
//
// 源包与插件/主题同走应用中心安装链路，区别在于落点：
//   - 插件/主题 → blog_plugins 表 + 前端组件/主题资产；
//   - 源包       → sources 表（每源一行）+ blog_plugins 登记（pack_type=source-pack，供卸载与「已安装」标记）。
//
// 源包是**纯配置包**：不注入任何可执行代码，装完即可在采集管理页看到源并触发 /collect/run。
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// sourcePackInstall POST /api/v1/admin/apps/install-source-pack
// multipart 字段 file=zip；或 Content-Type application/json 的 {"manifest":{...}}（内嵌 sources 的直投模式）。
func (a *API) sourcePackInstall(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Manifest json.RawMessage `json:"manifest"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Manifest) == 0 {
			writeErr(w, http.StatusBadRequest, "SOURCE_PACK_BAD_BODY", "需要 manifest JSON")
			return
		}
		a.installSourcePack(w, r, nil, body.Manifest)
		return
	}
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "SOURCE_PACK_BAD_MULTIPART", err.Error())
		return
	}
	fh, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "SOURCE_PACK_NO_FILE", "缺少 file 字段（zip）")
		return
	}
	defer fh.Close()
	raw, err := io.ReadAll(io.LimitReader(fh, 8<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "SOURCE_PACK_READ_FAILED", err.Error())
		return
	}
	a.installSourcePack(w, r, raw, nil)
}

// installSourcePack 源包安装主流程（zip 内容 或 裸 manifest 二选一）。
// 顺序：解析 → 校验（全过才落库）→ 事务写 sources → 登记 blog_plugins。
func (a *API) installSourcePack(w http.ResponseWriter, r *http.Request, zipRaw, manifestRaw []byte) {
	if a.sources == nil {
		writeErr(w, http.StatusNotFound, "SOURCES_DISABLED",
			"采集能力未启用（capability.collector 关闭或未装配 SourceStore），无法安装源包")
		return
	}
	var pack *service.SourcePack
	if len(zipRaw) > 0 {
		p, err := service.ParseSourcePackZip(zipRaw)
		if err != nil {
			writeSourcePackErr(w, err)
			return
		}
		pack = p
	} else {
		var m service.SourcePackManifest
		if err := json.Unmarshal(manifestRaw, &m); err != nil {
			writeErr(w, http.StatusBadRequest, "SOURCE_PACK_BAD_MANIFEST", "manifest 非法 JSON："+err.Error())
			return
		}
		pack = &service.SourcePack{Manifest: &m}
	}
	if err := service.ValidateSourcePack(pack, service.ValidateSourcePackOptions{
		ShellID:     a.shellID(),
		HostVersion: coreVersion,
	}); err != nil {
		writeSourcePackErr(w, err)
		return
	}
	m := pack.Manifest
	created, updated, err := a.sources.ImportSources(r.Context(), service.SourcePackToSources(m))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SOURCE_PACK_IMPORT_FAILED", err.Error())
		return
	}
	// 登记到应用中心（pack_type=source-pack）：供「已安装」标记与卸载时定位其导入的源
	pm := pluginManifest{
		ID:          m.ID,
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Kind:        "plugin",
		PackType:    service.SourcePackKind,
		// compliance 摘要进 settings_schema：应用中心卡片可展示合规声明（纯展示，非表单）
		SettingsSchema: []any{map[string]any{
			"key": "compliance", "type": "note", "title": "合规声明",
			"value": map[string]any{
				"target_site":     m.Compliance.TargetSite,
				"usage":           m.Compliance.Usage,
				"robots_allowed":  m.Compliance.RobotsAllowed,
				"fetch_frequency": m.Compliance.FetchFrequency,
				"copyright_note":  m.Compliance.CopyrightNote,
			},
		}},
	}
	ok, _, _, _ := a.upsertPluginRecord(w, r, &pm, "sourcepack.install")
	if !ok {
		return // 失败响应已写出（此时 sources 已写入，属极端情况：登记表失败源仍可用）
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"id":          m.ID,
		"version":     m.Version,
		"pack_type":   service.SourcePackKind,
		"created":     created,
		"updated":     updated,
		"total":       len(m.Sources),
		"files":       len(pack.Files),
		"capability":  "collector",
		"next_action": "POST /api/v1/collect/run {city} 触发采集；GET /api/v1/sources 查看已装源",
	})
}

// sourcePackList GET /api/v1/admin/apps/source-packs：已安装源包（含各自导入的源数量）。
func (a *API) sourcePackList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT p.id, p.name, p.version, COALESCE(p.description,''), COALESCE(p.author,''), p.enabled,
		        (SELECT COUNT(*) FROM sources s WHERE s.pack_id = p.id) AS n
		 FROM blog_plugins p WHERE p.pack_type = ? ORDER BY p.created_at ASC`, service.SourcePackKind)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SOURCE_PACK_LIST_FAILED", err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, ver, desc, author string
		var enabled, n int
		if err := rows.Scan(&id, &name, &ver, &desc, &author, &enabled, &n); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "version": ver, "description": desc,
			"author": author, "enabled": enabled == 1, "sources": n,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// sourcePackUninstall DELETE /api/v1/admin/apps/source-packs/{id}
// 删除该包导入的全部 sources（pack_id 精确匹配，不影响用户自建源）+ 移除应用中心登记。
func (a *API) sourcePackUninstall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "SOURCE_PACK_BAD_ID", "缺少源包 id")
		return
	}
	if a.sources == nil {
		writeErr(w, http.StatusNotFound, "SOURCES_DISABLED", "采集能力未启用")
		return
	}
	removed, err := a.sources.DeleteSourcesByPack(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SOURCE_PACK_UNINSTALL_FAILED", err.Error())
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`DELETE FROM blog_plugins WHERE id=? AND pack_type=?`, id, service.SourcePackKind); err != nil {
		writeErr(w, http.StatusInternalServerError, "SOURCE_PACK_UNINSTALL_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.homeOwnerID(), "sourcepack.uninstall", "blog_plugins",
		map[string]any{"id": id, "sources_removed": removed})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "sources_removed": removed})
}

// writeSourcePackErr 源包错误响应：带协议错误码的走 422，其余（zip 损坏等）走 400。
func writeSourcePackErr(w http.ResponseWriter, err error) {
	var pe *service.SourcePackErr
	if errors.As(err, &pe) {
		code := http.StatusUnprocessableEntity
		if pe.Code == "SOURCE_PACK_BAD_ZIP" || pe.Code == "SOURCE_PACK_NO_MANIFEST" ||
			pe.Code == "SOURCE_PACK_BAD_MANIFEST" || pe.Code == "SOURCE_PACK_BAD_ENTRY" {
			code = http.StatusBadRequest
		}
		writeErr(w, code, pe.Code, pe.Msg)
		return
	}
	writeErr(w, http.StatusBadRequest, "SOURCE_PACK_INVALID", err.Error())
}
