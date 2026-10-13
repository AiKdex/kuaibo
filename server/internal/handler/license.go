// license.go 实例许可证（0.5.x 市场多壳模型配套；自上游 AiKmap 移植，2026-09-17）。
//
// 分工：内核只做"状态查询 + key 激活/吊销 + 付费安装门禁"，不做价格/计费逻辑（契约解耦）。
// edition=community|pro：license.key 非空且验签通过 → pro；否则 community。
// 2026-09-19：接入 Entitlement-License 协议 v0.1 —— Ed25519 本地离线验签
// （公钥编译期内嵌，见 internal/entitle），key 形态 payload.signature，
// scopes 含 all/shell:aiklog 即在本壳生效；到期（exp）自动回落 community。
package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/entitle"
)

// licenseKeyCfg 许可证 key 配置项名（settings 表持久化，secret 类型）。
const licenseKeyCfg = "license.key"

// licenseInfo 许可证状态（对外结构）。
type licenseInfo struct {
	Edition   string   `json:"edition"`              // community|pro
	Valid     bool     `json:"valid"`                // 是否处于有效付费状态
	KeySet    bool     `json:"key_set"`              // 是否已配置 key
	Source    string   `json:"source"`               // local（entitle 本地验签）
	ExpiresAt string   `json:"expires_at,omitempty"` // 到期时间 RFC3339（exp>0 时填充）
	Jti       string   `json:"jti,omitempty"`        // key 唯一 id
	Scopes    []string `json:"scopes,omitempty"`     // 授权范围
	Reason    string   `json:"reason,omitempty"`     // key 已设但未生效时的原因（过期/非本壳/验签失败）
}

// currentLicense 解析当前配置的 key；有效返回载荷，无效返回错误（供状态与门禁共用）。
func (a *API) currentLicense() (*entitle.Payload, error) {
	key := strings.TrimSpace(a.cfg.GetString(licenseKeyCfg))
	if key == "" {
		return nil, errLicenseNotSet
	}
	return entitle.Validate(key)
}

// licenseEdition 当前 edition：license.key 验签通过且含本壳 scope → pro；
// 否则尊重 system.edition 显式覆盖；都没有 → community。
// （apps_market_catalog 付费门禁：tier=paid 且非 pro → 402 MARKET_LICENSE_REQUIRED）
func (a *API) licenseEdition() string {
	if p, err := a.currentLicense(); err == nil && p.IsPro() {
		return "pro"
	}
	if s := strings.TrimSpace(a.cfg.GetString("system.edition")); s != "" {
		return s
	}
	return "community"
}

// licenseStatus GET /api/v1/license：当前许可证状态（登录）。
func (a *API) licenseStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.licenseInfo())
}

// licenseInfo 组装状态（供本文件与市场安装门禁共用）。
func (a *API) licenseInfo() licenseInfo {
	key := strings.TrimSpace(a.cfg.GetString(licenseKeyCfg))
	if key == "" {
		edition := "community"
		if s := strings.TrimSpace(a.cfg.GetString("system.edition")); s != "" {
			edition = s
		}
		return licenseInfo{Edition: edition, Valid: false, KeySet: false, Source: "local"}
	}
	p, err := a.currentLicense()
	if err != nil {
		return licenseInfo{Edition: "community", Valid: false, KeySet: true, Source: "local", Reason: err.Error()}
	}
	info := licenseInfo{
		Edition: "community",
		Valid:   false,
		KeySet:  true,
		Source:  "local",
		Reason:  "key 有效但 edition 非 pro",
		Scopes:  p.Scopes,
		Jti:     p.Jti,
	}
	if p.Exp > 0 {
		info.ExpiresAt = time.Unix(p.Exp, 0).UTC().Format(time.RFC3339)
	}
	if p.IsPro() {
		info.Edition = "pro"
		info.Valid = true
		info.Reason = ""
	}
	return info
}

// licenseActivate POST /api/v1/license {key}：激活许可证（登录；幂等）。
// 激活即验签：无效 key 直接 422，不落库。
func (a *API) licenseActivate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "LICENSE_BAD_REQ", "无效请求体")
		return
	}
	key := strings.TrimSpace(req.Key)
	p, err := entitle.Validate(key)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "LICENSE_KEY_INVALID", err.Error())
		return
	}
	if _, err := a.cfg.Set(r.Context(), licenseKeyCfg, key, "secret",
		"实例许可证 key（Entitlement v0.1；Ed25519 本地验签）", "license:activate"); err != nil {
		writeErr(w, http.StatusInternalServerError, "LICENSE_SAVE_FAILED", err.Error())
		return
	}
	info := licenseInfo{
		Edition: p.Edition,
		Valid:   p.IsPro(),
		KeySet:  true,
		Source:  "local",
		Scopes:  p.Scopes,
		Jti:     p.Jti,
	}
	if p.Exp > 0 {
		info.ExpiresAt = time.Unix(p.Exp, 0).UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, info)
}

// licenseDeactivate DELETE /api/v1/license：吊销/清除许可证（登录）。
func (a *API) licenseDeactivate(w http.ResponseWriter, r *http.Request) {
	if err := a.cfg.Delete(r.Context(), licenseKeyCfg); err != nil {
		writeErr(w, http.StatusInternalServerError, "LICENSE_CLEAR_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.licenseInfo())
}

// errLicenseNotSet 未配置 license key。
var errLicenseNotSet = &licenseKeyError{msg: "未配置 license key"}

type licenseKeyError struct{ msg string }

func (e *licenseKeyError) Error() string { return e.msg }
