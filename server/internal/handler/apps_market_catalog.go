// apps_market_catalog.go 应用中心（在线目录）：浏览远程市场索引 + 一键安装。
//
// 与 apps_market.go（本地 zip 安装）互补：本文件负责"从应用中心在线目录拉取目录 →
// 一键下载安装"，复用现有 blog_plugins 引擎（免费应用阶段；付费需 pro 许可，当前恒为 free）。
// 协议与 AiKmap 应用市场对齐（index.json：plugins[]/themes[]，含 download_url+sha256+tier+target）。
package handler

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// marketIndex 市场索引协议（index.json）。
type marketIndex struct {
	SchemaVersion string         `json:"schema_version"`
	Plugins       []marketPlugin `json:"plugins"`
	Themes        []marketTheme  `json:"themes"`
}

// marketPlugin 目录中的插件条目（含服务端补充字段 installed/applicable）。
type marketPlugin struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Description     string   `json:"description"`
	Author          string   `json:"author"`
	DownloadURL     string   `json:"download_url"`
	SHA256          string   `json:"sha256"`
	Tier            string   `json:"tier"` // free|paid（付费预留；安装需 pro license）
	Cover           string   `json:"cover,omitempty"` // 市场卡片封面图 URL（可选）
	Target          []string `json:"target,omitempty"`
	MinCoreVersion  string   `json:"min_core_version,omitempty"`
	SettingsSchema  []any    `json:"settings_schema,omitempty"`
	// 服务端补充
	Installed  bool `json:"installed"`
	Applicable bool `json:"applicable"`
}

// marketTheme 目录中的主题条目（安装后登记进 blog_plugins，kind=theme）。
type marketTheme struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	DownloadURL string   `json:"download_url"`
	SHA256      string   `json:"sha256"`
	Tier        string   `json:"tier,omitempty"`  // free|paid（付费预留）
	Cover       string   `json:"cover,omitempty"` // 市场卡片封面图 URL（可选）
	Target      []string `json:"target,omitempty"`
	// 服务端补充
	Installed  bool `json:"installed"`
	Applicable bool `json:"applicable"`
}

// marketCache 市场索引内存缓存（5 分钟）。
var marketCache struct {
	at    time.Time
	index *marketIndex
	url   string
}

// marketIndexURL 返回市场索引地址（配置可覆盖）。
func (a *API) marketIndexURL() string {
	if u := a.cfg.GetString("plugin_market.index_url"); u != "" {
		return u
	}
	return "https://aikmap.cn/market/index.json" // 上游 2026-09-17 应用中心独立化：主系统统一索引（market.aikmap.com 归 kmap 自身使用，aikmap.aiai1.cn 旧源已下线）
}

// shellID 本实例壳标识（应用中心多壳模型：插件 target 含本壳才可安装）。
func (a *API) shellID() string {
	if s := a.cfg.GetString("plugin_market.shell_id"); s != "" {
		return s
	}
	return "aiklog"
}

// licenseEdition 移入 license.go（2026-09-17：license.key 有效 → pro；system.edition 覆盖；否则 community）。

// marketList GET /api/v1/admin/apps/market：市场插件/主题列表（缓存 5 分钟；失败返回错误）。
// 返回前补充 installed 标记（对照本实例已注册插件）与 applicable 标记（target 含本壳或通用）。
func (a *API) marketList(w http.ResponseWriter, r *http.Request) {
	u := a.marketIndexURL()
	if time.Since(marketCache.at) < 5*time.Minute && marketCache.index != nil && marketCache.url == u {
		writeJSON(w, http.StatusOK, a.marketView(marketCache.index))
		return
	}
	idx, err := a.fetchMarketIndex(r)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "MARKET_UNREACHABLE", "市场索引不可达："+err.Error())
		return
	}
	marketCache.index = idx
	marketCache.at = time.Now()
	marketCache.url = u
	writeJSON(w, http.StatusOK, a.marketView(idx))
}

// marketView 组装市场视图：installed=本实例已注册；applicable=本壳可安装（target 空或含本壳）。
func (a *API) marketView(idx *marketIndex) map[string]any {
	installed := map[string]bool{}
	if rows, err := a.db.Query(`SELECT id FROM blog_plugins`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				installed[id] = true
			}
		}
	}
	shell := a.shellID()
	out := &marketIndex{SchemaVersion: idx.SchemaVersion}
	for i := range idx.Plugins {
		p := idx.Plugins[i]
		p.Installed = installed[p.ID]
		p.Applicable = len(p.Target) == 0 || containsStr(p.Target, shell)
		out.Plugins = append(out.Plugins, p)
	}
	for i := range idx.Themes {
		t := idx.Themes[i]
		t.Installed = installed[t.ID]
		t.Applicable = len(t.Target) == 0 || containsStr(t.Target, shell)
		out.Themes = append(out.Themes, t)
	}
	return map[string]any{
		"schema_version": idx.SchemaVersion,
		"plugins":        out.Plugins,
		"themes":         out.Themes,
		"third_party":    a.thirdPartyListings(),
		"index_url":      a.marketIndexURL(),
		"shell":          shell,
		"edition":        a.licenseEdition(),
	}
}

// thirdPartyListings 已通过审核的第三方开发者条目（应用中心「社区应用」分区数据源）。
func (a *API) thirdPartyListings() []marketListing {
	rows, err := a.db.Query(`SELECT id, developer_id, kind, name, description, icon, screenshots, version,
		download_url, sha256, price_cents, tier, status, review_note, created_at, updated_at
		FROM market_listings WHERE status='approved' ORDER BY updated_at DESC`)
	if err != nil {
		return []marketListing{}
	}
	defer rows.Close()
	out := []marketListing{}
	for rows.Next() {
		m, err := scanListingRow(rows, false)
		if err != nil {
			continue
		}
		out = append(out, *m)
	}
	return out
}

// marketInstall POST /api/v1/admin/apps/market/install：安装市场插件/主题。
// body: {"id":"..."}（从市场索引取 download_url+sha256）或 {"url":"...","sha256":"...","manifest":{...}}（直接安装）。
func (a *API) marketInstall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        string `json:"id"`
		ListingID string `json:"listing_id"`
		URL       string `json:"url"`
		SHA       string `json:"sha256"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "MARKET_BAD_REQ", "无效请求体")
		return
	}
	dlURL, sha := req.URL, strings.ToLower(req.SHA)
	var kind string
	var indexSS []any
	// 按 id 从市场索引解析
	if req.ID != "" {
		idx, err := a.fetchMarketIndex(r)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "MARKET_UNREACHABLE", err.Error())
			return
		}
		var found *marketPlugin
		for i := range idx.Plugins {
			if idx.Plugins[i].ID == req.ID {
				found = &idx.Plugins[i]
				kind = "plugin"
				indexSS = found.SettingsSchema
				break
			}
		}
		if found == nil {
			for i := range idx.Themes {
				if idx.Themes[i].ID == req.ID {
					t := idx.Themes[i]
					found = &marketPlugin{
						ID: t.ID, Name: t.Name, Version: t.Version, Description: t.Description,
						Author: t.Author, DownloadURL: t.DownloadURL, SHA256: t.SHA256, Target: t.Target,
					}
					kind = "theme"
					break
				}
			}
		}
		if found == nil {
			writeErr(w, http.StatusNotFound, "MARKET_NOT_FOUND", "市场未收录该应用："+req.ID)
			return
		}
		// 多壳模型：target 非空且不含本壳 → 拒绝
		if len(found.Target) > 0 && !containsStr(found.Target, a.shellID()) {
			writeErr(w, http.StatusUnprocessableEntity, "MARKET_TARGET_MISMATCH",
				"该应用不适用于本壳（target="+strings.Join(found.Target, ",")+"，本壳="+a.shellID()+"）")
			return
		}
		if found.Tier == "paid" && a.licenseEdition() != "pro" {
			writeErr(w, http.StatusPaymentRequired, "MARKET_LICENSE_REQUIRED",
				"该应用为付费应用（tier=paid），需要 pro 许可证（请先在设置中激活 license key）")
			return
		}
		dlURL, sha = found.DownloadURL, strings.ToLower(found.SHA256)
	}
	// 第三方条目（已审核通过）：从 market_listings 取下载地址+校验和（开发者碰不到官方索引）
	if dlURL == "" && req.ListingID != "" {
		var dl, shaDB, kindDB, tierDB string
		lerr := a.db.QueryRowContext(r.Context(),
			`SELECT download_url, sha256, kind, tier FROM market_listings WHERE id=? AND status='approved'`, req.ListingID).
			Scan(&dl, &shaDB, &kindDB, &tierDB)
		if lerr != nil {
			writeErr(w, http.StatusNotFound, "LISTING_NOT_FOUND", "第三方条目不存在或未通过审核")
			return
		}
		if dl == "" {
			writeErr(w, http.StatusBadRequest, "LISTING_NO_PACKAGE", "该条目尚未上传应用包")
			return
		}
		dlURL, sha, kind = dl, strings.ToLower(shaDB), kindDB
		if tierDB == "paid" && a.licenseEdition() != "pro" {
			writeErr(w, http.StatusPaymentRequired, "MARKET_LICENSE_REQUIRED", "该应用为付费应用（tier=paid），需要 pro 许可证")
			return
		}
	}
	if dlURL == "" {
		writeErr(w, http.StatusBadRequest, "MARKET_BAD_REQ", "缺少下载地址（url / 市场 id / 第三方 listing_id）")
		return
	}
	// 下载（限流 16MiB）
	resp, err := http.Get(dlURL)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "MARKET_DL_FAIL", "应用包下载失败")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeErr(w, http.StatusBadGateway, "MARKET_DL_HTTP", fmt.Sprintf("应用包 HTTP %d", resp.StatusCode))
		return
	}
	tmp, err := os.CreateTemp("", "aiklog-app-*.zip")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(resp.Body, 16<<20)); err != nil {
		writeErr(w, http.StatusBadGateway, "MARKET_DL_FAIL", "应用包下载不完整")
		return
	}
	tmp.Close()
	// SHA-256 校验（市场声明了 sha 时必须匹配）
	if sha != "" {
		h := sha256.New()
		fh, _ := os.Open(tmp.Name())
		_, _ = io.Copy(h, fh)
		fh.Close()
		if got := hex.EncodeToString(h.Sum(nil)); got != sha {
			writeErr(w, http.StatusUnprocessableEntity, "MARKET_SHA_MISMATCH",
				fmt.Sprintf("SHA-256 校验失败：期望 %s，实际 %s", sha, got))
			return
		}
	}
	// 解包并校验 manifest
	m, err := installPluginZip(tmp.Name())
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "MARKET_BAD_PACKAGE", err.Error())
		return
	}
	// 应用中心来的类型：按索引分类（theme/plugin）强制覆盖 zip 的 kind（AiKmap 用 ui/capacity 语义）；
	// 直装（url 模式，kind 为空）则保留 zip 声明，由 upsertPluginManifest 归一。
	if kind != "" {
		m.Kind = kind
	}
	// 索引声明的 settings_schema 优先于 zip（zip 缺省时回退索引）
	if len(m.SettingsSchema) == 0 && len(indexSS) > 0 {
		m.SettingsSchema = indexSS
	}
	// 插件（非主题）声明了前端组件 → 必须命中内置注册表
	if m.Kind != "theme" && m.FrontendEntry != "" && !builtinFrontendEntries[m.FrontendEntry] {
		writeErr(w, http.StatusUnprocessableEntity, "PLUGIN_COMPONENT_NOT_REGISTERED",
			"前端组件 "+m.FrontendEntry+" 未在主系统注册（组件需随主系统构建）")
		return
	}
	// upsertPluginManifest 自行写回响应（成功/失败均含 ok 与 id）；失败已写错误，无需重复写
	// 主题：把包内声明式资产投放到 data/themes/<id>/（免编译热投放 → 装完即可在公网 /blog 生效）
	if m.Kind == "theme" {
		zr, err := zip.OpenReader(tmp.Name())
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "MARKET_BAD_PACKAGE", "主题包无法读取："+err.Error())
			return
		}
		files, err := deployThemeAssets(&zr.Reader, m.ID)
		zr.Close()
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "MARKET_THEME_DEPLOY_FAILED", err.Error())
			return
		}
		if len(files) == 0 {
			writeErr(w, http.StatusUnprocessableEntity, "MARKET_THEME_EMPTY",
				"主题包不含可用资产（需 ssr.css 或 page.html）；请主题开发者按《博客主题开发规范》打包")
			return
		}
		log.Printf("应用中心：主题 %s 已投放 %v → %s", m.ID, files, filepath.Join(themesRoot(), m.ID))
	}
	a.upsertPluginManifest(w, r, m, "app.install")
}

// fetchMarketIndex 拉取市场索引（不走缓存，供安装校验用）。
func (a *API) fetchMarketIndex(_ *http.Request) (*marketIndex, error) {
	u := a.marketIndexURL()
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var idx marketIndex
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("市场索引格式非法")
	}
	return &idx, nil
}

// installPluginZip 解包应用 zip 并校验：manifest.json（zip slip 防护）+ 版本；返回 manifest。
func installPluginZip(zipPath string) (*pluginManifest, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("应用包不是合法 zip：%w", err)
	}
	defer zr.Close()
	var m *pluginManifest
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
			return nil, fmt.Errorf("应用包含非法路径：%s", f.Name)
		}
		if name == "manifest.json" || strings.HasSuffix(name, "/manifest.json") {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("manifest 读取失败：%w", err)
			}
			b, err := io.ReadAll(io.LimitReader(rc, 1<<20))
			rc.Close()
			if err != nil {
				return nil, err
			}
			var pm pluginManifest
			if err := json.Unmarshal(b, &pm); err != nil {
				return nil, fmt.Errorf("manifest.json 非法 JSON")
			}
			if pm.ID == "" {
				return nil, fmt.Errorf("manifest 缺少 id")
			}
			m = &pm
			break
		}
	}
	if m == nil {
		return nil, fmt.Errorf("应用包缺少 manifest.json")
	}
	if m.MinCoreVersion != "" && !versionAtLeast(coreVersion, m.MinCoreVersion) {
		return nil, fmt.Errorf("主系统版本 %s 低于应用要求的 %s", coreVersion, m.MinCoreVersion)
	}
	return m, nil
}

// themeAssetMap 主题包内允许落盘的资产 → data/themes/<id>/ 下的固定文件名。
// 白名单之外一律忽略（防 zip 任意落盘）；兼容根目录与 theme/ 子目录两种打包习惯，
// 并兼容上游市场包的 theme.css 命名（统一落到 ssr.css）。
var themeAssetMap = map[string]string{
	"ssr.css":             "ssr.css",
	"theme.css":           "ssr.css",
	"theme/ssr.css":       "ssr.css",
	"theme/theme.css":     "ssr.css",
	"page.html":           "page.html",
	"theme/page.html":     "page.html",
	"manifest.json":       "manifest.json",
	"theme/manifest.json": "manifest.json",
}

// deployThemeAssets 把主题包内的声明式资产投放到 data/themes/<id>/。
// 这是"装 zip 即用、无需重新编译"的关键：SSR 侧样式/模板均为请求时读文件（mtime 缓存），
// 因此落盘即生效（SPA 交互版仍须随主系统构建，见主题交付规范）。
// 先全部读入内存再统一落盘，避免半途失败留下残缺主题目录。
func deployThemeAssets(zr *zip.Reader, id string) ([]string, error) {
	if !validThemeID(id) {
		return nil, fmt.Errorf("主题 id 非法（仅 [a-z0-9.-]，≤64 字符且不含 ..）：%s", id)
	}
	if zr == nil {
		return nil, fmt.Errorf("主题包无法解析")
	}
	// 1) 收集（白名单 + 大小上限）
	pending := map[string][]byte{}
	for _, f := range zr.File {
		name := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")
		dest, ok := themeAssetMap[name]
		if !ok || f.FileInfo().IsDir() {
			continue
		}
		limit := int64(512 << 10) // page.html / manifest.json
		if dest == "ssr.css" {
			limit = 256 << 10 // 与 externalThemeCSS 的读取上限一致
		}
		if f.UncompressedSize64 > uint64(limit) {
			return nil, fmt.Errorf("主题包 %s 超过大小上限（%d 字节）", name, limit)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败：%w", name, err)
		}
		b, err := io.ReadAll(io.LimitReader(rc, limit+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败：%w", name, err)
		}
		if int64(len(b)) > limit {
			return nil, fmt.Errorf("主题包 %s 超过大小上限（%d 字节）", name, limit)
		}
		pending[dest] = b
	}
	if len(pending) == 0 {
		return nil, nil
	}
	if _, ok := pending["ssr.css"]; !ok {
		if _, ok2 := pending["page.html"]; !ok2 {
			return nil, nil // 无 ssr.css 也无 page.html = 无可生效资产
		}
	}
	// 2) 落盘
	dir := filepath.Join(themesRoot(), id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建主题目录失败：%w", err)
	}
	out := make([]string, 0, len(pending))
	for dest, b := range pending {
		if err := os.WriteFile(filepath.Join(dir, dest), b, 0o644); err != nil {
			return out, fmt.Errorf("写入 %s 失败：%w", dest, err)
		}
		out = append(out, dest)
	}
	sort.Strings(out)
	return out, nil
}

// removeThemeAssets 卸载主题时清理外置资产目录（仅限 data/themes 直接子目录，防误删）。
func removeThemeAssets(id string) error {
	if !validThemeID(id) {
		return fmt.Errorf("主题 id 非法：%s", id)
	}
	root := themesRoot()
	dir := filepath.Join(root, id)
	if filepath.Dir(dir) != filepath.Clean(root) {
		return fmt.Errorf("拒绝删除非主题目录：%s", dir)
	}
	return os.RemoveAll(dir)
}
