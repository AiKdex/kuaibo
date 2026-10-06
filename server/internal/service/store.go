// store.go 商城服务层（B39）：商品 / 购物车 / 订单 / 优惠券 的持久化与业务规则。
//
// 设计要点：
//   - 金额一律「分」（int64），与 files.price_cents / store_orders.*_cents 同口径。
//   - 订单项快照商品标题/单价，避免改价后历史订单失真。
//   - 库存仅在订单「标记已付」时扣减（防超卖：pending 不锁库存），数字商品 stock=NULL 不限。
//   - 优惠券校验（门槛/有效期/次数/封顶）集中在 ValidateCoupon，下单与展示共用。
package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// ---- 数据结构 ----

type Product struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // digital | physical
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Body        string `json:"body"`
	PriceCents  int64  `json:"price_cents"`
	Currency    string `json:"currency"`
	Stock       *int64 `json:"stock"` // nil=不限
	SKU         string `json:"sku"`
	Cover       string `json:"cover"`
	Meta        string `json:"meta"` // JSON
	Status      string `json:"status"`
	Sort        int    `json:"sort"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type CartItem struct {
	ProductID string `json:"product_id"`
	Qty       int    `json:"qty"`
}

type Cart struct {
	Token     string     `json:"token"`
	UserID    string     `json:"user_id"`
	Items     []CartItem `json:"items"`
	CreatedAt int64      `json:"created_at"`
	UpdatedAt int64      `json:"updated_at"`
}

type OrderItem struct {
	ID             string `json:"id"`
	OrderID        string `json:"order_id"`
	ProductID      string `json:"product_id"`
	Title          string `json:"title"`
	Kind           string `json:"kind"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	Qty            int    `json:"qty"`
	SubtotalCents  int64  `json:"subtotal_cents"`
	Meta           string `json:"meta"`
	// B43 数字交付控制：token 是真正的高熵凭据（订单查询里的 contact_email 是下单自填、
	// 从未验证，不能当身份凭证）；count/max 为下载次数与上限（max=0 不限）。
	DownloadToken string `json:"download_token,omitempty"`
	DownloadCount int    `json:"download_count"`
	MaxDownloads  int    `json:"max_downloads"`
	CreatedAt     int64  `json:"created_at"`
}

type Order struct {
	ID            string      `json:"id"`
	OrderNo       string      `json:"order_no"`
	UserID        string      `json:"user_id"`
	SessionToken  string      `json:"session_token"`
	Status        string      `json:"status"` // pending|paid|fulfilled|refunded|cancelled|expired
	SubtotalCents int64       `json:"subtotal_cents"`
	DiscountCents int64       `json:"discount_cents"`
	TotalCents    int64       `json:"total_cents"`
	Currency      string      `json:"currency"`
	CouponCode    string      `json:"coupon_code"`
	Gateway       string      `json:"gateway"`
	GatewayData   string      `json:"gateway_data"`
	PaidAt        int64       `json:"paid_at"`
	ContactEmail  string      `json:"contact_email"`
	ContactName   string      `json:"contact_name"`
	Address       string      `json:"address"` // JSON
	Remark        string      `json:"remark"`
	ExpiresAt     int64       `json:"expires_at"`
	CreatedAt     int64       `json:"created_at"`
	UpdatedAt     int64       `json:"updated_at"`
	Items         []OrderItem `json:"items"`
}

type Coupon struct {
	ID               string  `json:"id"`
	Code             string  `json:"code"`
	Kind             string  `json:"kind"` // percent | fixed | free_shipping
	Value            float64 `json:"value"`
	MinSubtotalCents int64   `json:"min_subtotal_cents"`
	MaxDiscountCents int64   `json:"max_discount_cents"`
	StartsAt         int64   `json:"starts_at"`
	EndsAt           int64   `json:"ends_at"`
	UsageLimit       int     `json:"usage_limit"`
	UsedCount        int     `json:"used_count"`
	Status           string  `json:"status"`
	CreatedAt        int64   `json:"created_at"`
}

// ---- Store ----

type StoreStore struct {
	db  *sql.DB
	cfg ConfigReader
	// paidNotifier 支付成功钩子（B44 订单邮件通知），由装配层注入；nil=不发。
	//
	// 为什么挂在 service 而非各 handler 调用点：支付有 mock/微信/支付宝/Stripe 四条路径
	// （外加后台手动改状态），挂在 MarkPaid 内部才能**新增网关时自动覆盖**，不必每加一个补一次。
	paidNotifier func(ctx context.Context, o *Order) error
}

// SetPaidNotifier 注入支付成功钩子（幂等可覆盖）。通知失败**绝不影响支付结果**。
func (s *StoreStore) SetPaidNotifier(fn func(ctx context.Context, o *Order) error) {
	s.paidNotifier = fn
}

func NewStoreStore(db *sql.DB, cfg ConfigReader) *StoreStore {
	return &StoreStore{db: db, cfg: cfg}
}

func randID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- 商品 ----

func (s *StoreStore) CreateProduct(ctx context.Context, p *Product) error {
	now := time.Now().UnixMilli()
	if p.ID == "" {
		p.ID = "sp_" + randID(12)
	}
	if p.Slug == "" {
		p.Slug = p.ID
	}
	if p.Currency == "" {
		p.Currency = "CNY"
	}
	if p.Status == "" {
		p.Status = "draft"
	}
	p.CreatedAt, p.UpdatedAt = now, now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO store_products (id,kind,slug,title,summary,body,price_cents,currency,stock,sku,cover,meta,status,sort,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Kind, p.Slug, p.Title, p.Summary, p.Body, p.PriceCents, p.Currency, p.Stock, p.SKU, p.Cover, p.Meta, p.Status, p.Sort, p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *StoreStore) UpdateProduct(ctx context.Context, id string, patch map[string]any) (*Product, error) {
	cur, err := s.GetProduct(ctx, id)
	if err != nil {
		return nil, err
	}
	applyProductPatch(cur, patch)
	cur.UpdatedAt = time.Now().UnixMilli()
	_, err = s.db.ExecContext(ctx,
		`UPDATE store_products SET kind=?,slug=?,title=?,summary=?,body=?,price_cents=?,currency=?,stock=?,sku=?,cover=?,meta=?,status=?,sort=?,updated_at=? WHERE id=?`,
		cur.Kind, cur.Slug, cur.Title, cur.Summary, cur.Body, cur.PriceCents, cur.Currency, cur.Stock, cur.SKU, cur.Cover, cur.Meta, cur.Status, cur.Sort, cur.UpdatedAt, id)
	if err != nil {
		return nil, err
	}
	return cur, nil
}

func applyProductPatch(p *Product, patch map[string]any) {
	if v, ok := patch["kind"].(string); ok {
		p.Kind = v
	}
	if v, ok := patch["slug"].(string); ok {
		p.Slug = v
	}
	if v, ok := patch["title"].(string); ok {
		p.Title = v
	}
	if v, ok := patch["summary"].(string); ok {
		p.Summary = v
	}
	if v, ok := patch["body"].(string); ok {
		p.Body = v
	}
	if v, ok := patch["price_cents"].(float64); ok {
		p.PriceCents = int64(v)
	}
	if v, ok := patch["currency"].(string); ok {
		p.Currency = v
	}
	if v, ok := patch["stock"]; ok && v != nil {
		switch t := v.(type) {
		case float64:
			if t < 0 {
				p.Stock = nil
			} else {
				n := int64(t)
				p.Stock = &n
			}
		case nil:
			p.Stock = nil
		}
	}
	if v, ok := patch["sku"].(string); ok {
		p.SKU = v
	}
	if v, ok := patch["cover"].(string); ok {
		p.Cover = v
	}
	if v, ok := patch["meta"].(string); ok {
		p.Meta = v
	}
	if v, ok := patch["status"].(string); ok {
		p.Status = v
	}
	if v, ok := patch["sort"].(float64); ok {
		p.Sort = int(v)
	}
}

// CoverReferenced 判断 fileID 是否被某个「已发布」商品当作封面（cover）引用。
//
// 商品图床 /api/v1/store/media/{id} 据此放行：**只有上架商品的封面能公开取**，
// 任意站内 file id 都取不到 —— 它是商品图床，不是开放文件代理。
// 下架（draft）商品的封面即不可公开，避免「草稿期素材意外外泄」。
func (s *StoreStore) CoverReferenced(ctx context.Context, fileID string) (bool, error) {
	if strings.TrimSpace(fileID) == "" {
		return false, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM store_products WHERE cover=? AND status='published'`, fileID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ---- 退款（B45：原路退款 + 审计 + 部分退款） ----

// RefundStore 退款落库视图（handler/后台用）。
type RefundRecord struct {
	ID         string `json:"id"`
	OrderNo    string `json:"order_no"`
	Amount     int64  `json:"amount_cents"`
	Total      int64  `json:"total_cents"`
	Gateway    string `json:"gateway"`
	RefundNo   string `json:"refund_no"`
	Reason     string `json:"reason"`
	Operator   string `json:"operator"`
	CreatedAt  int64  `json:"created_at"`
}

// RefundedAmount 订单累计已退金额（分）。
func (s *StoreStore) RefundedAmount(ctx context.Context, orderNo string) (int64, error) {
	var sum sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT SUM(amount_cents) FROM store_refunds WHERE order_no=? AND status='succeeded'`, orderNo).Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum.Int64, nil
}

// ListRefunds 订单的退款流水（倒序）。
func (s *StoreStore) ListRefunds(ctx context.Context, orderNo string) ([]RefundRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,order_no,amount_cents,total_cents,gateway,refund_no,reason,operator,created_at
		 FROM store_refunds WHERE order_no=? ORDER BY created_at DESC`, orderNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RefundRecord, 0, 4)
	for rows.Next() {
		var r RefundRecord
		if err := rows.Scan(&r.ID, &r.OrderNo, &r.Amount, &r.Total, &r.Gateway, &r.RefundNo, &r.Reason, &r.Operator, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Refund 原路退款（B45）。
//
// 🔴 这是本次修复的核心：**网关没退成功，就绝不能把本地订单标成「已退款」**。
// 之前 Refund 只改本地状态，对微信/支付宝/Stripe 的真实订单等于「用户没收到钱、
// 后台却显示已退款」—— 运营会据此答复买家，纠纷直接落到商家身上。
// 现在流程是：校验 → **先调网关退款** → 网关成功才落库改状态；网关不支持/失败则
// 整体失败、状态不变，并返回可读原因（走 service.ErrRefundUnsupported）。
//
// amountCents<=0 表示全额退款；支持部分退款，但**累计退款不得超过实付金额**。
func (s *StoreStore) Refund(ctx context.Context, orderNo string, amountCents int64, reason, operator string) error {
	o, err := s.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return err
	}
	if o.Status != "paid" && o.Status != "fulfilled" {
		return errors.New("仅已支付/已交付订单可退款")
	}
	already, err := s.RefundedAmount(ctx, orderNo)
	if err != nil {
		return err
	}
	remaining := o.TotalCents - already
	if remaining <= 0 {
		return errors.New("该订单已全额退款")
	}
	if amountCents <= 0 {
		amountCents = remaining // 0/负数 = 退完剩余
	}
	if amountCents > remaining {
		return fmt.Errorf("退款金额 %s 超过可退余额 %s", formatYuan(amountCents), formatYuan(remaining))
	}

	// ① 先退网关。这是「不撒谎」的关键：网关失败/不支持 → 到此为止，本地状态不动。
	res, err := GatewayRefund(ctx, o.Gateway, RefundInput{
		OrderNo:     orderNo,
		AmountCents: amountCents,
		TotalCents:  o.TotalCents,
		Reason:      reason,
		GatewayData: o.GatewayData,
	})
	if err != nil {
		return err
	}

	// ② 网关退成功，才落库 + 改状态 + 回补库存。
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	// 退款审计（先写流水，再改状态 —— 便于事后对账）
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO store_refunds (id,order_no,amount_cents,total_cents,gateway,refund_no,reason,operator,status,created_at)
		 VALUES (?,?,?,?,?,?,?,?, 'succeeded', ?)`,
		"rf_"+randID(12), orderNo, amountCents, o.TotalCents, o.Gateway, res.RefundNo, reason, operator, now); err != nil {
		return err
	}
	// 全额退完才把订单置为 refunded；部分退款仍保持 paid/fulfilled（可继续退）
	newStatus := o.Status
	if already+amountCents >= o.TotalCents {
		newStatus = "refunded"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE store_orders SET status=?,updated_at=? WHERE id=?`, newStatus, now, o.ID); err != nil {
		return err
	}
	// 只有全额退款才回补库存（部分退款不返库存）
	if newStatus == "refunded" {
		for i := range o.Items {
			it := &o.Items[i]
			if it.Kind == "physical" {
				if _, err := tx.ExecContext(ctx, `UPDATE store_products SET stock=COALESCE(stock,0)+? WHERE id=?`, it.Qty, it.ProductID); err != nil {
					return err
				}
			}
		}
		if o.CouponCode != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE store_coupons SET used_count=MAX(0,used_count-1) WHERE code=?`, o.CouponCode); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ---- 退货申请（B46） ----

// ReturnRequest 退货申请视图。
type ReturnRequest struct {
	ID           string `json:"id"`
	OrderNo      string `json:"order_no"`
	ContactEmail string `json:"contact_email"`
	Reason       string `json:"reason"`
	Amount       int64  `json:"amount_cents"`
	Status       string `json:"status"` // pending|approved|rejected|refunded
	AdminNote    string `json:"admin_note"`
	Operator     string `json:"operator"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

const (
	ReturnPending  = "pending"
	ReturnApproved = "approved"
	ReturnRejected = "rejected"
	ReturnRefunded = "refunded"
)

// ErrReturnWindow 超出可申请期限。
var ErrReturnWindow = errors.New("已超出可申请退货的期限")

// CreateReturn 买家发起退货申请。
//
// 资格四查（避免被当刷单入口）：
//  1. 订单存在且属该邮箱（与 lookup 同一「订单号+邮箱」双因子口径）；
//  2. 订单须 paid/fulfilled（未付款/已退款的没有退货语义）；
//  3. 在可申请期限内（store.return_window_days，默认 7 天，从支付时间算）；
//  4. 同单至多一条待审申请（DB partial unique index 兜底并发）。
func (s *StoreStore) CreateReturn(ctx context.Context, orderNo, email, reason string, amountCents int64) (*ReturnRequest, error) {
	o, err := s.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(o.ContactEmail, strings.TrimSpace(email)) {
		return nil, errors.New("订单与邮箱不匹配")
	}
	if o.Status != "paid" && o.Status != "fulfilled" {
		return nil, errors.New("当前订单状态不可申请退货")
	}
	// 时效（ConfigReader 是 func(key) string，不是接口）
	days := 7
	if s.cfg != nil {
		if n, err := strconv.Atoi(strings.TrimSpace(s.cfg("store.return_window_days"))); err == nil && n >= 0 {
			days = n
		}
	}
	if days > 0 && o.PaidAt > 0 {
		deadline := o.PaidAt + int64(days)*86400_000
		if time.Now().UnixMilli() > deadline {
			return nil, ErrReturnWindow
		}
	}
	// 同单已有待审 → 直接返回那条，避免重复申请刷屏
	if r, err := s.pendingReturnOf(ctx, orderNo); err == nil && r != nil {
		return r, nil
	}
	now := time.Now().UnixMilli()
	r := &ReturnRequest{
		ID: "rt_" + randID(12), OrderNo: orderNo, ContactEmail: o.ContactEmail,
		Reason: strings.TrimSpace(reason), Amount: amountCents, Status: ReturnPending,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO store_returns (id,order_no,contact_email,reason,amount_cents,status,admin_note,operator,created_at,updated_at)
		 VALUES (?,?,?,?,?,? ,'','',?,?)`,
		r.ID, r.OrderNo, r.ContactEmail, r.Reason, r.Amount, r.Status, now, now); err != nil {
		// 并发下撞 partial unique index：退回已有那条
		if strings.Contains(err.Error(), "UNIQUE") {
			if r2, e2 := s.pendingReturnOf(ctx, orderNo); e2 == nil && r2 != nil {
				return r2, nil
			}
		}
		return nil, err
	}
	return r, nil
}

func (s *StoreStore) pendingReturnOf(ctx context.Context, orderNo string) (*ReturnRequest, error) {
	var r ReturnRequest
	err := s.db.QueryRowContext(ctx,
		`SELECT id,order_no,contact_email,reason,amount_cents,status,admin_note,operator,created_at,updated_at
		 FROM store_returns WHERE order_no=? AND status=? ORDER BY created_at DESC LIMIT 1`,
		orderNo, ReturnPending).
		Scan(&r.ID, &r.OrderNo, &r.ContactEmail, &r.Reason, &r.Amount, &r.Status, &r.AdminNote, &r.Operator, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListReturns 退货申请列表（status 空=全部，倒序）。
func (s *StoreStore) ListReturns(ctx context.Context, status string, limit int) ([]ReturnRequest, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := `SELECT id,order_no,contact_email,reason,amount_cents,status,admin_note,operator,created_at,updated_at FROM store_returns`
	var args []any
	if status != "" {
		q += ` WHERE status=?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ReturnRequest, 0, 8)
	for rows.Next() {
		var r ReturnRequest
		if err := rows.Scan(&r.ID, &r.OrderNo, &r.ContactEmail, &r.Reason, &r.Amount, &r.Status, &r.AdminNote, &r.Operator, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetReturn 按 id 取退货申请。
func (s *StoreStore) GetReturn(ctx context.Context, id string) (*ReturnRequest, error) {
	var r ReturnRequest
	err := s.db.QueryRowContext(ctx,
		`SELECT id,order_no,contact_email,reason,amount_cents,status,admin_note,operator,created_at,updated_at
		 FROM store_returns WHERE id=?`, id).
		Scan(&r.ID, &r.OrderNo, &r.ContactEmail, &r.Reason, &r.Amount, &r.Status, &r.AdminNote, &r.Operator, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// DecideReturn 审核退货申请：approve=true 走原路退款，false 直接驳回。
//
// 批准时**先退网关、退成功才把申请标 refunded**（与 B45 同一铁律）：
// 退款失败则申请保持 pending、订单状态不变，运营可稍后重试，
// 绝不会出现「申请显示已处理、用户没收到钱」。
func (s *StoreStore) DecideReturn(ctx context.Context, id, note, operator string, approve bool) (*ReturnRequest, error) {
	r, err := s.GetReturn(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.Status != ReturnPending {
		return nil, fmt.Errorf("该申请已处理（当前 %s）", r.Status)
	}
	now := time.Now().UnixMilli()
	if !approve {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE store_returns SET status=?,admin_note=?,operator=?,updated_at=? WHERE id=? AND status=?`,
			ReturnRejected, note, operator, now, id, ReturnPending); err != nil {
			return nil, err
		}
		return s.GetReturn(ctx, id)
	}
	// 批准：走原路退款（amount<=0 → 全额退完剩余可退余额，由 Refund 内部处理）
	if err := s.Refund(ctx, r.OrderNo, r.Amount, "退货申请通过："+firstLine(r.Reason), operator); err != nil {
		return nil, err // 保持 pending，运营可重试
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE store_returns SET status=?,admin_note=?,operator=?,updated_at=? WHERE id=?`,
		ReturnRefunded, note, operator, now, id); err != nil {
		return nil, err
	}
	return s.GetReturn(ctx, id)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	if len(s) > 60 {
		return s[:60]
	}
	return s
}

func (s *StoreStore) DeleteProduct(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM store_products WHERE id=?`, id)
	return err
}

func (s *StoreStore) GetProduct(ctx context.Context, id string) (*Product, error) {
	return s.scanProduct(s.db.QueryRowContext(ctx, `SELECT id,kind,slug,title,summary,body,price_cents,currency,stock,sku,cover,meta,status,sort,created_at,updated_at FROM store_products WHERE id=?`, id))
}

func (s *StoreStore) GetProductBySlug(ctx context.Context, slug string) (*Product, error) {
	return s.scanProduct(s.db.QueryRowContext(ctx, `SELECT id,kind,slug,title,summary,body,price_cents,currency,stock,sku,cover,meta,status,sort,created_at,updated_at FROM store_products WHERE slug=?`, slug))
}

func (s *StoreStore) scanProductRow(rows *sql.Rows) (*Product, error) {
	p := &Product{}
	var stock sql.NullInt64
	err := rows.Scan(&p.ID, &p.Kind, &p.Slug, &p.Title, &p.Summary, &p.Body, &p.PriceCents, &p.Currency, &stock, &p.SKU, &p.Cover, &p.Meta, &p.Status, &p.Sort, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if stock.Valid {
		v := stock.Int64
		p.Stock = &v
	}
	if p.Meta == "" {
		p.Meta = "{}"
	}
	return p, nil
}

func (s *StoreStore) scanProduct(row *sql.Row) (*Product, error) {
	p := &Product{}
	var stock sql.NullInt64
	err := row.Scan(&p.ID, &p.Kind, &p.Slug, &p.Title, &p.Summary, &p.Body, &p.PriceCents, &p.Currency, &stock, &p.SKU, &p.Cover, &p.Meta, &p.Status, &p.Sort, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if stock.Valid {
		v := stock.Int64
		p.Stock = &v
	}
	if p.Meta == "" {
		p.Meta = "{}"
	}
	return p, nil
}

func (s *StoreStore) ListProducts(ctx context.Context, publishedOnly bool) ([]*Product, error) {
	q := `SELECT id,kind,slug,title,summary,body,price_cents,currency,stock,sku,cover,meta,status,sort,created_at,updated_at FROM store_products`
	if publishedOnly {
		q += ` WHERE status='published'`
	}
	q += ` ORDER BY sort DESC, created_at DESC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Product, 0)
	for rows.Next() {
		p, err := s.scanProductRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// ---- 购物车 ----

func (s *StoreStore) GetCart(ctx context.Context, token string) (*Cart, error) {
	if token == "" {
		token = "cart_" + randID(16)
	}
	c := &Cart{Token: token}
	var items, userID string
	var created, updated int64
	err := s.db.QueryRowContext(ctx, `SELECT token,user_id,items,created_at,updated_at FROM store_carts WHERE token=?`, token).
		Scan(&c.Token, &userID, &items, &created, &updated)
	if err == sql.ErrNoRows {
		// 新购物车：空 items
		c.Items = []CartItem{}
		c.UserID = ""
		c.CreatedAt, c.UpdatedAt = time.Now().UnixMilli(), time.Now().UnixMilli()
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	c.UserID = userID
	c.CreatedAt, c.UpdatedAt = created, updated
	if err := json.Unmarshal([]byte(items), &c.Items); err != nil {
		c.Items = []CartItem{}
	}
	return c, nil
}

func (s *StoreStore) SaveCart(ctx context.Context, c *Cart) error {
	items, _ := json.Marshal(c.Items)
	now := time.Now().UnixMilli()
	if len(c.Items) == 0 {
		// 空车删除，避免脏数据堆积
		_, _ = s.db.ExecContext(ctx, `DELETE FROM store_carts WHERE token=?`, c.Token)
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO store_carts (token,user_id,items,created_at,updated_at) VALUES (?,?,?,?,?)
		 ON CONFLICT(token) DO UPDATE SET user_id=excluded.user_id, items=excluded.items, updated_at=excluded.updated_at`,
		c.Token, c.UserID, string(items), c.CreatedAt, now)
	c.UpdatedAt = now
	return err
}

// AddToCart 增加商品数量（qty<=0 视为移除）。
func (s *StoreStore) AddToCart(ctx context.Context, token, productID string, qty int) (*Cart, error) {
	c, err := s.GetCart(ctx, token)
	if err != nil {
		return nil, err
	}
	if qty <= 0 {
		c.Items = removeCartItem(c.Items, productID)
	} else {
		found := false
		for i := range c.Items {
			if c.Items[i].ProductID == productID {
				c.Items[i].Qty += qty
				found = true
				break
			}
		}
		if !found {
			c.Items = append(c.Items, CartItem{ProductID: productID, Qty: qty})
		}
	}
	if err := s.SaveCart(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// SetCartItem 设置某商品数量（qty<=0 移除）。
func (s *StoreStore) SetCartItem(ctx context.Context, token, productID string, qty int) (*Cart, error) {
	c, err := s.GetCart(ctx, token)
	if err != nil {
		return nil, err
	}
	if qty <= 0 {
		c.Items = removeCartItem(c.Items, productID)
	} else {
		found := false
		for i := range c.Items {
			if c.Items[i].ProductID == productID {
				c.Items[i].Qty = qty
				found = true
				break
			}
		}
		if !found {
			c.Items = append(c.Items, CartItem{ProductID: productID, Qty: qty})
		}
	}
	if err := s.SaveCart(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// RemoveCartItem 移除某商品。
func (s *StoreStore) RemoveCartItem(ctx context.Context, token, productID string) (*Cart, error) {
	c, err := s.GetCart(ctx, token)
	if err != nil {
		return nil, err
	}
	c.Items = removeCartItem(c.Items, productID)
	if err := s.SaveCart(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// BindCartUser 把匿名车绑定到用户（登录后合并）。
func (s *StoreStore) BindCartUser(ctx context.Context, token, userID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE store_carts SET user_id=? WHERE token=?`, userID, token)
	return err
}

func removeCartItem(items []CartItem, productID string) []CartItem {
	out := items[:0]
	for _, it := range items {
		if it.ProductID != productID {
			out = append(out, it)
		}
	}
	return out
}

// CartProducts 把购物车项展开为「商品 + 数量」（剔除已下架/缺货项并回写）。
func (s *StoreStore) CartProducts(ctx context.Context, c *Cart) ([]*Product, []CartItem, error) {
	kept := make([]CartItem, 0, len(c.Items))
	prods := make([]*Product, 0, len(c.Items))
	changed := false
	for _, it := range c.Items {
		p, err := s.GetProduct(ctx, it.ProductID)
		if err != nil || p.Status != "published" {
			changed = true
			continue
		}
		if p.Stock != nil && *p.Stock < int64(it.Qty) {
			if *p.Stock <= 0 {
				changed = true
				continue
			}
			it.Qty = int(*p.Stock)
			changed = true
		}
		kept = append(kept, it)
		prods = append(prods, p)
	}
	if changed {
		c.Items = kept
		_ = s.SaveCart(ctx, c)
	}
	return prods, kept, nil
}

// ---- 优惠券 ----

func (s *StoreStore) CreateCoupon(ctx context.Context, cp *Coupon) error {
	now := time.Now().UnixMilli()
	if cp.ID == "" {
		cp.ID = "cp_" + randID(12)
	}
	if cp.Code == "" {
		cp.Code = strings.ToUpper(randID(6))
	}
	if cp.Status == "" {
		cp.Status = "active"
	}
	cp.CreatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO store_coupons (id,code,kind,value,min_subtotal_cents,max_discount_cents,starts_at,ends_at,usage_limit,used_count,status,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		cp.ID, cp.Code, cp.Kind, cp.Value, cp.MinSubtotalCents, cp.MaxDiscountCents, cp.StartsAt, cp.EndsAt, cp.UsageLimit, cp.UsedCount, cp.Status, cp.CreatedAt)
	return err
}

func (s *StoreStore) UpdateCoupon(ctx context.Context, id string, patch map[string]any) (*Coupon, error) {
	cur, err := s.GetCoupon(ctx, id)
	if err != nil {
		return nil, err
	}
	applyCouponPatch(cur, patch)
	_, err = s.db.ExecContext(ctx,
		`UPDATE store_coupons SET code=?,kind=?,value=?,min_subtotal_cents=?,max_discount_cents=?,starts_at=?,ends_at=?,usage_limit=?,used_count=?,status=? WHERE id=?`,
		cur.Code, cur.Kind, cur.Value, cur.MinSubtotalCents, cur.MaxDiscountCents, cur.StartsAt, cur.EndsAt, cur.UsageLimit, cur.UsedCount, cur.Status, id)
	if err != nil {
		return nil, err
	}
	return cur, nil
}

func applyCouponPatch(cp *Coupon, patch map[string]any) {
	if v, ok := patch["code"].(string); ok {
		cp.Code = v
	}
	if v, ok := patch["kind"].(string); ok {
		cp.Kind = v
	}
	if v, ok := patch["value"].(float64); ok {
		cp.Value = v
	}
	if v, ok := patch["min_subtotal_cents"].(float64); ok {
		cp.MinSubtotalCents = int64(v)
	}
	if v, ok := patch["max_discount_cents"].(float64); ok {
		cp.MaxDiscountCents = int64(v)
	}
	if v, ok := patch["starts_at"].(float64); ok {
		cp.StartsAt = int64(v)
	}
	if v, ok := patch["ends_at"].(float64); ok {
		cp.EndsAt = int64(v)
	}
	if v, ok := patch["usage_limit"].(float64); ok {
		cp.UsageLimit = int(v)
	}
	if v, ok := patch["status"].(string); ok {
		cp.Status = v
	}
}

func (s *StoreStore) DeleteCoupon(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM store_coupons WHERE id=?`, id)
	return err
}

func (s *StoreStore) GetCoupon(ctx context.Context, id string) (*Coupon, error) {
	return s.scanCoupon(s.db.QueryRowContext(ctx, `SELECT id,code,kind,value,min_subtotal_cents,max_discount_cents,starts_at,ends_at,usage_limit,used_count,status,created_at FROM store_coupons WHERE id=?`, id))
}

func (s *StoreStore) GetCouponByCode(ctx context.Context, code string) (*Coupon, error) {
	return s.scanCoupon(s.db.QueryRowContext(ctx, `SELECT id,code,kind,value,min_subtotal_cents,max_discount_cents,starts_at,ends_at,usage_limit,used_count,status,created_at FROM store_coupons WHERE code=?`, code))
}

func (s *StoreStore) scanCoupon(row *sql.Row) (*Coupon, error) {
	cp := &Coupon{}
	err := row.Scan(&cp.ID, &cp.Code, &cp.Kind, &cp.Value, &cp.MinSubtotalCents, &cp.MaxDiscountCents, &cp.StartsAt, &cp.EndsAt, &cp.UsageLimit, &cp.UsedCount, &cp.Status, &cp.CreatedAt)
	if err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *StoreStore) ListCoupons(ctx context.Context) ([]*Coupon, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,code,kind,value,min_subtotal_cents,max_discount_cents,starts_at,ends_at,usage_limit,used_count,status,created_at FROM store_coupons ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Coupon, 0)
	for rows.Next() {
		cp, err := s.scanCouponRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cp)
	}
	return out, nil
}

func (s *StoreStore) scanCouponRow(rows *sql.Rows) (*Coupon, error) {
	cp := &Coupon{}
	err := rows.Scan(&cp.ID, &cp.Code, &cp.Kind, &cp.Value, &cp.MinSubtotalCents, &cp.MaxDiscountCents, &cp.StartsAt, &cp.EndsAt, &cp.UsageLimit, &cp.UsedCount, &cp.Status, &cp.CreatedAt)
	if err != nil {
		return nil, err
	}
	return cp, nil
}

// ValidateCoupon 校验优惠券并算出优惠额（分）。subtotalCents 为商品小计。
func (s *StoreStore) ValidateCoupon(ctx context.Context, code string, subtotalCents int64) (discountCents int64, coupon *Coupon, err error) {
	if code == "" {
		return 0, nil, nil
	}
	cp, err := s.GetCouponByCode(ctx, code)
	if err == sql.ErrNoRows {
		return 0, nil, errors.New("优惠券不存在")
	}
	if err != nil {
		return 0, nil, err
	}
	if cp.Status != "active" {
		return 0, nil, errors.New("优惠券已失效")
	}
	now := time.Now().UnixMilli()
	if cp.StartsAt > 0 && now < cp.StartsAt {
		return 0, nil, errors.New("优惠券未到生效时间")
	}
	if cp.EndsAt > 0 && now > cp.EndsAt {
		return 0, nil, errors.New("优惠券已过期")
	}
	if cp.UsageLimit > 0 && cp.UsedCount >= cp.UsageLimit {
		return 0, nil, errors.New("优惠券已领完")
	}
	if cp.MinSubtotalCents > 0 && subtotalCents < cp.MinSubtotalCents {
		return 0, nil, fmt.Errorf("未满 %s 元门槛", formatYuan(cp.MinSubtotalCents))
	}
	switch cp.Kind {
	case "percent":
		discountCents = int64(float64(subtotalCents) * cp.Value / 100.0)
	case "fixed":
		discountCents = int64(cp.Value * 100)
	case "free_shipping":
		discountCents = 0 // 本系统不单独计运费，免费运费等价于无额外优惠（占位）
	default:
		return 0, nil, errors.New("未知优惠券类型")
	}
	if cp.MaxDiscountCents > 0 && discountCents > cp.MaxDiscountCents {
		discountCents = cp.MaxDiscountCents
	}
	if discountCents > subtotalCents {
		discountCents = subtotalCents
	}
	return discountCents, cp, nil
}

func (s *StoreStore) IncrCouponUsage(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE store_coupons SET used_count=used_count+1 WHERE id=?`, id)
	return err
}

// ---- 订单 ----

func (s *StoreStore) CreateOrder(ctx context.Context, o *Order) error {
	if o.ID == "" {
		o.ID = "so_" + randID(12)
	}
	if o.OrderNo == "" {
		o.OrderNo = genOrderNo()
	}
	if o.Currency == "" {
		o.Currency = "CNY"
	}
	if o.Status == "" {
		o.Status = "pending"
	}
	now := time.Now().UnixMilli()
	o.CreatedAt, o.UpdatedAt = now, now
	if o.ExpiresAt == 0 {
		// 默认 30 分钟过期
		o.ExpiresAt = now + 30*60*1000
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO store_orders (id,order_no,user_id,session_token,status,subtotal_cents,discount_cents,total_cents,currency,coupon_code,gateway,gateway_data,paid_at,contact_email,contact_name,address,remark,expires_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		o.ID, o.OrderNo, o.UserID, o.SessionToken, o.Status, o.SubtotalCents, o.DiscountCents, o.TotalCents, o.Currency, o.CouponCode, o.Gateway, o.GatewayData, o.PaidAt, o.ContactEmail, o.ContactName, o.Address, o.Remark, o.ExpiresAt, o.CreatedAt, o.UpdatedAt)
	if err != nil {
		return err
	}
	for i := range o.Items {
		it := &o.Items[i]
		if it.ID == "" {
			it.ID = "soi_" + randID(10)
		}
		it.OrderID = o.ID
		it.CreatedAt = now
		_, err = tx.ExecContext(ctx,
			`INSERT INTO store_order_items (id,order_id,product_id,title,kind,unit_price_cents,qty,subtotal_cents,meta,download_token,download_count,max_downloads,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			it.ID, it.OrderID, it.ProductID, it.Title, it.Kind, it.UnitPriceCents, it.Qty, it.SubtotalCents, it.Meta, it.DownloadToken, it.DownloadCount, it.MaxDownloads, it.CreatedAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *StoreStore) GetOrderByNo(ctx context.Context, no string) (*Order, error) {
	o, err := s.scanOrder(s.db.QueryRowContext(ctx, orderCols+" WHERE order_no=?", no))
	if err != nil {
		return nil, err
	}
	if err := s.attachItems(ctx, o); err != nil {
		return nil, err
	}
	s.maybeExpire(ctx, o)
	return o, nil
}

func (s *StoreStore) GetOrder(ctx context.Context, id string) (*Order, error) {
	o, err := s.scanOrder(s.db.QueryRowContext(ctx, orderCols+" WHERE id=?", id))
	if err != nil {
		return nil, err
	}
	if err := s.attachItems(ctx, o); err != nil {
		return nil, err
	}
	s.maybeExpire(ctx, o)
	return o, nil
}

const orderCols = `SELECT id,order_no,user_id,session_token,status,subtotal_cents,discount_cents,total_cents,currency,coupon_code,gateway,gateway_data,paid_at,contact_email,contact_name,address,remark,expires_at,created_at,updated_at FROM store_orders`

func (s *StoreStore) scanOrder(row *sql.Row) (*Order, error) {
	o := &Order{}
	var addr string
	err := row.Scan(&o.ID, &o.OrderNo, &o.UserID, &o.SessionToken, &o.Status, &o.SubtotalCents, &o.DiscountCents, &o.TotalCents, &o.Currency, &o.CouponCode, &o.Gateway, &o.GatewayData, &o.PaidAt, &o.ContactEmail, &o.ContactName, &addr, &o.Remark, &o.ExpiresAt, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if addr == "" {
		o.Address = "{}"
	} else {
		o.Address = addr
	}
	if o.GatewayData == "" {
		o.GatewayData = "{}"
	}
	return o, nil
}

func (s *StoreStore) attachItems(ctx context.Context, o *Order) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id,order_id,product_id,title,kind,unit_price_cents,qty,subtotal_cents,meta,download_token,download_count,max_downloads,created_at FROM store_order_items WHERE order_id=? ORDER BY created_at`, o.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	o.Items = make([]OrderItem, 0)
	for rows.Next() {
		it := OrderItem{}
		var meta string
		if err := rows.Scan(&it.ID, &it.OrderID, &it.ProductID, &it.Title, &it.Kind, &it.UnitPriceCents, &it.Qty, &it.SubtotalCents, &meta, &it.DownloadToken, &it.DownloadCount, &it.MaxDownloads, &it.CreatedAt); err != nil {
			return err
		}
		if meta == "" {
			meta = "{}"
		}
		it.Meta = meta
		o.Items = append(o.Items, it)
	}
	return nil
}

// maybeExpire 懒标记过期（pending 且超过 expires_at）。
func (s *StoreStore) maybeExpire(ctx context.Context, o *Order) {
	if o.Status == "pending" && o.ExpiresAt > 0 && time.Now().UnixMilli() > o.ExpiresAt {
		o.Status = "expired"
		_, _ = s.db.ExecContext(ctx, `UPDATE store_orders SET status='expired',updated_at=? WHERE id=?`, time.Now().UnixMilli(), o.ID)
	}
}

type OrderFilter struct {
	Status string
	UserID string
	Token  string
	Limit  int
}

func (s *StoreStore) ListOrders(ctx context.Context, f OrderFilter) ([]*Order, error) {
	q := orderCols + " WHERE 1=1"
	args := []any{}
	if f.Status != "" {
		q += " AND status=?"
		args = append(args, f.Status)
	}
	if f.UserID != "" {
		q += " AND user_id=?"
		args = append(args, f.UserID)
	}
	if f.Token != "" {
		q += " AND session_token=?"
		args = append(args, f.Token)
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Order, 0)
	for rows.Next() {
		o, err := s.scanOrderRow(rows)
		if err != nil {
			return nil, err
		}
		s.maybeExpire(ctx, o)
		if err := s.attachItems(ctx, o); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func (s *StoreStore) scanOrderRow(rows *sql.Rows) (*Order, error) {
	o := &Order{}
	var addr string
	err := rows.Scan(&o.ID, &o.OrderNo, &o.UserID, &o.SessionToken, &o.Status, &o.SubtotalCents, &o.DiscountCents, &o.TotalCents, &o.Currency, &o.CouponCode, &o.Gateway, &o.GatewayData, &o.PaidAt, &o.ContactEmail, &o.ContactName, &addr, &o.Remark, &o.ExpiresAt, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if addr == "" {
		o.Address = "{}"
	} else {
		o.Address = addr
	}
	if o.GatewayData == "" {
		o.GatewayData = "{}"
	}
	return o, nil
}

// MarkPaid 幂等标记订单已付（回调/直 confirm 共用）：扣库存 + 数字商品发货 meta + 计数券。
func (s *StoreStore) MarkPaid(ctx context.Context, orderNo string) error {
	o, err := s.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return err
	}
	if o.Status == "paid" || o.Status == "fulfilled" {
		return nil // 幂等
	}
	if o.Status == "refunded" || o.Status == "cancelled" || o.Status == "expired" {
		return errors.New("订单状态不可支付")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	if _, err := tx.ExecContext(ctx, `UPDATE store_orders SET status='paid',paid_at=?,updated_at=? WHERE id=?`, now, now, o.ID); err != nil {
		return err
	}
	// 扣实物库存 + 数字商品填充下载 meta
	for i := range o.Items {
		it := &o.Items[i]
		if it.Kind == "physical" {
			if _, err := tx.ExecContext(ctx, `UPDATE store_products SET stock=MAX(0, COALESCE(stock,0)-?) WHERE id=? AND stock IS NOT NULL`, it.Qty, it.ProductID); err != nil {
				return err
			}
		} else {
			// 数字商品：把商品 meta 里的 download 链接写入订单项 meta，供用户中心下载
			var pm string
			_ = tx.QueryRowContext(ctx, `SELECT meta FROM store_products WHERE id=?`, it.ProductID).Scan(&pm)
			if pm != "" {
				_, _ = tx.ExecContext(ctx, `UPDATE store_order_items SET meta=? WHERE id=?`, pm, it.ID)
			}
			// B43：签发条目级下载令牌 + 次数上限。
			// 令牌是真正的高熵凭据 —— 订单里的 contact_email 是下单自填、从未验证过邮箱归属，
			// 不能当身份凭据用；下载次数上限则防止「一个链接被无限转发」。
			if _, err := tx.ExecContext(ctx,
				`UPDATE store_order_items SET download_token=?, max_downloads=? WHERE id=? AND download_token=''`,
				"dl_"+randID(24), maxDownloadsFromMeta(pm), it.ID); err != nil {
				return err
			}
		}
	}
	if o.CouponCode != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE store_coupons SET used_count=used_count+1 WHERE code=?`, o.CouponCode); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// B44：支付成功后发订单邮件（含数字商品下载令牌）。
	// 放在 Commit **之后**且**吞掉错误** —— 通知是旁路，绝不能让它把已付款的订单判成失败。
	if s.paidNotifier != nil {
		// 必须重读一遍：上面的事务刚给数字条目签发了 download_token，
		// 而 o 是进 MarkPaid 时读出来的**旧快照**（令牌为空）→ 邮件会退化成 email 链接。
		if fresh, ferr := s.GetOrderByNo(ctx, o.OrderNo); ferr == nil && fresh != nil {
			o = fresh
		}
		if err := s.paidNotifier(ctx, o); err != nil {
			// 仅记录，不回滚、不改订单状态
			log.Printf("store: 订单邮件通知失败 order=%s: %v", o.OrderNo, err)
		}
	}
	return nil
}

// Fulfill 标记已发货/已交付（实物发货、数字交付）。
func (s *StoreStore) Fulfill(ctx context.Context, orderNo string) error {
	o, err := s.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return err
	}
	if o.Status != "paid" {
		return errors.New("仅已支付订单可发货")
	}
	_, err = s.db.ExecContext(ctx, `UPDATE store_orders SET status='fulfilled',updated_at=? WHERE id=?`, time.Now().UnixMilli(), o.ID)
	return err
}

// SetOrderContact 落联系人与收货地址。
func (s *StoreStore) SetOrderContact(ctx context.Context, orderNo, email, name, address string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE store_orders SET contact_email=?,contact_name=?,address=?,updated_at=? WHERE order_no=?`, email, name, address, time.Now().UnixMilli(), orderNo)
	return err
}

// ---- 工具 ----

// DigitalFileID 数字商品的交付文件 id：读商品 meta JSON 的 file_id 字段。
//
// meta 约定（B39 数字交付）：{"file_id":"<files 表 id>"}
//   - 仅 kind=digital 使用；physical 走物流发货，无交付文件，返回 ""。
//   - meta 为空 / 非法 JSON / 无 file_id 一律返回 ""（调用方按「无可交付」处理，
//     不要因脏 meta panic 或 500）。
func DigitalFileID(meta string) string {
	meta = strings.TrimSpace(meta)
	if meta == "" || meta == "{}" {
		return ""
	}
	var m struct {
		FileID string `json:"file_id"`
	}
	if err := json.Unmarshal([]byte(meta), &m); err != nil {
		return ""
	}
	return strings.TrimSpace(m.FileID)
}

// maxDownloadsFromMeta 读商品 meta 里的 max_downloads（下载次数上限；0/缺省=不限）。
// meta 约定（B43）：{"file_id":"...","max_downloads":5}
func maxDownloadsFromMeta(meta string) int {
	meta = strings.TrimSpace(meta)
	if meta == "" || meta == "{}" {
		return 0
	}
	var m struct {
		MaxDownloads int `json:"max_downloads"`
	}
	if err := json.Unmarshal([]byte(meta), &m); err != nil {
		return 0
	}
	if m.MaxDownloads < 0 {
		return 0
	}
	return m.MaxDownloads
}

// DownloadGrant 数字商品下载授权结果。
type DownloadGrant struct {
	Item     *OrderItem
	Exhausted bool // 已达次数上限
}

// AuthorizeDownload 校验并**原子消耗**一次下载额度（B43）。
//
// 为什么要有这一步：原实现只校验「订单号 + 下单邮箱」，而邮箱是**下单时自填、从未验证**的，
// 不是身份凭据；且链接可被无限转发。这里把授权收敛到「条目级高熵令牌 + 次数上限」。
//
// 流程：
//  1. 定位条目（必须属本单且 kind=digital）；
//  2. 令牌非空 → 必须与请求 token 常量时间比对通过（或调用方已用登录身份/邮箱通过归属校验）；
//  3. 条件 UPDATE 扣减（download_count < max_downloads OR max_downloads=0）→ 影响 0 行即已用尽。
//
// token 传空且调用方已验归属时（老链接/登录用户）也允许，但仍受次数上限约束。
func (s *StoreStore) AuthorizeDownload(ctx context.Context, itemID, token string) (*DownloadGrant, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var stored, kind string
	var count, max int
	err = tx.QueryRowContext(ctx,
		`SELECT download_token, download_count, max_downloads, kind FROM store_order_items WHERE id=?`, itemID).
		Scan(&stored, &count, &max, &kind)
	if err != nil {
		return nil, err
	}
	if kind != "digital" {
		return nil, ErrNotDigital
	}
	if stored == "" {
		return nil, ErrNoToken
	}
	// 令牌比对：常量时间，避免计时侧信道
	if !hmac.Equal([]byte(stored), []byte(token)) {
		return nil, ErrBadToken
	}
	// 原子扣减：上限>0 时要求 count<max
	res, err := tx.ExecContext(ctx,
		`UPDATE store_order_items SET download_count=download_count+1
		 WHERE id=? AND (max_downloads=0 OR download_count < max_downloads)`, itemID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return &DownloadGrant{Exhausted: true}, nil
	}
	g := &DownloadGrant{Item: &OrderItem{}}
	if err := tx.QueryRowContext(ctx,
		`SELECT id,order_id,product_id,title,kind,unit_price_cents,qty,subtotal_cents,meta,download_token,download_count,max_downloads,created_at
		 FROM store_order_items WHERE id=?`, itemID).
		Scan(&g.Item.ID, &g.Item.OrderID, &g.Item.ProductID, &g.Item.Title, &g.Item.Kind,
			&g.Item.UnitPriceCents, &g.Item.Qty, &g.Item.SubtotalCents, &g.Item.Meta,
			&g.Item.DownloadToken, &g.Item.DownloadCount, &g.Item.MaxDownloads, &g.Item.CreatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return g, nil
}

// 下载授权错误（handler 映射为对应 HTTP 码与文案）。
var (
	ErrNoToken    = errors.New("该商品未签发下载令牌")
	ErrBadToken   = errors.New("下载令牌无效")
	ErrNotDigital = errors.New("非数字商品，无文件交付")
)

func genOrderNo() string {
	now := time.Now()
	h := sha256.Sum256([]byte(fmt.Sprintf("%d-%s", now.UnixNano(), randID(4))))
	return "A" + now.Format("20060102") + strings.ToUpper(hex.EncodeToString(h[:])[:8])
}

func formatYuan(cents int64) string {
	return strconv.FormatFloat(float64(cents)/100.0, 'f', 2, 64)
}
