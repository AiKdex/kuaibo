package handler

// cs_api.go 客服模块 HTTP 接口（SPEC-CS-001 M0）。
//
// 三类端点：
//  1. 公开挂件轨 /api/v1/public/cs/*   —— 匿名（白名单），访客收发消息
//  2. 后台工作台 /api/v1/cs/*         —— admin/blogAdminOnly 守卫
//  3. 渠道回调   /api/v1/cs/hooks/{ch}/{token} —— 匿名（白名单），外部平台 webhook
//
// 门控：所有后台端点先过 csBlocked（cs.enabled 总闸），关闭态逐端点 403。

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/service"
	"github.com/google/uuid"
)

// csSvc 取客服服务（惰性单例，走 db）。
func (a *API) csSvc() *service.CSService { return service.NewCS(a.db) }

// csBlocked 客服模块总闸。返回 true 表示已写完 403。
//
// 两道闸，与 org/family 的能力语义保持一致（关闭态 = 端点全关，而不是只藏 UI）：
//  1. capability.cs.inbox —— 能力总闸，默认关；关时所有客服端点（含匿名挂件轨）一律 403。
//     此前只查下述 cs.enabled，导致「能力关 → 侧栏不亮，但公开挂件 API 仍可匿名读写」。
//  2. cs.enabled —— 站长手动 kill switch，显式 "false" 即全关（用于出问题时快速止血）。
func (a *API) csBlocked(w http.ResponseWriter) bool {
	if a.cfg != nil {
		if v, ok := a.cfg.Get("cs.enabled"); ok {
			if s, _ := v.(string); s == "false" {
				writeErr(w, http.StatusForbidden, "CS_DISABLED", "客服模块未启用")
				return true
			}
		}
	}
	if on, _ := service.CapabilityEnabled(a.db, a.cfg, service.CapCSInbox); !on {
		writeErr(w, http.StatusForbidden, "CS_DISABLED", "客服模块未启用")
		return true
	}
	return false
}

// ---- 公开轨（挂件）----

// publicCSStart POST /api/v1/public/cs/start
// 挂件初始化：以 visitor_id 归一联系人 + 开启/复用一个会话，返回会话 id 与历史。
func (a *API) publicCSStart(w http.ResponseWriter, r *http.Request) {
	if a.csBlocked(w) {
		return
	}
	var req struct {
		VisitorID  string `json:"visitor_id"`
		DisplayName string `json:"display_name"`
		Email      string `json:"email"`
		Subject    string `json:"subject"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.VisitorID) == "" {
		writeErr(w, http.StatusBadRequest, "CS_BAD_REQ", "缺少 visitor_id")
		return
	}
	svc := a.csSvc()
	// 挂件会话：threadID 用 visitor_id 归并（同一访客重开=同一线程，历史可续）
	contact, err := svc.ResolveContact(r.Context(), "webchat", req.VisitorID, req.DisplayName, req.Email)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	conv, err := svc.UpsertConversation(r.Context(), contact.ID, "webchat", "webchat:"+req.VisitorID, req.Subject)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	msgs, _ := svc.ListMessages(r.Context(), conv.ID, 200)
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": conv.ID,
		"contact_id":      contact.ID,
		"display_name":    contact.DisplayName,
		"messages":        csMsgsOut(msgs),
	})
}

// publicCSSend POST /api/v1/public/cs/send —— 访客发消息（幂等：同 external_id 只落一条）。
func (a *API) publicCSSend(w http.ResponseWriter, r *http.Request) {
	if a.csBlocked(w) {
		return
	}
	var req struct {
		ConversationID string `json:"conversation_id"`
		VisitorID      string `json:"visitor_id"`
		ExternalID     string `json:"external_id"`
		Text           string `json:"text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.ConversationID) == "" || strings.TrimSpace(req.Text) == "" {
		writeErr(w, http.StatusBadRequest, "CS_BAD_REQ", "缺少 conversation_id 或 text")
		return
	}
	svc := a.csSvc()
	msgID, already, err := svc.AppendInbound(r.Context(), req.ConversationID, service.CSInbound{
		Channel:     "webchat",
		ExternalID:  req.ExternalID,
		ExternalID2: req.VisitorID,
		Text:        req.Text,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message_id": msgID, "already": already})
}

// publicCSPull GET /api/v1/public/cs/pull?conversation_id=  —— 访客拉新消息（断线重连）。
func (a *API) publicCSPull(w http.ResponseWriter, r *http.Request) {
	if a.csBlocked(w) {
		return
	}
	convID := r.URL.Query().Get("conversation_id")
	if convID == "" {
		writeErr(w, http.StatusBadRequest, "CS_BAD_REQ", "缺少 conversation_id")
		return
	}
	svc := a.csSvc()
	// 归属校验：会话必须存在；访客轨不泄露他人会话（无 token 机制下以不可猜测 id + 存在性为界）
	if _, err := svc.GetConversation(r.Context(), convID); err != nil {
		writeErr(w, http.StatusNotFound, "CS_NOT_FOUND", "会话不存在")
		return
	}
	msgs, err := svc.ListMessages(r.Context(), convID, 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": csMsgsOut(msgs)})
}

func csMsgsOut(in []service.CSMessage) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, m := range in {
		out = append(out, map[string]any{
			"id": m.ID, "direction": m.Direction, "author_type": m.AuthorType,
			"author_id": m.AuthorID, "text": m.Text, "created_at": m.CreatedAt,
		})
	}
	return out
}

// ---- 后台工作台 ----

// csInbox GET /api/v1/cs/inbox —— 会话列表。
func (a *API) csInbox(w http.ResponseWriter, r *http.Request) {
	if a.csBlocked(w) || !a.blogAdminOnly(w, r) {
		return
	}
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	convs, err := a.csSvc().ListConversations(r.Context(), status, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": convs})
}

// csConversationMessages GET /api/v1/cs/conversations/{id}/messages —— 会话消息。
func (a *API) csConversationMessages(w http.ResponseWriter, r *http.Request) {
	if a.csBlocked(w) || !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	msgs, err := a.csSvc().ListMessages(r.Context(), id, 500)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": csMsgsOut(msgs)})
}

// csConversationReply POST /api/v1/cs/conversations/{id}/reply —— 坐席回复（幂等 client_msg_id）。
func (a *API) csConversationReply(w http.ResponseWriter, r *http.Request) {
	if a.csBlocked(w) || !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		Text      string `json:"text"`
		ClientMsgID string `json:"client_msg_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.Text) == "" {
		writeErr(w, http.StatusBadRequest, "CS_BAD_REQ", "text 必填")
		return
	}
	svc := a.csSvc()
	conv, err := svc.GetConversation(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "CS_NOT_FOUND", "会话不存在")
		return
	}
	// 幂等：坐席端每次发送带 client_msg_id
	idem := strings.TrimSpace(req.ClientMsgID)
	if idem == "" {
		idem = "reply:" + uuidNew() // 未带则不保证幂等（仍可用）
	}
	if _, dup, err := svc.EnqueueOutbound(r.Context(), conv.ID, conv.Channel, idem, map[string]any{
		"to": conv.ContactID, "text": req.Text,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	} else if dup {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "duplicate": true})
		return
	}
	// 同步落一条 out 消息（挂件渠道即时可见；其他渠道由 worker 外推）
	if _, err := svc.AppendOutbound(r.Context(), conv.ID, "agent", a.curUserID(r), req.Text, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// csSetStatus POST /api/v1/cs/conversations/{id}/status
func (a *API) csSetStatus(w http.ResponseWriter, r *http.Request) {
	if a.csBlocked(w) || !a.blogAdminOnly(w, r) {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	switch req.Status {
	case "open", "pending", "resolved", "closed":
	default:
		writeErr(w, http.StatusBadRequest, "CS_BAD_STATUS", "status 非法")
		return
	}
	if err := a.csSvc().SetConversationStatus(r.Context(), r.PathValue("id"), req.Status); err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// csContacts GET /api/v1/cs/contacts
func (a *API) csContacts(w http.ResponseWriter, r *http.Request) {
	if a.csBlocked(w) || !a.blogAdminOnly(w, r) {
		return
	}
	// cs.contacts 关 → 只读历史：列表可看，但不再新建/合并联系人（写入点在 AppendInbound 侧）。
	items, err := a.csSvc().ListContacts(r.Context(), r.URL.Query().Get("q"), 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ---- 渠道回调（匿名）----

// csHook POST /api/v1/cs/hooks/{channel}/{token} —— 外部平台入站（幂等）。
// token 用渠道实例密钥（cs_channels.config_enc 派生）；不匹配直接 404（不泄露端点存在性）。
func (a *API) csHook(w http.ResponseWriter, r *http.Request) {
	if a.csBlocked(w) {
		return
	}
	// cs.channels 关 → 不接外部 webhook（与总闸独立：总闸管模块，能力管渠道这一轨）。
	if on, _ := service.CapabilityEnabled(a.db, a.cfg, service.CapCSChannels); !on {
		writeErr(w, http.StatusForbidden, "CS_CHANNELS_DISABLED", "客服渠道适配未启用")
		return
	}
	channel := r.PathValue("channel")
	token := r.PathValue("token")
	// 渠道密钥校验：取该渠道配置的 token 比对（配置缺失=该渠道未开通）
	ok := false
	var cfgEnc string
	_ = a.db.QueryRowContext(r.Context(), `SELECT config_enc FROM cs_channels WHERE channel=? AND enabled=1`, channel).Scan(&cfgEnc)
	if cfgEnc != "" {
		// 简化：config_enc 存明文 token 的哈希（base64 sha256），比对哈希
		if sha256Base64(token) == cfgEnc {
			ok = true
		}
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "CS_NOT_FOUND", "端点不存在")
		return
	}
	var payload struct {
		ExternalID  string `json:"external_id"`
		ThreadID    string `json:"thread_id"`
		SenderID    string `json:"sender_id"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Subject     string `json:"subject"`
		Text        string `json:"text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if strings.TrimSpace(payload.ExternalID) == "" {
		writeErr(w, http.StatusBadRequest, "CS_BAD_REQ", "缺少 external_id")
		return
	}
	svc := a.csSvc()
	contact, err := svc.ResolveContact(r.Context(), channel, payload.SenderID, payload.DisplayName, payload.Email)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	conv, err := svc.UpsertConversation(r.Context(), contact.ID, channel, payload.ThreadID, payload.Subject)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	// 幂等落库（重复投递 already=true，仍返 200）
	_, already, err := svc.AppendInbound(r.Context(), conv.ID, service.CSInbound{
		Channel:     channel,
		ExternalID:  payload.ExternalID,
		ThreadID:    payload.ThreadID,
		ExternalID2: payload.SenderID,
		DisplayName: payload.DisplayName,
		Text:        payload.Text,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "already": already})
}

// ---- 小工具 ----

// uuidNew 生成消息/幂等 id。
func uuidNew() string { return uuid.NewString() }

// sha256Base64 渠道 token 哈希（存库比对，避免明文落库）。
func sha256Base64(s string) string {
	sum := sha256.Sum256([]byte("aiklog-cs-token:" + s))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// ---- AI 起草（cs.ai_draft）----

// csConversationDraft POST /api/v1/cs/conversations/{id}/draft
//
// 依据知识库 FAQ + 会话最近若干条消息，生成一版「建议回复」（不落库、不外发），
// 由坐席确认后再走 /reply 发送——AI 只起草，不代替人回复。
//
// 依赖两处已建能力：a.kb（FAQ 检索）+ a.ai（模型）。任一未接线则 503，不静默降级。
func (a *API) csConversationDraft(w http.ResponseWriter, r *http.Request) {
	if a.csBlocked(w) || !a.blogAdminOnly(w, r) {
		return
	}
	if on, _ := service.CapabilityEnabled(a.db, a.cfg, service.CapCSAIDraft); !on {
		writeErr(w, http.StatusForbidden, "CS_AI_DISABLED", "客服 AI 起草未启用")
		return
	}
	id := r.PathValue("id")
	svc := a.csSvc()
	conv, err := svc.GetConversation(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "CS_NOT_FOUND", "会话不存在")
		return
	}
	msgs, err := svc.ListMessages(r.Context(), id, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CS_ERR", err.Error())
		return
	}
	// 问句 = 最近一条客户(in)消息；没有则用主题兜底。
	question := strings.TrimSpace(conv.Subject)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Direction == "in" && strings.TrimSpace(msgs[i].Text) != "" {
			question = strings.TrimSpace(msgs[i].Text)
			break
		}
	}
	if question == "" {
		writeErr(w, http.StatusBadRequest, "CS_BAD_REQ", "会话内没有可据此起草的客户提问")
		return
	}

	// 知识库召回：FAQ 优先（结构化答案，质量高于自由文本块）。
	var kb strings.Builder
	if a.kb != nil {
		if faqs, err := a.kb.SearchFAQ(r.Context(), a.homeSpaceID(), question, 4); err == nil {
			for i, f := range faqs {
				if i >= 3 {
					break
				}
				fmt.Fprintf(&kb, "### 问：%s\n答：%s\n", f.StandardQ, f.Answer)
			}
		}
	}
	if kb.Len() == 0 {
		writeErr(w, http.StatusServiceUnavailable, "CS_AI_NO_KB", "知识库没有可引用的内容，无法起草（避免模型凭空编造）")
		return
	}

	// AI 额度准入：与博客问答同一把闸（计费是增强项，计量故障不阻断业务）。
	if err := a.billingSvc().AddAIUsage(r.Context(), service.DefaultSiteID); err != nil {
		if errors.Is(err, service.ErrQuotaExceeded) {
			writeErr(w, http.StatusPaymentRequired, "QUOTA_EXCEEDED", err.Error())
			return
		}
		log.Printf("[billing] cs draft ai usage 计量失败：%v", err)
	}

	sys := "你是站点客服助手。仅依据下面「知识库摘录」回答；摘录里没有的，一律回复「这个问题需要人工确认，我暂时无法回答」，不要编造。用一句话给出可直接发送给访客的回复，不要署名、不要解释你在引用资料。"
	res, err := a.ai.ChatJSON(r.Context(), []ai.Msg{
		{Role: "system", Content: sys},
		{Role: "user", Content: "知识库摘录：\n" + kb.String() + "\n\n访客问题：" + question},
	}, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "CS_AI_ERR", "AI 起草失败："+err.Error())
		return
	}
	if res != nil && res.Error != "" {
		writeErr(w, http.StatusBadGateway, "CS_AI_ERR", "AI 起草失败："+res.Error)
		return
	}
	var content string
	if res != nil {
		content = res.Content
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "question": question,
		"draft": extractCSAnswer(content),
	})
}

// extractCSAnswer 从模型返回里取出正文（容忍 {"answer":...} 与纯文本两种形态）。
func extractCSAnswer(res string) string {
	s := strings.TrimSpace(res)
	if s == "" {
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(s), &m) == nil {
		for _, k := range []string{"answer", "reply", "text", "content"} {
			if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return s
}
