package service

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(repo.DDL); err != nil {
		t.Fatal(err)
	}
	return db
}

func mustStore(t *testing.T) *StoreStore {
	return NewStoreStore(newTestDB(t), func(string) string { return "" })
}

func TestValidateCoupon(t *testing.T) {
	s := mustStore(t)
	ctx := context.Background()
	// 满 1000 分（10 元）打 8 折
	if err := s.CreateCoupon(ctx, &Coupon{Code: "OFF20", Kind: "percent", Value: 20, MinSubtotalCents: 1000, MaxDiscountCents: 0}); err != nil {
		t.Fatal(err)
	}
	d, _, err := s.ValidateCoupon(ctx, "OFF20", 2000)
	if err != nil {
		t.Fatalf("期望校验通过: %v", err)
	}
	if d != 400 { // 2000*20%=400
		t.Fatalf("期望优惠 400 分，实际 %d", d)
	}
	// 未满门槛
	if _, _, err := s.ValidateCoupon(ctx, "OFF20", 500); err == nil {
		t.Fatal("期望门槛拦截")
	}
	// 不存在
	if _, _, err := s.ValidateCoupon(ctx, "NOPE", 5000); err == nil {
		t.Fatal("期望不存在拦截")
	}
}

func TestOrderFlowAndStock(t *testing.T) {
	s := mustStore(t)
	ctx := context.Background()
	stock := int64(5)
	if err := s.CreateProduct(ctx, &Product{Title: "实体书", Kind: "physical", PriceCents: 1000, Stock: &stock, Status: "published", Slug: "book"}); err != nil {
		t.Fatal(err)
	}
	p, _ := s.GetProductBySlug(ctx, "book")
	o := &Order{SessionToken: "cart_x", Gateway: "mock", Currency: "CNY"}
	o.Items = []OrderItem{{ProductID: p.ID, Title: p.Title, Kind: p.Kind, UnitPriceCents: p.PriceCents, Qty: 2, SubtotalCents: 2000}}
	o.SubtotalCents = 2000
	o.TotalCents = 2000
	if err := s.CreateOrder(ctx, o); err != nil {
		t.Fatal(err)
	}
	if o.OrderNo == "" {
		t.Fatal("订单号未生成")
	}
	// 下单不扣库存（pending）
	p2, _ := s.GetProductBySlug(ctx, "book")
	if p2.Stock == nil || *p2.Stock != 5 {
		t.Fatalf("下单不应扣库存，当前 %v", p2.Stock)
	}
	// 支付后扣库存
	if err := s.MarkPaid(ctx, o.OrderNo); err != nil {
		t.Fatal(err)
	}
	p3, _ := s.GetProductBySlug(ctx, "book")
	if p3.Stock == nil || *p3.Stock != 3 {
		t.Fatalf("支付后应扣 2 件，当前 %v", p3.Stock)
	}
	// 幂等
	if err := s.MarkPaid(ctx, o.OrderNo); err != nil {
		t.Fatalf("幂等应成功: %v", err)
	}
}
// B40 数字交付：商品 meta 的 file_id 解析（脏 meta 必须安全降级，不能 panic）。
func TestDigitalFileID(t *testing.T) {
	cases := []struct {
		name, meta, want string
	}{
		{"正常", `{"file_id":"f_abc123"}`, "f_abc123"},
		{"空 meta", ``, ""},
		{"空对象", `{}`, ""},
		{"无该字段", `{"other":"x"}`, ""},
		{"非法 JSON", `{not json`, ""},
		{"类型不符", `{"file_id":123}`, ""},
		{"前后空白", "  {\"file_id\":\"f_x\"}  ", "f_x"},
		{"空字符串值", `{"file_id":"  "}`, ""},
	}
	for _, c := range cases {
		if got := DigitalFileID(c.meta); got != c.want {
			t.Errorf("%s: DigitalFileID(%q) = %q, 期望 %q", c.name, c.meta, got, c.want)
		}
	}
}

// B43 下载次数上限解析：脏 meta / 负数 / 缺省都必须安全降级为「不限」。
func TestMaxDownloadsFromMeta(t *testing.T) {
	cases := []struct {
		name, meta string
		want       int
	}{
		{"缺省不限", `{}`, 0},
		{"空 meta", ``, 0},
		{"显式不限", `{"file_id":"f1","max_downloads":0}`, 0},
		{"正常 5 次", `{"file_id":"f1","max_downloads":5}`, 5},
		{"负数按不限", `{"max_downloads":-3}`, 0},
		{"非法 JSON", `{oops`, 0},
		{"类型不符", `{"max_downloads":"5"}`, 0},
	}
	for _, c := range cases {
		if got := maxDownloadsFromMeta(c.meta); got != c.want {
			t.Errorf("%s: maxDownloadsFromMeta(%q)=%d, 期望 %d", c.name, c.meta, got, c.want)
		}
	}
}
