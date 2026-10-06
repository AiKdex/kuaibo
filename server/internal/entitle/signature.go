// signature.go 市场制品签名验签（市场 v0.3）。
//
// 信任链（自上而下任一环断裂即拒绝）：
//
//	可信公钥（编译期内嵌，iss → pub32）
//	  → 索引签名（ed25519 over 规范化 index.json 去掉 signature 字段）
//	    → 每个条目的 sha256
//	      → 下载到的 zip 字节
//
// 设计要点：
//   - 索引自证「谁发的」，条目 sha256 自证「包没被换」，两者都验才安装，杜绝
//     「篡改远程索引塞恶意包」与「中间人换包」两类攻击；
//   - 签名失败**不静默降级**：远程索引无签名/验签不过直接拒绝（回落本地目录索引），
//     绝不因为「拿不到签名」就按未签名处理——那等于没做签名；
//   - 私钥永不进仓，只有公钥表在 public_keys.go。
package entitle

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
)

// ErrSignature 表示签名缺失或验签不通过。
var ErrSignature = errors.New("签名缺失或验签不通过")

// SignDetached 对 payload 签名，返回 base64 签名（供签发方/测试使用；私钥在签发方侧）。
func SignDetached(priv ed25519.PrivateKey, payload []byte) string {
	if len(priv) != ed25519.PrivateKeySize {
		return ""
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, payload))
}

// VerifyDetached 用签发方公钥验签 payload 的 base64 签名。
// iss 必须在编译期内嵌公钥表中；签名长度/编码非法一律判失败。
func VerifyDetached(iss string, payload []byte, sigB64 string) error {
	pub, ok := publicKeyFor(iss)
	if !ok {
		return ErrSignature
	}
	if sigB64 == "" {
		return ErrSignature
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrSignature
	}
	if !ed25519.Verify(pub, payload, sig) {
		return ErrSignature
	}
	return nil
}

// TrustedIssuer 报告某签发方是否在内嵌可信公钥表中（供设置页展示「已信任的签发方」）。
func TrustedIssuer(iss string) bool {
	_, ok := publicKeyFor(iss)
	return ok
}

// RegisterTestIssuer 供测试动态登记签发方公钥（仅测试使用；生产走编译期内嵌表）。
func RegisterTestIssuer(iss string, pub ed25519.PublicKey) {
	embeddedKeys[iss] = base64.StdEncoding.EncodeToString(pub)
}

// TrustedIssuers 列出全部已内嵌信任的签发方。
func TrustedIssuers() []string {
	out := make([]string, 0, len(embeddedKeys))
	for k := range embeddedKeys {
		out = append(out, k)
	}
	return out
}
