// blog_plugins.go 博客功能插件协议 + RSS 内置能力。
//
// 插件 = 声明（manifest）+ 前端组件。声明由第三方通过 API 注册（或启动内置注册），
// 后端只负责登记与启用状态；渲染由前端按 mount_points 在挂载点动态加载对应组件。
// 扩展点（mount_points）：post_bottom（文章页正文下）/ sidebar（列表页侧栏）/
// list_item（列表项附加）/ head（HTML head 输出区，预留）。
//
// RSS：GET /api/v1/blog/feed.xml（公开，白名单）——博客目录已发布文章聚合输出 RSS 2.0，
// 发布即更新（动态生成）；文章链接用对外 hash 路由 #/p/blog?path=…。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// pluginManifest 博客插件声明（协议字段；第三方按此注册）。
type pluginManifest struct {
	ID             string       `json:"id"`                         // 唯一 id（插件名，如 "blog-comments"）
	Name           string       `json:"name"`                       // 展示名
	Version        string       `json:"version"`                    // 语义化版本
	Description    string       `json:"description"`                // 一句话说明
	Author         string       `json:"author"`                     // 作者/组织
	Kind           string       `json:"kind,omitempty"`             // plugin|theme（应用中心区分安装类型）
	MountPoints    []string     `json:"mount_points"`               // 挂载点：post_bottom|sidebar|list_item|head
	APIPermissions []string     `json:"api_permissions"`            // 数据权限：blog.read / blog.write
	FrontendEntry  string       `json:"frontend_entry,omitempty"`   // 前端组件注册键（与前端注册表对应）
	MinCoreVersion string       `json:"min_core_version,omitempty"` // 要求的最低主系统版本（如 "1.0.0"）
	SettingsSchema []any        `json:"settings_schema,omitempty"`  // 动态设置表单声明（应用中心：settings_schema 驱动前端渲染）
	Enabled        *bool        `json:"enabled,omitempty"`          // 缺省 true
	Hooks          *pluginHooks `json:"hooks,omitempty"`            // 事件契约 v1：subscribe/publish 白名单（能力插件 v2 增量）
	// ---- 能力插件协议 v2（docs/能力插件协议-v2草案.md；REQ-004 落地口径见下方注释）----
	// 上游 v2 的 kind 取值 ui|capacity（有无后端能力入口），本壳 kind 已承载 plugin|theme
	// （应用中心安装类型分类），两者取值域冲突。按 REQ-004 方案 A：v2 的能力分类落到
	// capability_mode 列/字段，kind 保持本壳语义不动；同时兼容上游生包——manifest 里出现
	// kind=ui|capacity 时自动迁移到 capability_mode（见 normalizePluginV2Kind）。
	CapabilityMode string   `json:"capability_mode,omitempty"` // ui（默认）| capacity
	Capabilities   []string `json:"capabilities,omitempty"`    // collector|channel|im|rag|notify|webhook|media|office|storage|transcode|thumbnail|search
	BackendEntry   string   `json:"backend_entry,omitempty"`   // 后端实现入口键（capability 必填；内核按此键加载/校验）
	Routes         []string `json:"routes,omitempty"`          // 允许挂载的路由前缀白名单（仅 /api/v1/plugins/{plugin_id}/ 前缀）
	MinSchema      int      `json:"min_schema,omitempty"`      // schema 版本下限（0=不限）
	MaxSchema      *int     `json:"max_schema,omitempty"`      // schema 版本上限（nil=不限）
	// PackType 应用包类别：''=普通插件/主题（默认），'source-pack'=采集源包（SPEC-SP-001）。
	// 源包与插件同走应用中心安装链路（同一 blog_plugins 表），需可区分以便卸载时同步清理
	// 其导入的 sources 行。源包不产生前端组件、不注入后端代码，仅写采集源配置。
	PackType string `json:"pack_type,omitempty"`
}

// pluginHooks 事件订阅/发布声明（docs/事件总线契约.md §3 权限模型）。
type pluginHooks struct {
	Subscribe []string `json:"subscribe,omitempty"` // 订阅 topic 白名单：内核冻结清单或 {plugin_id}.{event}
	Publish   []string `json:"publish,omitempty"`   // 发布 topic 白名单（需 api_permissions 含 event:post）
}

// validatePluginHooks 校验插件事件声明（订阅/发布白名单 + publish 权限）。
// 返回 (错误码, 错误消息)；合法返回 ("", "")。
func validatePluginHooks(m pluginManifest) (string, string) {
	permOK := false
	for _, p := range m.APIPermissions {
		if p == "event:post" {
			permOK = true
			break
		}
	}
	for _, t := range m.Hooks.Subscribe {
		if !bus.ValidateTopic(t, m.ID) {
			return "PLUGIN_HOOK_TOPIC_INVALID",
				"订阅 topic " + t + " 不在内核冻结清单且无 {plugin_id}. 前缀（见 docs/事件总线契约.md）"
		}
	}
	for _, t := range m.Hooks.Publish {
		if !bus.ValidateTopic(t, m.ID) {
			return "PLUGIN_HOOK_TOPIC_INVALID",
				"发布 topic " + t + " 不在内核冻结清单且无 {plugin_id}. 前缀（见 docs/事件总线契约.md）"
		}
		if !permOK {
			return "PLUGIN_HOOK_PUBLISH_FORBIDDEN",
				"声明 hooks.publish 需要 api_permissions 含 event:post（发布权限未授予）"
		}
	}
	return "", ""
}

// pluginV2CapabilityNames 能力插件协议 v2 的 capability 白名单。
var pluginV2CapabilityNames = map[string]bool{
	"collector": true, "channel": true, "im": true, "rag": true,
	"notify": true, "webhook": true, "media": true, "office": true,
	"storage": true, "transcode": true, "thumbnail": true, "search": true,
}

// normalizePluginV2Kind 兼容上游生包：manifest 的 kind 命中 v2 取值域（ui|capacity）时，
// 迁移到 capability_mode，kind 归一为本壳语义（theme 保留，其余视作 plugin）。
// 本壳对外写入的 kind 恒为 plugin|theme——应用中心的安装类型分类依赖该列。
func normalizePluginV2Kind(m *pluginManifest) {
	if m.Kind == "ui" || m.Kind == "capacity" {
		// kind 来自上游生包的 v2 语义：迁移到 capability_mode。
		// capacity 恒覆盖显式 capability_mode——避免能力语义被降级成纯 UI 而不自知。
		if m.CapabilityMode == "" || m.Kind == "capacity" {
			m.CapabilityMode = m.Kind
		}
		m.Kind = "plugin"
	}
	if m.Kind == "" {
		m.Kind = "plugin"
	}
	if m.CapabilityMode == "" {
		m.CapabilityMode = "ui"
	}
}

// validatePluginV2 能力插件协议 v2 校验（docs/能力插件协议-v2草案.md §1、§4）。
// 返回 (错误码, 错误消息)；合法返回 ("", "")。
// 注：本壳把上游的 kind 域判定改到 capability_mode 上（REQ-004 方案 A），其余与上游一致。
func validatePluginV2(m pluginManifest, schemaVer int) (string, string) {
	mode := m.CapabilityMode
	if mode == "" {
		mode = "ui"
	}
	if mode != "ui" && mode != "capacity" {
		return "PLUGIN_CAPABILITY_MODE_INVALID",
			"capability_mode 仅支持 ui（v1.1 默认）| capacity（能力插件 v2）"
	}
	for _, c := range m.Capabilities {
		if !pluginV2CapabilityNames[c] {
			return "PLUGIN_CAPABILITY_INVALID",
				"capability " + c + " 不在白名单（collector|channel|im|rag|notify|webhook|media|office|storage|transcode|thumbnail|search）"
		}
	}
	if mode == "capacity" {
		if len(m.Capabilities) == 0 {
			return "PLUGIN_CAPABILITY_MISSING", "capability_mode=capacity 必须声明至少一个 capability"
		}
		if m.BackendEntry == "" {
			return "PLUGIN_BACKEND_MISSING", "capability_mode=capacity 必须声明 backend_entry（后端实现入口键）"
		}
	}
	// routes 前缀白名单：插件只能挂 /api/v1/plugins/{plugin_id}/ 前缀下的路由（§4 安全模型）
	prefix := "/api/v1/plugins/" + m.ID + "/"
	for _, r := range m.Routes {
		if !strings.HasPrefix(r, prefix) {
			return "PLUGIN_ROUTE_FORBIDDEN",
				"路由 " + r + " 越界：插件仅可挂载 " + prefix + " 前缀下的路由"
		}
	}
	// schema 版本区间（min_schema > 当前 或 max_schema < 当前 → 409 兼容性拒绝）
	if m.MinSchema > 0 && schemaVer < m.MinSchema {
		return "PLUGIN_CORE_TOO_OLD",
			"插件要求 schema ≥ " + strconv.Itoa(m.MinSchema) + "，当前 schema " + strconv.Itoa(schemaVer)
	}
	if m.MaxSchema != nil && *m.MaxSchema > 0 && schemaVer > *m.MaxSchema {
		return "PLUGIN_CORE_TOO_OLD",
			"插件仅支持 schema ≤ " + strconv.Itoa(*m.MaxSchema) + "，当前 schema " + strconv.Itoa(schemaVer)
	}
	return "", ""
}

// coreVersion 主系统核心版本（插件兼容矩阵基准；与 /api/v1/version 对齐）
const coreVersion = "1.0.0"

// builtinFrontendEntries 主系统内置前端插件组件清单（与 web/src/blogPlugins/builtin.js 注册表对齐）。
// frontend_entry 一致性校验：注册 manifest 时若声明了 frontend_entry，必须命中此清单（组件随主系统构建），
// 否则返回 422 PLUGIN_COMPONENT_NOT_REGISTERED（防止"登记了但前端渲染不出来"的悬空插件）。
// 纯后端插件（无前端组件，如 RSS 定制）frontend_entry 留空即可。
var builtinFrontendEntries = map[string]bool{
	"blog-meta-footer":    true,
	"blog-tag-badges":     true,
	"blog-meta-card":      true,
	"blog-seo-meta":       true,
	"blog-comments":       true,
	"blog-related-posts":  true,
	"blog-archives":       true,
	"blog-tag-cloud":      true,
	"blog-cover":          true,
	"blog-reader-ai":      true,
	"blog-stats":          true,
	"blog-voice-read":     true,
	"docs-toc":            true,
}

// versionAtLeast 简单语义化版本比较（a >= b）
func versionAtLeast(a, b string) bool {
	if b == "" || b == "0" {
		return true
	}
	pa := strings.Split(strings.TrimPrefix(a, "v"), ".")
	pb := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(pb); i++ {
		va, vb := 0, 0
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &va)
		}
		fmt.Sscanf(pb[i], "%d", &vb)
		if va != vb {
			return va > vb
		}
	}
	return true
}

// blogPluginList GET /api/v1/blog/plugins 已启用插件列表（公开：供前端按挂载点渲染）。
func (a *API) blogPluginList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, name, version, description, author, kind, mount_points, api_permissions, frontend_entry, min_core_version, settings_schema, hooks, enabled,
		        capability_mode, capabilities, backend_entry, routes, min_schema, max_schema
		 FROM blog_plugins ORDER BY created_at ASC`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PLUGIN_LIST_FAILED", err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, ver, desc, author, kind, mp, perm, fe, mcv, ss, hk string
		var enabled int
		var mode, caps, be, rts string
		var minSchema int
		var maxSchema *int
		if rows.Scan(&id, &name, &ver, &desc, &author, &kind, &mp, &perm, &fe, &mcv, &ss, &hk, &enabled,
			&mode, &caps, &be, &rts, &minSchema, &maxSchema) != nil {
			continue
		}
		mpA := []string{}
		_ = json.Unmarshal([]byte(mp), &mpA)
		permA := []string{}
		_ = json.Unmarshal([]byte(perm), &permA)
		ssA := []any{}
		_ = json.Unmarshal([]byte(ss), &ssA)
		hkO := map[string]any{}
		if hk != "" && hk != "{}" {
			_ = json.Unmarshal([]byte(hk), &hkO)
		}
		if kind == "" {
			kind = "plugin"
		}
		capsA := []string{}
		_ = json.Unmarshal([]byte(caps), &capsA)
		rtsA := []string{}
		_ = json.Unmarshal([]byte(rts), &rtsA)
		if mode == "" {
			mode = "ui"
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "version": ver, "description": desc, "author": author, "kind": kind,
			"mount_points": mpA, "api_permissions": permA, "frontend_entry": fe,
			"min_core_version": mcv, "settings_schema": ssA, "hooks": hkO, "enabled": enabled == 1,
			// 能力插件协议 v2（REQ-004：v2 kind 语义落 capability_mode，不复用 kind）
			"capability_mode": mode, "capabilities": capsA, "backend_entry": be, "routes": rtsA,
			"min_schema": minSchema, "max_schema": maxSchema,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// blogPluginUpsert POST /api/v1/blog/plugins 注册/更新插件（登录；幂等 upsert by id）。
func (a *API) blogPluginUpsert(w http.ResponseWriter, r *http.Request) {
	// H1 修复：插件注册（含 frontend_entry/hooks/routes）为站点级操作，仅 owner/admin 可写
	// （此前无守卫，任意登录用户可注册插件并挂载前端入口；同文件 blogPostCreate 已有 blogAuthorOnly，属漏加）。
	if !a.blogAdminOnly(w, r) {
		return
	}
	var m pluginManifest
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil || m.ID == "" {
		writeErr(w, http.StatusBadRequest, "PLUGIN_BAD_MANIFEST", "插件 id 必填，且须为合法 JSON manifest")
		return
	}
	// 兼容上游包：manifest 的 kind=ui|capacity 迁移到 capability_mode（REQ-004 方案 A）
	normalizePluginV2Kind(&m)
	if m.MinCoreVersion != "" && !versionAtLeast(coreVersion, m.MinCoreVersion) {
		writeErr(w, http.StatusConflict, "PLUGIN_CORE_TOO_OLD",
			"主系统版本 "+coreVersion+" 低于插件要求的 "+m.MinCoreVersion)
		return
	}
	// kind 校验：仅 plugin|theme（应用中心安装类型；ui/capacity 已由 normalizePluginV2Kind 迁移）
	if m.Kind != "plugin" && m.Kind != "theme" {
		writeErr(w, http.StatusBadRequest, "PLUGIN_BAD_KIND", "kind 仅支持 plugin|theme")
		return
	}
	// frontend_entry 一致性校验：声明了前端组件就必须命中内置清单（组件随主系统构建）
	if m.FrontendEntry != "" && !builtinFrontendEntries[m.FrontendEntry] {
		writeErr(w, http.StatusUnprocessableEntity, "PLUGIN_COMPONENT_NOT_REGISTERED",
			"前端组件 "+m.FrontendEntry+" 未在主系统注册（组件需随主系统构建；纯后端插件请留空 frontend_entry）")
		return
	}
	// 事件契约 v1：hooks.subscribe/publish 白名单校验（docs/事件总线契约.md §3）
	if m.Hooks != nil {
		if code, msg := validatePluginHooks(m); code != "" {
			writeErr(w, http.StatusUnprocessableEntity, code, msg)
			return
		}
	}
	// 能力插件协议 v2：capability_mode/capabilities/routes/schema 区间校验
	// （docs/能力插件协议-v2草案.md §1、§4）
	if code, msg := validatePluginV2(m, a.dbSchemaVersion()); code != "" {
		writeErr(w, http.StatusUnprocessableEntity, code, msg)
		return
	}
	a.upsertPluginManifest(w, r, &m, "plugin.register")
}

// upsertPluginManifest 写入 blog_plugins（幂等 by id）。市场安装与注册 API 共用，
// 故统一在此做 v2 归一与校验，避免绕过 validatePluginV2 的安装路径。
// upsertPluginManifest 写入 blog_plugins 并返回标准响应（{ok,id,kind,capability_mode,enabled}）。
func (a *API) upsertPluginManifest(w http.ResponseWriter, r *http.Request, m *pluginManifest, auditAction string) {
	ok, kind, capMode, enabled := a.upsertPluginRecord(w, r, m, auditAction)
	if !ok {
		return // 失败响应已由 upsertPluginRecord 写出
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": m.ID, "kind": kind, "capability_mode": capMode, "enabled": enabled == 1, "pack_type": m.PackType})
}

// upsertPluginRecord 写入 blog_plugins（幂等 by id），**不写成功响应**——供需要自定义响应体的
// 路径复用（如源包安装要一并返回导入的 sources 条数）。失败时自行写错误响应并返回 ok=false。
func (a *API) upsertPluginRecord(w http.ResponseWriter, r *http.Request, m *pluginManifest, auditAction string) (ok bool, kind, capMode string, enabled int) {
	// 兼容上游包：manifest 的 kind=ui|capacity 迁移到 capability_mode（REQ-004 方案 A）
	normalizePluginV2Kind(m)
	// 能力插件协议 v2 校验（安装路径与注册路径一致，防止市场包绕过校验层）
	if code, msg := validatePluginV2(*m, a.dbSchemaVersion()); code != "" {
		writeErr(w, http.StatusUnprocessableEntity, code, msg)
		return false, "", "", 0
	}
	mp, _ := json.Marshal(m.MountPoints)
	perm, _ := json.Marshal(m.APIPermissions)
	ss, _ := json.Marshal(m.SettingsSchema)
	if len(ss) == 0 {
		ss = []byte("[]")
	}
	hk := ""
	if m.Hooks != nil {
		hb, _ := json.Marshal(m.Hooks)
		hk = string(hb)
	}
	caps, _ := json.Marshal(m.Capabilities)
	if len(caps) == 0 {
		caps = []byte("[]")
	}
	rts, _ := json.Marshal(m.Routes)
	if len(rts) == 0 {
		rts = []byte("[]")
	}
	kind = m.Kind
	// 类型归一：theme 保留，其余一律 plugin（AiKlog 仅区分 插件/主题；
	// v2 的 ui|capacity 已在 normalizePluginV2Kind 迁到 capability_mode）
	if kind != "theme" {
		kind = "plugin"
	}
	capMode = m.CapabilityMode
	if capMode != "capacity" {
		capMode = "ui"
	}
	enabled = 1
	if m.Enabled != nil && !*m.Enabled {
		enabled = 0
	}
	now := time.Now().UnixMilli()
	_, err := a.db.ExecContext(r.Context(),
		`INSERT INTO blog_plugins (id, name, version, description, author, kind, mount_points, api_permissions, frontend_entry, min_core_version, settings_schema, hooks, enabled, created_at, updated_at,
		                          capability_mode, capabilities, backend_entry, routes, min_schema, max_schema, pack_type)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, version=excluded.version, description=excluded.description,
		   author=excluded.author, kind=excluded.kind, mount_points=excluded.mount_points, api_permissions=excluded.api_permissions,
		   frontend_entry=excluded.frontend_entry, min_core_version=excluded.min_core_version, settings_schema=excluded.settings_schema,
		   hooks=excluded.hooks, enabled=excluded.enabled, updated_at=excluded.updated_at,
		   capability_mode=excluded.capability_mode, capabilities=excluded.capabilities, backend_entry=excluded.backend_entry,
		   routes=excluded.routes, min_schema=excluded.min_schema, max_schema=excluded.max_schema,
		   pack_type=excluded.pack_type`,
		m.ID, m.Name, m.Version, m.Description, m.Author, kind, string(mp), string(perm), m.FrontendEntry, m.MinCoreVersion, string(ss), hk, enabled, now, now,
		capMode, string(caps), m.BackendEntry, string(rts), m.MinSchema, m.MaxSchema, m.PackType)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PLUGIN_UPSERT_FAILED", err.Error())
		return false, "", "", 0
	}
	_, _ = a.aud.Append(r.Context(), a.homeOwnerID(), auditAction, "blog_plugins", map[string]any{"id": m.ID, "version": m.Version})
	return true, kind, capMode, enabled
}

// blogPluginToggle POST /api/v1/blog/plugins/{id}/toggle 启用/禁用（登录）。
func (a *API) blogPluginToggle(w http.ResponseWriter, r *http.Request) {
	// H1 修复：启停站长插件仅 owner/admin（此前无守卫，任意登录用户可停用站点插件）
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	enable := r.URL.Query().Get("enable") != "0"
	v := 0
	if enable {
		v = 1
	}
	res, err := a.db.ExecContext(r.Context(),
		`UPDATE blog_plugins SET enabled=?, updated_at=? WHERE id=?`, v, time.Now().UnixMilli(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PLUGIN_TOGGLE_FAILED", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "PLUGIN_NOT_FOUND", "插件不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "enabled": enable})
}

// blogPluginDelete DELETE /api/v1/blog/plugins/{id} 卸载（登录；删记录，前端组件随注册表移除）。
func (a *API) blogPluginDelete(w http.ResponseWriter, r *http.Request) {
	// H1 修复：删除插件（含清理外置主题目录）仅 owner/admin
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	// 主题类：删除登记的同时清理外置资产目录 data/themes/<id>/（避免残留不可见的孤儿主题）
	var kind string
	_ = a.db.QueryRowContext(r.Context(), `SELECT COALESCE(kind,'') FROM blog_plugins WHERE id=?`, id).Scan(&kind)
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM blog_plugins WHERE id=?`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PLUGIN_DELETE_FAILED", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "PLUGIN_NOT_FOUND", "插件不存在")
		return
	}
	if kind == "theme" {
		if err := removeThemeAssets(id); err != nil {
			log.Printf("插件删除：清理主题资产目录失败 id=%s err=%v", id, err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// blogRSS GET /api/v1/blog/feed.xml 博客 RSS 2.0（公开白名单；动态生成，发布即更新）。
func (a *API) blogRSS(w http.ResponseWriter, r *http.Request) {
	// 博客对外开关（blog.open=false → RSS 下线）
	if v, ok := a.cfg.Get("blog.open"); ok {
		if s, _ := v.(string); s == "false" {
			http.Error(w, "blog closed", http.StatusNotFound)
			return
		}
	}
	var blogDirID string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(dir_id,'') FROM shares
		 WHERE owner_id=? AND token=? AND (revoked_at IS NULL OR revoked_at=0)
		   AND (expires_at IS NULL OR expires_at=0 OR expires_at>?)`,
		a.homeOwnerID(), service.BlogToken, time.Now().UnixMilli()).Scan(&blogDirID)
	if err != nil || blogDirID == "" {
		http.Error(w, "blog not active", http.StatusNotFound)
		return
	}
	files, err := a.collectBlogArticles(r.Context(), blogDirID, currentSiteID(r))
	if err != nil {
		http.Error(w, "blog dir unavailable", http.StatusInternalServerError)
		return
	}
	// 排序：更新时间倒序（normMillis 兼容历史秒级值，避免错序）
	// 2.1 置顶底座：RSS 与 publicPosts/壳侧 SSR 同源（global 置顶 → 时间序）
	sortPublicDirFiles(files)

	// B10-2 对外 origin 统一走 publicBaseURL：生产是 nginx → 127.0.0.1:8780 明文反代，
	// 后端侧 r.TLS 恒为 nil，此前在这里自算 scheme 会把 RSS 内所有链接写成 http。
	// publicBaseURL 读 X-Forwarded-Proto / X-Forwarded-Host，与 sitemap / robots / SSR 页面同源。
	base := a.publicBaseURL(r)
	// 2.7 站点基础域名：blog.base_url 优先（RSS/canonical/OG 同源）；留空=跟随请求 Host
	if bu := strings.TrimSpace(a.cfg.GetString("blog.base_url")); bu != "" {
		base = strings.TrimRight(bu, "/")
	}
	items := make([]string, 0, len(files))
	for _, it := range files {
		f := it.f
		title := xmlEscape(f.Name)
		// B10-2 item link 与 sitemap / 收录回指同址：SSR 文章页 {origin}/{slug}
		// （SPA 深链 /#/p/blog?path=... 对订阅器与搜索引擎不友好）
		link := base + "/" + urlQueryEscape(slugOf(f, it.path))
		desc := xmlEscape(clipPreview(it.preview))
		category := ""
		if idx := strings.IndexByte(it.path, '/'); idx > 0 {
			category = xmlEscape(it.path[:idx])
		}
		pub := time.UnixMilli(normMillis(f.UpdatedAt)).UTC().Format(time.RFC1123Z)
		items = append(items, fmt.Sprintf(
			"<item><title>%s</title><link>%s</link><guid isPermaLink=\"false\">%s</guid><description>%s</description><pubDate>%s</pubDate>%s</item>",
			title, link, service.BlogToken+"#"+it.path, desc, pub, catTag(category)))
	}
	now := time.Now().UTC().Format(time.RFC1123Z)
	lang := a.cfg.GetString("blog.locale")
	if lang == "" {
		lang = "zh-cn"
	}
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
<channel>
<title>` + xmlEscape(a.blogTitle()) + `</title>
<link>` + base + `/blog</link>
<description>` + xmlEscape(a.blogDesc()) + `</description>
<language>` + xmlEscape(lang) + `</language>
<lastBuildDate>` + now + `</lastBuildDate>
<atom:link href="` + base + `/api/v1/blog/feed.xml" rel="self" type="application/rss+xml"/>
` + strings.Join(items, "\n") + `
</channel>
</rss>`
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(feed))
}

// blogTitle/blogDesc 博客站点名称/简介：优先 settings.blog.title / blog.description（站点设置，RSS/OG/公开页同源）。
func (a *API) blogTitle() string {
	if v, ok := a.cfg.Get("blog.title"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return "爱库录"
}

func (a *API) blogDesc() string {
	if v, ok := a.cfg.Get("blog.description"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return "爱库录 · AI 知识库博客：目录即站点，文件即文章"
}

// catTag 生成 <category> 标签（空分类不输出）。
func catTag(c string) string {
	if c == "" {
		return ""
	}
	return "<category>" + c + "</category>"
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func urlQueryEscape(s string) string {
	// 与前端 encodeURIComponent 一致（保留 /，中文按 UTF-8 字节 %XX）
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~', c == '/':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// ---- 插件数据 KV（第三方读写通道；替代自由 DDL，天然插件隔离） ----

const (
	plugKVMaxKeyLen   = 128
	plugKVMaxValueLen = 64 << 10 // 64KB
)

// blogPluginKVGet GET /api/v1/blog/plugins/{id}/kv?key=..（公开读：评论等公开数据）
func (a *API) blogPluginKVGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	key := r.URL.Query().Get("key")
	if key == "" || len(key) > plugKVMaxKeyLen {
		writeErr(w, http.StatusBadRequest, "PLUGIN_KV_BAD_KEY", "key 必填且不超过 128 字符")
		return
	}
	var v string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT value FROM blog_plug_data WHERE plugin_id=? AND key=?`, id, key).Scan(&v)
	if err != nil {
		writeErr(w, http.StatusNotFound, "PLUGIN_KV_MISSING", "key 不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": v})
}

// blogPluginKVList GET /api/v1/blog/plugins/{id}/kv（公开）：?key= 单读，无 key 列出全部
func (a *API) blogPluginKVList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if key := r.URL.Query().Get("key"); key != "" {
		if len(key) > plugKVMaxKeyLen {
			writeErr(w, http.StatusBadRequest, "PLUGIN_KV_BAD_KEY", "key 不超过 128 字符")
			return
		}
		var v string
		err := a.db.QueryRowContext(r.Context(),
			`SELECT value FROM blog_plug_data WHERE plugin_id=? AND key=?`, id, key).Scan(&v)
		if err != nil {
			writeErr(w, http.StatusNotFound, "PLUGIN_KV_MISSING", "key 不存在")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": v})
		return
	}
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT key, value, updated_at FROM blog_plug_data WHERE plugin_id=? ORDER BY key`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PLUGIN_KV_LIST_FAILED", err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var k, v string
		var ts int64
		if rows.Scan(&k, &v, &ts) == nil {
			out = append(out, map[string]any{"key": k, "value": v, "updated_at": ts})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// blogPluginKVSet POST /api/v1/blog/plugins/{id}/kv（登录；upsert）
func (a *API) blogPluginKVSet(w http.ResponseWriter, r *http.Request) {
	// H1 修复：插件 KV（公开可读）写入仅 owner/admin，防任意登录用户污染站点公开配置
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "PLUGIN_KV_BAD_BODY", "请求体解析失败")
		return
	}
	if req.Key == "" || len(req.Key) > plugKVMaxKeyLen {
		writeErr(w, http.StatusBadRequest, "PLUGIN_KV_BAD_KEY", "key 必填且不超过 128 字符")
		return
	}
	if len(req.Value) > plugKVMaxValueLen {
		writeErr(w, http.StatusBadRequest, "PLUGIN_KV_TOO_BIG", "value 不超过 64KB")
		return
	}
	var cnt int
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM blog_plugins WHERE id=?`, id).Scan(&cnt); err != nil || cnt == 0 {
		writeErr(w, http.StatusNotFound, "PLUGIN_NOT_FOUND", "插件不存在")
		return
	}
	_, err := a.db.ExecContext(r.Context(),
		`INSERT INTO blog_plug_data (plugin_id, key, value, updated_at) VALUES (?,?,?,?)
		 ON CONFLICT(plugin_id, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		id, req.Key, req.Value, time.Now().UnixMilli())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PLUGIN_KV_SET_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": req.Key})
}

// blogPluginKVDelete DELETE /api/v1/blog/plugins/{id}/kv?key=..（登录）
func (a *API) blogPluginKVDelete(w http.ResponseWriter, r *http.Request) {
	// H1 修复：插件 KV 删除仅 owner/admin
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	key := r.URL.Query().Get("key")
	if key == "" {
		writeErr(w, http.StatusBadRequest, "PLUGIN_KV_BAD_KEY", "key 必填")
		return
	}
	_, err := a.db.ExecContext(r.Context(),
		`DELETE FROM blog_plug_data WHERE plugin_id=? AND key=?`, id, key)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PLUGIN_KV_DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// pluginSettingsSchema GET /api/v1/blog/plugins/{id}/settings-schema：插件动态设置表单声明。
// 应用中心安装带 settings_schema 的插件（如 RSS 的条目数/含摘要）后，前端据此渲染配置表单。
func (a *API) pluginSettingsSchema(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var schema string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT settings_schema FROM blog_plugins WHERE id=?`, id).Scan(&schema)
	if err != nil || schema == "" {
		writeErr(w, http.StatusNotFound, "PLUGIN_NOT_FOUND", "插件不存在或未声明设置")
		return
	}
	var out []any
	_ = json.Unmarshal([]byte(schema), &out)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "settings_schema": out})
}

// ---- 博客文章入库（Aikdex/采集投递稳定 API） ----

// blogPostCreate POST /api/v1/blog/posts（登录）
// 请求：{title, category?, content, front_matter?} → 写入博客目录并置 published（发布即对外可见）。
// category 为博客分类（子目录名）；不存在则自动创建。返回 {id, name, path}。
func (a *API) blogPostCreate(w http.ResponseWriter, r *http.Request) {
	// 多用户：仅站点管理员或授权作者可发布（owner/admin 天然放行）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	var req struct {
		Title       string         `json:"title"`
		Category    string         `json:"category"`
		Content     string         `json:"content"`
		FrontMatter map[string]any `json:"front_matter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BLOG_POST_BAD_BODY", "请求体解析失败")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeErr(w, http.StatusBadRequest, "BLOG_POST_NO_TITLE", "title 必填")
		return
	}
	if len([]rune(req.Title)) > 200 {
		writeErr(w, http.StatusBadRequest, "BLOG_POST_TITLE_TOO_LONG", "title 不超过 200 字符")
		return
	}
	ownerID := a.homeOwnerID()
	spaceID := a.homeSpaceID()

	// 分类目录（可选）：不存在则自动创建（分类 = 博客目录直接子目录）
	parentID := service.BlogDirID
	catName := strings.Trim(strings.TrimSpace(req.Category), "/")
	if catName != "" {
		if strings.ContainsAny(catName, "/\\") {
			writeErr(w, http.StatusBadRequest, "BLOG_POST_BAD_CATEGORY", "category 只能是一级分类名，不能含 /")
			return
		}
		// 查现有分类
		var cid string
		err := a.db.QueryRowContext(r.Context(),
			`SELECT id FROM files WHERE parent_id=? AND kind='dir' AND name=? AND deleted_at IS NULL`,
			service.BlogDirID, catName).Scan(&cid)
		if err != nil {
			dir, err := a.files.CreateDir(r.Context(), ownerID, spaceID, service.BlogDirID, catName, currentSiteID(r))
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "BLOG_POST_CAT_FAILED", err.Error())
				return
			}
			cid = dir.ID
		}
		parentID = cid
	}

	name := req.Title + ".md"
	doc, err := a.files.CreateDoc(r.Context(), ownerID, spaceID, parentID, name, req.Content, currentSiteID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "BLOG_POST_CREATE_FAILED", err.Error())
		return
	}
	// 多用户：记录文章作者（空 author_id 在展示时回落 owner；此处写实际发布者）
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE files SET author_id=? WHERE id=?`, a.curUserID(r), doc.ID); err != nil {
		log.Printf("[blogPostCreate] set author_id failed: %v", err)
	}
	// 发布（默认草稿 → published，对外可见）
	if err := a.setFileStatusInternal(r.Context(), doc.ID, "published"); err != nil {
		writeErr(w, http.StatusInternalServerError, "BLOG_POST_PUBLISH_FAILED", err.Error())
		return
	}
	// 生成公开稳定链接 slug（同名冲突自动后缀；之后改名不碎链）
	slug, _ := service.EnsureFileSlug(r.Context(), a.db, doc.ID, doc.Name)
	path := doc.Name
	if catName != "" {
		path = catName + "/" + doc.Name
	}
	_, _ = a.aud.Append(r.Context(), ownerID, "blog.post_create", "files", map[string]any{
		"id": doc.ID, "name": name, "path": path, "category": catName, "slug": slug,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": doc.ID, "name": name, "path": path, "slug": slug,
	})
}

// setFileStatusInternal 置文件状态（published/draft；博客发布用）
func (a *API) setFileStatusInternal(ctx context.Context, id, status string) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE files SET content_state=json_set(COALESCE(content_state,'{}'), '$.status', ?), updated_at=? WHERE id=?`,
		status, time.Now().UnixMilli(), id)
	return err
}
