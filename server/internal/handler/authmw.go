// Package handler 的 authmw.go：Bearer token 鉴权中间件。
// 安全加固（2026-09-13）：此前全部 /api/v1 端点裸奔，任何人可删改。
// 规则：
//   - 公开端点白名单（无需登录）：health/version、公开分享（shares token 内容）、
//     对外博客、静态前端资源（页面本身无数据，数据全走 API）。
//   - 其余 /api/v1/* 一律要求 Authorization: Bearer <token>；无效/过期 → 401。
//   - 会话闲置超 14 天踢出；总有效期 30 天。
package handler

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type ctxKey int

// ctxUserID 注入鉴权后的用户 id。
const ctxUserID ctxKey = 1

// publicPath 判定是否公开端点（含精确与方法敏感路径）。
func publicPath(r *http.Request) bool {
	p := r.URL.Path
	if !strings.HasPrefix(p, "/api/v1/") {
		return true // 静态前端资源公开（无数据面）
	}
	// 精确公开端点（按方法敏感：写操作一律不公开）
	switch {
	case p == "/api/v1/health" || p == "/api/v1/version":
		return true
	case p == "/api/v1/auth/login" && r.Method == http.MethodPost:
		return true
	case p == "/api/v1/auth/register" && r.Method == http.MethodPost:
		return true // 开放注册（受 site.registration_open 门禁；内部做限流/用户名校验）
	case p == "/api/v1/auth/email-code" && r.Method == http.MethodPost:
		return true // 邮箱验证码发送（注册/绑定流程第一步；内部做邮箱格式+限流校验）
	case p == "/api/v1/auth/config" && r.Method == http.MethodGet:
		return true // 登录页能力探测（多用户/开放注册/邮箱验证开关）公开读
	case p == "/api/v1/public/posts" && r.Method == http.MethodGet:
		return true
	case p == "/api/v1/public/tags" && r.Method == http.MethodGet:
		return true // 公开标签聚合（标签云 / 标签内页数据源）
	case p == "/api/v1/public/site" && r.Method == http.MethodGet:
		return true // 博客站点设置公开读（RSS/OG/主题同源）
	case p == "/api/v1/public/comments" && r.Method == http.MethodGet:
		return true // 评论公开读；写评论（POST /api/v1/comments）走登录
	case p == "/api/v1/public/unlock" && r.Method == http.MethodPost:
		return true // 2.2 文章密码解锁（匿名；校验失败 403，成功发会话级解锁凭证）
	case p == "/api/v1/public/comments" && r.Method == http.MethodPost:
		return true // 访客评论（受 blog.comments_guest 开关 + IP 限流；默认待审不直接公开）
	case p == "/api/v1/public/blog/ask" && r.Method == http.MethodPost:
		return true // 读者 AI 问答（分层配额：游客/用户/管理员三档 + 全站日预算；仅公开文章上下文）
	case p == "/api/v1/public/blog/ask/quota" && r.Method == http.MethodGet:
		return true // AI 问答余量预览（公开；不计数不调模型，主题可渲染剩余次数）
	case p == "/api/v1/public/blog/pv" && (r.Method == http.MethodPost || r.Method == http.MethodGet):
		return true // 公开 PV 计数
	case strings.HasPrefix(p, "/api/v1/public/media/") && r.Method == http.MethodGet:
		return true // 博客公开图片（仅 image/*，仅博客子树，handler 内二次校验）
	case p == "/api/v1/blog/feed.xml" && r.Method == http.MethodGet:
		return true
	case p == "/api/v1/blog/sitemap.xml" && r.Method == http.MethodGet:
		return true
	case p == "/api/v1/blog/plugins" && r.Method == http.MethodGet:
		return true // 插件列表公开（前端渲染用）；注册/启停/卸载走登录
	case p == "/api/v1/im/webhook/telegram" && r.Method == http.MethodPost:
		return true // IM webhook 仅 POST 回调公开
	case p == "/api/v1/im/webhook/wecom" && (r.Method == http.MethodPost || r.Method == http.MethodGet):
		return true // 企微回调 POST 消息 + GET URL 验证均公开
	case p == "/api/v1/im/status" && r.Method == http.MethodGet:
		return true
	}
	// 公开分享 token 内容（博客外链）——仅 GET；撤销分享（DELETE）等写操作走登录
	if strings.HasPrefix(p, "/api/v1/shares/") && r.Method == http.MethodGet &&
		(p == "/api/v1/shares/"+shareTokenPart(p) || strings.HasSuffix(p, "/content")) {
		return true
	}
	// 插件数据 KV 公开读（评论等公开数据；写操作仍鉴权）
	if strings.HasPrefix(p, "/api/v1/blog/plugins/") && strings.HasSuffix(p, "/kv") && r.Method == http.MethodGet {
		return true
	}
	// 插件设置 schema 公开读（无敏感信息；前端动态渲染设置表单）
	if strings.HasPrefix(p, "/api/v1/blog/plugins/") && strings.HasSuffix(p, "/settings-schema") && r.Method == http.MethodGet {
		return true
	}
	return false
}

// shareTokenPart 从 /api/v1/shares/{token}[/content] 提取 token 段（校验形态：uuid 或 32hex）。
func shareTokenPart(p string) string {
	rest := strings.TrimPrefix(p, "/api/v1/shares/")
	if rest == "" {
		return ""
	}
	seg := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		seg = rest[:i]
	}
	return seg
}

// authMiddleware 校验 Bearer token（公开端点放行）。
func (a *API) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		tok := bearerToken(r)
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "需要登录")
			return
		}
		var (
			uid       string
			expiresAt int64
			lastSeen  int64
		)
		err := a.db.QueryRowContext(r.Context(),
			`SELECT user_id, expires_at, last_seen_at FROM sessions WHERE token=?`, tok).
			Scan(&uid, &expiresAt, &lastSeen)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "会话无效或已过期，请重新登录")
			return
		}
		now := time.Now().Unix()
		if now > expiresAt || now-lastSeen > int64(sessionIdleTTL.Seconds()) {
			_, _ = a.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token=?`, tok)
			writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "会话已过期，请重新登录")
			return
		}
		// 滑动续期：距上次活跃超 1 小时才写库（降写放大）
		if now-lastSeen > 3600 {
			_, _ = a.db.ExecContext(r.Context(),
				`UPDATE sessions SET last_seen_at=? WHERE token=?`, now, tok)
		}
		ctx := context.WithValue(r.Context(), ctxUserID, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
