package handler

// billing_api.go 多租户 SaaS 计费 HTTP 接口（SPEC-BILLING）。
//
// 后台端点全部 isAdmin 守卫；公开端点只暴露套餐目录（安装页选档用）。
// 额度准入（超限拒绝）接在 AI 问答链路上——那里是真正会花钱的地方。

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

func (a *API) billingSvc() *service.BillingService { return service.NewBilling(a.db) }

// ---- 公开 ----

// publicBillingPlans GET /api/v1/public/billing/plans —— 套餐目录（匿名，只读）。
func (a *API) publicBillingPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := a.billingSvc().ListPlans(r.Context(), false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "BILLING_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": plans})
}

// ---- 后台 ----

// billingPlans GET /api/v1/admin/billing/plans —— 全部套餐（含停用）。
func (a *API) billingPlans(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		return
	}
	plans, err := a.billingSvc().ListPlans(r.Context(), true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "BILLING_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": plans})
}

// billingPlanUpdate POST /api/v1/admin/billing/plans —— 改套餐（价格/额度/开关）。
func (a *API) billingPlanUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		return
	}
	var p service.BillingPlan
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	if strings.TrimSpace(p.ID) == "" {
		writeErr(w, http.StatusBadRequest, "BILLING_BAD_REQ", "缺少 plan id")
		return
	}
	// 免费档必须常开：它是到期降档的兜底目标，停用会让所有过期站点失去档位
	if p.ID == "free" && !p.Enabled {
		writeErr(w, http.StatusBadRequest, "BILLING_FREE_IMMUTABLE", "免费档不可停用（到期降档依赖它兜底）")
		return
	}
	if err := a.billingSvc().SetPlan(r.Context(), &p); err != nil {
		if errors.Is(err, service.ErrPlanNotFound) {
			writeErr(w, http.StatusNotFound, "BILLING_PLAN_NOT_FOUND", "套餐不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "BILLING_ERR", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "billing.plan_update", "billing_plans", map[string]any{"id": p.ID})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// billingSiteSub GET /api/v1/admin/billing/sites/{site}/subscription
func (a *API) billingSiteSub(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		return
	}
	siteID := r.PathValue("site")
	if siteID == "" {
		siteID = service.DefaultSiteID
	}
	sub, err := a.billingSvc().GetSiteSubscription(r.Context(), siteID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "BILLING_ERR", err.Error())
		return
	}
	plan, _ := a.billingSvc().GetPlan(r.Context(), sub.PlanID)
	usage, _ := a.billingSvc().UsageOf(r.Context(), siteID, "")
	writeJSON(w, http.StatusOK, map[string]any{"subscription": sub, "plan": plan, "usage_today": usage})
}

// billingSiteSubscribe POST /api/v1/admin/billing/sites/{site}/subscription
// body: {plan_id, period_end_ms?, order_id?} —— 支付回调也会走这里落档。
func (a *API) billingSiteSubscribe(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		return
	}
	siteID := r.PathValue("site")
	if siteID == "" {
		siteID = service.DefaultSiteID
	}
	var req struct {
		PlanID     string `json:"plan_id"`
		PeriodEnd  int64  `json:"period_end_ms"`
		OrderID    string `json:"order_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.PlanID) == "" {
		writeErr(w, http.StatusBadRequest, "BILLING_BAD_REQ", "缺少 plan_id")
		return
	}
	sub, err := a.billingSvc().SubscribeSite(r.Context(), siteID, req.PlanID, "", req.PeriodEnd, req.OrderID)
	if err != nil {
		if errors.Is(err, service.ErrPlanNotFound) {
			writeErr(w, http.StatusNotFound, "BILLING_PLAN_NOT_FOUND", "套餐不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "BILLING_ERR", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "billing.subscribe", "site_subscriptions",
		map[string]any{"site_id": siteID, "plan_id": req.PlanID, "order_id": req.OrderID})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "subscription": sub})
}

// billingSiteCancel POST /api/v1/admin/billing/sites/{site}/subscription/cancel
func (a *API) billingSiteCancel(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		return
	}
	siteID := r.PathValue("site")
	if siteID == "" {
		siteID = service.DefaultSiteID
	}
	var req struct {
		Cancel bool `json:"cancel"` // true=到期后取消；false=立即降回免费
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Cancel {
		if err := a.billingSvc().CancelAtPeriodEnd(r.Context(), siteID, true); err != nil {
			writeErr(w, http.StatusInternalServerError, "BILLING_ERR", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": "cancel_at_period_end"})
		return
	}
	if _, err := a.billingSvc().SubscribeSite(r.Context(), siteID, "free", "free", 0, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, "BILLING_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": "downgraded"})
}

// billingUsage GET /api/v1/admin/billing/sites/{site}/usage?days=30
func (a *API) billingUsage(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		return
	}
	siteID := r.PathValue("site")
	if siteID == "" {
		siteID = service.DefaultSiteID
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 365 {
		days = 30
	}
	out := []service.SiteUsage{}
	for i := 0; i < days; i++ {
		day := dayOffset(-i)
		u, err := a.billingSvc().UsageOf(r.Context(), siteID, day)
		if err != nil {
			continue
		}
		out = append(out, *u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// dayOffset 返回相对今天偏移 n 天的 YYYY-MM-DD（n<0 表示过去）。
func dayOffset(n int) string {
	return time.Now().AddDate(0, 0, n).Format("2006-01-02")
}
