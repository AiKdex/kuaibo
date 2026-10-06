// public_keys.go 编译期内嵌验签公钥表（map[iss]raw32）。
//
// 公钥来源：上游（AiKmap 主系统）2026-09-19 回执交付，iss=aikmap-license。
// 换钥/加签发方：在此表追加条目即可（旧 key 继续验，新 key 按新 iss 走）。
package entitle

import (
	"crypto/ed25519"
	"encoding/base64"
)

// embeddedKeys 内嵌公钥表：iss → base64(raw 32B Ed25519 公钥)。
var embeddedKeys = map[string]string{
	"aikmap-license": "0aEys349vIDHxOc1D3naGulh3u9QFoWHHFOeyqCYO0Q=",
	// 站点数授权签发方（AiWebs 自签；私钥由运营侧保管，用于签发多站点 license）。
	"aiklog-multisite": "hMibZrtx6yrFO38fJ1a5qEJTPZ9KkSFKyjpwKieBNIk=",
	// 站点 license 自签发方（AiWebs 自签；iss=aiklog-license，私钥由运营侧本地保管）。
	// 用途：fork 独立签发正式 license，不依赖上游签发；上游 key（iss=aikmap-license）到货仍可双信任并存。
	"aiklog-license": "rUq7j0gQDRQiPqf+YQLQ7olyfqQDDJSwt+72vtvRjk4=",
}

// publicKeyFor 取签发方公钥。
func publicKeyFor(iss string) (ed25519.PublicKey, bool) {
	b64, ok := embeddedKeys[iss]
	if !ok {
		return nil, false
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, false
	}
	return ed25519.PublicKey(raw), true
}
