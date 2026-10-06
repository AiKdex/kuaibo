// apps_market.go 应用市场 v0.2：zip/manifest 安装（免费应用）。
// 协议见 docs/PLUGIN-MARKET.md。安装 = 校验 manifest 并登记到 blog_plugins。
package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// appInstallZip POST /api/v1/admin/apps/install-zip
// multipart 字段 file=zip；或 Content-Type application/json 的 {manifest:{...}}
func (a *API) appInstallZip(w http.ResponseWriter, r *http.Request) {
	// H2 修复：zip/manifest 安装会写插件注册表与主题目录，仅站长（owner/admin）可执行
	// （纵深防御：路由级 /api/v1/admin/ 前缀已统一拦截，此处再加 handler 层守卫）。
	if !a.blogAdminOnly(w, r) {
		return
	}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		var body struct {
			Manifest json.RawMessage `json:"manifest"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Manifest) == 0 {
			writeErr(w, http.StatusBadRequest, "APP_BAD_BODY", "需要 manifest JSON")
			return
		}
		a.installManifestJSON(w, r, body.Manifest, "")
		return
	}

	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "APP_BAD_MULTIPART", err.Error())
		return
	}
	fh, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "APP_NO_FILE", "缺少 file 字段（zip）")
		return
	}
	defer fh.Close()
	raw, err := io.ReadAll(io.LimitReader(fh, 8<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "APP_READ_FAILED", err.Error())
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "APP_BAD_ZIP", "无法解析 zip")
		return
	}
	var manifest []byte
	for _, zf := range zr.File {
		name := strings.ReplaceAll(zf.Name, "\\", "/")
		if name == "manifest.json" || strings.HasSuffix(name, "/manifest.json") {
			rc, err := zf.Open()
			if err != nil {
				continue
			}
			manifest, _ = io.ReadAll(io.LimitReader(rc, 1<<20))
			rc.Close()
			break
		}
	}
	if len(manifest) == 0 {
		writeErr(w, http.StatusBadRequest, "APP_NO_MANIFEST", "zip 内未找到 manifest.json")
		return
	}
	// 主题包：把声明式资产投放到 data/themes/<id>/，与在线安装同链路，
	// 落盘即生效（SSR 侧请求时读文件），无需重新编译主系统。
	// 上游 v2 语义兼容：市场包 manifest kind 可能是 ui/capacity，包内含 ssr.css 即按主题处理
	// （在线链路由索引分类强制覆盖 kind，本地直装无索引信息，以包内资产实证判定）。
	var pm pluginManifest
	isTheme := false
	if err := json.Unmarshal(manifest, &pm); err == nil && pm.ID != "" {
		// 源包分流（SPEC-SP-001）：kind=source-pack 或包内带 sources/ 目录 → 走采集源导入链路
		if pm.Kind == service.SourcePackKind || zipHasDir(zr, "sources") {
			a.installSourcePack(w, r, raw, nil)
			return
		}
		isTheme = pm.Kind == "theme"
		if !isTheme && zipHasRootFile(zr, "ssr.css") {
			isTheme = true
		}
		if isTheme {
			pm.Kind = "theme" // 归一：登记进 blog_plugins 用 kind=theme（否则被 upsert 归一成 plugin）
			files, err := deployThemeAssets(zr, pm.ID)
			if err != nil {
				writeErr(w, http.StatusUnprocessableEntity, "APP_THEME_DEPLOY_FAILED", err.Error())
				return
			}
			if len(files) == 0 {
				writeErr(w, http.StatusUnprocessableEntity, "APP_THEME_EMPTY",
					"主题包不含可用资产（需 ssr.css 或 page.html）；请按《博客主题开发规范》打包")
				return
			}
			log.Printf("本地 zip 安装：主题 %s 已投放 %v", pm.ID, files)
		}
	}
	kindOverride := ""
	if isTheme {
		kindOverride = "theme"
	}
	a.installManifestJSON(w, r, manifest, kindOverride)
}

func (a *API) installManifestJSON(w http.ResponseWriter, r *http.Request, raw []byte, kindOverride string) {
	var m pluginManifest
	if err := json.Unmarshal(raw, &m); err != nil || m.ID == "" {
		writeErr(w, http.StatusBadRequest, "APP_BAD_MANIFEST", "manifest 无效或缺 id")
		return
	}
	if m.MinCoreVersion != "" && !versionAtLeast(coreVersion, m.MinCoreVersion) {
		writeErr(w, http.StatusConflict, "PLUGIN_CORE_TOO_OLD",
			"主系统版本 "+coreVersion+" 低于应用要求的 "+m.MinCoreVersion)
		return
	}
	// 插件（非主题）声明了前端组件 → 必须命中内置注册表；主题包豁免（声明式资产走 data/themes 投放，
	// 无前端组件依赖；与在线 marketInstall 的豁免口径一致，避免主题包先落盘后 422）
	if kindOverride != "" {
		m.Kind = kindOverride // 主题判定覆盖（zip kind=ui 上游语义 → AiKlog 归一 theme）
	}
	if m.Kind != "theme" && m.FrontendEntry != "" && !builtinFrontendEntries[m.FrontendEntry] {
		writeErr(w, http.StatusUnprocessableEntity, "PLUGIN_COMPONENT_NOT_REGISTERED",
			"前端组件 "+m.FrontendEntry+" 未在主系统注册（当前仅支持内置组件入口）")
		return
	}
	// 免费应用阶段：仅允许未声明价格或 0（协议字段可后续扩展）
	if m.Enabled == nil {
		t := true
		m.Enabled = &t
	}
	a.upsertPluginManifest(w, r, &m, "app.install")
}

// zipHasDir 判定 zip 包内是否含指定顶层目录（源包识别：sources/ 目录存在即按源包处理）。
func zipHasDir(zr *zip.Reader, dir string) bool {
	prefix := dir + "/"
	for _, zf := range zr.File {
		n := strings.TrimPrefix(strings.ReplaceAll(zf.Name, "\\", "/"), "./")
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

// zipHasRootFile 判定 zip 包内是否含主题有效资产（能经 themeAssetMap 映射出 ssr.css 或 page.html；
// 兼容根目录、theme/ 子目录与上游 theme.css 命名——与 deployThemeAssets 的白名单口径一致）。
func zipHasRootFile(zr *zip.Reader, _ string) bool {
	for _, zf := range zr.File {
		n := strings.TrimPrefix(strings.ReplaceAll(zf.Name, "\\", "/"), "./")
		dest, ok := themeAssetMap[n]
		if ok && !zf.FileInfo().IsDir() && (dest == "ssr.css" || dest == "page.html") {
			return true
		}
	}
	return false
}
