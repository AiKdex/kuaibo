// mu.go —— 多用户基础（AiKlog 按 Option B 自带同构多用户；博客形态，无 spaces 概念）。
//
// 设计（相对 AiKmap 上游的适配）：
//   - AiKlog 是单博客系统，没有 spaces；用户归属直接用 users 表 + 角色 + 博客作者白名单。
//   - 登录会话层已多用户化（sessions.token → user_id，ctxUserID 注入，见 authmw.go）。
//   - 本文件补：
//       1) curUserID：把业务归属从"硬锚 owner"切换到"当前请求用户"；
//          无登录上下文（服务令牌/IM webhook/定时任务）回落系统 owner（homeOwnerID）。
//       2) isAdmin / blogAdminOnly / blogAuthorOnly / isBlogAuthor：角色 + 作者白名单守卫。
//       3) authConfig：公开能力探测（多用户/开放注册/邮箱验证开关），供前端决定注册入口。
//   - 多用户在 AiKlog 为常态（Option B），multiUser() 恒 true；若将来想退化单用户，
//     只需让本函数读 system.multi_user 开关即可（守卫逻辑不变）。

package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// multiUser AiKlog 采用 Option B 自带多用户，恒为 true。
// 历史约定：aikmap 底座做多用户、AiKlog 消费；Option B 下 AiKlog 自持同构 users/roles，
// 用户 id 与 aikmap 同款（32-hex UUIDv4）+ 同款 Argon2id，未来合并零成本。
func (a *API) multiUser() bool { return true }

// curUserID 返回当前请求用户；无登录上下文回落系统 owner（服务令牌/IM/定时任务场景）。
func (a *API) curUserID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxUserID).(string); ok && v != "" {
		return v
	}
	return a.homeOwnerID()
}

// isAdmin 判断当前用户是否 owner/admin。
func (a *API) isAdmin(r *http.Request) bool {
	uid := a.curUserID(r)
	var role string
	if err := a.db.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=?`, uid).Scan(&role); err != nil {
		return false
	}
	return role == "owner" || role == "admin"
}

// blogAdminOnly 博客管理守卫：仅 owner/admin 可管理站点级配置/插件（公开读不受限）。
func (a *API) blogAdminOnly(w http.ResponseWriter, r *http.Request) bool {
	if a.curUserID(r) != a.homeOwnerID() && !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "BLOG_ADMIN_REQUIRED", "仅站点管理员可管理博客")
		return false
	}
	return true
}

// isBlogAuthor 判断用户是否在作者白名单（settings blog.authors，逗号分隔 user id）。
// owner/admin 天然视为作者（blogAuthorOnly 已先放行，无需查白名单）。
func (a *API) isBlogAuthor(ctx context.Context, uid string) bool {
	v := a.cfg.GetString("blog.authors")
	if v == "" {
		return false
	}
	for _, part := range strings.Split(v, ",") {
		if strings.TrimSpace(part) == uid {
			return true
		}
	}
	return false
}

// blogAuthorOnly 博客发布守卫：owner/admin 或作者白名单内成员可发布/管理文章
// （插件与站点配置仍走 blogAdminOnly）。
func (a *API) blogAuthorOnly(w http.ResponseWriter, r *http.Request) bool {
	uid := a.curUserID(r)
	if uid == a.homeOwnerID() || a.isAdmin(r) {
		return true
	}
	if a.isBlogAuthor(r.Context(), uid) {
		return true
	}
	writeErr(w, http.StatusForbidden, "BLOG_AUTHOR_REQUIRED", "仅站点管理员或授权作者可发布文章")
	return false
}

// authConfig 处理 GET /api/v1/auth/config（公开）：前端据此决定注册入口/邮箱要求是否展示。
func (a *API) authConfig(w http.ResponseWriter, _ *http.Request) {
	emailEnabled := a.smtpEnabled() || a.smtpDebug()
	writeJSON(w, http.StatusOK, map[string]any{
		"multi_user":        a.multiUser(),
		"registration_open": a.cfg.GetBool("site.registration_open"),
		"email_required":    a.cfg.GetBool("site.email_required"),
		"email_enabled":     emailEnabled,
		"smtp_configured":   a.smtpEnabled(),
		// 兼容前端取 owner 展示名（公开页作者展示用）
		"owner_id": service.SystemOwnerID,
	})
}
