// ai_billing.go 平台模型用量管控端点（配额策略 + 增值 token 余额/充值/流水）。
//
// 与 ai_quota.go 的分工：ai_quota.go 只管读者 AI 问答（blog_ask 范围、按次数
// 分层）；本文件经 ai.Meter 覆盖**全部 AI 能力**（对话/摘要/向量/OCR/TTS/ASR/
// rerank…），token 级计量 + 余额计费。自备模型（provider=custom）不计量不限量。
//
//   - GET  /api/v1/admin/ai/quota-policy → 当前策略 + 今日按主体用量 + 余额列表
//   - PUT  /api/v1/admin/ai/quota-policy → 保存策略（settings 热生效）
//   - GET  /api/v1/admin/ai/balance?subject=&ledger_limit= → 余额 + 流水
//   - POST /api/v1/admin/ai/balance → 充值/扣减 {subject, delta, reason}
//
// 主体口径见 ai/meter.go："user:<uid>"（authmw 注入）/ "public"（匿名公开访客）。
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
)

// aiQuotaPolicyGet GET /admin/ai/quota-policy
func (a *API) aiQuotaPolicyGet(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	out := map[string]any{
		"policy": a.ai.QuotaPolicy(),
	}
	if m := a.ai.Meter(); m != nil {
		usage, _ := m.TodayUsage(1)
		balances, _ := m.Balances(100)
		out["today"] = usage
		out["balances"] = balances
	} else {
		out["today"] = []ai.UsageDailyRec{}
		out["balances"] = []ai.BalanceRec{}
		out["meter_disabled"] = true
	}
	writeJSON(w, http.StatusOK, out)
}

// aiQuotaPolicySave PUT /admin/ai/quota-policy
func (a *API) aiQuotaPolicySave(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	var req struct {
		UserDailyCalls    *int  `json:"user_daily_calls"`
		UserDailyTokens   *int  `json:"user_daily_tokens"`
		PublicDailyCalls  *int  `json:"public_daily_calls"`
		PublicDailyTokens *int  `json:"public_daily_tokens"`
		BillingEnabled    *bool `json:"billing_enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<15)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败："+err.Error())
		return
	}
	ctx := r.Context()
	for key, v := range map[string]*int{
		"ai.quota.user_daily_calls":    req.UserDailyCalls,
		"ai.quota.user_daily_tokens":   req.UserDailyTokens,
		"ai.quota.public_daily_calls":  req.PublicDailyCalls,
		"ai.quota.public_daily_tokens": req.PublicDailyTokens,
	} {
		if v == nil || *v < 0 {
			continue
		}
		if _, err := a.cfg.Set(ctx, key, strconv.Itoa(*v), "string", "AI 用量管控（0=不限）", "system"); err != nil {
			writeErr(w, http.StatusInternalServerError, "QUOTA_SAVE_FAILED", err.Error())
			return
		}
	}
	if req.BillingEnabled != nil {
		v := "false"
		if *req.BillingEnabled {
			v = "true"
		}
		if _, err := a.cfg.Set(ctx, "ai.billing.enabled", v, "string", "平台模型增值 token 计费开关", "system"); err != nil {
			writeErr(w, http.StatusInternalServerError, "QUOTA_SAVE_FAILED", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "policy": a.ai.QuotaPolicy()})
}

// aiBalanceGet GET /admin/ai/balance?subject=xxx
func (a *API) aiBalanceGet(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	m := a.ai.Meter()
	if m == nil {
		writeErr(w, http.StatusServiceUnavailable, "METER_DISABLED", "用量管控未启用")
		return
	}
	subject := r.URL.Query().Get("subject")
	if subject == "" {
		writeErr(w, http.StatusBadRequest, "SUBJECT_REQUIRED", "缺少 subject 参数（如 user:<uid> 或 public）")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("ledger_limit"))
	ledger, err := m.Ledger(subject, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "LEDGER_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subject": subject,
		"balance": m.Balance(subject),
		"ledger":  ledger,
	})
}

// aiBalanceGrant POST /admin/ai/balance {subject, delta, reason}
func (a *API) aiBalanceGrant(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	m := a.ai.Meter()
	if m == nil {
		writeErr(w, http.StatusServiceUnavailable, "METER_DISABLED", "用量管控未启用")
		return
	}
	var req struct {
		Subject string `json:"subject"`
		Delta   int    `json:"delta"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<15)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败："+err.Error())
		return
	}
	if req.Subject == "" || req.Delta == 0 {
		writeErr(w, http.StatusBadRequest, "BAD_REQUEST", "subject 与 delta（非 0）必填；delta 正=充值 负=扣减")
		return
	}
	bal, err := m.Grant(req.Subject, req.Delta, req.Reason, "admin")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "GRANT_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "subject": req.Subject, "balance": bal,
	})
}
