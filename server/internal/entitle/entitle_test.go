package entitle

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// upstreamTestKey 上游交付的测试 key（全壳 pro，30 天，jti=test-all-shells-001）。
const upstreamTestKey = "eyJlZGl0aW9uIjoicHJvIiwiZXhwIjoxNzkyMzE4MzcyLCJpYXQiOjE3ODk3MjYzNzIsImlzcyI6ImFpa21hcC1saWNlbnNlIiwianRpIjoidGVzdC1hbGwtc2hlbGxzLTAwMSIsInNjb3BlcyI6WyJhbGwiXX0.95GCzxcHouUVGojwUPltWnXmO-Tum_sYTtq92SXfd9kH0Lcp3Jx-gECm7PhI1MgBDRPHuQGvcI9y0FUSGE14DQ"

// testIssuer 本地测试签发方：同包测试注入临时公钥，用配对私钥自签各类负例。
var testIssuer = "test-license"
var testPub ed25519.PublicKey
var testPriv ed25519.PrivateKey

func init() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	testPub, testPriv = pub, priv
	embeddedKeys[testIssuer] = base64.StdEncoding.EncodeToString(pub)
}

func signKey(t *testing.T, iss string, p Payload) string {
	t.Helper()
	p.Iss = iss
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(testPriv, raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestValidateUpstreamTestKey(t *testing.T) {
	p, err := Validate(upstreamTestKey)
	if err != nil {
		t.Fatalf("上游测试 key 验签失败: %v", err)
	}
	if !p.IsPro() {
		t.Fatalf("edition 应为 pro，得到 %q", p.Edition)
	}
	if p.Iss != "aikmap-license" || p.Jti != "test-all-shells-001" {
		t.Fatalf("iss/jti 不符: %s/%s", p.Iss, p.Jti)
	}
	if !p.HasShell("aiklog") || !p.HasShell("aikmap") || !p.HasFeature("anything") {
		t.Fatalf("scopes=[all] 应通配全部壳/功能")
	}
}

func TestValidateTampered(t *testing.T) {
	i := strings.LastIndexByte(upstreamTestKey, '.')
	// 解码 payload → 篡改 jti → 重编码（保持 base64/JSON 合法，仅签名失配）
	raw := mustDecode(t, upstreamTestKey[:i])
	raw[len(raw)-6] = 'X' // 篡改 scopes 值内字符（"all"→"Xll"，JSON 仍合法，仅签名失配）
	tampered := base64.RawURLEncoding.EncodeToString(raw) + "." + upstreamTestKey[i+1:]
	if _, err := Validate(tampered); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("篡改 payload 应报签名无效，得到 %v", err)
	}
}

func TestValidateBadFormat(t *testing.T) {
	for _, k := range []string{"", "no-dot", ".leading", "trailing."} {
		if _, err := Validate(k); !errors.Is(err, ErrBadFormat) && !errors.Is(err, ErrBadPayload) {
			t.Fatalf("%q 应报格式错误，得到 %v", k, err)
		}
	}
}

func TestValidateUnknownIssuer(t *testing.T) {
	raw, _ := json.Marshal(Payload{Iss: "rogue", Scopes: []string{"all"}, Edition: "pro"})
	key := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	if _, err := Validate(key); !errors.Is(err, ErrUnknownIss) {
		t.Fatalf("未知签发方应拒绝，得到 %v", err)
	}
}

func TestValidateExpired(t *testing.T) {
	key := signKey(t, testIssuer, Payload{Scopes: []string{"all"}, Edition: "pro", Exp: time.Now().Add(-time.Hour).Unix()})
	if _, err := Validate(key); !errors.Is(err, ErrExpired) {
		t.Fatalf("过期 key 应拒绝，得到 %v", err)
	}
}

func TestValidateNotYetValid(t *testing.T) {
	key := signKey(t, testIssuer, Payload{Scopes: []string{"all"}, Edition: "pro", Iat: time.Now().Add(time.Hour).Unix(), Exp: time.Now().Add(2 * time.Hour).Unix()})
	if _, err := Validate(key); !errors.Is(err, ErrNotYetValid) {
		t.Fatalf("未生效 key 应拒绝，得到 %v", err)
	}
}

func TestValidateShellDenied(t *testing.T) {
	key := signKey(t, testIssuer, Payload{Scopes: []string{"shell:aikmap"}, Edition: "pro", Exp: time.Now().Add(time.Hour).Unix()})
	if _, err := Validate(key); !errors.Is(err, ErrShellDenied) {
		t.Fatalf("不含本壳 scope 应拒绝，得到 %v", err)
	}
}

func TestValidateShellScopeOK(t *testing.T) {
	key := signKey(t, testIssuer, Payload{Scopes: []string{"shell:aiklog"}, Edition: "pro", Exp: time.Now().Add(time.Hour).Unix()})
	p, err := Validate(key)
	if err != nil {
		t.Fatalf("shell:aiklog key 应通过: %v", err)
	}
	if !p.IsPro() || !p.HasShell("aiklog") || p.HasShell("aikmap") {
		t.Fatalf("壳 scope 判定不符")
	}
}

func TestValidatePaddingTolerant(t *testing.T) {
	i := strings.LastIndexByte(upstreamTestKey, '.')
	padded := base64.URLEncoding.EncodeToString(mustDecode(t, upstreamTestKey[:i])) + "." +
		base64.URLEncoding.EncodeToString(mustDecode(t, upstreamTestKey[i+1:]))
	if _, err := Validate(padded); err != nil {
		t.Fatalf("带 padding 的 key 应兼容: %v", err)
	}
}

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
