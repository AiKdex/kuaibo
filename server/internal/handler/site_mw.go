// site_mw.go 多站点路由中间件（SPEC-MS-001 M1）。
//
// 职责：从请求的 Host / Path 解析出当前站点（ResolveSite：自定义域名 > 子域名 > 子目录 > 默认站），
// 将 site_id 注入请求上下文；并提供从请求读取「站点配置（site_settings）」的 helper，
// 供公开博客 / RSS / Sitemap / 主题渲染按站点取值。
package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// ctxSiteID 注入解析后的站点 id（ctxKey=2，与 authmw 的 ctxUserID=1 同包不冲突）。
const ctxSiteID ctxKey = 2

// defaultSiteIDFallback 站点解析失败时的兜底站（与 service.defaultSiteID 对齐）。
const defaultSiteIDFallback = "default"

// siteMiddleware 解析当前请求的站点并注入 ctx；a.sites 未注入时退回默认站。
// 贯穿全部路由（含静态/SPA），保证 SSR 页与 /api/v1/public/* 同源解析。
func (a *API) siteMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siteID := defaultSiteIDFallback
		if a.sites != nil {
			if site, err := a.sites.ResolveSite(r.Context(), r.Host, r.URL.Path,
				a.cfg.GetString("system.base_domain")); err == nil && site != nil {
				siteID = site.ID
			}
		}
		ctx := context.WithValue(r.Context(), ctxSiteID, siteID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// currentSiteID 取当前请求的站点 id（注入失败回落默认站）。
func currentSiteID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxSiteID).(string); ok && v != "" {
		return v
	}
	return defaultSiteIDFallback
}

// operatingSiteID 返回当前请求「内容操作」应归属的站点：
//   - 管理员显式选择 ?site=<id>（且站点存在、未删）时优先，便于在单一域名下管理任意站点；
//   - 否则使用 Host/Path 解析出的站点（子域名/自定义域名/默认站），保证子域名站写入正确归属。
// 用于文件管理器的建站/上传/列表等操作，使后台可在解析站点或指定站点上下文中工作（SPEC-MS-001 M3）。
func (a *API) operatingSiteID(r *http.Request) string {
	if a.sites != nil && a.isAdmin(r) {
		if v := strings.TrimSpace(r.URL.Query().Get("site")); v != "" {
			if site, err := a.sites.GetSite(r.Context(), v); err == nil && site != nil && site.Status != "deleted" {
				return site.ID
			}
		}
	}
	return currentSiteID(r)
}

// resolveRequestSite 解析当前请求的站点实体（解析失败回落默认站）。
func (a *API) resolveRequestSite(r *http.Request) *service.Site {
	if a.sites == nil {
		return &service.Site{ID: defaultSiteIDFallback, Domain: "*", Status: "active"}
	}
	if site, err := a.sites.ResolveSite(r.Context(), r.Host, r.URL.Path,
		a.cfg.GetString("system.base_domain")); err == nil && site != nil {
		return site
	}
	return &service.Site{ID: defaultSiteIDFallback, Domain: "*", Status: "active"}
}

// requestSiteSettings 解析当前请求的站点配置；未命中（如自定义站尚未建配置）返回 nil，调用方回退默认值。
func (a *API) requestSiteSettings(r *http.Request) *service.SiteSettings {
	if a.sites == nil {
		return nil
	}
	site := a.resolveRequestSite(r)
	ss, err := a.sites.GetSiteSettings(r.Context(), site.ID)
	if err != nil {
		return nil
	}
	return ss
}
