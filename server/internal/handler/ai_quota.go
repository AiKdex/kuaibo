// ai_quota.go AI 能力分层配额（通用规范 v1.0 落地）。
// 四层身份（游客/登录用户/管理员）× 三道闸门（突发窗口/日配额/全站日预算）。
// 计数落库 ai_ask_usage（day+ident+scope 主键 upsert），重启不清零、可审计。
// 键名约定 blog.ai_ask_*（详见 docs/01-规范/AI能力开放与配额分层通用规范-v1.0.md）。
package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// aiCST 站点受众以北京时间为主，自然日按 UTC+8 切分（与运营报表口径一致）。
var aiCST = time.FixedZone("CST", 8*3600)

// aiAskDay 当前自然日（北京时间）。
func aiAskDay() string { return time.Now().In(aiCST).Format("2006-01-02") }

// aiAskIntSetting 读整数配置：未配置/非法回落默认值（0 有「显式关闭/不限」语义，故不能用 GetInt）。
func (a *API) aiAskIntSetting(key string, def int) int {
	if v := strings.TrimSpace(a.cfg.GetString(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// aiAskIdent 可选鉴权识别身份：Bearer token 有效 → u:<uid>（再按 role 分 admin/user 层）；
// 无 token / 会话失效 → ip:<ip> 游客。**不拒绝任何请求**，只影响配额档位。
func (a *API) aiAskIdent(r *http.Request) (ident, tier string) {
	if tok := bearerToken(r); tok != "" {
		var uid string
		var expiresAt, lastSeen int64
		err := a.db.QueryRowContext(r.Context(),
			`SELECT user_id, expires_at, last_seen_at FROM sessions WHERE token=?`, tok).
			Scan(&uid, &expiresAt, &lastSeen)
		if err == nil && time.Now().Unix() <= expiresAt {
			role := ""
			_ = a.db.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=?`, uid).Scan(&role)
			if role == "owner" || role == "admin" {
				return "u:" + uid, "admin"
			}
			return "u:" + uid, "user"
		}
	}
	return "ip:" + a.aiClientIP(r), "guest"
}

// aiClientIP 取客户端 IP（M12 修复：与登录限流同一口径，走 a.clientIP——
// 默认只用 RemoteAddr，仅 security.trust_proxy=true 时才信任 X-Real-IP/X-Forwarded-For，
// 否则访客可伪造请求头绕过 AI 问答配额。RemoteAddr 必须剥端口，否则每次连接都成新 ident）。
func (a *API) aiClientIP(r *http.Request) string {
	return a.clientIP(r)
}

// aiAskQuotaFor 身份层日配额。admin 0=不限；user 缺省 10；guest 缺省 3、显式 0=关闭游客使用。
func (a *API) aiAskQuotaFor(tier string) int {
	switch tier {
	case "admin":
		return a.aiAskIntSetting("blog.ai_ask_admin_quota", 0)
	case "user":
		return a.aiAskIntSetting("blog.ai_ask_user_quota", 10)
	default:
		return a.aiAskIntSetting("blog.ai_ask_guest_quota", 3)
	}
}

// aiAskUsed 某 ident 当日已用次数。
func (a *API) aiAskUsed(day, ident string) int {
	var n int
	_ = a.db.QueryRow(`SELECT COALESCE(hits,0) FROM ai_ask_usage WHERE day=? AND ident=? AND scope=?`,
		day, ident, "blog_ask").Scan(&n)
	return n
}

// aiAskDayTotal 全站当日总次数（日预算熔断用）。
func (a *API) aiAskDayTotal(day string) int {
	var n int
	_ = a.db.QueryRow(`SELECT COALESCE(SUM(hits),0) FROM ai_ask_usage WHERE day=? AND scope=?`,
		day, "blog_ask").Scan(&n)
	return n
}

// aiAskIncr 计数 +1（先计数后调用模型：失败也占额，防刷）。
func (a *API) aiAskIncr(day, ident string) {
	_, _ = a.db.Exec(`INSERT INTO ai_ask_usage(day,ident,scope,hits) VALUES(?,?,?,1)
		ON CONFLICT(day,ident,scope) DO UPDATE SET hits=hits+1`, day, ident, "blog_ask")
}

// aiAskGuard 配额闸门：通过则计数并返回 ""；不通过返回 (http码, code, 文案)。
// budget=true 时额外检查全站日预算（quota 端点预览用 false）。
func (a *API) aiAskGuard(r *http.Request, withIncr bool) (ident, tier string, limit, used, remaining int, httpCode int, code, msg string) {
	day := aiAskDay()
	ident, tier = a.aiAskIdent(r)
	limit = a.aiAskQuotaFor(tier)
	used = a.aiAskUsed(day, ident)

	// 游客被显式关闭
	if tier == "guest" && limit == 0 {
		return ident, tier, 0, used, 0, http.StatusTooManyRequests, "AI_QUOTA_GUEST_OFF",
			"游客 AI 问答未开放，登录后可使用"
	}
	remaining = limit - used
	if limit > 0 && remaining <= 0 {
		if tier == "guest" {
			return ident, tier, limit, used, 0, http.StatusTooManyRequests, "AI_QUOTA_GUEST",
				"今日游客提问次数已用完，登录后可获得更多次数"
		}
		return ident, tier, limit, used, 0, http.StatusTooManyRequests, "AI_QUOTA_USER",
			"今日提问次数已用完，明天再来吧"
	}
	// 全站日预算（0=不限）
	if budget := a.aiAskIntSetting("blog.ai_ask_daily_budget", 200); budget > 0 {
		if a.aiAskDayTotal(day) >= budget {
			return ident, tier, limit, used, remaining, http.StatusTooManyRequests, "AI_BUDGET_FUSE",
				"今日 AI 问答已达全站用量上限，明天恢复"
		}
	}
	if withIncr {
		a.aiAskIncr(day, ident)
		if limit > 0 {
			remaining = limit - used - 1
		}
	}
	return ident, tier, limit, used, remaining, 0, "", ""
}

// publicBlogAskQuota GET /api/v1/public/blog/ask/quota（公开）：当前身份余量预览，不计数不调模型。
func (a *API) publicBlogAskQuota(w http.ResponseWriter, r *http.Request) {
	if v, ok := a.cfg.Get("blog.ai_ask_open"); ok {
		if s, _ := v.(string); s == "false" {
			writeJSON(w, http.StatusOK, map[string]any{"open": false})
			return
		}
	}
	_, tier, limit, used, remaining, _, _, _ := a.aiAskGuard(r, false)
	writeJSON(w, http.StatusOK, map[string]any{
		"open": true, "tier": tier, "limit": limit, "used": used, "remaining": remaining,
	})
}

// adminAIUsage GET /api/v1/admin/ai/usage?days=7：按日聚合 + Top ident（运营/账单底座）。
func (a *API) adminAIUsage(w http.ResponseWriter, r *http.Request) {
	days := 7
	if n, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && n > 0 && n <= 90 {
		days = n
	}
	rows, err := a.db.Query(`SELECT day, SUM(hits) FROM ai_ask_usage WHERE scope=? GROUP BY day ORDER BY day DESC LIMIT ?`,
		"blog_ask", days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "AI_USAGE_ERR", "用量查询失败")
		return
	}
	defer rows.Close()
	daily := []map[string]any{}
	for rows.Next() {
		var day string
		var hits int
		_ = rows.Scan(&day, &hits)
		daily = append(daily, map[string]any{"day": day, "hits": hits})
	}
	rows2, err := a.db.Query(`SELECT ident, SUM(hits) FROM ai_ask_usage WHERE scope=? AND day=? GROUP BY ident ORDER BY SUM(hits) DESC LIMIT 10`,
		"blog_ask", aiAskDay())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "AI_USAGE_ERR", "用量查询失败")
		return
	}
	defer rows2.Close()
	top := []map[string]any{}
	for rows2.Next() {
		var ident string
		var hits int
		_ = rows2.Scan(&ident, &hits)
		// ident 脱敏：ip 只留前两段，u: 只留 id（后台管理员可见，但仍避免全量 IP 入页面缓存）
		if s, ok := strings.CutPrefix(ident, "ip:"); ok {
			parts := strings.Split(s, ".")
			if len(parts) >= 3 {
				ident = "ip:" + strings.Join(parts[:2], ".") + ".*"
			}
		}
		top = append(top, map[string]any{"ident": ident, "hits": hits})
	}
	writeJSON(w, http.StatusOK, map[string]any{"daily": daily, "top": top,
		"guest_quota": a.aiAskIntSetting("blog.ai_ask_guest_quota", 3),
		"user_quota":  a.aiAskIntSetting("blog.ai_ask_user_quota", 10)})
}
