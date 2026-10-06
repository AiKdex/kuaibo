package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/entitle"
)

// 测试用：生成一对 ed25519 密钥，并临时把它登记为可信签发方。
// 内嵌公钥表在 compile 期固定，故这里通过导出变量覆盖（仅测试使用）。
func withTrustedIssuer(t *testing.T, iss string) ed25519.PrivateKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// 直接写内嵌表（测试进程内），用 defer 还原不可行，故用唯一 iss 避免污染其他用例。
	entitle.RegisterTestIssuer(iss, pub)
	return priv
}

func sampleIdx() *marketIndex {
	return &marketIndex{
		SchemaVersion: "1",
		Plugins:       []marketPlugin{{ID: "p1", Name: "示例", Version: "1.0.0", SHA256: "abc"}},
	}
}

// 1) 正常流程：签名 → 验签通过。
func TestMarketIndexSignatureRoundTrip(t *testing.T) {
	iss := "test-issuer-roundtrip"
	priv := withTrustedIssuer(t, iss)

	idx := sampleIdx()
	payload, err := idx.canonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	idx.Signature = &marketSignature{Iss: iss, Sig: entitle.SignDetached(priv, payload)}
	if err := idx.verifySignature(); err != nil {
		t.Fatalf("合法索引应验签通过：%v", err)
	}
}

// 2) 篡改内容（改 sha256）→ 验签必须失败。
func TestMarketIndexSignatureRejectsTampered(t *testing.T) {
	iss := "test-issuer-tamper"
	priv := withTrustedIssuer(t, iss)

	idx := sampleIdx()
	payload, _ := idx.canonicalBytes()
	idx.Signature = &marketSignature{Iss: iss, Sig: entitle.SignDetached(priv, payload)}
	// 攻击者改条目内容
	idx.Plugins[0].SHA256 = "deadbeef"
	if err := idx.verifySignature(); err == nil {
		t.Fatal("内容被篡改后应验签失败")
	}
}

// 3) 未签名索引 → 拒绝（不允许「无签名即放行」）。
func TestMarketIndexSignatureRejectsUnsigned(t *testing.T) {
	idx := sampleIdx()
	if err := idx.verifySignature(); err == nil {
		t.Fatal("未签名索引应验签失败")
	}
}

// 4) 签发方不在可信表 → 拒绝。
func TestMarketIndexSignatureRejectsUntrustedIssuer(t *testing.T) {
	idx := sampleIdx()
	payload, _ := idx.canonicalBytes()
	// 用一个未登记的 iss
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	idx.Signature = &marketSignature{Iss: "not-trusted-xyz", Sig: entitle.SignDetached(priv, payload)}
	if err := idx.verifySignature(); err == nil {
		t.Fatal("未信任签发方应验签失败")
	}
}

// 5) 规范化字节稳定性：同样内容两次 canonicalBytes 必须一致（签发/验签才能对齐）。
func TestMarketIndexCanonicalStable(t *testing.T) {
	idx := sampleIdx()
	a, _ := idx.canonicalBytes()
	b, _ := idx.canonicalBytes()
	if string(a) != string(b) {
		t.Fatal("canonicalBytes 不稳定")
	}
	// 且不含 signature 字段
	if idx.Signature != nil {
		t.Fatal("前置条件：Signature 应为 nil")
	}
	var m map[string]any
	_ = json.Unmarshal(a, &m)
	if _, ok := m["signature"]; ok {
		t.Fatal("canonicalBytes 不应包含 signature 字段")
	}
	_ = base64.StdEncoding // 保持 import
}

// B47 回归：/market/index.json 曾用 writeMarketIndexView 重建文档，把 signature 丢了。
// 而各壳 fetchMarketIndex 对远程索引强制验签 → 自部署实例从官方拉目录恒 502，
// 「应用中心链接官方平台」根本不成立，装机统计也永无信号。
//
// 本用例钉死：分发端点输出的字节必须与签发方签的那份**等价**（即验签可通过）。
func TestMarketIndexRawPreservesSignature(t *testing.T) {
	iss := "test-issuer-raw"
	priv := withTrustedIssuer(t, iss)

	idx := sampleIdx()
	payload, err := idx.canonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	idx.Signature = &marketSignature{Iss: iss, Sig: entitle.SignDetached(priv, payload)}
	if err := idx.verifySignature(); err != nil {
		t.Fatalf("前置：构造的索引应先验签通过：%v", err)
	}

	// 模拟端点输出：writeMarketIndexRaw 用 json.Encoder 写结构体
	rec := httptest.NewRecorder()
	writeMarketIndexRaw(rec, idx)
	out := rec.Body.Bytes()

	// 落回结构体再验签：这正是壳侧 fetchMarketIndex 做的事
	var got marketIndex
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("端点输出不是合法 JSON：%v", err)
	}
	if got.Signature == nil {
		t.Fatal("🔴 端点输出丢了 signature —— 自部署实例将永远无法验签（这正是 B47 修掉的缺陷）")
	}
	if err := got.verifySignature(); err != nil {
		t.Fatalf("🔴 端点原样输出的索引应能验签通过，实际失败：%v", err)
	}
}
