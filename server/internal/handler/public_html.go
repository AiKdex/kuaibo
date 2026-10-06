// public_html.go 服务端渲染公开博客 HTML（SEO：爬虫可读正文，不依赖 SPA/哈希路由）。
// 路径：
//   GET /blog           文章列表
//   GET /blog/{slug}    单篇文章
//   GET /p/{slug}       兼容别名（与 SPA /p/:token 区分：此处按 slug 解析）
package handler

import (
	_ "embed"
	"encoding/json"
	"html"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
	"github.com/yuin/goldmark"
)

var mdGold = goldmark.New(goldmark.WithExtensions(mdMention{}))

//go:embed blog_home.html
var blogHomeHTML string

// blogHomeTmpl 官网门面页（根路径 /）：设计稿 blog_home.html，SSR 注入站点名/描述/JSON-LD。
var blogHomeTmpl = template.Must(template.New("home").Parse(blogHomeHTML))

// sortPostsForPublic 公开列表统一排序：置顶优先（content_state.front_matter.pinned=true），
// 其余按更新时间倒序。SSR 列表 / RSS / 公开 API 同源调用，保证三处顺序一致。
func sortPostsForPublic(files []dirFile) {
	isPinned := func(it dirFile) bool {
		if it.f == nil || it.f.ContentState == "" {
			return false
		}
		var st struct {
			Pinned      bool `json:"pinned"`
			FrontMatter struct {
				Pinned bool `json:"pinned"`
			} `json:"front_matter"`
		}
		if err := json.Unmarshal([]byte(it.f.ContentState), &st); err != nil {
			return false
		}
		return st.Pinned || st.FrontMatter.Pinned
	}
	sort.SliceStable(files, func(i, j int) bool {
		pi, pj := isPinned(files[i]), isPinned(files[j])
		if pi != pj {
			return pi
		}
		ui, uj := normMillis(files[i].f.UpdatedAt), normMillis(files[j].f.UpdatedAt)
		if ui != uj {
			return ui > uj
		}
		return files[i].path < files[j].path
	})
}

// blogHTMLHome GET / —— 官网门面页（静态 Haskell China 风单页 + SSR 元数据）。
// 与 /blog 列表页区分：这里讲产品，/blog 才是文章列表。
func (a *API) blogHTMLHome(w http.ResponseWriter, r *http.Request) {
	tid := a.blogThemeIDFor(r)
	files, _ := a.blogPostsHTML(r)
	sortPostsForPublic(files)
	items := make([]blogHTMLItem, 0, len(files))
	for _, it := range files {
		items = append(items, blogHTMLItem{
			Slug:    slugOf(it.f, it.path),
			Title:   titleOf(it.f, it.path),
			Preview: cleanPreviewHTML(it.preview),
			Date:    fmtBlogDate(it.f.UpdatedAt),
			Cat:     catOfPath(it.path),
		})
	}
	base := a.publicBaseURL(r)
	data := blogHTMLData{
		SiteName:    a.blogSiteName(r),
		Description: a.blogSiteDesc(r),
		Canonical:   base + "/",
		RSS:         base + "/api/v1/blog/feed.xml",
		JSONLD:      buildBlogJSONLD(true, a.blogSiteName(r), a.blogSiteDesc(r), base+"/", base+"/api/v1/blog/feed.xml", a.blogSiteName(r), "", "", items),
		ThemeID:     tid,
		ThemeCSS:    blogThemeCSS(tid),
		Head:        template.HTML(`<meta property="og:site_name" content="` + html.EscapeString(a.blogSiteName(r)) + `">`),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = blogHomeTmpl.Execute(w, data)
}

// blogHTMLLayout 公开页基础模板（极简、可被搜索引擎完整解析）。
var blogHTMLTmpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="zh-CN" data-theme="{{.ThemeID}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}} · {{.SiteName}}</title>
<meta name="description" content="{{.Description}}">
<link rel="canonical" href="{{.Canonical}}">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:url" content="{{.Canonical}}">
<meta property="og:type" content="{{if .IsList}}website{{else}}article{{end}}">
<link rel="alternate" type="application/rss+xml" title="{{.SiteName}}" href="{{.RSS}}">
<script type="application/ld+json">{{.JSONLD}}</script>
{{.Head}}
<style>
:root{--ink:#1a2332;--ink2:#3d4a5c;--ink3:#6b7a8d;--paper:#eef1f4;--line:#d0d7de;--jade:#0d7a6a;--card:#fff;--font-d:Georgia,'Songti SC','SimSun',serif}
*{box-sizing:border-box}
body{margin:0;font:15px/1.75 system-ui,-apple-system,'PingFang SC','Microsoft YaHei',sans-serif;color:var(--ink);background:var(--paper)}
.wrap{max-width:720px;margin:0 auto;padding:24px 20px 64px}
header.site{border-bottom:1px solid var(--line);background:rgba(255,255,255,.92)}
header.site .in{max-width:720px;margin:0 auto;padding:14px 20px;display:flex;gap:12px;align-items:baseline}
header.site a{color:var(--ink);text-decoration:none;font-family:var(--font-d);font-size:20px;font-weight:600}
header.site span{font-size:12px;color:var(--ink3)}
nav.top{margin-left:auto;display:flex;gap:12px;font-size:13px}
nav.top a{color:var(--ink2);text-decoration:none;font-weight:400}
nav.top a:hover{color:var(--jade)}
.ssr-thsw{margin-left:2px}
.ssr-thsw select{font:inherit;font-size:12px;color:var(--ink2);background:var(--card);border:1px solid var(--line);border-radius:999px;padding:3px 8px;cursor:pointer}
.ssr-thsw select:hover{color:var(--jade);border-color:var(--jade)}
article h1{font-family:var(--font-d);font-size:28px;line-height:1.3;margin:28px 0 10px}
.meta{font-size:12px;color:var(--ink3);margin-bottom:24px;font-variant-numeric:tabular-nums}
.body h1,.body h2,.body h3{font-family:var(--font-d);line-height:1.35}
.body pre{background:#1a2332;color:#d7e0ea;padding:14px 16px;border-radius:8px;overflow:auto;font-size:13px}
.body code{font-family:ui-monospace,Consolas,monospace;font-size:.92em}
.body img{max-width:100%;height:auto;border-radius:8px}
.body a{color:var(--jade)}
.list{list-style:none;margin:0;padding:0}
.list li{background:var(--card);border:1px solid var(--line);border-radius:10px;margin-bottom:12px;padding:16px 18px}
.list a{color:var(--ink);text-decoration:none;font-family:var(--font-d);font-size:18px;font-weight:600}
.list a:hover{color:var(--jade)}
.list .p{margin:6px 0 0;font-size:13px;color:var(--ink2)}
.list .m{margin-top:8px;font-size:11px;color:var(--ink3)}
.lead{margin:0 0 20px;color:var(--ink2);font-size:14px}
.lead a{color:var(--jade)}
footer{max-width:720px;margin:0 auto;padding:0 20px 40px;font-size:12px;color:var(--ink3)}
footer a{color:var(--jade)}
.cmts{margin-top:36px;border-top:1px solid var(--line);padding-top:20px}
.cmts-t{font-family:var(--font-d);font-size:18px;margin:0 0 14px}
.cmts-n{margin-left:6px;font-size:12px;color:var(--ink3);font-weight:400;font-variant-numeric:tabular-nums}
.cmts-l{list-style:none;margin:0;padding:0}
.cmt{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:14px 16px;margin-bottom:10px;scroll-margin-top:16px}
.cmt:target{border-color:var(--jade);box-shadow:0 0 0 3px rgba(13,122,106,.14)}
.cmt-m{display:flex;gap:10px;align-items:baseline;font-size:12px;color:var(--ink3);margin-bottom:6px}
.cmt-m b{color:var(--ink);font-size:13px}
.cmt-m time{margin-left:auto}
.cmt-b{font-size:14px;color:var(--ink2);white-space:pre-wrap;word-break:break-word}
.cmts-e{font-size:13px;color:var(--ink3)}
.cmts-e a{color:var(--jade)}
{{.ThemeCSS}}
</style>
</head>
<body>
<header class="site"><div class="in">
  <a href="/blog">{{.SiteName}}</a>
  <span>{{.SiteTag}}</span>
  <nav class="top">
    <a href="/blog">文章</a>
    <a href="{{.RSS}}">RSS</a>
    <a href="/">官网</a>
    {{if .Themes}}<form class="ssr-thsw" method="get" action="">
      <select name="theme" aria-label="主题" onchange="this.form.submit()">
      {{range .Themes}}<option value="{{.ID}}"{{if eq .ID $.ThemeID}} selected{{end}}>{{.Title}}</option>{{end}}
      </select>
    </form>{{end}}
  </nav>
</div></header>
<div class="wrap">
{{if .IsList}}
  <h1>全部文章</h1>
  <p class="lead">主题在控制台站点设置中修改；此页为服务端渲染（SEO）。</p>
  <ul class="list">
  {{range .Items}}
    <li>
      <a href="/{{.Slug}}">{{.Title}}</a>
      <p class="p">{{.Preview}}</p>
      <div class="m">{{.Date}}{{if .Cat}} · {{.Cat}}{{end}}</div>
    </li>
  {{end}}
  </ul>
  {{if not .Items}}<p>暂无公开文章</p>{{end}}
{{else}}
  <article>
    <h1>{{.Title}}</h1>
    <div class="meta">{{.Date}}{{if .Cat}} · {{.Cat}}{{end}} · <a href="/blog">返回列表</a></div>
    <div class="body">{{.BodyHTML}}</div>
  </article>
  <section class="cmts" id="comments">
    <h2 class="cmts-t">评论{{if .Comments}}<span class="cmts-n">{{len .Comments}}</span>{{end}}</h2>
    {{if .Comments}}<ol class="cmts-l">{{range .Comments}}
      <li class="cmt" id="comment-{{.ID}}">
        <div class="cmt-m"><b>{{.Author}}</b><time>{{.Date}}</time></div>
        <div class="cmt-b">{{.Body}}</div>
      </li>{{end}}
    </ol>{{else}}<p class="cmts-e">还没有评论。到 <a href="/app#/blog?view=public">交互版</a> 留下第一条。</p>{{end}}
  </section>
{{end}}
</div>
<footer>
  <span>{{.SiteName}} · 目录即站点，文件即文章</span>
  · <a href="{{.RSS}}">RSS</a>
  · <a href="/app#/blog?view=public">交互版</a>
  · <a href="/app#/desk">登录 / 注册</a>
</footer>
</body>
</html>`))

type blogHTMLData struct {
	SiteName    string
	SiteTag     string
	Title       string
	Description string
	Canonical   string
	RSS         string
	IsList      bool
	Items       []blogHTMLItem
	BodyHTML    template.HTML
	Date        string
	Cat         string
	JSONLD      template.JS
	Head        template.HTML // 额外 head 注入（OG 增强/主题扩展点；自定义 page.html 可选引用 {{.Head}}）
	ThemeID     string
	ThemeCSS    template.CSS
	Themes      []blogThemeOption // SSR 页头主题下拉（可选）
	Comments    []blogHTMLComment // 文章页评论项（仅文章页填充；B12 锚点闭合）
}

// blogThemeOption SSR 主题下拉项（同时也是 /public/site 的 theme_options 契约）。
type blogThemeOption struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type blogHTMLItem struct {
	Slug    string
	Title   string
	Preview string
	Date    string
	Cat     string
}

// blogHTMLComment SSR 文章页评论项（B12）。
// Body 按**纯文本**输出：评论是访客可写内容，SSR 侧不再解析 Markdown ——
// 多一层解析就多一层 XSS 面，换行交给 CSS white-space:pre-wrap 保留即可。
type blogHTMLComment struct {
	ID     string
	Author string
	Date   string
	Body   string
}

func (a *API) blogOpen(r *http.Request) bool {
	if v, ok := a.cfg.Get("blog.open"); ok {
		if s, _ := v.(string); s == "false" {
			return false
		}
	}
	return true
}

func (a *API) blogSiteName(r *http.Request) string {
	if ss := a.requestSiteSettings(r); ss != nil && ss.Title != "" {
		return ss.Title
	}
	if v, ok := a.cfg.Get("blog.title"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return "爱库录"
}

func (a *API) blogSiteDesc(r *http.Request) string {
	if ss := a.requestSiteSettings(r); ss != nil {
		if ss.Subtitle != "" {
			return ss.Subtitle
		}
		if ss.SEODescription != "" {
			return ss.SEODescription
		}
	}
	if v, ok := a.cfg.Get("blog.description"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return "爱库录 · AI 知识库博客：目录即站点，文件即文章"
}

func (a *API) blogThemeID() string {
	if v, ok := a.cfg.Get("blog.theme"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return "aiklog"
}

// ============ 外置主题（第三方无需改 Go 源码） ============
// 约定：data/themes/<id>/ssr.css 为服务端渲染页（列表 / 文章）的样式；
// 同目录 manifest.json 可写 {"title":"展示名","version":"1.0.0"}。
// 路径按「可执行文件上级目录」解析（bin/../data/themes），与存储根一致，不依赖 cwd。

var themeIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)

// validThemeID 主题 id 合法性：小写字母数字与 . -（兼容市场 id，如 com.aiklog.theme-paper）；
// 显式拒绝 ".."，与 filepath.Join 配合防路径穿越。
func validThemeID(id string) bool {
	return id != "" && !strings.Contains(id, "..") && themeIDRe.MatchString(id)
}

type themeCSSItem struct {
	css string
	mod time.Time
}

var themeCSSCache sync.Map // id -> themeCSSItem

func themesRoot() string {
	base := "data/themes"
	if exe, err := os.Executable(); err == nil {
		base = filepath.Join(filepath.Dir(exe), "..", "data", "themes")
	}
	return base
}

// externalThemeCSS 读取外置主题样式；不存在返回空串（回退内置分支）。
func externalThemeCSS(id string) string {
	if !themeIDRe.MatchString(id) {
		return ""
	}
	p := filepath.Join(themesRoot(), id, "ssr.css")
	st, err := os.Stat(p)
	if err != nil {
		return ""
	}
	if v, ok := themeCSSCache.Load(id); ok {
		if it, ok := v.(themeCSSItem); ok && it.mod.Equal(st.ModTime()) {
			return it.css
		}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(b))
	if len(s) > 256<<10 { // 上限 256KB，防止误塞巨型文件拖垮页面
		s = s[:256<<10]
	}
	themeCSSCache.Store(id, themeCSSItem{css: s, mod: st.ModTime()})
	return s
}

type themePageItem struct {
	tmpl *template.Template
	mod  time.Time
}

var themePageCache sync.Map // id -> themePageItem

// externalThemePage 读取外置主题的可选页面模板 data/themes/<id>/page.html。
// 存在则覆盖内置 SSR 模板（第三方主题可免编译改版式，不只是换色）；
// 缺失 / 超限 / 解析失败一律返回 nil → 回退内置模板，绝不影响公开页可用性。
func externalThemePage(id string) *template.Template {
	if !validThemeID(id) {
		return nil
	}
	p := filepath.Join(themesRoot(), id, "page.html")
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return nil
	}
	if v, ok := themePageCache.Load(id); ok {
		if it, ok := v.(themePageItem); ok && it.mod.Equal(st.ModTime()) {
			return it.tmpl
		}
	}
	b, err := os.ReadFile(p)
	if err != nil || len(b) > 512<<10 { // 上限 512KB
		return nil
	}
	t, err := template.New("theme-" + id).Parse(string(b))
	if err != nil {
		log.Printf("外置主题 %s 的 page.html 解析失败，回退内置模板：%v", id, err)
		return nil
	}
	themePageCache.Store(id, themePageItem{tmpl: t, mod: st.ModTime()})
	return t
}

// blogPageTemplate 选择 SSR 渲染模板：外置主题 page.html 优先，其次内置模板。
func blogPageTemplate(id string) *template.Template {
	if t := externalThemePage(id); t != nil {
		return t
	}
	return blogHTMLTmpl
}

// builtinThemes 内置主题（内置 SSR 样式）；外置目录中的主题会追加在后面。
var builtinThemes = []blogThemeOption{
	{ID: "aiklog", Title: "爱库录"},
	{ID: "minimal", Title: "极简阅读"},
	{ID: "docs", Title: "技术文档"},
	{ID: "paper", Title: "暖纸情报"},
	{ID: "elevated", Title: "高端暗色"},
	{ID: "parchment", Title: "暖纸杂志"},
	{ID: "emforum", Title: "论坛社区"},
	{ID: "brutal", Title: "新粗野"},
	{ID: "aiknav", Title: "好站导航 AikNav"},
	{ID: "chenxi", Title: "晨曦笔记 Chenxi"},
	{ID: "jaded", Title: "翡翠 Jaded"},
	{ID: "zhicang", Title: "知藏 Zhicang"},
	{ID: "zircon", Title: "青璃 Zircon"},
	{ID: "default", Title: "默认"},
}

// blogThemeOptions 下拉项：内置 + data/themes 下的外置主题。
func blogThemeOptions(cur string) []blogThemeOption {
	out := make([]blogThemeOption, 0, len(builtinThemes)+4)
	seen := map[string]bool{}
	for _, t := range builtinThemes {
		seen[t.ID] = true
		out = append(out, t)
	}
	if entries, err := os.ReadDir(themesRoot()); err == nil {
		ids := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() && validThemeID(e.Name()) && !seen[e.Name()] {
				ids = append(ids, e.Name())
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			title := id
			if b, err := os.ReadFile(filepath.Join(themesRoot(), id, "manifest.json")); err == nil {
				var m struct {
					Title string `json:"title"`
					Name  string `json:"name"` // 市场主题包 manifest 用 name（上游协议字段），与 title 兼容
				}
				if json.Unmarshal(b, &m) == nil {
					if t := strings.TrimSpace(m.Title); t != "" {
						title = t
					} else if t := strings.TrimSpace(m.Name); t != "" {
						title = t
					}
				}
			}
			out = append(out, blogThemeOption{ID: id, Title: title})
		}
	}
	// 当前主题可能既不在内置也不在目录里（刚删掉），补上避免下拉丢选中态
	if cur != "" && !seen[cur] {
		found := false
		for _, t := range out {
			if t.ID == cur {
				found = true
				break
			}
		}
		if !found {
			out = append(out, blogThemeOption{ID: cur, Title: cur})
		}
	}
	return out
}

// isKnownThemeID 判定主题 id 是否为「已知主题」：内置白名单（builtinThemes，含纯 SPA 轨主题）
// 或 data/themes/<id>/ 下的外置主题（含只提供 page.html 版式的包）。
// 用途：?theme= 只接受已知主题，避免任意合法字符串被回显进 data-theme、并被下拉的
// 「补选中态」逻辑渲染成一个幽灵选项（如 ?theme=qiuzhi 曾出现在公开 SSR 页的下拉里）。
func isKnownThemeID(id string) bool {
	if !validThemeID(id) {
		return false
	}
	for _, t := range builtinThemes {
		if t.ID == id {
			return true
		}
	}
	base := filepath.Join(themesRoot(), id)
	if st, err := os.Stat(filepath.Join(base, "ssr.css")); err == nil && !st.IsDir() {
		return true
	}
	if st, err := os.Stat(base); err == nil && st.IsDir() {
		return true
	}
	return false
}

// blogThemeIDFor 解析当前请求使用的主题：?theme= 覆盖 > 站点设置 default_theme > 全局配置。
// id 须「合法且已知」：字符合法（防注入/路径穿越）+ 命中 builtinThemes 或 data/themes/，否则回落站点默认（未知 id 不再被回显）。
// 站点关闭访客切换（site_settings.allow_visitor_theme_switch=0）时强制站点默认主题。
func (a *API) blogThemeIDFor(r *http.Request) string {
	base := "aiklog"
	allowSwitch := true
	if ss := a.requestSiteSettings(r); ss != nil {
		if ss.DefaultTheme != "" {
			base = ss.DefaultTheme
		}
		allowSwitch = ss.AllowVisitorThemeSwitch
	} else if t := a.blogThemeID(); t != "" {
		base = t
	}
	if !allowSwitch {
		return base
	}
	if r != nil {
		if q := strings.TrimSpace(r.URL.Query().Get("theme")); q != "" && isKnownThemeID(q) {
			return q
		}
	}
	return base
}

// blogThemeCSS 按主题 id 注入 SSR 覆盖样式（与前端主题气质对齐）。
// 优先外置 data/themes/<id>/ssr.css，其次内置分支；文章页与列表页共用，
// 保证「文章页样式跟着主题走」。
func blogThemeCSS(id string) template.CSS {
	if css := externalThemeCSS(id); css != "" {
		return template.CSS("/* 外置主题：" + id + " */\n" + css)
	}
	switch id {
	case "minimal":
		return template.CSS(`
:root{--ink:#1c1917;--ink2:#57534e;--ink3:#a8a29e;--paper:#fafaf9;--line:#e7e5e4;--jade:#0f766e;--card:#fff}
.wrap,header.site .in,footer{max-width:640px}
header.site{background:transparent;text-align:center;border-bottom-color:var(--line)}
header.site .in{justify-content:center;flex-wrap:wrap}
nav.top{margin-left:0;width:100%;justify-content:center;margin-top:8px}
article h1{font-size:26px;font-weight:600}
.list li{border:none;border-bottom:1px solid var(--line);border-radius:0;background:transparent;padding:22px 0}
.list a{font-size:22px}
`)
	case "docs":
		return template.CSS(`
:root{--ink:#0f172a;--ink2:#334155;--ink3:#94a3b8;--paper:#f8fafc;--line:#e2e8f0;--jade:#0369a1;--card:#fff;--font-d:ui-sans-serif,system-ui,'PingFang SC','Microsoft YaHei',sans-serif}
.wrap,header.site .in,footer{max-width:860px}
header.site{background:#fff}
header.site a{font-family:var(--font-d);font-weight:700}
.list li{border-radius:0;border-left:3px solid var(--jade);border-top:none;border-right:none;border-bottom:1px solid var(--line)}
article h1{font-size:26px;font-weight:700;letter-spacing:-.02em}
.body h1,.body h2,.body h3{font-family:var(--font-d);font-weight:700}
`)
	case "paper":
		return template.CSS(`
:root{--ink:#14202b;--ink2:#3d4d59;--ink3:#7a8a96;--paper:#f7f4ee;--line:#e4ddd2;--jade:#0d5f6e;--card:#fff}
body{background:var(--paper)}
header.site{background:linear-gradient(135deg,#0a4a52 0%,#0d5f6e 50%,#073e47 100%);border:none}
header.site a{color:#fff;font-family:system-ui,sans-serif}
header.site span,header.site nav.top a{color:rgba(255,255,255,.88)}
header.site nav.top a:hover{color:#fff}
.wrap,header.site .in,footer{max-width:860px}
.list li{border:none;border-left:3px solid #ff6b4a;border-radius:12px;box-shadow:0 1px 2px rgba(20,32,43,.05)}
article h1{font-family:system-ui,'PingFang SC',sans-serif;font-weight:700}
footer{color:#7a8a96}
footer a{color:#0d5f6e}
`)
	case "default":
		return template.CSS(`
:root{--ink:#111827;--ink2:#4b5563;--ink3:#9ca3af;--paper:#f9fafb;--line:#e5e7eb;--jade:#2563eb;--card:#fff;--font-d:system-ui,-apple-system,'PingFang SC',sans-serif}
.wrap,header.site .in,footer{max-width:800px}
header.site a,article h1,.list a{font-family:var(--font-d)}
.list li{border-radius:6px}
`)
	case "elevated":
		return template.CSS(`
:root{--ink:#e8eef2;--ink2:#a9bcc7;--ink3:#7b93a1;--paper:#080d12;--line:#1b2732;--jade:#4fd1c5;--card:#0e151c;--font-d:'Songti SC',Georgia,serif}
body{background:var(--paper);color:var(--ink)}
header.site{background:rgba(8,13,18,.9);border-bottom-color:var(--line)}
header.site a{color:var(--ink)}
header.site span{color:var(--ink3)}
nav.top a{color:var(--ink2)}
nav.top a:hover{color:var(--jade)}
.wrap,header.site .in,footer{max-width:760px}
article h1{font-family:var(--font-d);font-weight:600;letter-spacing:.01em}
.body h1,.body h2,.body h3{font-family:var(--font-d)}
.body pre{background:#0e151c;border:1px solid var(--line)}
.body a,.lead a,footer a{color:var(--jade)}
.list li{background:var(--card);border-color:var(--line);border-left:3px solid var(--jade);border-radius:12px}
.list a{color:var(--ink)}
.list a:hover{color:var(--jade)}
.list .p{color:var(--ink2)}
.list .m{color:var(--ink3)}
footer{color:var(--ink3)}
`)
	case "parchment":
		return template.CSS(`
:root{--ink:#2b2118;--ink2:#5b4b3b;--ink3:#8a7660;--paper:#f6efe3;--line:#e2d5c3;--jade:#b06a1b;--card:#fffdf8;--font-d:Georgia,'Songti SC',serif}
body{background:var(--paper);color:var(--ink)}
header.site{background:var(--card);border-bottom:2px solid var(--line)}
header.site a{font-family:var(--font-d);font-size:22px;color:var(--ink)}
nav.top a{color:var(--ink2)}
nav.top a:hover{color:var(--jade)}
.wrap,header.site .in,footer{max-width:820px}
article h1{font-family:var(--font-d);font-size:30px}
.body h1,.body h2,.body h3{font-family:var(--font-d);color:var(--ink)}
.body pre{background:#2b2118;color:#f6efe3}
.body a,.lead a,footer a{color:var(--jade)}
.list li{background:var(--card);border:1px solid var(--line);border-radius:14px}
.list a{font-family:var(--font-d);color:var(--ink)}
.list a:hover{color:var(--jade)}
footer{color:var(--ink3)}
`)
	case "emforum":
		return template.CSS(`
:root{--ink:#eaf0f7;--ink2:#b3c2d4;--ink3:#7d8ea3;--paper:#0d1b2a;--line:#1e3247;--jade:#d4af37;--card:#12263a;--font-d:'PingFang SC',system-ui,sans-serif}
body{background:var(--paper);color:var(--ink)}
header.site{background:linear-gradient(180deg,#0d1b2a,#10253a);border-bottom:2px solid var(--jade)}
header.site a{color:var(--jade);font-family:var(--font-d)}
header.site span{color:var(--ink3)}
nav.top a{color:var(--ink2)}
nav.top a:hover{color:var(--jade)}
.wrap,header.site .in,footer{max-width:900px}
article h1{font-family:var(--font-d);font-weight:700}
.body h1,.body h2,.body h3{font-family:var(--font-d);color:#fff}
.body h2{border-left:4px solid var(--jade);padding-left:10px}
.body pre{background:#081420;color:#e6edf5;border:1px solid var(--line)}
.body a,.lead a,footer a{color:var(--jade)}
.list li{background:var(--card);border:1px solid var(--line);border-left:3px solid var(--jade);border-radius:8px}
.list a{color:var(--ink);font-family:var(--font-d)}
.list a:hover{color:var(--jade)}
.list .p{color:var(--ink2)}
.list .m{color:var(--ink3)}
footer{color:var(--ink3)}
`)
	default: // aiklog
		return template.CSS(`
:root{--ink:#1a2332;--ink2:#3d4a5c;--ink3:#6b7a8d;--paper:#eef1f4;--line:#d0d7de;--jade:#0d7a6a;--card:#fff}
header.site a{letter-spacing:.04em}
header.site a::after{content:'.';color:var(--jade)}
.list li{border-left:3px solid var(--jade)}
`)
	}
}

func (a *API) blogPostsHTML(r *http.Request) ([]dirFile, error) {
	var blogDirID string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(dir_id,'') FROM shares
		 WHERE owner_id=? AND token=? AND (revoked_at IS NULL OR revoked_at=0)
		   AND (expires_at IS NULL OR expires_at=0 OR expires_at>?)`,
		a.homeOwnerID(), service.BlogToken, time.Now().UnixMilli()).Scan(&blogDirID)
	if err != nil || blogDirID == "" {
		return nil, err
	}
	// 公开轨内容隔离：按解析出的站点过滤（currentSiteID 由 siteMiddleware 注入）。
	return a.collectBlogArticles(r.Context(), blogDirID, currentSiteID(r))
}

// buildBlogJSONLD 列表=Blog+ItemList；文章=BlogPosting（schema.org）。
func buildBlogJSONLD(isList bool, siteName, desc, canonical, rss, title, dateISO, bodyText string, items []blogHTMLItem) template.JS {
	type item struct {
		Type    string `json:"@type"`
		Name    string `json:"name"`
		URL     string `json:"url"`
		DateMod string `json:"dateModified,omitempty"`
	}
	var v any
	if isList {
		posts := make([]item, 0, len(items))
		for _, it := range items {
			posts = append(posts, item{
				Type:    "BlogPosting",
				Name:    it.Title,
				URL:     strings.TrimSuffix(canonical, "/") + "/" + it.Slug,
				DateMod: it.Date,
			})
		}
		v = map[string]any{
			"@context":        "https://schema.org",
			"@type":           "Blog",
			"name":            siteName,
			"description":     desc,
			"url":             canonical,
			"headline":        title,
			"blogPost":        posts,
			"sameAs":          []string{rss},
			"inLanguage":      "zh-CN",
			"publisher":       map[string]any{"@type": "Organization", "name": siteName},
		}
	} else {
		wordCount := 0
		for _, r := range bodyText {
			if r != ' ' && r != '\n' && r != '\t' {
				wordCount++
			}
		}
		v = map[string]any{
			"@context":      "https://schema.org",
			"@type":         "BlogPosting",
			"headline":      title,
			"description":   desc,
			"url":           canonical,
			"mainEntityOfPage": map[string]any{"@type": "WebPage", "@id": canonical},
			"dateModified":  dateISO,
			"inLanguage":    "zh-CN",
			"wordCount":     wordCount,
			"publisher": map[string]any{
				"@type": "Organization",
				"name":  siteName,
			},
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(b)
}

func slugOf(f *service.File, path string) string {
	if f != nil && f.Slug != "" {
		return f.Slug
	}
	base := path
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".md")
	base = strings.TrimSuffix(base, ".markdown")
	return base
}

func titleOf(f *service.File, path string) string {
	base := path
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".md")
	base = strings.TrimSuffix(base, ".markdown")
	return base
}

func catOfPath(path string) string {
	if i := strings.IndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return ""
}

func fmtBlogDate(ms int64) string {
	if ms <= 0 {
		return ""
	}
	if ms < 1e12 {
		ms *= 1000
	}
	return time.UnixMilli(ms).Format("2006-01-02")
}

func fmtBlogISO(ms int64) string {
	if ms <= 0 {
		return ""
	}
	if ms < 1e12 {
		ms *= 1000
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func cleanPreviewHTML(s string) string {
	s = strings.ReplaceAll(s, "（降级摘要）", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// blogHTMLIndex GET /blog
func (a *API) blogHTMLIndex(w http.ResponseWriter, r *http.Request) {
	tid := a.blogThemeIDFor(r) // ?theme= 覆盖站点设置（SSR 页头下拉）
	if !a.blogOpen(r) {
		http.NotFound(w, r)
		return
	}
	files, _ := a.blogPostsHTML(r)
	sortPostsForPublic(files) // 置顶优先 + 更新时间倒序（与首页/RSS/公开 API 同源）
	items := make([]blogHTMLItem, 0, len(files))
	for _, it := range files {
		items = append(items, blogHTMLItem{
			Slug:    slugOf(it.f, it.path),
			Title:   titleOf(it.f, it.path),
			Preview: cleanPreviewHTML(it.preview),
			Date:    fmtBlogDate(it.f.UpdatedAt),
			Cat:     catOfPath(it.path),
		})
	}
	base := a.publicBaseURL(r)
	data := blogHTMLData{
		SiteName:    a.blogSiteName(r),
		SiteTag:     "目录即站点，文件即文章",
		Title:       "全部文章",
		Description: a.blogSiteDesc(r),
		Canonical:   base + "/blog",
		RSS:         base + "/api/v1/blog/feed.xml",
		IsList:      true,
		Items:       items,
		JSONLD:      buildBlogJSONLD(true, a.blogSiteName(r), a.blogSiteDesc(r), base, base+"/api/v1/blog/feed.xml", "全部文章", "", "", items),
		ThemeID:     tid,
		ThemeCSS:    blogThemeCSS(tid),
		Themes:      blogThemeOptions(tid),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = blogPageTemplate(tid).Execute(w, data)
}

// blogPostComments 取文章「已通过」评论（SSR 用）。
//
// 存在的理由：归因区块（B3）的回指链接是 {origin}/{slug}#comment-{id}，
// 而 SSR 文章页此前**完全不渲染评论** → 锚点在 SSR 侧无落点，点了停在页首。
// 这里为每条评论输出 id="comment-{id}"，锚点即闭合；顺带让评论内容进入 SSR（SEO 收益）。
//
// 上限 200 条：文章页是首屏路径，不能为超长评论区把 TTFB 拖垮（SPA 侧仍有全量列表）。
// 作者名解析与 commentsList 同序：访客昵称 > display_name > 角色兜底（管理员 / 读者）。
func (a *API) blogPostComments(r *http.Request, fileID string) []blogHTMLComment {
	if fileID == "" {
		return nil
	}
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT c.id, COALESCE(c.guest_name,''), COALESCE(u.display_name,''), COALESCE(u.role,''), c.body, c.created_at
		 FROM comments c LEFT JOIN users u ON u.id = c.user_id
		 WHERE c.file_id=? AND c.status='approved'
		 ORDER BY c.created_at ASC LIMIT 200`, fileID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]blogHTMLComment, 0, 8)
	for rows.Next() {
		var id, guest, uname, urole, body string
		var createdAt int64
		if rows.Scan(&id, &guest, &uname, &urole, &body, &createdAt) != nil {
			continue
		}
		author := strings.TrimSpace(guest)
		if author == "" {
			author = strings.TrimSpace(uname)
		}
		if author == "" {
			if urole == "owner" || urole == "admin" {
				author = "管理员"
			} else {
				author = "读者"
			}
		}
		out = append(out, blogHTMLComment{ID: id, Author: author, Date: fmtBlogDate(createdAt), Body: body})
	}
	return out
}

// blogHTMLPost GET /blog/{slug} 与 GET /{slug}（中文根路径，对齐 emlog 等习惯）
func (a *API) blogHTMLPost(w http.ResponseWriter, r *http.Request) {
	tid := a.blogThemeIDFor(r) // ?theme= 覆盖站点设置（SSR 页头下拉）
	if !a.blogOpen(r) {
		http.NotFound(w, r)
		return
	}
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	// 保留单段路径：交给静态 SPA，不当文章 slug
	switch slug {
	case "app", "favicon.ico", "index.html":
		StaticHandler().ServeHTTP(w, r)
		return
	case "blog", "media", "api", "robots.txt":
		http.NotFound(w, r)
		return
	}
	files, err := a.blogPostsHTML(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var hit *dirFile
	for i := range files {
		if slugOf(files[i].f, files[i].path) == slug {
			hit = &files[i]
			break
		}
	}
	if hit == nil {
		http.NotFound(w, r)
		return
	}
	// 单篇 SEO：seo_title/seo_desc 优先，缺省回退 标题/摘要（2.4 单篇 SEO 元数据）
	title := titleOf(hit.f, hit.path)
	if t := strings.TrimSpace(hit.seoTitle); t != "" {
		title = t
	}
	// C2 修复：付费/密码文章此前在 SSR 页被整页绕过 —— 闸门只在分享通道做了
	// （shares.go requireArticleUnlock / requirePaidAccess），而这里（/blog/{slug} 与 /{slug}）
	// 直接读全文并渲染，正文连同 JSON-LD 里的 raw 全文一并输出，密码保护与付费墙形同虚设。
	// 改为：先用与分享轨同源的"纯判定探针"（不写响应）判锁定，锁定态绝不读正文。
	locked := a.articleLocked(r, hit.f.ID) || a.paidLocked(r, hit.f.ID)
	updatedAt := hit.f.UpdatedAt
	raw := ""
	var htmlOut strings.Builder
	if locked {
		msg := "该文章已加密，请输入访问密码后阅读。"
		if a.articleAccessMode(r, hit.f.ID) == "paid" {
			msg = "该内容为付费内容，完成支付后即可阅读。"
		}
		hint := "正文已按作者设置对外隐藏，摘要之外的全文仅在解锁后可见。"
		if pv := cleanPreviewHTML(hit.preview); pv != "" {
			hint = "摘要预览：" + pv
		}
		htmlOut.WriteString(`<div class="post-locked" style="padding:24px;border:1px dashed rgba(128,128,128,.45);border-radius:10px">`)
		htmlOut.WriteString(`<p style="margin:0 0 8px;font-weight:600">` + html.EscapeString(msg) + `</p>`)
		htmlOut.WriteString(`<p style="margin:0;font-size:13px;opacity:.75">` + html.EscapeString(hint) + `</p>`)
		htmlOut.WriteString(`</div>`)
	} else {
		// 读全文
		rc, f, cerr := a.files.Content(r.Context(), hit.f.ID)
		if cerr != nil {
			http.NotFound(w, r)
			return
		}
		defer rc.Close()
		var sb strings.Builder
		buf := make([]byte, 32*1024)
		for {
			n, er := rc.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if er != nil {
				break
			}
		}
		raw = sb.String()
		updatedAt = f.UpdatedAt
		if err := mdGold.Convert([]byte(raw), &htmlOut); err != nil {
			htmlOut.WriteString("<pre>" + html.EscapeString(raw) + "</pre>")
		}
	}
	base := a.publicBaseURL(r)
	desc := cleanPreviewHTML(hit.preview)
	if d := strings.TrimSpace(hit.seoDesc); d != "" {
		desc = d
	}
	if desc == "" {
		desc = a.blogSiteDesc(r)
	}
	// OG 增强：og:image（封面，相对路径补全为绝对）+ article:published_time + og:site_name（2.4/2.7）
	cover := strings.TrimSpace(hit.cover)
	if cover != "" && strings.HasPrefix(cover, "/") {
		cover = base + cover
	}
	var hb strings.Builder
	hb.WriteString(`<meta property="og:site_name" content="` + html.EscapeString(a.blogSiteName(r)) + `">`)
	if cover != "" {
		hb.WriteString("\n" + `<meta property="og:image" content="` + html.EscapeString(cover) + `">`)
	}
	hb.WriteString("\n" + `<meta property="article:published_time" content="` + fmtBlogISO(updatedAt) + `">`)
	data := blogHTMLData{
		SiteName:    a.blogSiteName(r),
		SiteTag:     "目录即站点，文件即文章",
		Title:       title,
		Description: desc,
		Canonical:   base + "/" + slug,
		RSS:         base + "/api/v1/blog/feed.xml",
		IsList:      false,
		BodyHTML:    template.HTML(htmlOut.String()),
		Date:        fmtBlogDate(updatedAt),
		Cat:         catOfPath(hit.path),
		JSONLD:      buildBlogJSONLD(false, a.blogSiteName(r), desc, base+"/"+slug, base+"/api/v1/blog/feed.xml", title, fmtBlogISO(updatedAt), raw, nil),
		ThemeID:     tid,
		ThemeCSS:    blogThemeCSS(tid),
		Themes:      blogThemeOptions(tid),
		Head:        template.HTML(hb.String()),
		Comments:    a.blogPostComments(r, hit.f.ID),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = blogPageTemplate(tid).Execute(w, data)
}
