// account.go —— 账号与站点配置（多用户：开放注册 / 邮箱验证码 / 个人资料）。
//
//	POST /api/v1/auth/register    开放注册（受 site.registration_open 门禁；site.email_required 时强制邮箱+验证码）
//	POST /api/v1/auth/email-code  发送邮箱验证码（register|bind）
//	POST /api/v1/auth/email-bind  登录用户绑定邮箱
//	PUT  /api/v1/auth/profile      更新个人资料（昵称/签名/头像）
//
// SMTP 由 settings 白名单键 smtp.* 配置；未配置且未开 debug 时验证码接口返回 503。
// 代码自 AiKmap 上游同构移植，去掉 spaces 概念（博客用 homeOwnerID 归属）。
package handler

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"regexp"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

var emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)

// smtpEnabled 发信能力是否可用（开关 + host）。
func (a *API) smtpEnabled() bool {
	return a.cfg.GetString("smtp.enable") == "true" && strings.TrimSpace(a.cfg.GetString("smtp.host")) != ""
}

// smtpDebug 验证码调试模式（验证码写日志+随响应返回，仅开发用）。
func (a *API) smtpDebug() bool {
	return a.cfg.GetString("smtp.debug_code") == "true"
}

// smtpSend 经 SMTP 发信（标准库 net/smtp；user/pass 为空=免认证；端口 465 尝试 SSL，其余 STARTTLS）。
func smtpSend(host, port, user, pass, from, to, subject, body string) error {
	addr := host + ":" + port
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	msg := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		body
	if port == "465" {
		return smtpSendSSL(addr, host, auth, from, to, msg)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

func tlsDial(addr string) (*tls.Conn, error) {
	return tls.Dial("tcp", addr, &tls.Config{ServerName: addrHost(addr)})
}

func addrHost(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

func smtpSendSSL(addr, host string, auth smtp.Auth, from, to, msg string) error {
	conn, err := tlsDial(addr)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	return w.Close()
}

// genEmailCode 6 位数字验证码。
func genEmailCode() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%06d", n%1000000)
}

// emailCodeSend 发送验证码：POST /api/v1/auth/email-code。
func (a *API) emailCodeSend(w http.ResponseWriter, r *http.Request) {
	if !a.smtpEnabled() && !a.smtpDebug() {
		writeErr(w, http.StatusServiceUnavailable, "EMAIL_NOT_CONFIGURED", "发送邮箱未配置（设置页 → 发信邮箱），暂不能发送验证码")
		return
	}
	var req struct {
		Email   string `json:"email"`
		Purpose string `json:"purpose"` // register|bind
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if !emailRe.MatchString(email) || len(email) > 200 {
		writeErr(w, http.StatusBadRequest, "BAD_EMAIL", "邮箱格式不正确")
		return
	}
	purpose := req.Purpose
	if purpose != "register" && purpose != "bind" {
		purpose = "register"
	}
	if purpose == "bind" {
		uid := a.curUserID(r)
		if uid == "" {
			writeErr(w, http.StatusUnauthorized, "LOGIN_REQUIRED", "请先登录")
			return
		}
	}
	// 邮箱已被占用（register/bind 均不可用他人邮箱）
	var exists string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT id FROM users WHERE email=?`, email).Scan(&exists); err == nil {
		writeErr(w, http.StatusConflict, "EMAIL_TAKEN", "该邮箱已被其他账号使用")
		return
	}
	ip := a.clientIP(r)
	if ok, _ := loginThrottler.allow("ec:" + ip + "|" + email); !ok {
		writeErr(w, http.StatusTooManyRequests, "CODE_RATE_LIMITED", "发送过于频繁，请稍后再试")
		return
	}
	now := time.Now()
	code := genEmailCode()
	_, _ = a.db.ExecContext(r.Context(),
		`DELETE FROM email_codes WHERE email=? OR expires_at<?`, email, now.UnixMilli())
	_, err := a.db.ExecContext(r.Context(),
		`INSERT INTO email_codes (id, email, code, purpose, expires_at, created_at) VALUES (?,?,?,?,?,?)`,
		newID(), email, code, purpose, now.Add(10*time.Minute).UnixMilli(), now.UnixMilli())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CODE_SAVE_FAILED", "验证码保存失败")
		return
	}
	subject := "验证码 " + code + "（" + purposeText(purpose) + "）"
	body := "你的验证码是：" + code + "\n10 分钟内有效。若非本人操作请忽略本邮件。"
	if a.smtpEnabled() && !a.smtpDebug() {
		from := strings.TrimSpace(a.cfg.GetString("smtp.from"))
		if from == "" {
			from = strings.TrimSpace(a.cfg.GetString("smtp.user"))
		}
		if from == "" {
			from = email
		}
		if err := smtpSend(
			a.cfg.GetString("smtp.host"), a.cfg.GetString("smtp.port"),
			a.cfg.GetString("smtp.user"), a.cfg.GetString("smtp.pass"),
			from, email, subject, body); err != nil {
			writeErr(w, http.StatusInternalServerError, "EMAIL_SEND_FAILED", "邮件发送失败："+err.Error())
			return
		}
	}
	// 发送成功即计数（审计#2：防验证码轰炸；失败路径不计数以免误伤正常重试）
	loginThrottler.hit("ec:" + ip + "|" + email)
	if a.smtpDebug() {
		log.Printf("[email-code] debug_mode: %s -> code=%s purpose=%s", email, code, purpose)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "debug_code": code, "debug": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func purposeText(p string) string {
	if p == "bind" {
		return "绑定邮箱"
	}
	return "注册账号"
}

// emailCodeValid 校验验证码并一次性消费（抢占 used_at）。
func (a *API) emailCodeValid(ctx context.Context, email, code, purpose string) error {
	var id string
	err := a.db.QueryRowContext(ctx,
		`SELECT id FROM email_codes WHERE email=? AND code=? AND purpose=? AND used_at IS NULL AND expires_at>?`,
		email, code, purpose, time.Now().UnixMilli()).Scan(&id)
	if err != nil {
		return fmt.Errorf("验证码错误或已过期")
	}
	res, err := a.db.ExecContext(ctx,
		`UPDATE email_codes SET used_at=? WHERE id=? AND used_at IS NULL`, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("验证码已被使用")
	}
	return nil
}

// emailBind 绑定邮箱：POST /api/v1/auth/email-bind（登录；email+code）。
func (a *API) emailBind(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "LOGIN_REQUIRED", "请先登录")
		return
	}
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if !emailRe.MatchString(email) || len(email) > 200 {
		writeErr(w, http.StatusBadRequest, "BAD_EMAIL", "邮箱格式不正确")
		return
	}
	if len(req.Code) != 6 {
		writeErr(w, http.StatusBadRequest, "BAD_CODE", "验证码为 6 位数字")
		return
	}
	if err := a.emailCodeValid(r.Context(), email, req.Code, "bind"); err != nil {
		writeErr(w, http.StatusBadRequest, "CODE_INVALID", err.Error())
		return
	}
	var exists string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT id FROM users WHERE email=? AND id<>?`, email, uid).Scan(&exists); err == nil {
		writeErr(w, http.StatusConflict, "EMAIL_TAKEN", "该邮箱已被其他账号使用")
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE users SET email=?, updated_at=? WHERE id=?`, email, time.Now().Unix(), uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "BIND_FAILED", "绑定失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "email": email})
}

// profileUpdate 更新个人资料：PUT /api/v1/auth/profile（登录）。
func (a *API) profileUpdate(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "LOGIN_REQUIRED", "请先登录")
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
		Bio         string `json:"bio"`
		Avatar      string `json:"avatar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Bio = strings.TrimSpace(req.Bio)
	req.Avatar = strings.TrimSpace(req.Avatar)
	if len([]rune(req.DisplayName)) > 60 {
		writeErr(w, http.StatusBadRequest, "BAD_NAME", "昵称最长 60 字")
		return
	}
	if len([]rune(req.Bio)) > 200 {
		writeErr(w, http.StatusBadRequest, "BAD_BIO", "签名最长 200 字")
		return
	}
	if len([]rune(req.Avatar)) > 500 {
		writeErr(w, http.StatusBadRequest, "BAD_AVATAR", "头像地址过长")
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE users SET display_name=?, bio=?, avatar=?, updated_at=? WHERE id=?`,
		req.DisplayName, req.Bio, req.Avatar, time.Now().Unix(), uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "PROFILE_FAILED", "保存失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// authRegister 处理 POST /api/v1/auth/register（开放注册：role=member + 自动建会话）。
// 受 site.registration_open 门禁；site.email_required=true 时强制邮箱+6 位验证码。
func (a *API) authRegister(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.GetBool("site.registration_open") {
		writeErr(w, http.StatusForbidden, "REGISTER_CLOSED", "站点未开放注册，请联系管理员开通账号")
		return
	}
	ip := a.clientIP(r)
	if ok, _ := loginThrottler.allow("reg:" + ip); !ok {
		writeErr(w, http.StatusTooManyRequests, "LOGIN_RATE_LIMITED", "注册尝试过于频繁，请稍后再试")
		return
	}
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Code        string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Username) > 32 {
		writeErr(w, http.StatusBadRequest, "BAD_USERNAME", "用户名长度需在 3-32 位之间")
		return
	}
	for _, c := range req.Username {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c > 127) {
			writeErr(w, http.StatusBadRequest, "BAD_USERNAME", "用户名仅限字母、数字、下划线、短横线或中文")
			return
		}
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "WEAK_PASSWORD", "密码至少 8 位")
		return
	}
	var exists string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT id FROM users WHERE username=?`, req.Username).Scan(&exists); err == nil {
		writeErr(w, http.StatusConflict, "USER_EXISTS", "用户名已被占用")
		loginThrottler.hit("reg:" + ip) // 审计#2：撞用户名=重复尝试，计入注册限流
		return
	}
	// 邮箱：site.email_required 时必填且需验证码；否则可选（提供则校验验证码）。
	email := strings.TrimSpace(strings.ToLower(req.Email))
	emailRequired := a.cfg.GetBool("site.email_required")
	if emailRequired && email == "" {
		writeErr(w, http.StatusBadRequest, "EMAIL_REQUIRED", "本站注册需填写邮箱并通过验证码")
		loginThrottler.hit("reg:" + ip)
		return
	}
	if email != "" {
		if !emailRe.MatchString(email) || len(email) > 200 {
			writeErr(w, http.StatusBadRequest, "BAD_EMAIL", "邮箱格式不正确")
			return
		}
		if !a.smtpEnabled() && !a.smtpDebug() {
			writeErr(w, http.StatusServiceUnavailable, "EMAIL_NOT_CONFIGURED", "发信邮箱未配置，暂不支持邮箱注册")
			return
		}
		if emailRequired || req.Code != "" {
			if len(req.Code) != 6 {
				writeErr(w, http.StatusBadRequest, "BAD_CODE", "请先获取验证码")
		loginThrottler.hit("reg:" + ip)
				return
			}
			if err := a.emailCodeValid(r.Context(), email, req.Code, "register"); err != nil {
				writeErr(w, http.StatusBadRequest, "CODE_INVALID", err.Error())
		loginThrottler.hit("reg:" + ip)
				return
			}
		}
		var taken string
		if err := a.db.QueryRowContext(r.Context(),
			`SELECT id FROM users WHERE email=?`, email).Scan(&taken); err == nil {
			writeErr(w, http.StatusConflict, "EMAIL_TAKEN", "该邮箱已被其他账号使用")
		loginThrottler.hit("reg:" + ip)
			return
		}
	}
	uid := newID()
	now := time.Now().Unix()
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO users (id, username, pass_hash, display_name, email, role, status, preferences, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		uid, req.Username, service.HashPassword(req.Password), req.DisplayName, email, "member", "active", "{}", now, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "USER_CREATE_FAILED", "创建用户失败")
		return
	}
	// 签发会话（注册即登录）
	token, err := newSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SESSION_CREATE_FAILED", "创建会话失败")
		return
	}
	expires := now + int64(sessionTTL.Seconds())
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO sessions(token, user_id, created_at, expires_at, last_seen_at) VALUES(?,?,?,?,?)`,
		token, uid, now, expires, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "SESSION_CREATE_FAILED", "创建会话失败")
		return
	}
	loginThrottler.reset("reg:" + ip)
	_, _ = a.aud.Append(r.Context(), uid, "auth.register", "users", map[string]any{"ok": true})
	setSessionCookie(w, r, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": expires,
		"user": map[string]any{
			"id": uid, "username": req.Username, "role": "member",
			"display_name": req.DisplayName,
		},
	})
}

// adminUsersList 处理 GET /api/v1/admin/users（owner/admin 可见）。
func (a *API) adminUsersList(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, username, display_name, role, status, created_at FROM users ORDER BY created_at`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "USERS_READ_FAILED", err.Error())
		return
	}
	defer rows.Close()
	type u struct {
		ID          string `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		Status      string `json:"status"`
		CreatedAt   int64  `json:"created_at"`
	}
	list := []u{}
	for rows.Next() {
		var x u
		if err := rows.Scan(&x.ID, &x.Username, &x.DisplayName, &x.Role, &x.Status, &x.CreatedAt); err != nil {
			continue
		}
		list = append(list, x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": list})
}

// adminUsersUpdate 处理 PUT /api/v1/admin/users/{id}（改 role / status；owner 自身不可被降级/禁用）。
func (a *API) adminUsersUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	id := r.PathValue("id")
	var req struct {
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	if req.Role != "" && req.Role != "owner" && req.Role != "admin" && req.Role != "member" && req.Role != "viewer" {
		writeErr(w, http.StatusBadRequest, "BAD_ROLE", "非法角色")
		return
	}
	if req.Status != "" && req.Status != "active" && req.Status != "disabled" && req.Status != "invited" {
		writeErr(w, http.StatusBadRequest, "BAD_STATUS", "非法状态")
		return
	}
	var curRole string
	if err := a.db.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=?`, id).Scan(&curRole); err != nil {
		writeErr(w, http.StatusNotFound, "USER_NOT_FOUND", "用户不存在")
		return
	}
	// 保护：owner 账号不可被降级或禁用（防止把系统锁死）
	if curRole == "owner" {
		writeErr(w, http.StatusForbidden, "OWNER_PROTECTED", "owner 账号不可修改")
		return
	}
	set := []string{}
	args := []any{}
	if req.Role != "" {
		set = append(set, "role=?")
		args = append(args, req.Role)
	}
	if req.Status != "" {
		set = append(set, "status=?")
		args = append(args, req.Status)
	}
	if len(set) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	args = append(args, id)
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE users SET `+strings.Join(set, ",")+` WHERE id=?`, args...); err != nil {
		writeErr(w, http.StatusInternalServerError, "USER_UPDATE_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "admin.users_update", "users",
		map[string]any{"target": id, "role": req.Role, "status": req.Status})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
