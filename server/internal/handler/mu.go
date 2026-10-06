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
)

// multiUser AiKlog 采用 Option B 自带多用户，恒为 true。
// 历史约定：aikmap 底座做多用户、AiKlog 消费；Option B 下 AiKlog 自持同构 users/roles，
// 用户 id 与 aikmap 同款（32-hex UUIDv4）+ 同款 Argon2id，未来合并零成本。
func (a *API) multiUser() bool { return true }

// curUserID 返回当前请求用户；无登录上下文回落系统 owner（服务令牌/IM/定时任务场景）。
// ⚠️ 仅用于「业务归属」；**权限判定一律不得用它**（匿名请求会回落成 owner → 误判提权）。
// 权限判定请用 isAdmin / blogAdminOnly / blogAuthorOnly（它们只信任显式登录身份）。
func (a *API) curUserID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxUserID).(string); ok && v != "" {
		return v
	}
	return a.homeOwnerID()
}

// ctxUID 返回**显式登录身份**（authMiddleware 注入的 ctxUserID）；未登录/公开路径返回 ("", false)。
// 服务令牌/IM webhook/定时任务由 authMiddleware 注入 SystemOwnerID，故仍算显式身份。
func ctxUID(r *http.Request) (string, bool) {
	v, ok := r.Context().Value(ctxUserID).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// curHomeSpaceID 返回当前用户的 home 空间 id（多用户隔离：WebDAV 挂载导入等按用户 own 空间落地）。
// 无 home 空间（老库/异常）返回空串，调用方按空处理。
func (a *API) curHomeSpaceID(r *http.Request) string {
	uid := a.curUserID(r)
	var sid string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT id FROM spaces WHERE owner_id=? AND kind='home'`, uid).Scan(&sid); err != nil {
		return ""
	}
	return sid
}

// isAdmin 判断当前请求是否 owner/admin。
// 仅信任 authMiddleware 注入的登录身份（ctxUID）；未登录/公开路径一律 false，
// 杜绝"curUserID 回落 owner"导致的误判提权（M5：公开端点误调 isAdmin 不再放行）。
// 服务令牌/IM webhook/定时任务等内部场景由 authMiddleware 注入 SystemOwnerID，仍正常判定。
func (a *API) isAdmin(r *http.Request) bool {
	uid, ok := ctxUID(r)
	if !ok {
		return false
	}
	var role string
	if err := a.db.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=?`, uid).Scan(&role); err != nil {
		return false
	}
	return role == "owner" || role == "admin"
}

// blogAdminOnly 博客管理守卫：仅 owner/admin 可管理站点级配置/插件（公开读不受限）。
func (a *API) blogAdminOnly(w http.ResponseWriter, r *http.Request) bool {
	uid, ok := ctxUID(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "BLOG_ADMIN_REQUIRED", "仅站点管理员可管理博客")
		return false
	}
	if uid != a.homeOwnerID() && !a.isAdmin(r) {
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
// 同样只信任显式登录身份（M5）：匿名/公开路径不因 curUserID 回落而获准。
func (a *API) blogAuthorOnly(w http.ResponseWriter, r *http.Request) bool {
	uid, ok := ctxUID(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "BLOG_AUTHOR_REQUIRED", "仅站点管理员或授权作者可发布文章")
		return false
	}
	if uid == a.homeOwnerID() || a.isAdmin(r) {
		return true
	}
	if a.isBlogAuthor(r.Context(), uid) {
		return true
	}
	writeErr(w, http.StatusForbidden, "BLOG_AUTHOR_REQUIRED", "仅站点管理员或授权作者可发布文章")
	return false
}

// writeActor 文件写操作的归属主体（C3 纵深防御配套）。
// AiKlog 为单博客形态：博客内容统一落在站长 home 空间，作者/管理员写的是同一棵树。
// 因此这里把「已授权写博客的用户」归一为站长主体；未被授权的一般登录用户返回**自身 uid**，
// 由 service 层 requireSpaceWrite 拒绝——即便将来某个写端点漏加 handler 守卫，也不会越权。
func (a *API) writeActor(r *http.Request) string {
	uid, ok := ctxUID(r)
	if !ok {
		return a.homeOwnerID() // 内部调用（服务令牌/IM webhook/定时任务）按站长主体处理
	}
	if uid == a.homeOwnerID() || a.isAdmin(r) || a.isBlogAuthor(r.Context(), uid) {
		return a.homeOwnerID()
	}
	return uid
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
		// M13 修复：不再返回 owner_id（公开接口暴露站长 user id；前端无任何消费方）
	})
}
