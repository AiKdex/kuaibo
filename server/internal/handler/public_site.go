// public_site.go 博客站点设置公开读取：GET /api/v1/public/site（无需登录）。
// 数据源 = settings 表 blog.* 白名单键（博客管理页可改）；供公开页/主题/RSS/OG 同源使用。
// 默认值：爱库录品牌（可被 DB 设置覆盖）。
package handler

import "net/http"

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
func (a *API) blogSiteGet(w http.ResponseWriter, r *http.Request) {
	keys := []string{"blog.title", "blog.description", "blog.logo", "blog.footer", "blog.seo_default", "blog.theme",
		"blog.custom_css", "blog.custom_js", "blog.base_url", "blog.locale", "blog.ai_ask_open"}
	def := a.blogSiteDefaults()
	out := map[string]string{
		"title":       def["title"],
		"description": def["description"],
		"logo":        def["logo"],
		"footer":      def["footer"],
		"seo_default": def["seo_default"],
		"theme":       def["theme"],
		"custom_css":  def["custom_css"],
		"custom_js":   def["custom_js"],
		"base_url":    def["base_url"],
		"locale":      def["locale"],
		"ai_ask_open": "true", // 默认开；settings 有显式 false 时被下方循环覆盖，主题据此隐藏 AI 对话窗
	}
	for _, k := range keys {
		if v, ok := a.cfg.Get(k); ok {
			if s, _ := v.(string); s != "" {
				key := k[len("blog."):]
				out[key] = s
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
