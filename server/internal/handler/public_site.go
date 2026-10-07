// public_site.go 博客站点设置公开读取：GET /api/v1/public/site（无需登录）。
// 数据源优先级（SPEC-MS-001 M1）：per-site 配置 site_settings > 全局 blog.* 默认值。
// 默认值：爱库录品牌（可被 DB 设置覆盖）。
package handler

import (
	"encoding/json"
	"net/http"
)

// blogSiteDefaults 博客站点设置默认值（与前端 PublicHome 静态文案对齐）。
func (a *API) blogSiteDefaults() map[string]string {
	return map[string]string{
		"title":       "爱库录",
		"description": "爱库录 · AI 知识库博客：目录即站点，文件即文章",
		"logo":        "",
		"footer":      "爱库录 AiKlog · 数据主权在发布者",
		"seo_default": "",
		"theme":       "aiklog",
		"custom_css":  "",
		"custom_js":   "",
		"base_url":    "",
		"locale":      "zh-cn",
	}
}

// blogSiteGet 读取博客站点设置（公开；RSS/OG/公开页同源）。
// 优先 per-site 配置（site_settings），未命中回退全局 blog.* 配置；最终回退默认值。
func (a *API) blogSiteGet(w http.ResponseWriter, r *http.Request) {
	def := a.blogSiteDefaults()
	out := map[string]string{
		"title":                      def["title"],
		"description":                def["description"],
		"logo":                       def["logo"],
		"footer":                     def["footer"],
		"seo_default":                def["seo_default"],
		"theme":                      def["theme"],
		"custom_css":                 def["custom_css"],
		"custom_js":                  def["custom_js"],
		"base_url":                   def["base_url"],
		"locale":                     def["locale"],
		"ai_ask_open":                "true",
		"allow_visitor_theme_switch": "true",
	}
	if ss := a.requestSiteSettings(r); ss != nil {
		// per-site 配置覆盖默认值
		if ss.Title != "" {
			out["title"] = ss.Title
		}
		if ss.Subtitle != "" {
			out["description"] = ss.Subtitle
		} else if ss.SEODescription != "" {
			out["description"] = ss.SEODescription
		}
		if ss.DefaultTheme != "" {
			out["theme"] = ss.DefaultTheme
		}
		if ss.Locale != "" {
			out["locale"] = ss.Locale
		}
		out["allow_visitor_theme_switch"] = boolToStr(ss.AllowVisitorThemeSwitch)
		// 兜底字段（logo/favicon/footer/seo_default/custom_css/custom_js/icp 等）从 Ext 读取
		var ext map[string]any
		if len(ss.Ext) > 0 {
			_ = json.Unmarshal(ss.Ext, &ext)
		}
		if ext != nil {
			for _, k := range []string{"logo", "footer", "seo_default", "custom_css", "custom_js", "base_url", "favicon", "icp"} {
				if v, ok := ext[k].(string); ok && v != "" {
					out[k] = v
				}
			}
		}
		// 修复：default 站点恒存在（EnsureDefaultSite），per-site 站点记录存在但字段留空时，
		// 必须回退全局 blog.*，否则「站点设置 → 博客名称（站点标题）」改 blog.title 对访客页不生效
		// （此前该回退分支是死代码，与 SSR blogSiteName 行为不一致）。
		for field, key := range map[string]string{
			"title": "blog.title", "description": "blog.description", "logo": "blog.logo",
			"footer": "blog.footer", "seo_default": "blog.seo_default", "theme": "blog.theme",
			"custom_css": "blog.custom_css", "custom_js": "blog.custom_js", "base_url": "blog.base_url", "locale": "blog.locale",
		} {
			if out[field] == def[field] {
				if v, ok := a.cfg.Get(key); ok {
					if s, _ := v.(string); s != "" {
						out[field] = s
					}
				}
			}
		}
	} else {
		// 回退：全局 blog.* 配置（站点配置表未建/未命中时）
		keys := []string{"blog.title", "blog.description", "blog.logo", "blog.footer", "blog.seo_default", "blog.theme",
			"blog.custom_css", "blog.custom_js", "blog.base_url", "blog.locale", "blog.ai_ask_open"}
		for _, k := range keys {
			if v, ok := a.cfg.Get(k); ok {
				if s, _ := v.(string); s != "" {
					key := k[len("blog."):]
					out[key] = s
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"site": out,
		// 主题候选：内置 + data/themes 下已安装的外置主题（应用中心装 theme 包即出现，
		// 无需改前端白名单 / 重新编译）。前端据此渲染下拉，静态页主题会标注"交互版回退默认"。
		"theme_options": blogThemeOptions(out["theme"]),
	})
}

// boolToStr 布尔转 "true"/"false"（与既有 ai_ask_open 字符串约定一致）。
func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
