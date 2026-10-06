// im_binding.go IM 绑定 Web 端点（B7）。
//
//	GET    /api/v1/im/bindings        当前用户的 IM 绑定列表（含平台可用状态）
//	POST   /api/v1/im/bindings/code   生成绑定码（IM 端发 `/bind <码>` 消费）
//	DELETE /api/v1/im/bindings/{id}   解绑（只能删自己的；按 id+user_id 双条件）
//
// 为什么是「网页端生成码 → IM 端消费」：IM 里不适合输入账号密码，
// 一次性短时效的码把 Web 的已登录身份安全地搬到 IM 会话。
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// imBindPlatforms 支持的 IM 平台（与适配器一致）。
var imBindPlatforms = []string{"telegram", "wecom"}

// imBindPlatformStatus 各平台是否已在站点侧配置可用。
func (a *API) imBindPlatformStatus() map[string]any {
	return map[string]any{
		"telegram": map[string]any{
			"enabled":     a.cfg.GetString("im.telegram.bot_token") != "",
			"display_name": "Telegram",
		},
		"wecom": map[string]any{
			"enabled":     a.wecomCfg().Enabled(),
			"display_name": "企业微信",
		},
	}
}

// imBindingsList GET /api/v1/im/bindings
func (a *API) imBindingsList(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if a.imBinds == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "platforms": a.imBindPlatformStatus(), "enabled": false})
		return
	}
	items, err := a.imBinds.ListByUser(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "IM_BIND_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"platforms": a.imBindPlatformStatus(),
		"enabled":   true,
	})
}

// imBindingsCreateCode POST /api/v1/im/bindings/code {platform?}
// platform 可空 = 该码可用于任意平台。
func (a *API) imBindingsCreateCode(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if a.imBinds == nil {
		writeErr(w, http.StatusServiceUnavailable, "IM_BIND_DISABLED", "IM 绑定功能未启用")
		return
	}
	var req struct {
		Platform string `json:"platform"`
	}
	// body 允许为空
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform != "" && !imBindPlatformSupported(platform) {
		writeErr(w, http.StatusBadRequest, "IM_BAD_PLATFORM", "平台仅支持 telegram / wecom")
		return
	}
	code, exp, err := a.imBinds.CreateCode(r.Context(), uid, platform, service.IMBindCodeTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "IM_BIND_CODE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":        code,
		"expires_at":  exp,
		"ttl_seconds": int(service.IMBindCodeTTL.Seconds()),
		"platform":    platform,
		"hint":        "在 IM 里发送：/bind " + code,
	})
}

// imBindingsDelete DELETE /api/v1/im/bindings/{id}
func (a *API) imBindingsDelete(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if a.imBinds == nil {
		writeErr(w, http.StatusServiceUnavailable, "IM_BIND_DISABLED", "IM 绑定功能未启用")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeErr(w, http.StatusBadRequest, "IM_BAD_ID", "缺少绑定 id")
		return
	}
	if err := a.imBinds.Unbind(r.Context(), uid, id); err != nil {
		if errors.Is(err, service.ErrBindNotFound) {
			writeErr(w, http.StatusNotFound, "IM_BIND_NOT_FOUND", "绑定不存在或不属于当前用户")
			return
		}
		writeErr(w, http.StatusInternalServerError, "IM_BIND_DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// imBindPlatformSupported 平台白名单校验。
func imBindPlatformSupported(p string) bool {
	for _, s := range imBindPlatforms {
		if s == p {
			return true
		}
	}
	return false
}
