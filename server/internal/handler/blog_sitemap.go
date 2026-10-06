// blog_sitemap.go 公开 sitemap.xml（SEO）：首页 + 公开博客文章。
package handler

import (
	"encoding/xml"
	"net/http"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type urlset struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// blogSitemap GET /api/v1/blog/sitemap.xml（公开）
func (a *API) blogSitemap(w http.ResponseWriter, r *http.Request) {
	if v, ok := a.cfg.Get("blog.open"); ok {
		if s, _ := v.(string); s == "false" {
			writeErr(w, http.StatusNotFound, "BLOG_CLOSED", "博客已关闭")
			return
		}
	}
	base := a.publicBaseURL(r)
	set := urlset{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	set.URLs = append(set.URLs,
		sitemapURL{Loc: base + "/"},
		sitemapURL{Loc: base + "/blog"},
	)

	var blogDirID string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(dir_id,'') FROM shares
		 WHERE owner_id=? AND token=? AND (revoked_at IS NULL OR revoked_at=0)
		   AND (expires_at IS NULL OR expires_at=0 OR expires_at>?)`,
		a.homeOwnerID(), service.BlogToken, time.Now().UnixMilli()).Scan(&blogDirID)
	if err == nil && blogDirID != "" {
		if files, err := a.collectBlogArticles(r.Context(), blogDirID, currentSiteID(r)); err == nil {
			for _, it := range files {
				p := dirPost(service.BlogToken, "read", 0, it)
				u := sitemapURL{Loc: base + "/" + urlQueryEscape(slugOf(it.f, it.path))}
				if up, _ := p.File["updated_at"].(int64); up > 0 {
					ms := up
					if ms < 1e12 {
						ms *= 1000
					}
					u.LastMod = time.UnixMilli(ms).UTC().Format("2006-01-02")
				}
				set.URLs = append(set.URLs, u)
			}
		}
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(set)
}

// publicBaseURL 根据请求推断对外 origin（反代后用 X-Forwarded-Proto/Host）。
func (a *API) publicBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}
