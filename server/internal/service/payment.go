// payment.go 支付网关适配层（商城 B39）。
//
// 设计原则（与内核「不直接对接支付商」一致）：
//   - 内核只定义 Gateway 接口 + 注册表 + 四个适配器（mock / wechat / alipay / stripe）。
//   - 适配器从 settings 读取密钥（store.pay.*），**缺密钥即 Configured()=false**，
//     收银台前端据此隐藏不可用通道，不会在创建订单时炸。
//   - 收款成功由网关异步回调 POST /api/v1/store/notify/{gateway} 幂等标记订单已付
//     （签名校验同 pay/notify 模型）；mock 通道为演示用，走专用直 confirm 端点。
//
// 金额单位：分（与 store_orders.total_cents 同口径）。
package service

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ConfigReader 读取 settings 的闭包（解耦 config 包，便于测试注入）。
type ConfigReader func(key string) string

// ChargeInput 创建收款单的输入。
type ChargeInput struct {
	OrderNo     string
	TotalCents  int64
	Currency    string
	Title       string
	Description string
	ClientIP    string
	ReturnURL   string
	NotifyURL   string
}

// ChargeResult 创建收款单的返回（前端据此渲染二维码 / 跳转）。
type ChargeResult struct {
	PayURL  string         // 跳转/收银台地址（alipay / stripe / mock）
	CodeURL string         // 二维码内容（wechat native）
	Raw     map[string]any // 透传给前端备用
}

// NotifyResult 网关回调的解析结果。
type NotifyResult struct {
	OrderNo string
	Success bool
	PaidAt  int64
}

// PaymentGateway 支付网关适配接口。
type PaymentGateway interface {
	Name() string
	Configured() bool
	CreateCharge(ctx context.Context, in ChargeInput) (*ChargeResult, error)
	VerifyNotify(ctx context.Context, body []byte, headers map[string]string) (*NotifyResult, error)
}

// ---- 原路退款（可选能力） ----

// ErrRefundUnsupported 网关未实现原路退款。**这是必须让调用方看到的失败**：
// 订单在本地标成「已退款」而用户没收到钱，是最坏的一种状态 —— 运营会照着后台
// 告诉买家「已退」，纠纷就从平台转移到商家身上了。
var ErrRefundUnsupported = errors.New("该支付通道尚未实现原路退款")

// Refunder 可选接口：支持网关侧原路退款。
//
// 刻意**不做**成 PaymentGateway 的必选方法：真实网关的退款 API 形态差异很大
// （微信 v3 要双向证书、支付宝要 RSA 签名、Stripe 要先解析 payment_intent），
// 做成必选会逼所有网关填一个「假装成功」的实现 —— 那正是本次要消灭的毛病。
// 用可选接口：未实现就返回 ErrRefundUnsupported，service 层据此**拒绝**改本地状态。
type Refunder interface {
	// RefundSupported 报告该通道当前是否**真的能**发起退款（配置齐备）。
	RefundSupported() bool
	// Refund 发起原路退款。amount_cents < total_cents 即部分退款。
	Refund(ctx context.Context, in RefundInput) (*RefundResult, error)
}

type RefundInput struct {
	OrderNo     string
	AmountCents int64 // 本次退款金额（分）
	TotalCents  int64 // 原订单实付金额（分），部分退款必需
	Reason      string
	// GatewayData 下单时存下的网关响应（Stripe 靠它取 checkout session id）。
	GatewayData string
}

type RefundResult struct {
	RefundNo string         // 网关退款单号（对账用）
	Raw      map[string]any // 透传备用
}

// GatewayRefund 对网关发起原路退款；网关未实现/未就绪时返回明确错误。
func GatewayRefund(ctx context.Context, name string, in RefundInput) (*RefundResult, error) {
	g, ok := GetGateway(name)
	if !ok {
		return nil, ErrRefundUnsupported
	}
	rf, ok := g.(Refunder)
	if !ok {
		return nil, fmt.Errorf("%w：%s", ErrRefundUnsupported, name)
	}
	if !rf.RefundSupported() {
		// 措辞要诚实：这里**分不清**是「没实现」还是「实现了但配置不全」，
		// 所以别断言原因 —— 运营看到错误的归因会照着去排查错的方向。
		return nil, fmt.Errorf("%w：%s（未实现或配置不全）", ErrRefundUnsupported, name)
	}
	return rf.Refund(ctx, in)
}

// ---- 注册表 ----

var gatewayRegistry = map[string]PaymentGateway{}

// RegisterGateway 注册一个支付网关（同名覆盖）。
func RegisterGateway(g PaymentGateway) { gatewayRegistry[g.Name()] = g }

// GetGateway 按名称取网关。
func GetGateway(name string) (PaymentGateway, bool) {
	g, ok := gatewayRegistry[name]
	return g, ok
}

// ListConfiguredGateways 返回已配置（可用）的网关名（按固定顺序）。
func ListConfiguredGateways() []string {
	order := []string{"mock", "wechat", "alipay", "stripe"}
	out := make([]string, 0, len(order))
	for _, n := range order {
		if g, ok := gatewayRegistry[n]; ok && g.Configured() {
			out = append(out, n)
		}
	}
	return out
}

// RegisterStoreGateways 按 settings 装配四个网关（main 启动调用一次）。
func RegisterStoreGateways(r ConfigReader) {
	RegisterGateway(&mockGateway{})
	RegisterGateway(&wechatGateway{r: r})
	RegisterGateway(&alipayGateway{r: r})
	RegisterGateway(&stripeGateway{r: r})
}

// ---- 通用工具 ----

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- 极简 XML <-> map（仅用于微信支付原生报文，结构扁平、无嵌套） ----
// 注意：Go 的 regexp 为 RE2 引擎，不支持反向引用，故用非贪婪标签匹配 + 逐段解析。

func xmlMapToStr(params map[string]string) string {
	var sb strings.Builder
	sb.WriteString("<xml>")
	for k, v := range params {
		sb.WriteString("<")
		sb.WriteString(k)
		sb.WriteString("><![CDATA[")
		sb.WriteString(v)
		sb.WriteString("]]></")
		sb.WriteString(k)
		sb.WriteString(">")
	}
	sb.WriteString("</xml>")
	return sb.String()
}

// xmlStrToMap 解析扁平微信报文：逐个匹配 <tag>value</tag>。
// RE2 不支持反向引用，闭合标签用 [\\w]+ 而非 \\1 捕获。
func xmlStrToMap(s string) (map[string]string, error) {
	mp := map[string]string{}
	// 先剥掉 CDATA 包裹
	cdata := regexp.MustCompile(`<!\[CDATA\[(.*?)\]\]>`)
	s = cdata.ReplaceAllString(s, "$1")
	tagRe := regexp.MustCompile(`<([\w]+)>([^<]*)</[\w]+>`)
	for _, m := range tagRe.FindAllStringSubmatch(s, -1) {
		mp[m[1]] = m[2]
	}
	if len(mp) == 0 {
		return nil, errors.New("无法解析微信 XML 报文")
	}
	return mp, nil
}

// pemDecode 宽容解析 PEM（兼容含/不含 -----BEGIN----- 头）。
func pemDecode(pemStr string) (*pem.Block, error) {
	pemStr = strings.TrimSpace(pemStr)
	if !strings.HasPrefix(pemStr, "-----BEGIN") {
		// 可能是单行 base64 或去头去尾，尝试直接 base64 解码
		b, err := base64.StdEncoding.DecodeString(pemStr)
		if err != nil {
			return nil, err
		}
		return &pem.Block{Bytes: b}, nil
	}
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("PEM 解析失败")
	}
	return block, nil
}

// ---- Mock 网关（演示 / 离线可用，永远 Configured） ----

type mockGateway struct{}

func (m *mockGateway) Name() string     { return "mock" }
func (m *mockGateway) Configured() bool { return true }
func (m *mockGateway) CreateCharge(_ context.Context, in ChargeInput) (*ChargeResult, error) {
	return &ChargeResult{PayURL: "/store/pay/mock/" + in.OrderNo, Raw: map[string]any{"order_no": in.OrderNo}}, nil
}
func (m *mockGateway) VerifyNotify(_ context.Context, _ []byte, _ map[string]string) (*NotifyResult, error) {
	return nil, errors.New("mock 网关不使用异步回调，走专用 confirm 端点")
}

// mock 原路退款：演示通道没有真实资金流，「退款」即时成功。
// 之所以仍走完整 Refunder 契约，是为了让 mock 链路能端到端验证退款流程
// （含部分退款、次数/金额校验、审计落库），而真实通道接上即可复用同一条路径。
func (m *mockGateway) RefundSupported() bool { return true }

func (m *mockGateway) Refund(_ context.Context, in RefundInput) (*RefundResult, error) {
	if in.AmountCents <= 0 {
		return nil, errors.New("退款金额必须大于 0")
	}
	if in.AmountCents > in.TotalCents {
		return nil, errors.New("退款金额不能超过订单实付金额")
	}
	no := "mockrf_" + randID(10)
	return &RefundResult{RefundNo: no, Raw: map[string]any{
		"order_no": in.OrderNo, "amount_cents": in.AmountCents, "reason": in.Reason,
	}}, nil
}

// ---- 微信支付（Native 扫码，v2 统一下单 + 异步回调验签） ----

type wechatGateway struct{ r ConfigReader }

func (g *wechatGateway) Name() string { return "wechat" }
func (g *wechatGateway) Configured() bool {
	return strings.TrimSpace(g.r("store.pay.wechat_appid")) != "" &&
		strings.TrimSpace(g.r("store.pay.wechat_mch_id")) != "" &&
		strings.TrimSpace(g.r("store.pay.wechat_apikey")) != ""
}

// wxSign v2 签名：参数字典序拼接 + key，MD5 取大写（忽略空值）。
func wxSign(params map[string]string, apiKey string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if params[k] == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(params[k])
		sb.WriteString("&")
	}
	sb.WriteString("key=")
	sb.WriteString(apiKey)
	sum := md5.Sum([]byte(sb.String()))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func (g *wechatGateway) CreateCharge(ctx context.Context, in ChargeInput) (*ChargeResult, error) {
	if !g.Configured() {
		return nil, errors.New("微信支付未配置（缺 appid / mch_id / apikey）")
	}
	appid := g.r("store.pay.wechat_appid")
	mchID := g.r("store.pay.wechat_mch_id")
	apiKey := g.r("store.pay.wechat_apikey")
	params := map[string]string{
		"appid":          appid,
		"mch_id":         mchID,
		"nonce_str":      randHex(16),
		"body":           in.Title,
		"out_trade_no":   in.OrderNo,
		"total_fee":      fmt.Sprintf("%d", in.TotalCents),
		"spbill_create_ip": in.ClientIP,
		"notify_url":     in.NotifyURL,
		"trade_type":     "NATIVE",
	}
	params["sign"] = wxSign(params, apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mch.weixin.qq.com/pay/unifiedorder", strings.NewReader(xmlMapToStr(params)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	mp, err := xmlStrToMap(string(data))
	if err != nil {
		return nil, err
	}
	if mp["return_code"] != "SUCCESS" || mp["result_code"] != "SUCCESS" {
		return nil, fmt.Errorf("微信下单失败：%s %s", mp["return_msg"], mp["err_code_des"])
	}
	return &ChargeResult{CodeURL: mp["code_url"], Raw: map[string]any{"code_url": mp["code_url"]}}, nil
}

func (g *wechatGateway) VerifyNotify(_ context.Context, body []byte, _ map[string]string) (*NotifyResult, error) {
	mp, err := xmlStrToMap(string(body))
	if err != nil {
		return nil, err
	}
	apiKey := g.r("store.pay.wechat_apikey")
	sign := mp["sign"]
	delete(mp, "sign")
	if wxSign(mp, apiKey) != strings.ToUpper(sign) {
		return nil, errors.New("微信回调签名校验失败")
	}
	if mp["result_code"] != "SUCCESS" || mp["return_code"] != "SUCCESS" {
		return &NotifyResult{Success: false}, nil
	}
	return &NotifyResult{OrderNo: mp["out_trade_no"], Success: true, PaidAt: time.Now().UnixMilli()}, nil
}

// ---- 支付宝（电脑网站支付，RSA2 加签 + 异步回调验签） ----

// 微信原路退款：**刻意未实现**。
// 微信 v3 的「申请退款」需要商户 API 证书（apiclient_key.pem + 商户证书）+ 平台证书公钥
// 做双向 TLS，这套材料本仓没有、也无法凭空造。返回 ErrRefundUnsupported 让
// service 层拒绝把本地订单标成「已退款」，而不是对用户撒一个不成立的谎。
// 待接入时实现 Refunder 即可，上层（service/handler/前端）无需改动。
func (g *wechatGateway) RefundSupported() bool { return false }
func (g *wechatGateway) Refund(context.Context, RefundInput) (*RefundResult, error) {
	return nil, fmt.Errorf("%w：微信支付 v3 退款需配置商户 API 证书（apiclient_key.pem）", ErrRefundUnsupported)
}

type alipayGateway struct{ r ConfigReader }

// 支付宝原路退款：**刻意未实现**。`alipay.trade.refund` 需要能解析 out_trade_no → 交易号
// 的映射，且退款语义（部分退款/多次退款）要与回调对账严格对齐；本仓尚无该映射与对账链路。
// 同微信，返回明确失败而非假成功。
func (g *alipayGateway) RefundSupported() bool { return false }
func (g *alipayGateway) Refund(context.Context, RefundInput) (*RefundResult, error) {
	return nil, fmt.Errorf("%w：支付宝退款需先落地 trade_no 映射与对账链路", ErrRefundUnsupported)
}

func (g *alipayGateway) Name() string { return "alipay" }
func (g *alipayGateway) Configured() bool {
	return strings.TrimSpace(g.r("store.pay.alipay_appid")) != "" &&
		strings.TrimSpace(g.r("store.pay.alipay_private_key")) != ""
}

func aliSign(params map[string]string, privateKeyPEM string) (string, error) {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(params[k])
	}
	block, err := pemDecode(privateKeyPEM)
	if err != nil {
		return "", err
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		pk, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return "", err
		}
		rsaKey, ok := pk.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("支付宝私钥类型错误")
		}
		key = rsaKey
	}
	hashed := sha256.Sum256([]byte(sb.String()))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func (g *alipayGateway) CreateCharge(_ context.Context, in ChargeInput) (*ChargeResult, error) {
	if !g.Configured() {
		return nil, errors.New("支付宝未配置（缺 appid / 应用私钥）")
	}
	params := map[string]string{
		"app_id":      g.r("store.pay.alipay_appid"),
		"method":      "alipay.trade.page.pay",
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"notify_url":  in.NotifyURL,
		"return_url":  in.ReturnURL,
		"biz_content": fmt.Sprintf(`{"out_trade_no":"%s","product_code":"FAST_INSTANT_TRADE_PAY","total_amount":"%.2f","subject":"%s"}`, in.OrderNo, float64(in.TotalCents)/100.0, in.Title),
	}
	sign, err := aliSign(params, g.r("store.pay.alipay_private_key"))
	if err != nil {
		return nil, err
	}
	params["sign"] = sign
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	payURL := "https://openapi.alipay.com/gateway.do?" + q.Encode()
	return &ChargeResult{PayURL: payURL, Raw: map[string]any{"pay_url": payURL}}, nil
}

func (g *alipayGateway) VerifyNotify(_ context.Context, _ []byte, _ map[string]string) (*NotifyResult, error) {
	return nil, errors.New("支付宝验签由 handler 解析表单后调用 VerifyAlipayForm")
}

// VerifyAlipayForm 用支付宝公钥验签表单（handler 调用）。
func (g *alipayGateway) VerifyAlipayForm(form url.Values) (bool, string) {
	pubPEM := g.r("store.pay.alipay_public_key")
	if strings.TrimSpace(pubPEM) == "" {
		return false, ""
	}
	signB64 := form.Get("sign")
	form.Del("sign")
	form.Del("sign_type")
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(form.Get(k))
	}
	block, err := pemDecode(pubPEM)
	if err != nil {
		return false, ""
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		cert, err2 := x509.ParseCertificate(block.Bytes)
		if err2 != nil {
			return false, ""
		}
		pub = cert.PublicKey
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return false, ""
	}
	sig, err := base64.StdEncoding.DecodeString(signB64)
	if err != nil {
		return false, ""
	}
	hashed := sha256.Sum256([]byte(sb.String()))
	if rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hashed[:], sig) != nil {
		return false, ""
	}
	return form.Get("trade_status") == "TRADE_SUCCESS" || form.Get("trade_status") == "TRADE_FINISHED", form.Get("out_trade_no")
}

// ---- Stripe（Checkout Session，密钥签名 + webhook 验签） ----

// stripe 原路退款：两步 —— 先用下单时存下的 checkout session id 换 payment_intent，
// 再对 payment_intent 发起退款（Stripe 不支持直接按 session 退款）。
func (g *stripeGateway) RefundSupported() bool { return g.Configured() }

func (g *stripeGateway) Refund(ctx context.Context, in RefundInput) (*RefundResult, error) {
	if !g.Configured() {
		return nil, errors.New("Stripe 未配置（缺 secret / publishable key）")
	}
	if in.AmountCents <= 0 {
		return nil, errors.New("退款金额必须大于 0")
	}
	if in.AmountCents > in.TotalCents {
		return nil, errors.New("退款金额不能超过订单实付金额")
	}
	secret := g.r("store.pay.stripe_secret")
	sessID := stripeSessionID(in.GatewayData)
	if sessID == "" {
		return nil, errors.New("订单未记录 Stripe checkout session，无法定位原路退款目标（该单可能由旧版本创建）")
	}
	// 1) 取 payment_intent
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.stripe.com/v1/checkout/sessions/"+url.PathEscape(sessID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	sdata, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe 查询会话失败(%d)：%s", resp.StatusCode, string(sdata))
	}
	var sess struct {
		PaymentIntent string `json:"payment_intent"`
		Status        string `json:"status"`
	}
	if err := json.Unmarshal(sdata, &sess); err != nil {
		return nil, err
	}
	if sess.PaymentIntent == "" {
		return nil, fmt.Errorf("stripe 会话 %s 尚无 payment_intent（status=%s，可能未支付）", sessID, sess.Status)
	}
	// 2) 发起退款
	form := url.Values{}
	form.Set("payment_intent", sess.PaymentIntent)
	form.Set("amount", fmt.Sprintf("%d", in.AmountCents))
	if in.Reason != "" {
		form.Set("reason", "requested_by_customer")
		_ = in.Reason // 详细原因进本地审计表，不透传（Stripe 的 reason 是枚举）
	}
	rreq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.stripe.com/v1/refunds", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	rreq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rreq.Header.Set("Authorization", "Bearer "+secret)
	rresp, err := http.DefaultClient.Do(rreq)
	if err != nil {
		return nil, err
	}
	defer rresp.Body.Close()
	rdata, _ := io.ReadAll(rresp.Body)
	if rresp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe 退款失败(%d)：%s", rresp.StatusCode, string(rdata))
	}
	var rr struct {
		ID    string `json:"id"`
		Amount int64 `json:"amount"`
	}
	if err := json.Unmarshal(rdata, &rr); err != nil {
		return nil, err
	}
	return &RefundResult{RefundNo: rr.ID, Raw: map[string]any{
		"payment_intent": sess.PaymentIntent, "amount_cents": rr.Amount,
	}}, nil
}

// stripeSessionID 从下单时存下的 gateway_data（CreateCharge 的 Raw）里取 checkout session id。
func stripeSessionID(gatewayData string) string {
	if strings.TrimSpace(gatewayData) == "" {
		return ""
	}
	var mp map[string]any
	if err := json.Unmarshal([]byte(gatewayData), &mp); err != nil {
		return ""
	}
	if s, ok := mp["id"].(string); ok {
		return s
	}
	return ""
}

type stripeGateway struct{ r ConfigReader }

func (g *stripeGateway) Name() string { return "stripe" }
func (g *stripeGateway) Configured() bool {
	return strings.TrimSpace(g.r("store.pay.stripe_secret")) != "" &&
		strings.TrimSpace(g.r("store.pay.stripe_publishable")) != ""
}

func (g *stripeGateway) CreateCharge(ctx context.Context, in ChargeInput) (*ChargeResult, error) {
	if !g.Configured() {
		return nil, errors.New("Stripe 未配置（缺 secret / publishable key）")
	}
	secret := g.r("store.pay.stripe_secret")
	body := url.Values{}
	body.Set("mode", "payment")
	body.Set("success_url", in.ReturnURL+"?order_no="+in.OrderNo+"&status=success")
	body.Set("cancel_url", in.ReturnURL+"?order_no="+in.OrderNo+"&status=cancel")
	body.Set("client_reference_id", in.OrderNo)
	body.Set("line_items[0][price_data][currency]", strings.ToLower(in.Currency))
	body.Set("line_items[0][price_data][product_data][name]", in.Title)
	body.Set("line_items[0][price_data][unit_amount]", fmt.Sprintf("%d", in.TotalCents))
	body.Set("line_items[0][quantity]", "1")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var mp map[string]any
	if err := json.Unmarshal(data, &mp); err != nil {
		return nil, err
	}
	if urlv, ok := mp["url"].(string); ok {
		return &ChargeResult{PayURL: urlv, Raw: mp}, nil
	}
	return nil, fmt.Errorf("stripe 创建会话失败：%s", string(data))
}

func (g *stripeGateway) VerifyNotify(_ context.Context, body []byte, headers map[string]string) (*NotifyResult, error) {
	whSecret := g.r("store.pay.stripe_webhook_secret")
	if strings.TrimSpace(whSecret) == "" {
		return nil, errors.New("Stripe webhook secret 未配置")
	}
	sig := headers["Stripe-Signature"]
	if sig == "" {
		return nil, errors.New("缺少 Stripe-Signature 头")
	}
	var ts, v1 string
	for _, part := range strings.Split(sig, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			if kv[0] == "t" {
				ts = kv[1]
			} else if kv[0] == "v1" {
				v1 = kv[1]
			}
		}
	}
	mac := hmac.New(sha256.New, []byte(whSecret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(v1)) != 1 {
		return nil, errors.New("Stripe 签名校验失败")
	}
	var ev struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				ClientReferenceID string `json:"client_reference_id"`
				PaymentStatus     string `json:"payment_status"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, err
	}
	if ev.Type == "checkout.session.completed" && ev.Data.Object.PaymentStatus == "paid" {
		return &NotifyResult{OrderNo: ev.Data.Object.ClientReferenceID, Success: true, PaidAt: time.Now().UnixMilli()}, nil
	}
	return &NotifyResult{Success: false}, nil
}
