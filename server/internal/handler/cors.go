// cors.go CORS 中间件：外部频道站（qiuzhi/zhaobiao/lvshi 子域名等）跨源读取知识库数据用。
// 设计：默认关闭（未配置 = 同源，最安全）；配置 server.cors_origins 逗号分隔白名单后
// 仅放行匹配来源；"*" 表示放行全部来源（公开展示场景）。
package handler

import (
	"net/http"
	"strings"
)

// corsMiddleware 按白名单放行跨源请求；未配置时原样透传（关闭 CORS）。
func corsMiddleware(next http.Handler, allowedOrigins string) http.Handler {
	origins := parseOrigins(allowedOrigins)
	if len(origins) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && originAllowed(origin, origins) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			h.Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// parseOrigins 解析白名单配置：逗号分隔；"*" 与空元素忽略。
func parseOrigins(cfg string) []string {
	var out []string
	for _, s := range strings.Split(cfg, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, strings.TrimSuffix(s, "/"))
	}
	return out
}

// originAllowed 判断来源是否在白名单内。
func originAllowed(origin string, origins []string) bool {
	origin = strings.TrimSuffix(origin, "/")
	for _, o := range origins {
		if o == "*" || o == origin {
			return true
		}
	}
	return false
}
