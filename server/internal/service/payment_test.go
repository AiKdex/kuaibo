package service

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestMockGateway(t *testing.T) {
	g := &mockGateway{}
	if !g.Configured() {
		t.Fatal("mock 应永远可用")
	}
	res, err := g.CreateCharge(context.Background(), ChargeInput{OrderNo: "T1", TotalCents: 1000, Currency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.PayURL, "T1") {
		t.Fatalf("mock 应返回含订单号的支付地址: %s", res.PayURL)
	}
}

func TestWeChatSignAndVerify(t *testing.T) {
	apiKey := "test_api_key_123"
	params := map[string]string{
		"appid": "wx123", "mch_id": "m1", "out_trade_no": "N1", "total_fee": "100",
		"return_code": "SUCCESS", "result_code": "SUCCESS",
	}
	sign := wxSign(params, apiKey)
	if sign == "" {
		t.Fatal("签名不应为空")
	}
	params["sign"] = sign
	xml := xmlMapToStr(params)
	g := &wechatGateway{r: func(string) string { return apiKey }}
	res, err := g.VerifyNotify(context.Background(), []byte(xml), nil)
	if err != nil {
		t.Fatalf("验签应成功: %v", err)
	}
	if res.OrderNo != "N1" || !res.Success {
		t.Fatalf("解析结果异常: %+v", res)
	}
}

func TestStripeVerify(t *testing.T) {
	wh := "whsec_test_123"
	body := `{"type":"checkout.session.completed","data":{"object":{"client_reference_id":"S1","payment_status":"paid"}}}`
	ts := "1700000000"
	mac := hmac.New(sha256.New, []byte(wh))
	mac.Write([]byte(ts + "."))
	mac.Write([]byte(body))
	sig := hex.EncodeToString(mac.Sum(nil))
	g := &stripeGateway{r: func(string) string { return wh }}
	// 复刻 header 解析
	hdr := "t=" + ts + ",v1=" + sig
	var pts, pv1 string
	for _, part := range strings.Split(hdr, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			if kv[0] == "t" {
				pts = kv[1]
			} else if kv[0] == "v1" {
				pv1 = kv[1]
			}
		}
	}
	t.Logf("parsed ts=%q v1=%q sig=%s", pts, pv1, sig)
	res, err := g.VerifyNotify(context.Background(), []byte(body), map[string]string{"Stripe-Signature": "t=" + ts + ",v1=" + sig})
	if err != nil {
		t.Fatalf("stripe 验签应成功: %v", err)
	}
	if res.OrderNo != "S1" || !res.Success {
		t.Fatalf("stripe 结果异常: %+v", res)
	}
}

func TestAlipayVerify(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pubBytes, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	g := &alipayGateway{r: func(k string) string {
		switch k {
		case "store.pay.alipay_appid":
			return "appid_test"
		case "store.pay.alipay_private_key":
			return string(privPEM)
		case "store.pay.alipay_public_key":
			return string(pubPEM)
		}
		return ""
	}}
	if !g.Configured() {
		t.Fatal("alipay 应已配置")
	}
	// 构造表单并签名
	form := url.Values{}
	form.Set("out_trade_no", "A1")
	form.Set("trade_status", "TRADE_SUCCESS")
	form.Set("app_id", "appid_test")
	signStr := "app_id=appid_test&out_trade_no=A1&trade_status=TRADE_SUCCESS"
	hashed := sha256.New()
	hashed.Write([]byte(signStr))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	form.Set("sign", base64.StdEncoding.EncodeToString(sig))
	form.Set("sign_type", "RSA2")
	ok, no := g.VerifyAlipayForm(form)
	if !ok || no != "A1" {
		t.Fatalf("alipay 验签应成功: ok=%v no=%s", ok, no)
	}
}

// B45：mock 网关的 Refunder 契约（原路退款 + 部分退款 + 金额边界）。
func TestMockGatewayRefund(t *testing.T) {
	m := &mockGateway{}
	if !m.RefundSupported() {
		t.Fatal("mock 应支持退款")
	}
	// 全额
	r1, err := m.Refund(context.Background(), RefundInput{OrderNo: "A1", AmountCents: 1000, TotalCents: 1000})
	if err != nil || r1.RefundNo == "" {
		t.Fatalf("全额退款应成功并返回单号: %v %+v", err, r1)
	}
	// 部分
	r2, err := m.Refund(context.Background(), RefundInput{OrderNo: "A1", AmountCents: 300, TotalCents: 1000})
	if err != nil || r2.RefundNo == "" {
		t.Fatalf("部分退款应成功: %v", err)
	}
	// 退款单号应互不相同（可对账）
	if r1.RefundNo == r2.RefundNo {
		t.Fatal("两笔退款单号不应相同")
	}
	// 边界：0 / 负数 / 超额
	for _, c := range []struct{ name string; amt int64 }{
		{"零金额", 0}, {"负金额", -5}, {"超额", 1001},
	} {
		if _, err := m.Refund(context.Background(), RefundInput{OrderNo: "A1", AmountCents: c.amt, TotalCents: 1000}); err == nil {
			t.Errorf("%s 应被拒绝", c.name)
		}
	}
}

// B45：未实现 Refunder 的网关必须返回 ErrRefundUnsupported ——
// service 层据此拒绝改本地状态，这是「不撒谎」的关键。
func TestGatewayRefundUnsupported(t *testing.T) {
	for _, name := range []string{"wechat", "alipay"} {
		_, err := GatewayRefund(context.Background(), name, RefundInput{OrderNo: "A1", AmountCents: 100, TotalCents: 100})
		if !errors.Is(err, ErrRefundUnsupported) {
			t.Errorf("%s 应返回 ErrRefundUnsupported，实际: %v", name, err)
		}
	}
	// 未注册网关同样
	if _, err := GatewayRefund(context.Background(), "nope", RefundInput{}); !errors.Is(err, ErrRefundUnsupported) {
		t.Errorf("未注册网关应返回 ErrRefundUnsupported，实际: %v", err)
	}
}
