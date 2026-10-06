// store.go 商城接口层（B39）：商品 / 购物车 / 收银台 / 订单 / 优惠券 / 支付网关回调。
//
// 路由分三档：
//  1. 公开（匿名可达）：商品目录/详情、购物车、收银台下单、订单查询、可用支付通道、mock 直 confirm。
//  2. 异步回调（匿名可达，HMAC/签名校验）：/store/notify/{gateway} —— 与 pay/notify 同模型。
//  3. 管理（isAdmin）：商品 CRUD、订单管理（发货/退款）、优惠券 CRUD、网关配置洞察。
//
// 购物车以匿名 cookie（store_cart）承载；登录态下单回填 user_id 并绑定购物车。
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// alipayFormVerifier 支付宝表单验签适配（*alipayGateway 已实现，跨包类型断言到本地接口）。
type alipayFormVerifier interface {
	VerifyAlipayForm(form url.Values) (bool, string)
}

// ---- 购物车 cookie 工具 ----

func (a *API) storeCartToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie("store_cart"); err == nil && c.Value != "" {
		return c.Value
	}
	tok := "cart_" + randomToken(16)
	http.SetCookie(w, &http.Cookie{
		Name: "store_cart", Value: tok, Path: "/", MaxAge: 60 * 60 * 24 * 365, HttpOnly: false, SameSite: http.SameSiteLaxMode,
	})
	return tok
}

// ---- 公开：商品封面图 ----

// storeMediaGet 商品封面图：GET /api/v1/store/media/{id}
//
// 与博客的 /api/v1/public/media/{id} 分开：那条限「博客子树内文件」，商品封面不在博客子树，
// 复用会 404（DDL 里 cover 本就允许存站内 file id）。本端点是**受限商品图床**：
//  1. id 必须是合法 file id（isSafeID）；
//  2. 必须被某个**已发布**商品当作 cover 引用（service.CoverReferenced）→ 任意 file id 取不到，
//     不是开放文件代理；草稿商品封面也不放行，避免素材意外外泄；
//  3. 只放行**位图**图片（image/* 但排除 svg）—— svg 可带脚本，内联渲染即 XSS 面。
func (a *API) storeMediaGet(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	id := r.PathValue("id")
	if id == "" || !isSafeID(id) {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "非法文件 ID")
		return
	}
	ok, err := a.store.CoverReferenced(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	if !ok {
		// 与公开媒体端点一致：不存在/未引用一律 404，不泄漏「文件存不存在」
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	rc, f, err := a.files.Content(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	defer rc.Close()
	if !isStoreImageMime(f.Mime) {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	w.Header().Set("Content-Type", f.Mime)
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	// 与 publicMediaGet 同款：本地盘返回 *os.File（ReadSeeker）→ 走 ServeContent 支持 Range/条件请求
	if rs, ok := rc.(io.ReadSeeker); ok {
		http.ServeContent(w, r, f.Name, time.UnixMilli(f.UpdatedAt), rs)
		return
	}
	_, _ = io.Copy(w, rc)
}

// isStoreImageMime 只放行位图：image/* 且排除 svg（svg 可内联脚本 = XSS 面）。
func isStoreImageMime(mime string) bool {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	if m == "image/svg+xml" {
		return false
	}
	return strings.HasPrefix(m, "image/")
}

// ---- 公开：商品目录 / 详情 ----

func (a *API) storeProducts(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var list []*service.Product
	var err error
	if q != "" {
		all, e := a.store.ListProducts(r.Context(), true)
		if e != nil {
			writeErr(w, http.StatusInternalServerError, "STORE_ERR", e.Error())
			return
		}
		q = strings.ToLower(q)
		for _, p := range all {
			if strings.Contains(strings.ToLower(p.Title), q) || strings.Contains(strings.ToLower(p.Summary), q) {
				list = append(list, p)
			}
		}
	} else {
		list, err = a.store.ListProducts(r.Context(), true)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (a *API) storeProductDetail(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	slug := r.PathValue("slug")
	p, err := a.store.GetProductBySlug(r.Context(), slug)
	if err != nil || p.Status != "published" {
		writeErr(w, http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在或已下架")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ---- 公开：购物车 ----

func (a *API) storeCartGet(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	tok := a.storeCartToken(w, r)
	cart, err := a.store.GetCart(r.Context(), tok)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	prods, items, err := a.store.CartProducts(r.Context(), cart)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.cartView(cart, prods, items))
}

func (a *API) cartView(cart *service.Cart, prods []*service.Product, items []service.CartItem) map[string]any {
	type line struct {
		Product *service.Product `json:"product"`
		Qty     int              `json:"qty"`
	}
	lines := make([]line, 0, len(items))
	var subtotal int64
	for i, it := range items {
		lines = append(lines, line{Product: prods[i], Qty: it.Qty})
		subtotal += prods[i].PriceCents * int64(it.Qty)
	}
	return map[string]any{"token": cart.Token, "lines": lines, "subtotal_cents": subtotal}
}

func (a *API) storeCartAdd(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	tok := a.storeCartToken(w, r)
	var req struct {
		ProductID string `json:"product_id"`
		Qty       int    `json:"qty"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || req.ProductID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "缺少 product_id")
		return
	}
	if req.Qty == 0 {
		req.Qty = 1
	}
	cart, err := a.store.AddToCart(r.Context(), tok, req.ProductID, req.Qty)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	prods, items, _ := a.store.CartProducts(r.Context(), cart)
	writeJSON(w, http.StatusOK, a.cartView(cart, prods, items))
}

func (a *API) storeCartSet(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	tok := a.storeCartToken(w, r)
	var req struct {
		ProductID string `json:"product_id"`
		Qty       int    `json:"qty"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || req.ProductID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "缺少 product_id")
		return
	}
	cart, err := a.store.SetCartItem(r.Context(), tok, req.ProductID, req.Qty)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	prods, items, _ := a.store.CartProducts(r.Context(), cart)
	writeJSON(w, http.StatusOK, a.cartView(cart, prods, items))
}

func (a *API) storeCartRemove(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	tok := a.storeCartToken(w, r)
	var req struct {
		ProductID string `json:"product_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || req.ProductID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "缺少 product_id")
		return
	}
	cart, err := a.store.RemoveCartItem(r.Context(), tok, req.ProductID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	prods, items, _ := a.store.CartProducts(r.Context(), cart)
	writeJSON(w, http.StatusOK, a.cartView(cart, prods, items))
}

func (a *API) storeCartClear(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	tok := a.storeCartToken(w, r)
	cart, err := a.store.GetCart(r.Context(), tok)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	cart.Items = []service.CartItem{}
	if err := a.store.SaveCart(r.Context(), cart); err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.cartView(cart, nil, nil))
}

// ---- 公开：收银台 ----

func (a *API) storeGateways(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	labels := map[string]string{"mock": "演示支付（无需真实账户）", "wechat": "微信支付", "alipay": "支付宝", "stripe": "Stripe 国际卡"}
	avail := make([]map[string]any, 0)
	for _, n := range []string{"mock", "wechat", "alipay", "stripe"} {
		g, ok := service.GetGateway(n)
		if !ok {
			continue
		}
		avail = append(avail, map[string]any{"name": n, "label": labels[n], "configured": g.Configured()})
	}
	currency := a.cfg.GetString("store.currency")
	if currency == "" {
		currency = "CNY"
	}
	writeJSON(w, http.StatusOK, map[string]any{"gateways": avail, "currency": currency})
}

func (a *API) storeCheckout(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	var req struct {
		Gateway      string `json:"gateway"`
		CouponCode   string `json:"coupon_code"`
		ContactEmail string `json:"contact_email"`
		ContactName  string `json:"contact_name"`
		Address      string `json:"address"`
		Remark       string `json:"remark"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	if req.Gateway == "" {
		req.Gateway = "mock"
	}
	gw, ok := service.GetGateway(req.Gateway)
	if !ok || !gw.Configured() {
		writeErr(w, http.StatusBadRequest, "GATEWAY_UNAVAILABLE", "所选支付通道不可用")
		return
	}
	tok := a.storeCartToken(w, r)
	cart, err := a.store.GetCart(r.Context(), tok)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	prods, items, err := a.store.CartProducts(r.Context(), cart)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	if len(items) == 0 {
		writeErr(w, http.StatusBadRequest, "CART_EMPTY", "购物车为空")
		return
	}
	// 小计
	var subtotal int64
	order := &service.Order{Gateway: req.Gateway, CouponCode: strings.TrimSpace(req.CouponCode)}
	for i, it := range items {
		p := prods[i]
		lineSub := p.PriceCents * int64(it.Qty)
		subtotal += lineSub
		order.Items = append(order.Items, service.OrderItem{
			ProductID: p.ID, Title: p.Title, Kind: p.Kind,
			UnitPriceCents: p.PriceCents, Qty: it.Qty, SubtotalCents: lineSub,
		})
	}
	order.SubtotalCents = subtotal
	// 优惠券
	discount, _, cerr := a.store.ValidateCoupon(r.Context(), order.CouponCode, subtotal)
	if cerr != nil && order.CouponCode != "" {
		writeErr(w, http.StatusBadRequest, "COUPON_INVALID", cerr.Error())
		return
	}
	order.DiscountCents = discount
	order.TotalCents = subtotal - discount
	if order.TotalCents < 0 {
		order.TotalCents = 0
	}
	// 联系信息
	order.ContactEmail = req.ContactEmail
	order.ContactName = req.ContactName
	if req.Address == "" {
		order.Address = "{}"
	} else {
		order.Address = req.Address
	}
	order.Remark = req.Remark
	order.SessionToken = tok
	// 登录态回填
	if uid, ok := ctxUID(r); ok {
		order.UserID = uid
		_ = a.store.BindCartUser(r.Context(), tok, uid)
	}
	if order.Currency == "" {
		order.Currency = a.cfg.GetString("store.currency")
	}
	if order.Currency == "" {
		order.Currency = "CNY"
	}
	// 落库
	if err := a.store.CreateOrder(r.Context(), order); err != nil {
		writeErr(w, http.StatusInternalServerError, "ORDER_FAIL", err.Error())
		return
	}
	// 调网关
	cr, cerr := gw.CreateCharge(r.Context(), service.ChargeInput{
		OrderNo: order.OrderNo, TotalCents: order.TotalCents, Currency: order.Currency,
		Title: "爱库录商城订单", Description: order.OrderNo,
		ClientIP: clientIP(r), ReturnURL: storeReturnURL(r), NotifyURL: storeNotifyURL(r, req.Gateway),
	})
	if cerr != nil {
		writeErr(w, http.StatusBadRequest, "CHARGE_FAIL", cerr.Error())
		return
	}
	gd, _ := json.Marshal(map[string]any{"pay_url": cr.PayURL, "code_url": cr.CodeURL, "raw": cr.Raw})
	_, _ = a.db.ExecContext(r.Context(), `UPDATE store_orders SET gateway_data=? WHERE id=?`, string(gd), order.ID)
	// 清空购物车
	cart.Items = []service.CartItem{}
	_ = a.store.SaveCart(r.Context(), cart)

	writeJSON(w, http.StatusOK, map[string]any{
		"order": order.OrderNo,
		"payment": map[string]any{
			"gateway": req.Gateway, "pay_url": cr.PayURL, "code_url": cr.CodeURL, "total_cents": order.TotalCents, "currency": order.Currency,
		},
	})
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.Split(x, ",")[0]
	}
	return r.RemoteAddr
}

func storeReturnURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	// 支付完成后回到订单查询页（SPA 路由 /#/store/orders，读取 query 里的 order_no + email）。
	return scheme + "://" + r.Host + "/store/orders"
}

func storeNotifyURL(r *http.Request, gw string) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host + "/api/v1/store/notify/" + gw
}

// ---- 公开：订单查询（按邮箱 + 订单号） ----

func (a *API) storeOrderLookup(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	no := strings.TrimSpace(r.URL.Query().Get("order_no"))
	if email == "" || no == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "需提供 email 与 order_no")
		return
	}
	o, err := a.store.GetOrderByNo(r.Context(), no)
	if err != nil {
		writeErr(w, http.StatusNotFound, "ORDER_NOT_FOUND", "订单不存在")
		return
	}
	if !strings.EqualFold(o.ContactEmail, email) {
		writeErr(w, http.StatusForbidden, "ORDER_FORBIDDEN", "订单与邮箱不匹配")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (a *API) storeOrderDetail(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	no := r.PathValue("no")
	// 🔴 订单详情含联系人/邮箱/地址等 PII：与 lookup 同口径，必须「订单号 + 下单邮箱」双因子匹配。
	// 否则任何人拿到（或猜到）订单号即可读取他人订单详情。
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if email == "" {
		writeErr(w, http.StatusForbidden, "ORDER_FORBIDDEN", "需提供下单邮箱以校验订单归属")
		return
	}
	o, err := a.store.GetOrderByNo(r.Context(), no)
	if err != nil {
		writeErr(w, http.StatusNotFound, "ORDER_NOT_FOUND", "订单不存在")
		return
	}
	if !strings.EqualFold(o.ContactEmail, email) {
		writeErr(w, http.StatusForbidden, "ORDER_FORBIDDEN", "订单与邮箱不匹配")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// storeOrderDownload 数字商品交付：GET /api/v1/store/order/{no}/download/{itemId}?email=
//
// 授权四道闸（任一不过即拒）：
//  1. 订单存在；
//  2. 归属：「订单号 + 下单邮箱」双因子匹配（同 lookup 口径），或已登录且 user_id == 订单归属人；
//  3. 状态：订单须 paid/fulfilled —— **未支付不交付**，否则拿到订单号即可白拿商品；
//  4. 条目：itemId 必须是本单条目，且 kind=digital（实体走物流，无文件）。
//
// 交付物取自商品 meta.file_id（管理员在商品里指定的文件），由 files 服务按 id 取，
// 不接受任意路径，杜绝穿越。
func (a *API) storeOrderDownload(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	no := r.PathValue("no")
	itemID := r.PathValue("itemId")
	o, err := a.store.GetOrderByNo(r.Context(), no)
	if err != nil {
		writeErr(w, http.StatusNotFound, "ORDER_NOT_FOUND", "订单不存在")
		return
	}
	// 闸 2：已支付才交付
	if o.Status != "paid" && o.Status != "fulfilled" {
		writeErr(w, http.StatusForbidden, "NOT_PAID", "订单尚未支付，暂不可下载")
		return
	}
	// 闸 3：条目归属本单 + 数字商品
	var item *service.OrderItem
	for i := range o.Items {
		if o.Items[i].ID == itemID {
			item = &o.Items[i]
			break
		}
	}
	if item == nil {
		writeErr(w, http.StatusNotFound, "ITEM_NOT_FOUND", "订单中没有该商品")
		return
	}
	if item.Kind != "digital" {
		writeErr(w, http.StatusBadRequest, "NOT_DIGITAL", "实体商品通过物流发货，不提供文件下载")
		return
	}
	// 闸 4：解析交付物 —— **必须在扣次数之前**。
	// 顺序很重要：若先扣次数再发现文件没配好，买家会为一个根本交付不了的商品白白烧掉下载额度。
	p, err := a.store.GetProduct(r.Context(), item.ProductID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在")
		return
	}
	fileID := service.DigitalFileID(p.Meta)
	if fileID == "" {
		writeErr(w, http.StatusNotFound, "NO_FILE", "该商品未配置交付文件，请联系商家")
		return
	}
	// 闸 5（B43）：授权 + 次数上限，原子扣减。
	//
	// 授权三选一（**令牌优先**）：
	//   a. 条目级 download_token 匹配 —— 高熵凭据，**自足**：邮件里的下载链接只带 token 即可，
	//      不依赖邮箱，也不要求用户登录。这正是令牌的用途。
	//   b. 登录用户本人（uid == order.user_id）
	//   c. 下单邮箱双因子（兼容旧链接；但邮箱是下单自填、从未验证，不能单独算强凭据）
	// 三者都不匹配 → 403。无论走哪条，**都受 max_downloads 约束**。
	token := r.URL.Query().Get("token")
	effective := token
	if effective == "" {
		// 无 token：回退到归属凭据（登录本人或邮箱匹配）
		owned := false
		if uid, ok := ctxUID(r); ok && o.UserID != "" && uid == o.UserID {
			owned = true
		}
		if !owned {
			email := strings.TrimSpace(r.URL.Query().Get("email"))
			if email == "" || !strings.EqualFold(o.ContactEmail, email) {
				writeErr(w, http.StatusForbidden, "ORDER_FORBIDDEN", "下载令牌无效或订单与邮箱不匹配")
				return
			}
		}
		effective = item.DownloadToken // 用条目自身令牌走同一原子扣减，次数上限照常生效
	}
	grant, err := a.store.AuthorizeDownload(r.Context(), itemID, effective)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBadToken):
			writeErr(w, http.StatusForbidden, "BAD_TOKEN", "下载令牌无效")
			return
		case errors.Is(err, service.ErrNoToken):
			writeErr(w, http.StatusNotFound, "NO_FILE", "该商品未签发下载令牌，请联系商家")
			return
		case errors.Is(err, service.ErrNotDigital):
			writeErr(w, http.StatusBadRequest, "NOT_DIGITAL", "非数字商品，无文件交付")
			return
		default:
			writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
			return
		}
	}
	if grant == nil || grant.Exhausted {
		writeErr(w, http.StatusTooManyRequests, "DOWNLOAD_LIMIT", "该商品下载次数已用尽，请联系商家")
		return
	}
	rc, f, err := a.files.Content(r.Context(), fileID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "交付文件不存在")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeAttachName(f.Name, item.Title)+"\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := io.Copy(w, rc); err != nil {
		return
	}
}

// sanitizeAttachName 生成安全的附件文件名：去掉路径分隔符与引号（防 header 注入），
// 为空时回退到商品名。
func sanitizeAttachName(name, fallback string) string {
	s := strings.NewReplacer("/", "_", "\\", "_", "\"", "_", "\r", "", "\n", "").Replace(name)
	s = strings.TrimSpace(s)
	if s == "" {
		s = fallback
	}
	if s == "" {
		s = "download"
	}
	return s
}

// ---- 公开：mock 直 confirm（演示支付） ----

func (a *API) storeMockPay(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	no := r.PathValue("no")
	// 🔴 演示直确认只允许确认「演示通道」下的订单：否则任何人拿订单号（收银台响应里就带着）
	// 调本端点，即可把微信/支付宝等真实通道的订单直接置为已支付 —— 等于免费下单。
	o, err := a.store.GetOrderByNo(r.Context(), no)
	if err != nil {
		writeErr(w, http.StatusNotFound, "ORDER_NOT_FOUND", "订单不存在")
		return
	}
	if o.Gateway != "mock" {
		writeErr(w, http.StatusBadRequest, "GATEWAY_MISMATCH", "该订单不是演示支付订单，请走原支付通道")
		return
	}
	if err := a.store.MarkPaid(r.Context(), no); err != nil {
		writeErr(w, http.StatusBadRequest, "PAY_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "order": no, "status": "paid"})
}

// ---- 公开：支付网关异步回调 ----

func (a *API) storeNotifyWeChat(w http.ResponseWriter, r *http.Request) {
	gw, ok := service.GetGateway("wechat")
	if !ok {
		writeErr(w, http.StatusNotFound, "GATEWAY", "网关未注册")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	res, err := gw.VerifyNotify(r.Context(), body, nil)
	if err != nil || res == nil || !res.Success {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte("<xml><return_code><![CDATA[FAIL]]></return_code></xml>"))
		return
	}
	_ = a.store.MarkPaid(r.Context(), res.OrderNo)
	w.Header().Set("Content-Type", "text/xml")
	w.Write([]byte("<xml><return_code><![CDATA[SUCCESS]]></return_code></xml>"))
}

func (a *API) storeNotifyAlipay(w http.ResponseWriter, r *http.Request) {
	gw, ok := service.GetGateway("alipay")
	if !ok {
		writeErr(w, http.StatusNotFound, "GATEWAY", "网关未注册")
		return
	}
	if err := r.ParseForm(); err != nil {
		w.Write([]byte("fail"))
		return
	}
	ag, ok := gw.(alipayFormVerifier)
	if !ok {
		w.Write([]byte("fail"))
		return
	}
	success, no := ag.VerifyAlipayForm(r.Form)
	if !success {
		w.Write([]byte("failure"))
		return
	}
	_ = a.store.MarkPaid(r.Context(), no)
	w.Write([]byte("success"))
}

func (a *API) storeNotifyStripe(w http.ResponseWriter, r *http.Request) {
	gw, ok := service.GetGateway("stripe")
	if !ok {
		writeErr(w, http.StatusNotFound, "GATEWAY", "网关未注册")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	res, err := gw.VerifyNotify(r.Context(), body, map[string]string{"Stripe-Signature": r.Header.Get("Stripe-Signature")})
	if err != nil || res == nil || !res.Success {
		writeErr(w, http.StatusBadRequest, "VERIFY_FAIL", "签名校验失败")
		return
	}
	_ = a.store.MarkPaid(r.Context(), res.OrderNo)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- 管理：商品 CRUD ----

func (a *API) adminStoreProducts(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	list, err := a.store.ListProducts(r.Context(), false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (a *API) adminStoreProductCreate(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	var p service.Product
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	if p.Title == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "商品标题必填")
		return
	}
	if err := a.store.CreateProduct(r.Context(), &p); err != nil {
		writeErr(w, http.StatusInternalServerError, "CREATE_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) adminStoreProductUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	id := r.PathValue("id")
	var patch map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&patch); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	p, err := a.store.UpdateProduct(r.Context(), id, patch)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "UPDATE_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) adminStoreProductDelete(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	if err := a.store.DeleteProduct(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, http.StatusInternalServerError, "DELETE_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- 管理：订单 ----

func (a *API) adminStoreOrders(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	status := r.URL.Query().Get("status")
	list, err := a.store.ListOrders(r.Context(), service.OrderFilter{Status: status, Limit: 200})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (a *API) adminStoreOrderDetail(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	no := r.PathValue("no")
	o, err := a.store.GetOrderByNo(r.Context(), no)
	if err != nil {
		writeErr(w, http.StatusNotFound, "ORDER_NOT_FOUND", "订单不存在")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (a *API) adminStoreOrderFulfill(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	if err := a.store.Fulfill(r.Context(), r.PathValue("no")); err != nil {
		writeErr(w, http.StatusBadRequest, "FULFILL_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// adminStoreOrderRefund 后台退款（isAdmin）。支持部分退款；网关退失败则本地状态不变。
func (a *API) adminStoreOrderRefund(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	no := r.PathValue("no")
	var req struct {
		AmountCents int64  `json:"amount_cents"` // <=0 表示全额（退完剩余可退余额）
		Reason      string `json:"reason"`
	}
	// 允许空 body（= 全额退款），故解码失败不直接报错
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
	}
	operator := ""
	if uid, ok := ctxUID(r); ok {
		operator = uid
	}
	err := a.store.Refund(r.Context(), no, req.AmountCents, strings.TrimSpace(req.Reason), operator)
	if err != nil {
		// 网关不支持/退款失败：明确回绝，**绝不改本地状态**
		if errors.Is(err, service.ErrRefundUnsupported) {
			writeErr(w, http.StatusBadRequest, "REFUND_UNSUPPORTED", err.Error())
			return
		}
		writeErr(w, http.StatusBadRequest, "REFUND_FAIL", err.Error())
		return
	}
	after, _ := a.store.GetOrderByNo(r.Context(), no)
	refunded, _ := a.store.RefundedAmount(r.Context(), no)
	status := ""
	if after != nil {
		status = after.Status
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "order": no, "status": status, "refunded_cents": refunded,
	})
}

// adminStoreOrderRefunds 退款流水（对账用）。
func (a *API) adminStoreOrderRefunds(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	list, err := a.store.ListRefunds(r.Context(), r.PathValue("no"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

// ---- 管理：优惠券 ----

func (a *API) adminStoreCoupons(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	list, err := a.store.ListCoupons(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (a *API) adminStoreCouponCreate(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	var cp service.Coupon
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&cp); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	if err := a.store.CreateCoupon(r.Context(), &cp); err != nil {
		writeErr(w, http.StatusInternalServerError, "CREATE_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cp)
}

func (a *API) adminStoreCouponUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	id := r.PathValue("id")
	var patch map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&patch); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	cp, err := a.store.UpdateCoupon(r.Context(), id, patch)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "UPDATE_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cp)
}

func (a *API) adminStoreCouponDelete(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	if err := a.store.DeleteCoupon(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, http.StatusInternalServerError, "DELETE_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- 管理：网关配置洞察（哪些已配置，供设置页） ----

func (a *API) adminStoreConfig(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	labels := map[string]string{"mock": "演示支付", "wechat": "微信支付", "alipay": "支付宝", "stripe": "Stripe"}
	out := make([]map[string]any, 0)
	for _, n := range []string{"mock", "wechat", "alipay", "stripe"} {
		g, ok := service.GetGateway(n)
		if !ok {
			continue
		}
		out = append(out, map[string]any{"name": n, "label": labels[n], "configured": g.Configured()})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"gateways": out,
		"currency": orDefault(a.cfg.GetString("store.currency"), "CNY"),
		"keys": map[string]bool{
			"wechat": a.cfg.GetString("store.pay.wechat_appid") != "",
			"alipay": a.cfg.GetString("store.pay.alipay_appid") != "",
			"stripe": a.cfg.GetString("store.pay.stripe_secret") != "",
		},
	})
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// ---- 后台：订单导出（CSV，便于对账） ----

func (a *API) adminStoreOrdersExport(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	list, err := a.store.ListOrders(r.Context(), service.OrderFilter{Limit: 1000})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=store_orders.csv")
	w.Write([]byte("\xEF\xBB\xBF")) // BOM
	w.Write([]byte("订单号,状态,商品小计,优惠,应付,币种,优惠券,网关,联系人,邮箱,创建时间,支付时间\n"))
	for _, o := range list {
		w.Write([]byte(strconv.Quote(o.OrderNo) + "," + o.Status + "," +
			strconv.FormatInt(o.SubtotalCents, 10) + "," + strconv.FormatInt(o.DiscountCents, 10) + "," +
			strconv.FormatInt(o.TotalCents, 10) + "," + o.Currency + "," + o.CouponCode + "," + o.Gateway + "," +
			strconv.Quote(o.ContactName) + "," + strconv.Quote(o.ContactEmail) + "," +
			time.UnixMilli(o.CreatedAt).Format("2006-01-02 15:04:05") + "," +
			time.UnixMilli(o.PaidAt).Format("2006-01-02 15:04:05") + "\n"))
	}
}
