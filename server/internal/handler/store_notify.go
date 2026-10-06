// store_notify.go 商城订单邮件通知（B44）。
//
// 支付成功后把「订单摘要 + 数字商品下载链接（含条目级令牌）」发到买家下单邮箱。
// 复用账号模块既有的 smtpSend / smtpEnabled（同一套 smtp.* 配置，不新增配置项）。
//
// 设计要点：
//   - **旁路**：通知失败只记日志，绝不影响支付结果（见 service.MarkPaid 的调用处）。
//   - **优雅降级**：SMTP 未配置时静默跳过（返回 nil），演示/离线实例不产生噪音日志。
//   - **令牌直给**：邮件里的链接是自足的（只带 token），买家点开即可下载，无需登录。
package handler

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// storePaidNotifier 组装并发送订单支付成功邮件。返回 error 仅用于记录日志。
func (a *API) storePaidNotifier(ctx context.Context, o *service.Order) error {
	// 演示/离线实例未配 SMTP：静默跳过，不算失败
	if a == nil || a.cfg == nil || !a.smtpEnabled() {
		return nil
	}
	to := strings.TrimSpace(o.ContactEmail)
	if to == "" || !emailRe.MatchString(to) {
		return fmt.Errorf("订单 %s 无有效联系邮箱，跳过通知", o.OrderNo)
	}

	subject := fmt.Sprintf("【爱库录】订单 %s 已支付", o.OrderNo)
	body := a.buildOrderPaidMail(ctx, o)

	host := a.cfg.GetString("smtp.host")
	port := a.cfg.GetString("smtp.port")
	if port == "" {
		port = "465"
	}
	return smtpSend(host, port,
		a.cfg.GetString("smtp.user"), a.cfg.GetString("smtp.pass"),
		a.cfg.GetString("smtp.from"), to, subject, body)
}

// buildOrderPaidMail 纯文本邮件正文（text/plain，兼容性最好）。
func (a *API) buildOrderPaidMail(ctx context.Context, o *service.Order) string {
	var b strings.Builder
	fmt.Fprintf(&b, "订单 %s 已支付成功。\n\n", o.OrderNo)
	fmt.Fprintf(&b, "应付金额：%s\n", formatYuanForMail(o.TotalCents, o.Currency))
	if o.DiscountCents > 0 {
		fmt.Fprintf(&b, "优惠抵扣：-%s\n", formatYuanForMail(o.DiscountCents, o.Currency))
	}
	if o.CouponCode != "" {
		fmt.Fprintf(&b, "使用优惠券：%s\n", o.CouponCode)
	}
	fmt.Fprintf(&b, "\n商品明细：\n")
	for _, it := range o.Items {
		fmt.Fprintf(&b, "  · %s × %d  %s\n", it.Title, it.Qty, formatYuanForMail(it.SubtotalCents, o.Currency))
	}

	// 数字商品：给出自足下载链接（带条目级令牌）
	dl := a.collectDownloadLinks(ctx, o)
	if len(dl) > 0 {
		fmt.Fprintf(&b, "\n数字商品下载：\n")
		for _, d := range dl {
			fmt.Fprintf(&b, "  · %s\n    %s\n", d.title, d.url)
		}
		if lim := commonDownloadLimit(o); lim > 0 {
			fmt.Fprintf(&b, "\n每个商品限下载 %d 次。\n", lim)
		} else {
			fmt.Fprintf(&b, "\n")
		}
		fmt.Fprintf(&b, "提示：链接含专属下载凭据，请勿转发给他人。\n")
	} else {
		fmt.Fprintf(&b, "\n实体商品将按订单联系信息安排发货。\n")
	}

	if o.ContactName != "" {
		fmt.Fprintf(&b, "\n%s\n", o.ContactName)
	}
	return b.String()
}

type mailDownloadLink struct {
	title string
	url   string
}

// collectDownloadLinks 为订单里的数字商品生成带令牌的下载链接。
//
// 需要条目级 download_token（B43 签发）。若某条目缺 token（老订单/未签发），
// 退化为「订单号 + 邮箱」形式（服务端仍会校验归属与次数上限），保证链接不是死的。
func (a *API) collectDownloadLinks(ctx context.Context, o *service.Order) []mailDownloadLink {
	base, _ := ctx.Value(ctxOrigin).(string)
	if base == "" {
		return nil
	}
	out := make([]mailDownloadLink, 0, len(o.Items))
	for _, it := range o.Items {
		if it.Kind != "digital" {
			continue
		}
		q := "token=" + url.QueryEscape(it.DownloadToken)
		if it.DownloadToken == "" {
			q = "email=" + url.QueryEscape(o.ContactEmail)
		}
		out = append(out, mailDownloadLink{
			title: it.Title,
			// 🔴 必须带 /api/v1 前缀：真实端点是 /api/v1/store/order/...，
			// 漏掉会落到 SPA 的 index.html 回退上 → 买家点开看到的是首页 HTML（200 而不是报错），
			// 这种「静默错链」比 404 更难排查。
			url: fmt.Sprintf("%s/api/v1/store/order/%s/download/%s?%s",
				strings.TrimRight(base, "/"), url.PathEscape(o.OrderNo), url.PathEscape(it.ID), q),
		})
	}
	return out
}

// commonDownloadLimit 取本单数字条目的**公共**下载上限；不一致（或存在不限）时返回 0。
func commonDownloadLimit(o *service.Order) int {
	limit := 0
	first := true
	for _, it := range o.Items {
		if it.Kind != "digital" {
			continue
		}
		if first {
			limit = it.MaxDownloads
			first = false
			continue
		}
		if it.MaxDownloads != limit {
			return 0
		}
	}
	return limit
}

func formatYuanForMail(cents int64, currency string) string {
	sym := ""
	if currency == "CNY" {
		sym = "¥"
	} else if currency != "" {
		sym = currency + " "
	}
	return fmt.Sprintf("%s%.2f", sym, float64(cents)/100.0)
}
