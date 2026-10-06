// Package handler 的 auth.go 实现登录鉴权：Bearer token 会话。
// 安全加固（2026-09-13）：此前全部 /api/v1 端点裸奔（任何人可删改）。
// 设计：
//   - POST /api/v1/auth/login   → 用户名+密码校验（Argon2id）→ 签发 session token（随机 32B hex）
//   - POST /api/v1/auth/logout  → 吊销当前 token
//   - POST /api/v1/auth/password→ 改密（旧密校验 + 吊销其余会话，保留当前）
//   - GET  /api/v1/auth/me      → 当前用户信息
//   - token 有效期 30 天，滑动续期（每次请求刷新 last_seen，超 14 天未用强制重登）
package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

const (
	sessionTTL      = 30 * 24 * time.Hour // 会话总有效期
	sessionIdleTTL  = 14 * 24 * time.Hour // 闲置踢出阈值
	sessionCookie   = "aikmap_session"    // 保留：未来 cookie 模式
	authHeaderToken = "Authorization"
)
const (
	loginFailMax   = 5                // 15 分钟窗口内失败次数上限
	loginFailWin   = 15 * time.Minute // 限流窗口
)

// 登录限流（进程内存）：IP 与 IP+账号 双维度独立计数，超限 429。
type loginThrottle struct {
	mu   sync.Mutex
	fail map[string]*failWin
}
type failWin struct {
	count int
	until int64 // 窗口起点（unix 秒）
}

var loginThrottler = &loginThrottle{fail: map[string]*failWin{}}

// clientIP 取客户端 IP（H8 修复：不再无条件信任 X-Forwarded-For）。
// 默认直连场景取 RemoteAddr；仅当显式开启 security.trust_proxy（部署在可信反代后）时，
// 才信任 X-Forwarded-For 首段 / X-Real-IP——防止攻击者伪造请求头绕过登录限流与审计溯源。
func (a *API) clientIP(r *http.Request) string {
	if a.cfg != nil && a.cfg.GetBool("security.trust_proxy") {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i > 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			return strings.TrimSpace(xr)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// throttleMaxEntries loginThrottle.fail 最大条目数（攻击者可伪造海量 IP 撑爆内存，超限整体重建）
const throttleMaxEntries = 5000

// sweepLocked 清理过期窗口；条目数超上限时整体重建（防御性丢弃旧窗口）。
// 调用方须持有 t.mu。
func (t *loginThrottle) sweepLocked() {
	if len(t.fail) < throttleMaxEntries {
		return
	}
	now := time.Now().Unix()
	for k, w := range t.fail {
		if now-w.until >= int64(loginFailWin.Seconds()) {
			delete(t.fail, k)
		}
	}
	if len(t.fail) > throttleMaxEntries {
		t.fail = map[string]*failWin{}
	}
}

// allow 返回是否放行；被限流时返回剩余等待秒数（0 表示放行）。
func (t *loginThrottle) allow(key string) (bool, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked()
	now := time.Now().Unix()
	w, ok := t.fail[key]
	if !ok {
		return true, 0
	}
	if now-w.until >= int64(loginFailWin.Seconds()) {
		delete(t.fail, key)
		return true, 0
	}
	if w.count >= loginFailMax {
		remain := int(loginFailWin.Seconds()) - int(now-w.until)
		if remain < 1 {
			remain = 1
		}
		return false, remain
	}
	return true, 0
}

// hit 记录一次失败（窗口滑动：超窗重置）。
func (t *loginThrottle) hit(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked()
	now := time.Now().Unix()
	w, ok := t.fail[key]
	if !ok || now-w.until >= int64(loginFailWin.Seconds()) {
		t.fail[key] = &failWin{count: 1, until: now}
		return
	}
	w.count++
}

// 登录成功清除失败计数
func (t *loginThrottle) reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fail, key)
}


// authLogin 处理 POST /api/v1/auth/login
func (a *API) authLogin(w http.ResponseWriter, r *http.Request) {
	// 登录限流：IP 维度 + IP+账号 维度（任一超限即 429）
	ip := a.clientIP(r)
	if ok, _ := loginThrottler.allow("ip:" + ip); !ok {
		writeErr(w, http.StatusTooManyRequests, "LOGIN_RATE_LIMITED", "登录尝试过于频繁，请稍后再试")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQUEST", "用户名和密码不能为空")
		return
	}
	userKey := "u:" + ip + "|" + req.Username
	if ok, _ := loginThrottler.allow(userKey); !ok {
		writeErr(w, http.StatusTooManyRequests, "LOGIN_RATE_LIMITED", "登录尝试过于频繁，请稍后再试")
		return
	}
	var (
		id       string
		passHash string
		status   string
		role     string
		display  string
	)
	err := a.db.QueryRowContext(r.Context(),
		`SELECT id, pass_hash, status, role, display_name FROM users WHERE username=?`,
		req.Username).Scan(&id, &passHash, &status, &role, &display)
	if err != nil {
		// 用户不存在与密码错误同响应（防用户名枚举）
		loginThrottler.hit(userKey)
		loginThrottler.hit("ip:" + ip) // 审计#2：IP 维度同计数，防换号撞库
		writeErr(w, http.StatusUnauthorized, "AUTH_FAILED", "用户名或密码错误")
		return
	}
	// M2 修复：先验密码再查状态——错误密码与"用户不存在"响应完全一致，
	// 账号状态（禁用/待激活）仅在密码正确时才区分，缩小账号枚举面。
	if !service.VerifyPassword(passHash, req.Password) {
		loginThrottler.hit(userKey)
		loginThrottler.hit("ip:" + ip) // 审计#2：IP 维度同计数，防换号撞库
		writeErr(w, http.StatusUnauthorized, "AUTH_FAILED", "用户名或密码错误")
		return
	}
	if status != "active" {
		writeErr(w, http.StatusForbidden, "ACCOUNT_DISABLED", "账号已被禁用")
		return
	}
	token, err := newSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SESSION_CREATE_FAILED", "创建会话失败")
		return
	}
	now := time.Now().Unix()
	expires := now + int64(sessionTTL.Seconds())
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO sessions(token, user_id, created_at, expires_at, last_seen_at) VALUES(?,?,?,?,?)`,
		token, id, now, expires, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "SESSION_CREATE_FAILED", "创建会话失败")
		return
	}
	loginThrottler.reset(userKey)
	loginThrottler.reset("ip:" + ip)
	_, _ = a.aud.Append(r.Context(), id, "auth.login", "sessions", map[string]any{"ok": true})
	// 双轨：HttpOnly cookie（浏览器自动携带，防 XSS 窃取）+ JSON token（API 客户端/第三方用）
	setSessionCookie(w, r, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": expires,
		"user": map[string]any{
			"id": id, "username": req.Username, "role": role,
			"display_name": display,
		},
	})
}

// authLogout 处理 POST /api/v1/auth/logout（吊销当前 token）
func (a *API) authLogout(w http.ResponseWriter, r *http.Request) {
	tok := bearerToken(r)
	if tok != "" {
		_, _ = a.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token=?`, tok)
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// authPassword 处理 POST /api/v1/auth/password（改密：旧密校验 → 新密写入 → 吊销其他会话）
func (a *API) authPassword(w http.ResponseWriter, r *http.Request) {
	uid, ok := ctxUID(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	if len(req.NewPassword) < 8 {
		writeErr(w, http.StatusBadRequest, "WEAK_PASSWORD", "新密码至少 8 位")
		return
	}
	var passHash string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT pass_hash FROM users WHERE id=?`, uid).Scan(&passHash); err != nil {
		writeErr(w, http.StatusInternalServerError, "USER_READ_FAILED", "读取用户失败")
		return
	}
	if !service.VerifyPassword(passHash, req.OldPassword) {
		writeErr(w, http.StatusUnauthorized, "OLD_PASSWORD_WRONG", "旧密码不正确")
		return
	}
	newHash := service.HashPassword(req.NewPassword)
	now := time.Now().Unix()
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE users SET pass_hash=?, updated_at=? WHERE id=?`, newHash, now, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "PASSWORD_UPDATE_FAILED", "密码更新失败")
		return
	}
	// 吊销除当前外的全部会话（改密后其他设备强制重登）
	tok := bearerToken(r)
	if _, err := a.db.ExecContext(r.Context(),
		`DELETE FROM sessions WHERE user_id=? AND token<>?`, uid, tok); err != nil {
		writeErr(w, http.StatusInternalServerError, "SESSION_REVOKE_FAILED", "吊销会话失败")
		return
	}
	_, _ = a.aud.Append(r.Context(), uid, "auth.password", "users", map[string]any{"ok": true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// authMe 处理 GET /api/v1/auth/me
func (a *API) authMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := ctxUID(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	var username, display, role string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT username, display_name, role FROM users WHERE id=?`, uid).
		Scan(&username, &display, &role)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "USER_READ_FAILED", "读取用户失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{"id": uid, "username": username, "role": role, "display_name": display},
	})
}

// newSessionToken 生成 32 字节随机 hex token。
func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// bearerToken 从 Authorization: Bearer <token> 提取 token。
func bearerToken(r *http.Request) string {
	h := r.Header.Get(authHeaderToken)
	if len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	// HttpOnly cookie 兜底（浏览器 SPA 双轨：新会话走 cookie，API 客户端仍用 Bearer）
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

// setSessionCookie 登录成功写入 HttpOnly cookie（SameSite=Strict 防跨站 CSRF；
// Secure 按反代/HTTPS 判断：X-Forwarded-Proto=https、X-Forwarded-Scheme=https 或 TLS 直连时开启，
// 本机 http 不设——兼容本地开发；aiklog.com 等 HTTPS 部署自动生效）
func setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := r.TLS != nil ||
		strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") ||
		strings.EqualFold(r.Header.Get("X-Forwarded-Scheme"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// clearSessionCookie 登出/会话失效时清除 cookie
func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil ||
		strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") ||
		strings.EqualFold(r.Header.Get("X-Forwarded-Scheme"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}
