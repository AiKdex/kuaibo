// Package entitle 实例许可证（Entitlement）验签 —— Entitlement-License 协议 v0.1。
//
// 上游协议（AiKmap 应用中心，2026-09-19 上游回执交付）：
//   - key 形态：<base64url(payloadJSON)>.<base64url(signature)>
//   - payload = {iss, sub, scopes[], edition, iat, exp, jti, tid}
//   - 签名算法 Ed25519，签名对象为**解码后的 payload JSON 原文字节**
//     （已用上游测试 key 实测确认，非 JWS 的 base64 串签名）
//   - 公钥编译期内嵌（public_keys.go），本地离线验签
//   - scopes 模型："all"（全生态）/ "shell:<id>"（壳授权）/ "feature:<name>"（功能授权）；
//     含本壳（aiklog）即在本壳生效
//   - v0.1 边界：无实例强绑定（sub 弱绑定，留空跳过）、无吊销黑名单；
//     v0.2 引入绑定+黑名单后测试 key 将受限
package entitle

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 当前壳标识（AiKlog 壳；scope 检查用）。
const ShellID = "aiklog"

// 签发方标识（与内嵌公钥表对应）。
const Issuer = "aikmap-license"

// Payload license 载荷（协议 v0.1 字段集）。
type Payload struct {
	Iss     string   `json:"iss"`                // 签发方（aikmap-license）
	Sub     string   `json:"sub,omitempty"`      // 实例标识（v0.1 弱绑定，留空跳过）
	Scopes  []string `json:"scopes"`             // all | shell:<id> | feature:<name>
	Edition string   `json:"edition"`            // pro|free|community…
	Iat     int64    `json:"iat"`                // 签发时间（unix 秒）
	Exp     int64    `json:"exp"`                // 到期时间（unix 秒；0=永不过期）
	Jti     string   `json:"jti,omitempty"`      // key 唯一 id
	Tid     string   `json:"tid,omitempty"`      // 交易/订单 id（预留）
	Seats   int      `json:"seats,omitempty"`   // 站点数授权额度（多站点 license 用）
}

// 验签错误（分类判定用 errors.Is）。
var (
	ErrBadFormat    = errors.New("entitle: key 格式无效（应为 payload.signature 两段 base64url）")
	ErrBadPayload   = errors.New("entitle: payload 解析失败")
	ErrBadSignature = errors.New("entitle: 签名无效")
	ErrUnknownIss   = errors.New("entitle: 未知签发方（无对应内嵌公钥）")
	ErrExpired      = errors.New("entitle: license 已过期")
	ErrNotYetValid  = errors.New("entitle: license 尚未生效")
	ErrShellDenied  = errors.New("entitle: license 不含本壳授权（scopes 无 all/shell:aiklog）")
	// ErrFeatureDenied license 不含所需功能授权（scopes 无 all/feature:<name>）。
	ErrFeatureDenied = errors.New("entitle: license 不含所需功能授权")
)

// MultisiteFeature 站点数授权功能名（scope feature:multisite）。
const MultisiteFeature = "multisite"


// Validate 校验实例 license key：格式 → 验签 → iss → 时间窗 → 本壳 scope。
// 通过返回载荷；任何一步失败返回错误（错误可 errors.Is 分类）。
func Validate(key string) (*Payload, error) {
	p, err := bareValidate(key)
	if err != nil {
		return nil, err
	}
	if !p.HasShell(ShellID) {
		return nil, ErrShellDenied
	}
	return p, nil
}

// ValidateProduct 校验「功能/能力」类许可证（如站点数授权）：格式 → 验签 → iss → 时间窗，
// 但不要求本壳 scope（scope 含 all 或 feature:<name> 即可）。调用方用 HasFeature 判定具体功能。
func ValidateProduct(key string, feature string) (*Payload, error) {
	p, err := bareValidate(key)
	if err != nil {
		return nil, err
	}
	if !p.HasScope("all") && !p.HasFeature(feature) {
		return nil, ErrFeatureDenied
	}
	return p, nil
}

// bareValidate 完成格式/验签/iss/时间窗校验（不含 scope 判定），供 Validate/ValidateProduct 复用。
func bareValidate(key string) (*Payload, error) {
	key = strings.TrimSpace(key)
	dot := strings.LastIndexByte(key, '.')
	if dot <= 0 || dot == len(key)-1 {
		return nil, ErrBadFormat
	}
	p64, s64 := key[:dot], key[dot+1:]
	raw, err := base64.RawURLEncoding.DecodeString(p64)
	if err != nil {
		// 容错：带 padding 的 base64url
		if raw, err = base64.URLEncoding.DecodeString(p64); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadPayload, err)
		}
	}
	sig, err := base64.RawURLEncoding.DecodeString(s64)
	if err != nil {
		if sig, err = base64.URLEncoding.DecodeString(s64); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadSignature, err)
		}
	}
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadPayload, err)
	}
	pub, ok := publicKeyFor(p.Iss)
	if !ok {
		return nil, fmt.Errorf("%w: iss=%q", ErrUnknownIss, p.Iss)
	}
	if len(sig) != ed25519.SignatureSize || !ed25519.Verify(pub, raw, sig) {
		return nil, ErrBadSignature
	}
	now := time.Now().Unix()
	if p.Exp > 0 && now > p.Exp {
		return nil, ErrExpired
	}
	if p.Iat > 0 && now < p.Iat {
		return nil, ErrNotYetValid
	}
	return &p, nil
}

// IsPro edition=pro（付费态判定）。
func (p *Payload) IsPro() bool { return p.Edition == "pro" }

// HasScope 是否含指定 scope（精确匹配或 "all" 通配）。
func (p *Payload) HasScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == "all" || s == scope {
			return true
		}
	}
	return false
}

// HasShell 是否授权本壳（all / shell:<id>）。
func (p *Payload) HasShell(id string) bool {
	return p.HasScope("shell:" + id)
}

// HasFeature 是否授权指定功能（all / feature:<name>）。
func (p *Payload) HasFeature(name string) bool {
	return p.HasScope("feature:" + name)
}
