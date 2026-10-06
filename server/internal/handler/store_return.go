// store_return.go 退货申请接口层（B46）：买家发起 + 商家审核。
//
// 状态机：pending（待审）→ refunded（已退款，退款成功）/ rejected（已驳回）
// 批准时走 service.Refund（原路退款），退款失败则申请保持 pending、订单状态不变 ——
// 绝不会出现「申请显示已处理、用户没收到钱」。
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// ---- 公开：退货申请（买家侧） ----

// storeReturnCreate 买家发起退货：POST /api/v1/store/orders/return
//
// 授权沿用订单查询同一口径 —— **订单号 + 下单邮箱**双因子，二者缺一不可。
// 邮箱虽是下单自填（未做所有权验证），但退货是「主动降低商家利益」的申请，
// 攻击面比「下载」小得多；真正的约束靠资格四查 + 同单仅一条待审。
func (a *API) storeReturnCreate(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeErr(w, http.StatusNotFound, "STORE_DISABLED", "商城未启用")
		return
	}
	var req struct {
		OrderNo     string `json:"order_no"`
		Email       string `json:"email"`
		Reason      string `json:"reason"`
		AmountCents int64  `json:"amount_cents"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	if strings.TrimSpace(req.OrderNo) == "" || strings.TrimSpace(req.Email) == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "需提供订单号与下单邮箱")
		return
	}
	if len([]rune(req.Reason)) < 2 {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "请填写退货原因（至少 2 个字）")
		return
	}
	rt, err := a.store.CreateReturn(r.Context(), strings.TrimSpace(req.OrderNo),
		strings.TrimSpace(req.Email), req.Reason, req.AmountCents)
	if err != nil {
		if errors.Is(err, service.ErrReturnWindow) {
			writeErr(w, http.StatusBadRequest, "RETURN_WINDOW", err.Error())
			return
		}
		writeErr(w, http.StatusBadRequest, "RETURN_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"return": rt})
}

// ---- 管理：退货审核 ----

func (a *API) adminStoreReturns(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	list, err := a.store.ListReturns(r.Context(), r.URL.Query().Get("status"), 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "STORE_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

// adminStoreReturnDecide 审核：approve=true 走原路退款；退款失败则申请保持 pending、订单状态不变。
func (a *API) adminStoreReturnDecide(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	var req struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
	}
	operator := ""
	if uid, ok := ctxUID(r); ok {
		operator = uid
	}
	rt, err := a.store.DecideReturn(r.Context(), r.PathValue("id"), strings.TrimSpace(req.Note), operator, req.Approve)
	if err != nil {
		// 批准但网关退款失败 → 申请仍 pending，这里如实回绝
		if errors.Is(err, service.ErrRefundUnsupported) {
			writeErr(w, http.StatusBadRequest, "REFUND_UNSUPPORTED", err.Error())
			return
		}
		writeErr(w, http.StatusBadRequest, "RETURN_DECIDE_FAIL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"return": rt})
}
