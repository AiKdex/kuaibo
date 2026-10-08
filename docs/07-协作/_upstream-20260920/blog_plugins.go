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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// pluginManifest 博客插件声明（协议字段；第三方按此注册）。
type pluginManifest struct {
	ID             string       `json:"id"`                          // 唯一 id（插件名，如 "blog-comments"）
	Name           string       `json:"name"`                        // 展示名
	Version        string       `json:"version"`                     // 语义化版本
	Description    string       `json:"description"`                 // 一句话说明
	Author         string       `json:"author"`                      // 作者/组织
	MountPoints    []string     `json:"mount_points"`                // 挂载点：post_bottom|sidebar|list_item|head
	APIPermissions []string     `json:"api_permissions"`             // 数据权限：blog.read / blog.write / event:post
	FrontendEntry  string       `json:"frontend_entry,omitempty"`    // 前端组件注册键（与前端注册表对应）
	MinCoreVersion string       `json:"min_core_version,omitempty"`  // 要求的最低主系统版本（如 "1.0.0"）
	SettingsSchema []any        `json:"settings_schema,omitempty"`   // 动态设置表单声明（[{key,label,type,default,options}]）
	Enabled        *bool        `json:"enabled,omitempty"`           // 缺省 true
	Hooks          *pluginHooks `json:"hooks,omitempty"`             // 事件契约 v1：subscribe/publish 白名单（能力插件 v2 增量）
	// ---- 能力插件协议 v2（docs/能力插件协议-v2草案.md；kind 空=v1.1 "ui" 兼容）----
	Kind         string   `json:"kind,omitempty"`         // "ui"（v1.1 默认）| "capacity"
	Capabilities []string `json:"capabilities,omitempty"` // collector|channel|im|rag|notify|webhook（kind=capacity 必填）
	BackendEntry string   `json:"backend_entry,omitempty"` // 后端实现入口键（kind=capacity 必填；内核按此键加载/校验）
	Routes       []string `json:"routes,omitempty"`        // 允许挂载的路由前缀白名单（仅 /api/v1/plugins/{plugin_id}/ 前缀）
	MinSchema    int      `json:"min_schema,omitempty"`    // schema 版本下限（0=不限）
	MaxSchema    *int     `json:"max_schema,omitempty"`    // schema 版本上限（nil=不限）
}

// pluginHooks 事件订阅/发布声明（docs/事件总线契约.md §3 权限模型）。
type pluginHooks struct {
	Subscribe []string `json:"subscribe,omitempty"` // 订阅 topic 白名单：内核冻结清单或 {plugin_id}.{event}
	Publish   []string `json:"publish,omitempty"`   // 发布 topic 白名单（需 api_permissions 含 event:post）
}

// coreVersion 主系统核心版本（插件兼容矩阵基准；与 /api/v1/version 对齐）
const coreVersion = "1.0.0"

// builtinFrontendEntries 主系统内置前端插件组件清单（与 web/src/blogPlugins/builtin.js 注册表对齐）。
// frontend_entry 一致性校验：注册 manifest 时若声明了 frontend_entry，必须命中此清单（组件随主系统构建），
// 否则返回 422 PLUGIN_COMPONENT_NOT_REGISTERED（防止"登记了但前端渲染不出来"的悬空插件）。
// 纯后端插件（无前端组件，如 RSS 定制）frontend_entry 留空即可。
var builtinFrontendEntries = map[string]bool{
	"blog-meta-footer": true,
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
		`SELECT id, name, version, description, author, mount_points, api_permissions, frontend_entry, min_core_version, settings_schema, hooks, kind, capabilities, backend_entry, routes, min_schema, max_schema, enabled
		 FROM blog_plugins ORDER BY created_at ASC`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PLUGIN_LIST_FAILED", err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, ver, desc, author, mp, perm, fe, mcv, ss, hk, kind, caps, be, rts string
		var minSchema int
		var maxSchema *int
		var enabled int
		if rows.Scan(&id, &name, &ver, &desc, &author, &mp, &perm, &fe, &mcv, &ss, &hk, &kind, &caps, &be, &rts, &minSchema, &maxSchema, &enabled) != nil {
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
		capsA := []string{}
		_ = json.Unmarshal([]byte(caps), &capsA)
		rtsA := []string{}
		_ = json.Unmarshal([]byte(rts), &rtsA)
		out = append(out, map[string]any{
			"id": id, "name": name, "version": ver, "description": desc, "author": author,
			"mount_points": mpA, "api_permissions": permA, "frontend_entry": fe,
			"min_core_version": mcv, "settings_schema": ssA, "hooks": hkO, "enabled": enabled == 1,
			"kind": kind, "capabilities": capsA, "backend_entry": be, "routes": rtsA,
			"min_schema": minSchema, "max_schema": maxSchema,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// blogPluginUpsert POST /api/v1/blog/plugins 注册/更新插件（登录；幂等 upsert by id）。
func (a *API) blogPluginUpsert(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	var m pluginManifest
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil || m.ID == "" {
		writeErr(w, http.StatusBadRequest, "PLUGIN_BAD_MANIFEST", "插件 id 必填，且须为合法 JSON manifest")
		return
	}
	if m.MinCoreVersion != "" && !versionAtLeast(coreVersion, m.MinCoreVersion) {
		writeErr(w, http.StatusConflict, "PLUGIN_CORE_TOO_OLD",
			"主系统版本 "+coreVersion+" 低于插件要求的 "+m.MinCoreVersion)
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
	// 能力插件协议 v2：kind/capabilities/routes/schema 区间校验（docs/能力插件协议-v2草案.md §1、§4）
	if code, msg := validatePluginV2(m, a.dbSchemaVersion()); code != "" {
		writeErr(w, http.StatusUnprocessableEntity, code, msg)
		return
	}
	if err := a.registerPlugin(r, &m); err != nil {
		writeErr(w, http.StatusInternalServerError, "PLUGIN_UPSERT_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": m.ID})
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

// validatePluginV2 能力插件协议 v2 校验（docs/能力插件协议-v2草案.md §1、§4）。
// kind 空 = v1.1 "ui" 兼容（跳过 v2 强约束）；返回 (错误码, 错误消息)；合法 ("", "")。
func validatePluginV2(m pluginManifest, schemaVer int) (string, string) {
	kind := m.Kind
	if kind == "" {
		kind = "ui"
	}
	if kind != "ui" && kind != "capacity" {
		return "PLUGIN_KIND_INVALID", "kind 仅支持 ui（v1.1 默认）| capacity（能力插件 v2）"
	}
	// capabilities 白名单（能力枚举：collector|channel|im|rag|notify|webhook|media|office|storage|transcode|thumbnail|search）
	allowedCap := map[string]bool{"collector": true, "channel": true, "im": true, "rag": true, "notify": true, "webhook": true, "media": true, "office": true, "storage": true, "transcode": true, "thumbnail": true, "search": true}
	for _, c := range m.Capabilities {
		if !allowedCap[c] {
			return "PLUGIN_CAPABILITY_INVALID", "capability " + c + " 不在白名单（collector|channel|im|rag|notify|webhook|media|office|storage|transcode|thumbnail|search）"
		}
	}
	if kind == "capacity" {
		if len(m.Capabilities) == 0 {
			return "PLUGIN_CAPABILITY_MISSING", "kind=capacity 必须声明至少一个 capability"
		}
		if m.BackendEntry == "" {
			return "PLUGIN_BACKEND_MISSING", "kind=capacity 必须声明 backend_entry（后端实现入口键）"
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

// registerPlugin 将 manifest 写入 blog_plugins（幂等 upsert）。供注册 API 与市场安装共用。
func (a *API) registerPlugin(r *http.Request, m *pluginManifest) error {
	mp, _ := json.Marshal(m.MountPoints)
	perm, _ := json.Marshal(m.APIPermissions)
	ss, _ := json.Marshal(m.SettingsSchema)
	hk := ""
	if m.Hooks != nil {
		hb, _ := json.Marshal(m.Hooks)
		hk = string(hb)
	}
	caps, _ := json.Marshal(m.Capabilities)
	rts, _ := json.Marshal(m.Routes)
	kind := m.Kind
	if kind == "" {
		kind = "ui"
	}
	enabled := 1
	if m.Enabled != nil && !*m.Enabled {
		enabled = 0
	}
	now := time.Now().UnixMilli()
	_, err := a.db.ExecContext(r.Context(),
		`INSERT INTO blog_plugins (id, name, version, description, author, mount_points, api_permissions, frontend_entry, min_core_version, settings_schema, hooks, kind, capabilities, backend_entry, routes, min_schema, max_schema, enabled, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, version=excluded.version, description=excluded.description,
		   author=excluded.author, mount_points=excluded.mount_points, api_permissions=excluded.api_permissions,
		   frontend_entry=excluded.frontend_entry, min_core_version=excluded.min_core_version,
		   settings_schema=excluded.settings_schema, hooks=excluded.hooks,
		   kind=excluded.kind, capabilities=excluded.capabilities, backend_entry=excluded.backend_entry,
		   routes=excluded.routes, min_schema=excluded.min_schema, max_schema=excluded.max_schema,
		   enabled=excluded.enabled, updated_at=excluded.updated_at`,
		m.ID, m.Name, m.Version, m.Description, m.Author, string(mp), string(perm), m.FrontendEntry, m.MinCoreVersion, string(ss), hk,
		kind, string(caps), m.BackendEntry, string(rts), m.MinSchema, m.MaxSchema, enabled, now, now)
	return err
}

// blogPluginToggle POST /api/v1/blog/plugins/{id}/toggle 启用/禁用（登录）。
func (a *API) blogPluginToggle(w http.ResponseWriter, r *http.Request) {
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
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM blog_plugins WHERE id=?`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PLUGIN_DELETE_FAILED", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "PLUGIN_NOT_FOUND", "插件不存在")
		return
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
		a.curUserID(r), service.BlogToken, time.Now().UnixMilli()).Scan(&blogDirID)
	if err != nil || blogDirID == "" {
		http.Error(w, "blog not active", http.StatusNotFound)
		return
	}
	files, err := a.collectDirFiles(r.Context(), blogDirID)
	if err != nil {
		http.Error(w, "blog dir unavailable", http.StatusInternalServerError)
		return
	}
	// 2.1 置顶底座：RSS 与 publicPosts/壳侧 SSR 同源（global 置顶 → 时间序）
	sortPublicDirFiles(files)

	base := a.publicBase(r)
	items := make([]string, 0, len(files))
	for _, it := range files {
		f := it.f
		title := xmlEscape(f.Name)
		link := base + "/#/p/" + service.BlogToken + "?path=" + urlQueryEscape(it.path)
		desc := xmlEscape(clipPreview(it.preview))
		category := ""
		if idx := strings.IndexByte(it.path, '/'); idx > 0 {
			category = xmlEscape(it.path[:idx])
		}
		pub := time.UnixMilli(f.UpdatedAt).UTC().Format(time.RFC1123Z)
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
<link>` + base + `/#/blog?view=public</link>
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
	return "AiKmap 博客"
}

func (a *API) blogDesc() string {
	if v, ok := a.cfg.Get("blog.description"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return "由 AiKmap 知识库发布的公开文章"
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

// ---- 博客文章入库（Aikdex/采集投递稳定 API） ----

// blogPostCreate POST /api/v1/blog/posts（登录）
// 请求：{title, category?, content, front_matter?} → 写入博客目录并置 published（发布即对外可见）。
// category 为博客分类（子目录名）；不存在则自动创建。返回 {id, name, path}。
func (a *API) blogPostCreate(w http.ResponseWriter, r *http.Request) {
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
	// 多作者：文章固定落博客目录所在空间（owner 空间），owner_id=空间归属者；
	// author_id=实际作者（记录到 files.author_id），保证作者可管理自己的文章。
	ownerID := a.homeOwnerID()
	spaceID := a.homeSpaceID()
	actorID := a.curUserID(r)

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
			dir, err := a.files.CreateDir(r.Context(), ownerID, spaceID, service.BlogDirID, catName)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "BLOG_POST_CAT_FAILED", err.Error())
				return
			}
			cid = dir.ID
		}
		parentID = cid
	}

	name := req.Title + ".md"
	doc, err := a.files.CreateDoc(r.Context(), ownerID, spaceID, parentID, name, req.Content)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "BLOG_POST_CREATE_FAILED", err.Error())
		return
	}
	// 记作者归属（author_id；owner 发布时等于自身，语义一致）
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE files SET author_id=? WHERE id=?`, actorID, doc.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "BLOG_POST_AUTHOR_FAILED", "记录作者失败")
		return
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
	_, _ = a.aud.Append(r.Context(), actorID, "blog.post_create", "files", map[string]any{
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
