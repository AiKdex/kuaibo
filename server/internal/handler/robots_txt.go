// robots_txt.go 公开 robots.txt（SEO）
package handler

import (
	"net/http"
)

// blogRobots GET /robots.txt
func (a *API) blogRobots(w http.ResponseWriter, r *http.Request) {
	base := a.publicBaseURL(r)
	body := "User-agent: *\nAllow: /\nDisallow: /app#/desk\nDisallow: /api/\nSitemap: " + base + "/api/v1/blog/sitemap.xml\n"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(body))
}
