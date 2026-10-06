// site_admin.go 多站点管理后台（SPEC-MS-001 M3）：站点 CRUD + 站点设置 + 站点数授权管理。
//
// 路由（均在 routes.go 注册，需 owner/admin）：
//   GET    /api/v1/admin/sites                  列出全部站点 + cap 汇总 + 授权列表
//   POST   /api/v1/admin/sites                  新建站点（受 cap 闸门，超额 402）
//   GET    /api/v1/admin/sites/{id}              取站点
//   PUT    /api/v1/admin/sites/{id}              更新站点身份/路由/状态
//   DELETE /api/v1/admin/sites/{id}              软删（保留数据）
//   GET    /api/v1/admin/sites/{id}/settings     取站点设置
//   PUT    /api/v1/admin/sites/{id}/settings     更新站点设置
//   GET    /api/v1/admin/sites/licenses          列出站点数授权 + cap 汇总
//   POST   /api/v1/admin/sites/licenses          签发/录入一张站点数授权（运营侧，验签后写入）
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/entitle"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// adminSitesList GET /api/v1/admin/sites：全部站点 + cap 汇总 + 授权列表。
func (a *API) adminSitesList(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	ctx := r.Context()
	sites, err := a.sites.ListSites(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SITES_LIST_FAILED", err.Error())
		return
	}
	base, perm, sub, total, used, err := a.sites.SiteCapSummary(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SITES_CAP_FAILED", err.Error())
		return
	}
	licenses, _ := a.sites.ListSiteLicenses(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"site_cap": map[string]any{
			"base":        base,
			"permanent":   perm,
			"subscription": sub,
			"total":       total,
			"used":        used,
		},
		"licenses": licenses,
		"sites":    sites,
	})
}

// adminSiteCreate POST /api/v1/admin/sites：新建站点（受 cap 闸门；超额返回 402）。
func (a *API) adminSiteCreate(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	ctx := r.Context()
	var body struct {
		Slug      string `json:"slug"`
		Domain    string `json:"domain"`
		Subdomain string `json:"subdomain"`
		PathPrefix string `json:"path_prefix"`
		OwnerID   string `json:"owner_id"`
		Title     string `json:"title"`
		Subtitle  string `json:"subtitle"`
		Locale    string `json:"locale"`
		DefaultTheme string `json:"default_theme"`
		AllowVisitorThemeSwitch *bool `json:"allow_visitor_theme_switch"`
		SEODescription string `json:"seo_description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "SITE_BAD_BODY", "无效请求体")
		return
	}
	body.Slug = sanitizeSlug(body.Slug)
	if body.Slug == "" || body.Slug == defaultSiteIDFallback {
		writeErr(w, http.StatusBadRequest, "SITE_BAD_SLUG", "slug 必填且不能为 default")
		return
	}
	// cap 闸门：已用 >= 上限 → 402 引导购买（SPEC-MS-001 §5.4）。
	_, _, _, total, used, err := a.sites.SiteCapSummary(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SITES_CAP_FAILED", err.Error())
		return
	}
	if used >= total {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{
			"error":     "SITE_LIMIT_REACHED",
			"message":   "站点数已达当前授权上限，请通过应用中心购买站点数授权后重试",
			"used":      used,
			"cap_total": total,
		})
		return
	}
	site, err := a.sites.CreateSite(ctx, serviceSiteFromBody(&body))
	if err != nil {
		writeErr(w, http.StatusConflict, "SITE_CREATE_FAILED", err.Error())
		return
	}
	// 初始化该站设置（继承请求体或默认）；未显式传开关时默认允许访客切换主题。
	allowSwitch := true
	if body.AllowVisitorThemeSwitch != nil {
		allowSwitch = *body.AllowVisitorThemeSwitch
	}
	ss := service.SiteSettings{
		SiteID:                  site.ID,
		DefaultTheme:            body.DefaultTheme,
		AllowVisitorThemeSwitch: allowSwitch,
		Title:                   body.Title,
		Subtitle:                body.Subtitle,
		Locale:                  body.Locale,
		SEODescription:          body.SEODescription,
	}
	if err := a.sites.UpsertSiteSettings(ctx, &ss); err != nil {
		writeErr(w, http.StatusInternalServerError, "SITE_SETTINGS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"site": site})
}

// adminSiteGet GET /api/v1/admin/sites/{id}。
func (a *API) adminSiteGet(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	site, err := a.sites.GetSite(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "SITE_NOT_FOUND", "站点不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"site": site})
}

// adminSiteUpdate PUT /api/v1/admin/sites/{id}：更新身份/路由/状态。
func (a *API) adminSiteUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	site, err := a.sites.GetSite(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "SITE_NOT_FOUND", "站点不存在")
		return
	}
	var body struct {
		Slug      string `json:"slug"`
		Domain    string `json:"domain"`
		Subdomain string `json:"subdomain"`
		PathPrefix string `json:"path_prefix"`
		OwnerID   string `json:"owner_id"`
		Status    string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "SITE_BAD_BODY", "无效请求体")
		return
	}
	if body.Slug != "" {
		site.Slug = sanitizeSlug(body.Slug)
	}
	if body.Domain != "" {
		site.Domain = body.Domain
	}
	if body.Subdomain != "" {
		site.Subdomain = body.Subdomain
	}
	if body.PathPrefix != "" {
		site.PathPrefix = body.PathPrefix
	}
	if body.OwnerID != "" {
		site.OwnerID = body.OwnerID
	}
	if body.Status != "" {
		site.Status = body.Status
	}
	if err := a.sites.UpdateSite(ctx, site); err != nil {
		writeErr(w, http.StatusConflict, "SITE_UPDATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"site": site})
}

// adminSiteDelete DELETE /api/v1/admin/sites/{id}：软删（保留数据，已建站保留但禁新建）。
func (a *API) adminSiteDelete(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == defaultSiteIDFallback {
		writeErr(w, http.StatusBadRequest, "SITE_DELETE_DEFAULT", "默认站不可删除")
		return
	}
	if err := a.sites.DeleteSite(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "SITE_DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// adminSiteSettingsGet GET /api/v1/admin/sites/{id}/settings。
func (a *API) adminSiteSettingsGet(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	ss, err := a.sites.GetSiteSettings(r.Context(), id)
	if err != nil {
		// 未建配置 → 返回默认骨架（前端表单可直接渲染）。
		writeJSON(w, http.StatusOK, map[string]any{"settings": defaultSiteSettingsFor(id)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": ss})
}

// adminSiteSettingsUpdate PUT /api/v1/admin/sites/{id}/settings：更新站点设置。
func (a *API) adminSiteSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	var body struct {
		Title                    string          `json:"title"`
		Subtitle                 string          `json:"subtitle"`
		Locale                   string          `json:"locale"`
		DefaultTheme             string          `json:"default_theme"`
		AllowVisitorThemeSwitch  bool            `json:"allow_visitor_theme_switch"`
		SEODescription           string          `json:"seo_description"`
		SEOKeywords              string          `json:"seo_keywords"`
		SEOTitle                 string          `json:"seo_title"`
		Ext                      json.RawMessage `json:"ext"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "SITE_SETTINGS_BAD_BODY", "无效请求体")
		return
	}
	ss := service.SiteSettings{
		SiteID:                  id,
		Title:                   body.Title,
		Subtitle:                body.Subtitle,
		Locale:                  body.Locale,
		DefaultTheme:            body.DefaultTheme,
		AllowVisitorThemeSwitch: body.AllowVisitorThemeSwitch,
		SEODescription:          body.SEODescription,
		SEOKeywords:             body.SEOKeywords,
		SEOTitle:                body.SEOTitle,
		Ext:                     body.Ext,
	}
	if err := a.sites.UpsertSiteSettings(ctx, &ss); err != nil {
		writeErr(w, http.StatusInternalServerError, "SITE_SETTINGS_FAILED", err.Error())
		return
	}
	// 注：默认主题以 site_settings 为权威来源（/public/site 直读该表），
	// 不写入全局 settings scope——per-site 配置不复用 instance scope（SPEC-MS-001 v1.1.0 定稿）。
	writeJSON(w, http.StatusOK, map[string]any{"settings": ss})
}

// adminSiteLicenses GET /api/v1/admin/sites/licenses：授权列表 + cap 汇总。
func (a *API) adminSiteLicenses(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	ctx := r.Context()
	licenses, _ := a.sites.ListSiteLicenses(ctx)
	base, perm, sub, total, used, err := a.sites.SiteCapSummary(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SITES_CAP_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"licenses": licenses,
		"site_cap": map[string]any{
			"base":         base,
			"permanent":    perm,
			"subscription": sub,
			"total":        total,
			"used":         used,
		},
	})
}

// adminSiteLicenseGrant POST /api/v1/admin/sites/licenses：签入一张站点数授权（运营侧录入）。
// body: {"license_key":"<entitle key feature:multisite>"}；验签通过后幂等写 site_licenses。
func (a *API) adminSiteLicenseGrant(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	ctx := r.Context()
	var body struct {
		LicenseKey string `json:"license_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.LicenseKey) == "" {
		writeErr(w, http.StatusBadRequest, "LICENSE_BAD_BODY", "需要 license_key")
		return
	}
	lic, err := a.grantSiteLicenseKey(ctx, body.LicenseKey)
	if err != nil {
		code, c, msg := licenseErrResponse(err)
		writeErr(w, code, c, msg)
		return
	}
	_, _, _, total, used, _ := a.sites.SiteCapSummary(ctx)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "license": lic, "cap_total": total, "used": used})
}

// ─────────────────────────────────────────────────────────────────────────────
// 内部辅助
// ─────────────────────────────────────────────────────────────────────────────

// serviceSiteFromBody 把请求体映射为 Site（ID 留空由 CreateSite 生成）。
func serviceSiteFromBody(b *struct {
	Slug           string `json:"slug"`
	Domain         string `json:"domain"`
	Subdomain      string `json:"subdomain"`
	PathPrefix     string `json:"path_prefix"`
	OwnerID        string `json:"owner_id"`
	Title          string `json:"title"`
	Subtitle       string `json:"subtitle"`
	Locale         string `json:"locale"`
	DefaultTheme   string `json:"default_theme"`
	AllowVisitorThemeSwitch *bool `json:"allow_visitor_theme_switch"`
	SEODescription string `json:"seo_description"`
}) *service.Site {
	return &service.Site{
		Slug:       b.Slug,
		Domain:     b.Domain,
		Subdomain:  b.Subdomain,
		PathPrefix: b.PathPrefix,
		OwnerID:    b.OwnerID,
		Status:     "active",
	}
}

// defaultSiteSettingsFor 返回一个站点的默认设置骨架（未建记录时前端渲染用）。
func defaultSiteSettingsFor(siteID string) *service.SiteSettings {
	return &service.SiteSettings{
		SiteID:                  siteID,
		DefaultTheme:            "aiklog",
		AllowVisitorThemeSwitch: true,
		Locale:                  "zh-CN",
	}
}

// licenseTypeOf 由载荷到期时间推断授权类型（永久 / 订阅）。
func licenseTypeOf(p *entitle.Payload) string {
	if p.Exp > 0 {
		return "subscription"
	}
	return "permanent"
}

// sanitizeSlug 规整 slug（小写、去空格、截断）。
func sanitizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// firstNonEmpty 返回第一个非空串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}


// ─────────────────────────────────────────────────────────────────────────────
// 站点数授权（M2/M4）：共用签发逻辑（后台手工录入 + 应用中心分发）。
// ─────────────────────────────────────────────────────────────────────────────

// errLicenseNoSeats 授权未包含有效站点数（调用方用 errors.Is 判定）。
var errLicenseNoSeats = errors.New("entitle: license 未包含有效站点数")

// grantSiteLicenseKey 验签一张站点数授权 key 并幂等写入 site_licenses。
// 后台手工录入（adminSiteLicenseGrant）与应用中心分发（marketInstall kind=license）共用。
func (a *API) grantSiteLicenseKey(ctx context.Context, key string) (*service.SiteLicense, error) {
	payload, err := entitle.ValidateProduct(key, entitle.MultisiteFeature)
	if err != nil {
		return nil, err
	}
	if payload.Seats <= 0 {
		return nil, errLicenseNoSeats
	}
	lic := &service.SiteLicense{
		ID:          firstNonEmpty(payload.Jti, "lic-"+time.Now().Format("20060102")+"-"+randomToken(4)),
		Edition:     payload.Edition,
		Seats:       payload.Seats,
		LicenseType: licenseTypeOf(payload),
		ExpiresAt:   payload.Exp,
		Status:      "active",
		GrantedAt:   time.Now().Unix(),
	}
	if err := a.sites.GrantSiteLicense(ctx, lic); err != nil {
		return nil, err
	}
	return lic, nil
}

// licenseErrResponse 把授权校验错误映射为 HTTP 状态码 + 业务错误码 + 文案。
func licenseErrResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, errLicenseNoSeats):
		return http.StatusUnprocessableEntity, "LICENSE_NO_SEATS", "授权未包含有效站点数"
	case errors.Is(err, entitle.ErrExpired):
		return http.StatusUnprocessableEntity, "LICENSE_EXPIRED", "授权已过期"
	case errors.Is(err, entitle.ErrFeatureDenied):
		return http.StatusUnprocessableEntity, "LICENSE_FEATURE_DENIED", "授权不含多站点功能（scope 需 all 或 feature:multisite）"
	case errors.Is(err, entitle.ErrBadSignature):
		return http.StatusUnprocessableEntity, "LICENSE_BAD_SIGNATURE", "授权签名无效"
	case errors.Is(err, entitle.ErrUnknownIss):
		return http.StatusUnprocessableEntity, "LICENSE_UNKNOWN_ISSUER", "授权签发方未知"
	case errors.Is(err, entitle.ErrBadFormat), errors.Is(err, entitle.ErrBadPayload):
		return http.StatusUnprocessableEntity, "LICENSE_BAD_FORMAT", "授权格式无效"
	default:
		return http.StatusUnprocessableEntity, "LICENSE_INVALID", "授权无效：" + err.Error()
	}
}

// installLicenseFromBytes 应用中心分发站点数授权（SPEC-MS-001 M4）：授权包为纯文本 entitle key
// （非 zip）；验签通过后写入 site_licenses，即时提升站点数上限。
func (a *API) installLicenseFromBytes(w http.ResponseWriter, r *http.Request, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		writeErr(w, http.StatusUnprocessableEntity, "LICENSE_EMPTY", "授权包为空")
		return
	}
	lic, err := a.grantSiteLicenseKey(r.Context(), key)
	if err != nil {
		code, c, msg := licenseErrResponse(err)
		writeErr(w, code, c, msg)
		return
	}
	_, _, _, total, used, _ := a.sites.SiteCapSummary(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "license": lic, "cap_total": total, "used": used})
}
