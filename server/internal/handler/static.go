// Package handler 提供 HTTP/WS 入口。
// static.go 内嵌前端构建产物（单二进制原则，实施文档 §2.5）：Go 二进制直接提供 Web UI。
// 构建：先 `cd web && npm run build`，再 `go build`（go:embed 自动包含 dist）。
package handler

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:webdist
var webFS embed.FS

// StaticHandler 返回前端静态资源处理器（SPA：未知路径回退 index.html）。
func StaticHandler() http.Handler {
	sub, err := fs.Sub(webFS, "webdist")
	if err != nil {
		// 理论不可达（embed 编译期保证）
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/" {
			serveIndex(w, r, sub)
			return
		}
		// 存在则直接服务；否则 SPA 回退
		if f, err := sub.Open(strings.TrimPrefix(p, "/")); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, sub)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, sub fs.FS) {
	data, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// serveFavicon 显式服务根级 favicon。若不单独注册，单段带点路径 /favicon.svg 会被
// 博客通配路由 GET /{slug} 抢走而返回 404（见 routes.go）；此处直接交给 StaticHandler，
// 由 http.FileServer 按扩展名给出正确 Content-Type（image/svg+xml）。
func (a *API) serveFavicon(w http.ResponseWriter, r *http.Request) {
	StaticHandler().ServeHTTP(w, r)
}

// serveCsWidget 显式服务客服挂件脚本（webdist 根级 cs-widget.js）。
// 与 serveFavicon 同理：单段带点路径会被博客通配路由 GET /{slug} 抢走返回 404，
// 故必须显式注册；由 http.FileServer 按 .js 出 application/javascript。
func (a *API) serveCsWidget(w http.ResponseWriter, r *http.Request) {
	StaticHandler().ServeHTTP(w, r)
}
