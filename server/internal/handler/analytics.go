// analytics.go 内容数据中心（A5）：阅读按天聚合 + 单篇热力图 + 公开热门榜 + 站级看板。
//
//	口径：view_events 按 (file_id, day) 聚合阅读事件，写入口唯一 = 公开页 PV 上报
//	（publicBlogPV，首访去重后与 files.view_count 同步累加），因此两个口径始终一致。
//	可见性：热门榜复用 collectDirFilesOpt 的过滤（草稿 / 定时未发布 / 站点隔离），
//	不另写 SQL，避免与公开列表口径分叉导致草稿外泄。
//
// 移植来源：上游 AiKmap.cn handler/analytics.go（3a9c2dc），按本壳鉴权与多站点约定改造
// （鉴权改用 blogAdminOnly、可见性改用 collectDirFilesOpt、列表按 site_id 隔离）。
package handler

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// viewEventDay 本地时区当天 key（与写入侧格式一致，YYYY-MM-DD）。
func viewEventDay(t time.Time) string { return t.Format("2006-01-02") }

// recordViewEvent 按天累加某文章的阅读次数。
// 与 files.view_count 在同一分支调用（首访去重后），保证热力图与累计阅读量不打架。
func (a *API) recordViewEvent(ctx context.Context, fileID string) {
	if fileID == "" {
		return
	}
	_, _ = a.db.ExecContext(ctx,
		`INSERT INTO view_events (file_id, day, count) VALUES (?, ?, 1)
		 ON CONFLICT(file_id, day) DO UPDATE SET count = count + 1`,
		fileID, viewEventDay(time.Now()))
}

// analyticsDays 解析 days 查询参数（1..90，非法/越界回落 def）。
func analyticsDays(r *http.Request, def int) int {
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 90 {
			return n
		}
	}
	return def
}

// heatSeries 取近 N 天阅读序列（缺天补 0）。
// fileID 为空 = 全站汇总；非空 = 单篇。
func (a *API) heatSeries(ctx context.Context, fileID string, days int) ([]map[string]any, int64, error) {
	from := viewEventDay(time.Now().AddDate(0, 0, -(days - 1)))
	q := `SELECT day, count FROM view_events WHERE day>=?`
	args := []any{from}
	if fileID != "" {
		q = `SELECT day, count FROM view_events WHERE file_id=? AND day>=?`
		args = []any{fileID, from}
	}
	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	byDay := map[string]int64{}
	for rows.Next() {
		var d string
		var c int64
		if rows.Scan(&d, &c) == nil {
			byDay[d] += c
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	out := make([]map[string]any, 0, days)
	var total int64
	start := time.Now().AddDate(0, 0, -(days - 1))
	for i := 0; i < days; i++ {
		day := viewEventDay(start.AddDate(0, 0, i))
		c := byDay[day]
		total += c
		out = append(out, map[string]any{"day": day, "count": c})
	}
	return out, total, nil
}

// fileHeatmap GET /api/v1/files/{id}/heatmap?days=30：单篇近 N 天阅读热力图（缺天补 0）。
// 站长后台数据面。
func (a *API) fileHeatmap(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "FILE_ID_REQUIRED", "缺少文件 id")
		return
	}
	days := analyticsDays(r, 30)
	series, total, err := a.heatSeries(r.Context(), id, days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "HEATMAP_FAILED", err.Error())
		return
	}
	// 累计阅读量（files.view_count）与近 N 天窗口量分开给出：前者是历史总量，后者用于趋势。
	var lifetime int64
	_ = a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(view_count,0) FROM files WHERE id=?`, id).Scan(&lifetime)
	writeJSON(w, http.StatusOK, map[string]any{
		"file_id": id, "days": days, "total": total, "lifetime": lifetime, "series": series,
	})
}

// popularPosts GET /api/v1/public/popular?limit=10：公开热门文章（按 view_count 降序）。
// 匿名可读（authmw 白名单）；可见性沿用公开列表口径，草稿/定时未发布不会出现在榜单。
func (a *API) popularPosts(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	items, err := a.collectBlogArticles(r.Context(), service.BlogDirID, currentSiteID(r))
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"posts": []any{}})
			return
		}
		writeErr(w, http.StatusInternalServerError, "POPULAR_FAILED", err.Error())
		return
	}
	// 一次性补齐 view_count：collectDirFilesOpt 刻意不查该列（常态列表不需要），
	// 榜单才需要排序依据，故在此单独取，避免给常规列表加开销。
	vc := map[string]int64{}
	if len(items) > 0 {
		ids := make([]any, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.f.ID)
		}
		ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		if rows, err := a.db.QueryContext(r.Context(),
			`SELECT id, COALESCE(view_count,0) FROM files WHERE id IN (`+ph+`)`, ids...); err == nil {
			for rows.Next() {
				var id string
				var n int64
				if rows.Scan(&id, &n) == nil {
					vc[id] = n
				}
			}
			rows.Close()
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		vi, vj := vc[items[i].f.ID], vc[items[j].f.ID]
		if vi != vj {
			return vi > vj
		}
		return items[i].f.UpdatedAt > items[j].f.UpdatedAt // 同阅读量按更新倒序
	})
	if len(items) > limit {
		items = items[:limit]
	}
	posts := make([]map[string]any, 0, len(items))
	for _, it := range items {
		posts = append(posts, map[string]any{
			"id":         it.f.ID,
			"name":       it.f.Name,
			"slug":       it.f.Slug,
			"path":       it.path,
			"view_count": vc[it.f.ID],
			"updated_at": it.f.UpdatedAt,
			"author":     it.author,
			"cover":      it.cover,
			"excerpt":    it.excerpt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": posts})
}

// adminAnalyticsOverview GET /api/v1/admin/analytics/overview?days=30：站级阅读看板。
// 返回全站按天序列 + 窗口内 top 文章；站长后台。
func (a *API) adminAnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	days := analyticsDays(r, 30)
	series, total, err := a.heatSeries(r.Context(), "", days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "OVERVIEW_FAILED", err.Error())
		return
	}
	from := viewEventDay(time.Now().AddDate(0, 0, -(days - 1)))
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT v.file_id,
		        SUM(v.count) AS window_views,
		        COALESCE(f.name,''), COALESCE(f.slug,''), COALESCE(f.view_count,0)
		 FROM view_events v
		 LEFT JOIN files f ON f.id = v.file_id
		 WHERE v.day >= ?
		 GROUP BY v.file_id
		 ORDER BY window_views DESC
		 LIMIT 20`, from)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "OVERVIEW_FAILED", err.Error())
		return
	}
	defer rows.Close()
	top := []map[string]any{}
	for rows.Next() {
		var fid, name, slug string
		var win, life int64
		if rows.Scan(&fid, &win, &name, &slug, &life) != nil {
			continue
		}
		top = append(top, map[string]any{
			"id": fid, "name": name, "slug": slug,
			"window_views": win, "view_count": life,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days": days, "total": total, "series": series, "top": top,
	})
}
