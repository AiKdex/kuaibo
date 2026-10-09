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

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/entitle"
	"github.com/AiKMAP/AiKmap/server/internal/service"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// marketIndex 市场索引协议（index.json）。
type marketIndex struct {
	SchemaVersion string          `json:"schema_version"`
	Plugins       []marketPlugin  `json:"plugins"`
	Themes        []marketTheme   `json:"themes"`
	Licenses      []marketLicense `json:"licenses"`
	// AiSupplies 平台 AI 供给条目（B38）：平台供模型套餐（aikgw 网关 key + 初始额度）。
	AiSupplies []marketAiSupply `json:"ai_supplies,omitempty"`
	// 市场 v0.3 签名：sign = base64(ed25519(规范化索引字节))，iss = 签发方。
	// 规范化 = 本结构体去掉 Signature 字段后的 json.Marshal（Go 字段序稳定）。
	Signature *marketSignature `json:"signature,omitempty"`
}

// marketSignature 索引/制品签名信封。
type marketSignature struct {
	Iss string `json:"iss"` // 签发方 iss（须在内嵌可信公钥表 entitle 中）
	Sig string `json:"sig"` // base64(ed25519 签名)
}

// verifySignature 验索引签名。签发方不在内嵌可信公钥表、或签名不匹配 → 失败。
// 注意：**不做「拿不到签名就当没签名」的降级**——那等于没做签名。
func (idx *marketIndex) verifySignature() error {
	if idx == nil || idx.Signature == nil {
		return fmt.Errorf("市场索引未签名")
	}
	payload, err := idx.canonicalBytes()
	if err != nil {
		return err
	}
	return entitle.VerifyDetached(idx.Signature.Iss, payload, idx.Signature.Sig)
}

// canonicalBytes 规范化待签名字节：Signature 置空后的 JSON 编码。
// 签发方与验签方必须用同一份代码生成，保证字节级一致。
func (idx *marketIndex) canonicalBytes() ([]byte, error) {
	cp := *idx
	cp.Signature = nil
	return json.Marshal(&cp)
}

// marketLicense 目录中的「站点数授权」条目（SPEC-MS-001 M4）：download_url 指向纯文本 entitle key
// （scope feature:multisite，含 seats）；安装=验签后写入 site_licenses，即时提升站点数上限。
type marketLicense struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Edition     string   `json:"edition,omitempty"`
	Seats       int      `json:"seats,omitempty"`
	LicenseType string   `json:"license_type,omitempty"` // permanent | subscription
	DownloadURL string   `json:"download_url"`
	SHA256      string   `json:"sha256"`
	Tier        string   `json:"tier,omitempty"`
	Target      []string `json:"target,omitempty"`
	// 服务端补充
	Installed  bool `json:"installed"`
	Applicable bool `json:"applicable"`
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
	Price           string   `json:"price,omitempty"`           // 展示价（如 ¥99/年；上游 2026-09-24 商城化新增）
	BillingPeriod   string   `json:"billing_period,omitempty"`  // yearly|monthly|once（可选）
	PurchaseURL     string   `json:"purchase_url,omitempty"`    // 购买引导（上游暂未下发；下发后前端展示「去购买」）
	Cover           string   `json:"cover,omitempty"` // 市场卡片封面图 URL（可选）
	Target          []string `json:"target,omitempty"`
	MinCoreVersion  string   `json:"min_core_version,omitempty"`
	SettingsSchema  []any    `json:"settings_schema,omitempty"`
	// 内置能力应用（builtin=true）：能力随主系统编译（如家族传承/组织架构），
	// 无安装包可下载——"安装"=登记 blog_plugins capacity 记录（见 installBuiltinCapacity）。
	Builtin     bool     `json:"builtin,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"` // 命中的内核能力名（builtin 应用必填）
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
	Price       string   `json:"price,omitempty"`          // 展示价（上游 2026-09-24 商城化新增）
	BillingPeriod string `json:"billing_period,omitempty"` // yearly|monthly|once（可选）
	PurchaseURL string   `json:"purchase_url,omitempty"`   // 购买引导（上游暂未下发）
	Cover       string   `json:"cover,omitempty"` // 市场卡片封面图 URL（可选）
	Target      []string `json:"target,omitempty"`
	// 内置源码主题（builtin=true）：SPA 组件随主系统构建分发，无安装包可下载——
	// 「安装」= 登记 blog_plugins(kind=theme)（见 installBuiltinTheme），卸载即撤销登记。
	Builtin bool `json:"builtin,omitempty"`
	// 服务端补充
	Installed  bool `json:"installed"`
	Applicable bool `json:"applicable"`
}

// builtinSourceThemeEntries 内置源码主题条目（2026-10-09 主题改版：只常驻 aiklog，
// 其余 17 个 SPA 主题进应用中心「主题」类别按需安装）。注入在远程索引 themes 之前。
func builtinSourceThemeEntries() []marketTheme {
	out := make([]marketTheme, 0, len(service.BuiltinSpaThemes))
	for _, t := range service.BuiltinSpaThemes {
		out = append(out, marketTheme{
			ID:          t.ID,
			Name:        t.Title,
			Version:     coreVersion,
			Description: "内置源码主题 · 随主系统构建分发，安装即启用（SPA 交互版 + 公开静态页令牌）",
			Author:      "爱库录",
			Tier:        "free",
			Builtin:     true,
			Applicable:  true,
		})
	}
	return out
}

// installBuiltinTheme 内置源码主题安装：无 zip 可下载，「安装」= 登记 blog_plugins
// (kind=theme, enabled=1)。SPA 组件已随主系统构建（懒 chunk），登记后前端
// /public/blog/themes 立即可见 → 切换器出现、站点主题白名单放行，即时生效无需重启。
func (a *API) installBuiltinTheme(w http.ResponseWriter, r *http.Request, entry *marketPlugin) {
	if !service.IsBuiltinSpaTheme(entry.ID) {
		writeErr(w, http.StatusUnprocessableEntity, "MARKET_BAD_REQ", "非内置源码主题，请提供主题包 zip")
		return
	}
	if entry.Name == "" {
		entry.Name = entry.ID
	}
	now := time.Now().UnixMilli()
	ver := entry.Version
	if ver == "" {
		ver = coreVersion
	}
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO blog_plugins (id, name, version, description, author, kind, enabled, created_at, updated_at)
		 VALUES (?,?,?,?,?,'theme',1,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, version=excluded.version, description=excluded.description,
		   author=excluded.author, kind='theme', enabled=1, updated_at=excluded.updated_at`,
		entry.ID, entry.Name, ver, entry.Description, entry.Author, now, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	log.Printf("应用中心：内置源码主题 %s 已登记（kind=theme，即时生效）", entry.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": entry.ID, "kind": "theme", "enabled": true, "builtin": true,
		"apply": "immediate",
		"message": "已安装：主题立即可用（博客设置/切换器中选择即可生效）",
	})
}

// marketAiSupply 目录中的「平台 AI 供给」条目（B38）。
// download_url 指向纯文本 JSON 供给凭据（schema=aiklog-ai-supply/v1：网关地址 + gw key +
// 初始 token 额度）；开通 = 下载验 sha256 后解析凭据 → 登记/追加网关 provider + 站点池充值。
// 与 license 同模式：索引整体 ed25519 验签，凭据文件靠 sha256 + 索引签名背书。
type marketAiSupply struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Description   string   `json:"description"`
	Tier          string   `json:"tier,omitempty"`          // free|paid（付费预留；购买走 purchase_url）
	Price         string   `json:"price,omitempty"`         // 展示价（如 ¥9.9/月）
	BillingPeriod string   `json:"billing_period,omitempty"` // monthly|yearly|once
	PurchaseURL   string   `json:"purchase_url,omitempty"`   // 购买引导
	Provider      string   `json:"provider,omitempty"`       // 凭据缺省 provider 名提示（安装以凭据为准）
	Target        []string `json:"target,omitempty"`
	DownloadURL   string   `json:"download_url"`
	SHA256        string   `json:"sha256"`
	// 服务端补充
	Installed  bool `json:"installed"`  // 本实例已登记对应网关 provider
	Applicable bool `json:"applicable"` // target 含本壳
}

// marketCache 市场索引内存缓存（5 分钟）。
// M4 修复：并发读写加锁（/market/index.json 与 /admin/apps/market 两轨并发访问同一缓存，
// 无锁时 go test -race 可复现 data race；两处入口共用 marketCache.mu）。
var marketCache struct {
	mu    sync.Mutex
	at    time.Time
	index *marketIndex
	url   string
}

// 官方应用中心站点（独立页面 /apps 的「打开官网」目标）。
//
// 产品约定（B33）：自部署实例的「应用中心」恒指向官方站点，不跟随用户自己的站点——
// 用户装完 AiKlog 后点应用中心，看到的应是我们维护的官方目录，而不是某次部署的快照。
// 该地址只用于前端跳转；目录数据仍走 marketIndexURL()（默认同一个官方索引）。
const officialMarketSiteURL = "https://aikmap.cn/market"

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
	marketCache.mu.Lock()
	defer marketCache.mu.Unlock()
	key := a.marketSourceKey()
	if time.Since(marketCache.at) < 5*time.Minute && marketCache.index != nil && marketCache.url == key {
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
	marketCache.url = key
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
	granted := map[string]bool{}
	if rows, err := a.db.Query(`SELECT id FROM site_licenses`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				granted[id] = true
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
	for i := range idx.Licenses {
		l := idx.Licenses[i]
		l.Installed = granted[l.ID]
		l.Applicable = len(l.Target) == 0 || containsStr(l.Target, shell)
		out.Licenses = append(out.Licenses, l)
	}
	for i := range idx.AiSupplies {
		s := idx.AiSupplies[i]
		pn := s.Provider
		if pn == "" {
			pn = aiSiteSupplyProvider
		}
		_, s.Installed = a.ai.Providers()[pn]
		s.Applicable = len(s.Target) == 0 || containsStr(s.Target, shell)
		out.AiSupplies = append(out.AiSupplies, s)
	}
	return map[string]any{
		"schema_version": idx.SchemaVersion,
		"plugins":        out.Plugins,
		"themes":         out.Themes,
		"licenses":       out.Licenses,
		"ai_supplies":    out.AiSupplies,
		"index_url":      a.marketIndexURL(),
		"site_url":       officialMarketSiteURL, // 独立应用中心页的「打开官网」目标
		"shell":          shell,
		"edition":        a.licenseEdition(),
	}
}

// marketInstall POST /api/v1/admin/apps/market/install：安装市场插件/主题。
// body: {"id":"..."}（从市场索引取 download_url+sha256）或 {"url":"...","sha256":"...","manifest":{...}}（直接安装）。
func (a *API) marketInstall(w http.ResponseWriter, r *http.Request) {
	// H2 修复：市场安装（插件/主题/Agent 工具，含任意 zip/URL 下载 → 写入主题目录与插件注册表）
	// 仅站长（owner/admin）可执行；该端点同时挂在 /api/v1/blog/market/install（不带 /admin/ 前缀），
	// 故必须在本处显式守卫，不能只靠路由级 admin 前缀拦截。
	if !a.blogAdminOnly(w, r) {
		return
	}
	var req struct {
		ID   string `json:"id"`
		URL  string `json:"url"`
		SHA  string `json:"sha256"`
		Kind string `json:"kind"` // 直装可显式声明（plugin|theme|license）；按 id 装时由索引类别覆盖
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "MARKET_BAD_REQ", "无效请求体")
		return
	}
	dlURL, sha := req.URL, strings.ToLower(req.SHA)
	kind := strings.TrimSpace(req.Kind)
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
						Builtin: t.Builtin,
					}
					kind = "theme"
					break
				}
			}
		}
		if found == nil {
			for i := range idx.Licenses {
				if idx.Licenses[i].ID == req.ID {
					l := idx.Licenses[i]
					found = &marketPlugin{
						ID: l.ID, Name: l.Name, Version: l.Version, Description: l.Description,
						DownloadURL: l.DownloadURL, SHA256: l.SHA256, Target: l.Target, Tier: l.Tier,
					}
					kind = "license"
					break
				}
			}
		}
		if found == nil {
			for i := range idx.AiSupplies {
				if idx.AiSupplies[i].ID == req.ID {
					s := idx.AiSupplies[i]
					found = &marketPlugin{
						ID: s.ID, Name: s.Name, Version: s.Version, Description: s.Description,
						DownloadURL: s.DownloadURL, SHA256: s.SHA256, Target: s.Target, Tier: s.Tier,
					}
					kind = "ai_supply"
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
		// 授权类商品与 AI 供给包本身即授权/凭据载体，不受 pro 门禁约束
		// （否则形成"买授权需先买授权"死锁；凭据文件受索引签名 + sha256 保护）。
		if kind != "license" && kind != "ai_supply" && found.Tier == "paid" && a.licenseEdition() != "pro" {
			writeErr(w, http.StatusPaymentRequired, "MARKET_LICENSE_REQUIRED",
				"该应用为付费应用（tier=paid），需要 pro 许可证（请先在设置中激活 license key）")
			return
		}
		// 内置能力应用（builtin=true）：无安装包，安装=登记 blog_plugins capacity 记录（免下载免解包）。
		// 生效时机遵循能力装配语义（restart）：重启主系统后路由挂载、侧栏出现入口。
		if found.Builtin && kind == "theme" {
			a.installBuiltinTheme(w, r, found)
			return
		}
		if found.Builtin {
			a.installBuiltinCapacity(w, r, found)
			return
		}
		dlURL, sha = found.DownloadURL, strings.ToLower(found.SHA256)
	}
	if dlURL == "" {
		writeErr(w, http.StatusBadRequest, "MARKET_BAD_REQ", "缺少下载地址（url 或市场 id）")
		return
	}
	// 下载（限流 16MiB；15s 超时 + 逐跳 SSRF 校验）
	resp, err := marketHTTPClient.Get(dlURL)
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
	// 站点数授权分发（SPEC-MS-001 M4）：license 包为纯文本 entitle key（非 zip），
	// 验签通过后写入 site_licenses；必须在解包 zip 之前分流。
	if kind == "license" {
		raw, rerr := os.ReadFile(tmp.Name())
		if rerr != nil {
			writeErr(w, http.StatusUnprocessableEntity, "LICENSE_READ_FAILED", rerr.Error())
			return
		}
		a.installLicenseFromBytes(w, r, string(raw))
		return
	}
	// 平台 AI 供给分发（B38）：凭据为纯文本 JSON（非 zip），验 sha256 后解析开通；
	// 必须在解包 zip 之前分流（同 license）。
	if kind == "ai_supply" {
		raw, rerr := os.ReadFile(tmp.Name())
		if rerr != nil {
			writeErr(w, http.StatusUnprocessableEntity, "AI_SUPPLY_READ_FAILED", rerr.Error())
			return
		}
		a.installAiSupplyFromBytes(w, r, req.ID, string(raw))
		return
	}
	// 解包并校验 manifest
	m, err := installPluginZip(tmp.Name())
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "MARKET_BAD_PACKAGE", err.Error())
		return
	}
	// 源包分流（SPEC-SP-001）：manifest.kind=source-pack 走采集源导入链路，
	// 不进插件登记（必须在下面「索引分类强制覆盖 kind」之前判断，否则会被归成 plugin）。
	if m.Kind == service.SourcePackKind {
		raw, rerr := os.ReadFile(tmp.Name())
		if rerr != nil {
			writeErr(w, http.StatusUnprocessableEntity, "SOURCE_PACK_READ_FAILED", rerr.Error())
			return
		}
		a.installSourcePack(w, r, raw, nil)
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

// installBuiltinCapacity 内置能力应用登记：家族传承/组织架构等随主系统编译的能力，
// "安装"=写一条 blog_plugins 记录（capability_mode='capacity' + capabilities 声明），
// CapabilityEnabled 第 2 层命中后启动装配（重启生效）；停用/卸载即熄灭（数据保留）。
// 🔴 不走 upsertPluginRecord：v2 校验的 capability 白名单只收插件侧能力（collector/im/...），
// 内核能力名（family/org）不在其列，且这些能力无后端入口键——直写登记，语义等价于老数据口径。
func (a *API) installBuiltinCapacity(w http.ResponseWriter, r *http.Request, entry *marketPlugin) {
	if len(entry.Capabilities) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "MARKET_BAD_REQ", "内置能力应用缺少 capabilities 声明")
		return
	}
	caps, _ := json.Marshal(entry.Capabilities)
	now := time.Now().UnixMilli()
	ver := entry.Version
	if ver == "" {
		ver = coreVersion
	}
	_, err := a.db.ExecContext(r.Context(),
		`INSERT INTO blog_plugins (id, name, version, description, author, kind, enabled, created_at, updated_at,
		                           capability_mode, capabilities)
		 VALUES (?,?,?,?,?,?,1,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, version=excluded.version, description=excluded.description,
		   author=excluded.author, enabled=1, updated_at=excluded.updated_at,
		   capability_mode=excluded.capability_mode, capabilities=excluded.capabilities`,
		entry.ID, entry.Name, ver, entry.Description, entry.Author, "plugin", now, now, "capacity", string(caps))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	// installed 标记对照 blog_plugins.id（marketView 口径），此处 id 即条目 id → 列表立显「已安装」
	log.Printf("应用中心：内置能力应用 %s 已登记（capabilities=%s，重启主系统后装配生效）", entry.ID, string(caps))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": entry.ID, "kind": "plugin", "capability_mode": "capacity",
		"enabled": true, "builtin": true, "apply": "restart",
		"message": "已安装；重启主系统后生效（侧栏出现入口、路由挂载）",
	})
}

// aiSiteSupplyProvider 应用中心平台 AI 供给的缺省 provider 名（凭据未声明 provider 时使用）。
const aiSiteSupplyProvider = "aikgw"

// installAiSupplyFromBytes 平台 AI 供给凭据开通（B38）。
// 凭据 = 纯文本 JSON（schema=aiklog-ai-supply/v1）：网关地址 + gw key + 初始 token 额度。
// 落地三件事：
//  1. 登记/追加网关 provider：keys 池追加去重（增量包 = 池里加新 key，不缩容不覆盖）；
//  2. 站点池充值：meter 主体 "site"（全站共享额度，主体余额不足时扣费自动兜底，见 meter.go）；
//  3. 不擅自改写能力绑定 —— 提示站长在设置页把对应能力切到该 provider。
func (a *API) installAiSupplyFromBytes(w http.ResponseWriter, r *http.Request, entryID, raw string) {
	var cred struct {
		Schema        string            `json:"schema"`
		GatewayURL    string            `json:"gateway_url"`
		GatewayKey    string            `json:"gateway_key"`
		Provider      string            `json:"provider"`
		Caps          []string          `json:"caps"`
		Model         string            `json:"model"`
		Models        map[string]string `json:"models"`
		InitialTokens int               `json:"initial_tokens"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &cred); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "AI_SUPPLY_BAD_CRED", "供给凭据不是合法 JSON："+err.Error())
		return
	}
	if cred.Schema != "" && cred.Schema != "aiklog-ai-supply/v1" {
		writeErr(w, http.StatusUnprocessableEntity, "AI_SUPPLY_SCHEMA", "供给凭据 schema 不支持："+cred.Schema)
		return
	}
	gw := strings.TrimRight(strings.TrimSpace(cred.GatewayURL), "/")
	if !strings.HasPrefix(gw, "http://") && !strings.HasPrefix(gw, "https://") {
		writeErr(w, http.StatusUnprocessableEntity, "AI_SUPPLY_BAD_URL", "供给凭据 gateway_url 非法（须 http/https 地址）")
		return
	}
	if strings.TrimSpace(cred.GatewayKey) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "AI_SUPPLY_NO_KEY", "供给凭据缺少 gateway_key")
		return
	}
	name := strings.TrimSpace(cred.Provider)
	if name == "" {
		name = aiSiteSupplyProvider
	}
	caps := cred.Caps
	if len(caps) == 0 {
		caps = []string{"llm"}
	}
	def := &ai.ProviderDef{
		Endpoint: gw,
		Model:    strings.TrimSpace(cred.Model),
		Models:   cred.Models,
		Caps:     caps,
		Note:     "平台 AI 供给（key 由平台网关签发，应用中心开通）",
	}
	// keys 追加去重：老 key 保留（池不缩容），新 key 入池
	keys := []string{}
	seen := map[string]bool{}
	if d, ok := a.ai.Providers()[name]; ok {
		def.ModelCands = d.ModelCands
		def.VoiceCands = d.VoiceCands
		def.Builtin = d.Builtin
		for _, k := range d.Keys {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	if !seen[cred.GatewayKey] {
		keys = append(keys, cred.GatewayKey)
	}
	def.Keys = keys
	if err := ai.SaveProviderDef(a.db, name, def); err != nil {
		writeErr(w, http.StatusInternalServerError, "AI_SUPPLY_SAVE_FAILED", err.Error())
		return
	}
	a.ai.UpsertProvider(name, def)
	a.ai.DropProviderPools(name) // 密钥/endpoint 变更后强制重建 token 池
	granted, balance := 0, 0
	if m := a.ai.Meter(); m != nil && cred.InitialTokens > 0 {
		if bal, err := m.Grant(ai.SiteSubject, cred.InitialTokens, "应用中心供给包:"+entryID, "app_center"); err == nil {
			granted = cred.InitialTokens
			balance = bal
		} else {
			log.Printf("应用中心：AI 供给 %s 站点池充值失败（provider 已登记，可手工充值）：%v", entryID, err)
		}
	}
	log.Printf("应用中心：平台 AI 供给 %s 已开通（provider=%s，keys=%d，站点池 +%d）", entryID, name, len(def.Keys), granted)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": entryID, "kind": "ai_supply", "provider": name,
		"endpoint": gw, "keys": len(def.Keys),
		"granted": granted, "balance": balance,
		"message": fmt.Sprintf("平台 AI 供给已开通（provider=%s）。请在 设置 → AI 里把要用的能力切换到该提供方。", name),
	})
}

// fetchMarketIndex 拉取市场索引（不走缓存，供安装校验用）。
// localMarketIndexPath 本实例自维护市场目录（自有市场源优先）：data/market/index.json。
// 存在即作为市场目录直接读盘（不经 HTTP —— 严禁指向本实例 /market/index.json，
// 该文件本身是拉源生成的合并视图，自引用会递归死循环）；不存在时回落远程索引。
func localMarketIndexPath() string {
	base := filepath.Join("data", "market")
	if exe, err := os.Executable(); err == nil {
		base = filepath.Join(filepath.Dir(exe), "..", "data", "market")
	}
	return filepath.Join(base, "index.json")
}

// marketSourceKey 市场源缓存键：本地目录含 mtime（改文件即生效，不走 5 分钟等待）；远程为索引 URL。
func (a *API) marketSourceKey() string {
	p := localMarketIndexPath()
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return "local|" + p + "|" + st.ModTime().Format("20060102150405.000000000")
	}
	return "remote|" + a.marketIndexURL()
}

// marketHTTPClient 市场索引/应用包下载用客户端：15s 超时 + 逐跳 SSRF 校验
// （管理员配置的 URL 也会被重定向到内网，必须按铁律接 SafeCheckRedirect）。
var marketHTTPClient = &http.Client{
	Timeout:       15 * time.Second,
	CheckRedirect: service.SafeCheckRedirect,
}

// fetchMarketIndex 取市场索引：本地市场目录 data/market/index.json 优先；不存在回落远程。
// 远程索引必须**通过 ed25519 签名验签**才采信（市场 v0.3）：验签失败直接报错，
// 不按未签名处理——否则远程索引就成了任意方可否决包体的通道。
func (a *API) fetchMarketIndex(r *http.Request) (*marketIndex, error) {
	p := localMarketIndexPath()
	b, err := os.ReadFile(p)
	if err == nil {
		var idx marketIndex
		if err := json.Unmarshal(b, &idx); err != nil {
			return nil, fmt.Errorf("本地市场目录 %s 格式非法：%v", p, err)
		}
		return &idx, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("本地市场目录 %s 读取失败：%v", p, err)
	}
	u := a.marketIndexURL()
	if strings.TrimSpace(u) == "" {
		return nil, fmt.Errorf("未配置市场索引地址（market.index_url）且无本地市场目录")
	}
	// B47：显式构造请求以携带匿名身份头（install_id / version / shell）。
	// 这三个头是**纯附加信息** —— 官方侧不接收也不影响任何功能。
	// 官方侧由此把回源请求去重成「装机数」并统计版本分布（见 market_telemetry.go）。
	// 用入参 r 的 context：调用方（marketList / marketIndexFile）都传了真实请求，
	// 保留取消链路（客户端断开时立即中止 15s 的远端拉取，不空耗连接）。
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("市场索引地址非法：%w", err)
	}
	a.attachMarketIdentity(req)
	resp, err := marketHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("市场索引拉取失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("市场索引 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("市场索引读取不完整：%w", err)
	}
	var idx marketIndex
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("市场索引格式非法")
	}
	if err := idx.verifySignature(); err != nil {
		return nil, fmt.Errorf("市场索引验签失败：%w", err)
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
