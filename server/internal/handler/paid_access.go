package handler

// paid_access.go 内容付费访问（生产站变现底座 B 项；自上游 AiKmap 主系统移植，2026-09-19）。
//
//   - 文章/文件级付费：files.access_mode='paid' + price_cents（分）+ paid_preview
//     （'none'=不预览只显示元数据；'partial'=截断预览（由读取方按 preview 字符数截断）；空=完整预览）。
//   - 公开正文闸门：requirePaidAccess 在 requireArticleUnlock（密码）之后检查；未持有有效
//     grant → 402 PAYMENT_REQUIRED（附价格与预览策略，前端出付费页）。
//   - 凭证：access_grants 表（grant_token 随机、可绑定 grantee、可过期）；公开请求带
//     X-Grant-Token 或 ?grant= 即放行。
//   - 通用支付回调：POST /api/v1/pay/notify —— 内核不直接对接支付商，由支付对接 tool/插件
//     完成收款后以 HMAC（settings pay.notify_secret，兼容旧键 pay.webhook_secret）转发本回调；回调幂等签发 grant。
//
// 移植适配说明（AiKlog 与主系统的差异）：
//   - 主系统 blogAuthorOnly → 本 fork blogAdminOnly（同义，本 fork 统一命名为 admin）；
//   - isBlogSubDir 在本 fork 自实现（递归 CTE，覆盖博客根 BlogDirID 的任意层级后代）；
//   - 主系统 blogFileMeta / contentStateExtras 未移植：AiKlog 已于 P0 打通
//     POST /api/v1/blog/posts/meta（node_type/fields 写入）与 service.NodeType/Fields 读取，
//     同能力不重复建端点。

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// ---- 付费访问模式 ----

// articleAccessMode 读访问模式：none|password|paid（password 沿用 access_pwd 字段）。
func (a *API) articleAccessMode(r *http.Request, fileID string) string {
	var mode string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(access_mode,'none') FROM files WHERE id=?`, fileID).Scan(&mode)
	if err != nil {
		return "none"
	}
	return mode
}

// paidMeta 读付费配置（价格分 + 预览策略）。
func (a *API) paidMeta(r *http.Request, fileID string) (priceCents int64, preview string) {
	_ = a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(price_cents,0), COALESCE(paid_preview,'') FROM files WHERE id=?`, fileID).Scan(&priceCents, &preview)
	return
}

// isBlogSubDir 判断给定文件是否位于博客目录树内（博客根 service.BlogDirID 的任意层级后代）。
// 上游主系统同义函数；本 fork 用递归 CTE 实现，覆盖「博客根 → 分类目录 → 文章」多层结构。
func (a *API) isBlogSubDir(ctx context.Context, fileID string) bool {
	if fileID == "" {
		return false
	}
	var cnt int
	err := a.db.QueryRowContext(ctx, `
		WITH RECURSIVE blogtree(id) AS (
			SELECT id FROM files WHERE parent_id=? AND deleted_at IS NULL
			UNION
			SELECT f.id FROM files f JOIN blogtree t ON f.parent_id=t.id WHERE f.deleted_at IS NULL
		)
		SELECT COUNT(*) FROM blogtree WHERE id=?`, service.BlogDirID, fileID).Scan(&cnt)
	return err == nil && cnt > 0
}

// ---- grant 凭证（access_grants 表） ----

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// issuePaidGrant 签发付费访问凭证（落库；grantee 可为空=兑换码制）。返回 token。
func (a *API) issuePaidGrant(fileID, grantee, source string, ttlMillis int64) (string, error) {
	tok := randomToken(16)
	exp := int64(0)
	if ttlMillis > 0 {
		exp = time.Now().UnixMilli() + ttlMillis
	}
	_, err := a.db.ExecContext(context.Background(),
		`INSERT INTO access_grants (id, file_id, grantee, grant_token, source, expires_at, created_at) VALUES (?,?,?,?,?,?,?)`,
		randomToken(8), fileID, grantee, tok, source, exp, time.Now().UnixMilli())
	if err != nil {
		return "", err
	}
	return tok, nil
}

// paidGrantValid 校验 grant 凭证（匹配 file_id 且未过期；expires_at=0 永不过期）。
func (a *API) paidGrantValid(r *http.Request, fileID, tok string) bool {
	if tok == "" {
		return false
	}
	var exp int64
	var cnt int
	err := a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*), COALESCE(MAX(expires_at),0) FROM access_grants
		 WHERE file_id=? AND grant_token=?`, fileID, tok).Scan(&cnt, &exp)
	if err != nil || cnt == 0 {
		return false
	}
	return exp == 0 || exp > time.Now().UnixMilli()
}

var errPaidRequired = errors.New("payment required")

// requirePaidAccess 付费闸门：access_mode='paid' 且无有效 grant → 402 PAYMENT_REQUIRED
// （附 price_cents/preview 供前端付费页）。返回 errPaidRequired 供调用方区分。
func (a *API) requirePaidAccess(w http.ResponseWriter, r *http.Request, fileID string) error {
	if a.articleAccessMode(r, fileID) != "paid" {
		return nil
	}
	tok := r.Header.Get("X-Grant-Token")
	if tok == "" {
		tok = r.URL.Query().Get("grant")
	}
	if tok != "" && a.paidGrantValid(r, fileID, tok) {
		return nil
	}
	price, preview := a.paidMeta(r, fileID)
	writeJSON(w, http.StatusPaymentRequired, map[string]any{
		"error": map[string]any{"code": "PAYMENT_REQUIRED", "message": "该内容为付费内容，请完成支付后访问"},
		"paid":  map[string]any{"price_cents": price, "preview": preview},
	})
	return errPaidRequired
}

// paidLocked 无副作用的付费闸门探针（不写响应）。C2 修复用：
// requirePaidAccess 的形态是"写 402 JSON"，HTML 页面需要的是"少渲染正文"。
func (a *API) paidLocked(r *http.Request, fileID string) bool {
	if a.articleAccessMode(r, fileID) != "paid" {
		return false
	}
	tok := r.Header.Get("X-Grant-Token")
	if tok == "" {
		tok = r.URL.Query().Get("grant")
	}
	return !(tok != "" && a.paidGrantValid(r, fileID, tok))
}

// grantTokenOf 从请求取付费凭证（头优先、query 兜底）。分享/SSR 两轨共用同一取值口径。
func grantTokenOf(r *http.Request) string {
	if tok := r.Header.Get("X-Grant-Token"); tok != "" {
		return tok
	}
	return r.URL.Query().Get("grant")
}

// ---- 管理端设置 ----

// blogPostsPaid POST /api/v1/blog/posts/paid {"id","mode":"none|password|paid","price_cents":分,"preview":"none|partial|full","password":"…(mode=password 时)"}
// 文章访问控制统一设置：none/密码/付费（密码沿用现有 access_pwd 字段）。
func (a *API) blogPostsPaid(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	var req struct {
		ID         string `json:"id"`
		Mode       string `json:"mode"` // none|password|paid
		PriceCents int64  `json:"price_cents"`
		Preview    string `json:"preview"` // none|partial|full
		Password   string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	if req.Mode == "" {
		req.Mode = "none"
	}
	switch req.Mode {
	case "none", "password", "paid":
	default:
		writeErr(w, http.StatusBadRequest, "BAD_MODE", "mode 仅支持 none/password/paid")
		return
	}
	if req.Mode == "paid" && req.PriceCents <= 0 {
		writeErr(w, http.StatusBadRequest, "PRICE_REQUIRED", "付费内容需设置价格（分）")
		return
	}
	if req.Preview == "" {
		req.Preview = "partial"
	}
	switch req.Preview {
	case "none", "partial", "full":
	default:
		writeErr(w, http.StatusBadRequest, "BAD_PREVIEW", "preview 仅支持 none/partial/full")
		return
	}
	if !a.isBlogSubDir(r.Context(), req.ID) {
		writeErr(w, http.StatusForbidden, "BLOG_FILES_OUT_OF_SCOPE", "只能设置博客目录内的文章")
		return
	}
	// password 沿用 access_pwd；paid 清空 access_pwd（互斥）
	var pwd any
	if req.Mode == "password" {
		if req.Password == "" {
			writeErr(w, http.StatusBadRequest, "PASSWORD_REQUIRED", "密码模式需提供密码")
			return
		}
		pwd = hashAccessPwd(req.Password)
	}
	price := int64(0)
	if req.Mode == "paid" {
		price = req.PriceCents
	}
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE files SET access_mode=?, price_cents=?, paid_preview=?, access_pwd=?, updated_at=? WHERE id=?`,
		req.Mode, price, req.Preview, pwd, time.Now().UnixMilli(), req.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "POST_PAID_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.posts_paid_"+req.Mode, "files",
		map[string]any{"id": req.ID, "price_cents": price, "preview": req.Preview})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- 通用支付回调 ----

// paidSecret 支付回调签名密钥。
// 键名以上游契约为准（pay.notify_secret），同时兼容本 fork 早期使用的旧键
// pay.webhook_secret，避免已配置站点升级后回调失效。
func (a *API) paidSecret() string {
	if s := strings.TrimSpace(a.cfg.GetString("pay.notify_secret")); s != "" {
		return s
	}
	return strings.TrimSpace(a.cfg.GetString("pay.webhook_secret"))
}

// payNotify POST /api/v1/pay/notify
// 通用支付回调：支付对接 tool/插件完成收款后，以 HMAC（settings pay.notify_secret）转发。
// payload: {provider, out_trade_no, file_id, amount_cents, grantee(可选), ttl_sec(可选), ts, sig}
// sig = hex(hmac_sha256(secret, provider|out_trade_no|file_id|amount_cents|ts))
// 幂等：同 out_trade_no 重复回调不重复签发（记录于 audit）；返回 grant_token。
func (a *API) payNotify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider    string `json:"provider"`
		OutTradeNo  string `json:"out_trade_no"`
		FileID      string `json:"file_id"`
		AmountCents int64  `json:"amount_cents"`
		Grantee     string `json:"grantee,omitempty"`
		TTLSec      int64  `json:"ttl_sec,omitempty"` // 0=永久
		Ts          string `json:"ts"`
		Sig         string `json:"sig"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	if req.Provider == "" || req.OutTradeNo == "" || req.FileID == "" || req.AmountCents <= 0 || req.Ts == "" || req.Sig == "" {
		writeErr(w, http.StatusBadRequest, "BAD_PAYLOAD", "缺少必填字段")
		return
	}
	secret := a.paidSecret()
	if secret == "" {
		writeErr(w, http.StatusServiceUnavailable, "PAY_NOT_CONFIGURED", "支付回调密钥未配置（settings pay.notify_secret）")
		return
	}
	// HMAC 校验
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.Join([]string{req.Provider, req.OutTradeNo, req.FileID, strconv.FormatInt(req.AmountCents, 10), req.Ts}, "|")))
	expect := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expect), []byte(req.Sig)) != 1 {
		writeErr(w, http.StatusForbidden, "BAD_SIGNATURE", "回调签名校验失败")
		return
	}
	// H7 修复①：回调时间窗防重放。ts 已纳入 HMAC 载荷，但仅此不足以防重放——
	// 同一份 (provider|out_trade_no|file_id|amount|ts) 载荷可被无限次重发。
	// 这里校验新鲜度（±30 分钟，兼容秒/毫秒两种单位），过期回调一律拒收并记审计。
	tsMs, perr := strconv.ParseInt(req.Ts, 10, 64)
	if perr != nil {
		writeErr(w, http.StatusBadRequest, "BAD_TS", "回调时间戳非法")
		return
	}
	if tsMs < 1e12 {
		tsMs *= 1000 // 秒 → 毫秒
	}
	nowMs := time.Now().UnixMilli()
	if tsMs < nowMs-30*60*1000 || tsMs > nowMs+30*60*1000 {
		_, _ = a.aud.Append(r.Context(), "", "pay.notify_stale", "payment",
			map[string]any{"provider": req.Provider, "out_trade_no": req.OutTradeNo, "ts": req.Ts})
		writeErr(w, http.StatusForbidden, "STALE_NOTIFY", "回调已过期或时间偏差过大")
		return
	}
	// 幂等：同 provider+out_trade_no 已处理过则复用原凭证
	var existing string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT grant_token FROM access_grants WHERE source=? AND grantee=? LIMIT 1`,
		"pay:"+req.Provider+":"+req.OutTradeNo, req.Grantee).Scan(&existing)
	if err == nil && existing != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "grant_token": existing, "duplicate": true})
		return
	}
	// H7 修复②③：交付前核验「文章确为付费」且「回调金额不低于定价」。
	// 修复②：仅接受 access_mode='paid' 的文章（防对免费/密码文章伪造回调白拿凭证）。
	// 修复③：金额下限校验（防 1 分钱回调买下高价文章——签名只保证"消息未被篡改"，
	//        不保证"金额与商品一致"，故必须与库内 price_cents 对账）。
	var artPrice int64
	var artMode string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(price_cents,0), COALESCE(access_mode,'none') FROM files WHERE id=?`, req.FileID).
		Scan(&artPrice, &artMode); err != nil {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "文章不存在")
		return
	}
	if artMode != "paid" {
		_, _ = a.aud.Append(r.Context(), "", "pay.notify_not_paid", "files",
			map[string]any{"file_id": req.FileID, "mode": artMode, "out_trade_no": req.OutTradeNo})
		writeErr(w, http.StatusBadRequest, "NOT_PAID", "文章未开启付费模式（access_mode≠paid）")
		return
	}
	if artPrice > 0 && req.AmountCents < artPrice {
		_, _ = a.aud.Append(r.Context(), "", "pay.notify_amount_mismatch", "files",
			map[string]any{"file_id": req.FileID, "paid": req.AmountCents, "price": artPrice, "out_trade_no": req.OutTradeNo})
		writeErr(w, http.StatusPaymentRequired, "AMOUNT_TOO_LOW", "回调金额低于文章定价")
		return
	}
	tok, err := a.issuePaidGrant(req.FileID, req.Grantee, "pay:"+req.Provider+":"+req.OutTradeNo, req.TTLSec*1000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "GRANT_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), "", "pay.notify_grant", "files",
		map[string]any{"provider": req.Provider, "out_trade_no": req.OutTradeNo, "file_id": req.FileID, "amount_cents": req.AmountCents})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "grant_token": tok})
}
