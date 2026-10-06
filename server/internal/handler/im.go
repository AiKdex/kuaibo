// im.go IM 网关 HTTP 入口 + 业务分发（移植自 AiKmap.cn，并新增「发博客」意图）。
//
//	POST /api/v1/im/webhook/telegram   Telegram webhook（解析→意图→分发→异步回复）
//	GET  /api/v1/im/webhook/wecom     WeCom 回调 URL 验证
//	POST /api/v1/im/webhook/wecom     WeCom 回调消息
//	GET  /api/v1/im/status            IM 配置状态
//	PUT  /api/v1/im/telegram/config   配置 Telegram Bot Token / 白名单
//	PUT  /api/v1/im/wecom/config      配置企业微信
//
// 设计要点：
//   - 适配器（internal/im）只做 HTTP 出入，业务分发在本文件；
//   - 动作复用现有服务层：/post → 在博客目录建文件并发布；/draft → 建文件存草稿；
//     /search /list /get 直接查博客目录；开放请求 → Agent；
//   - 身份映射（B7）：按 (platform, platform_user_id) 查 im_bindings 得到站内 user；
//     **未绑定则回退系统 owner**（零破坏既有单用户部署），可选 allowed_chats 白名单；
//     `/bind <码>` 消费网页端生成的绑定码，`/unbind` 解除；
//   - 回复异步发送，不阻塞 webhook 响应。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/im"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

const (
	imMaxText = 6000 // 入库/发博文本截断
	imMaxRep  = 2800 // 回复截断（Telegram 单条上限）
)

// imRouter 全局意图路由器（无状态）。
var imRouter = im.NewIntentRouter()

// tgAdapter Telegram 适配器（无状态）。
var tgAdapter = im.TelegramAdapter{}

// wecomAdapter 企业微信适配器（无状态）。
var wecomAdapter = im.WeComAdapter{}

// imWebhook Telegram webhook：POST /api/v1/im/webhook/telegram。
func (a *API) imWebhook(w http.ResponseWriter, r *http.Request) {
	token := a.cfg.GetString("im.telegram.bot_token")
	if token == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "handled": false, "reason": "im_disabled"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "read body"})
		return
	}
	msg, err := tgAdapter.Parse(r.Context(), body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if msg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "handled": false, "reason": "ignored"})
		return
	}
	// 白名单：im.telegram.allowed_chats（逗号分隔 chat_id；空=全部）
	if allowed := a.cfg.GetString("im.telegram.allowed_chats"); allowed != "" {
		hit := false
		for _, c := range strings.Split(allowed, ",") {
			if strings.TrimSpace(c) == msg.ChatID {
				hit = true
				break
			}
		}
		if !hit {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "handled": false, "reason": "chat_not_allowed"})
			return
		}
	}
	ctx := r.Context()
	// 幂等：同平台同 msg_id 只处理一次（IM 平台重发/用户连发防护）
	if msg.MsgID != "" {
		var dup int
		if err := a.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM im_ingest WHERE platform=? AND msg_id=?`, msg.Platform, msg.MsgID).Scan(&dup); err == nil && dup > 0 {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "handled": true, "reason": "duplicate"})
			return
		}
	}
	intent := imRouter.Route(msg.Text)
	actorID := a.imActor(ctx, msg)
	reply := a.imDispatch(ctx, r, msg, intent, actorID)
	// 记录幂等键（仅已成功处理的消息；入库失败不记，允许重试）
	if reply.OK && msg.MsgID != "" {
		fid := ""
		if f, ok := reply.FileID.(string); ok {
			fid = f
		}
		_, _ = a.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO im_ingest (id, platform, msg_id, intent, file_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			newID(), msg.Platform, msg.MsgID, string(intent.Type), fid, time.Now().Unix())
	}
	// 异步发送，不阻塞 webhook
	go func() {
		bctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := tgAdapter.Send(bctx, token, msg.ChatID, reply.Text); err != nil {
			_, _ = a.aud.Append(context.Background(), actorID, "im.send_failed", msg.ChatID, map[string]any{
				"platform": "telegram", "err": err.Error(),
			})
		}
	}()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "handled": reply.OK})
}

// imDispatch 意图分发：每个动作复用现有服务层。
// actorID 是本条消息的**站内身份**（已绑定用户；未绑定时由 imActor 回退为系统 owner）。
func (a *API) imDispatch(ctx context.Context, r *http.Request, msg *im.Msg, intent im.IntentResult, actorID string) *im.Reply {
	// 语音消息（family.8.2）：不走文本意图路由——受理后异步转写（im_voice.go），
	// family.enabled 时归档为人生事件；wecom 适配器暂不解析语音，仅 Telegram 语音会走到这里。
	if msg != nil && msg.MsgType == "voice" {
		return a.imVoiceIncoming(msg, actorID)
	}
	switch intent.Type {
	case im.IntentStart, im.IntentHelp:
		return &im.Reply{Text: imHelpText(), OK: true}
	case im.IntentBind: // B7：/bind <码> 绑定 IM 身份
		return a.imBindCode(ctx, msg, intent.Params["query"])
	case im.IntentUnbind: // B7：/unbind 解除绑定
		return a.imUnbindIM(ctx, msg)
	case im.IntentPost:
		return a.imPost(ctx, r, intent.Params["query"], true, actorID)
	case im.IntentDraft:
		return a.imPost(ctx, r, intent.Params["query"], false, actorID)
	case im.IntentText: // 兜底：未走前缀/正则的纯文本碎片 → 博客草稿
		return a.imPost(ctx, r, intent.Params["query"], false, actorID)
	case im.IntentURL: // 裸链接 → 博客草稿（内容为链接）
		return a.imPost(ctx, r, intent.Params["url"], false, actorID)
	case im.IntentSearch:
		return a.imSearch(ctx, intent.Params["query"])
	case im.IntentList:
		return a.imList(ctx)
	case im.IntentRetrieve:
		return a.imRetrieve(ctx, r, intent.Params["query"])
	case im.IntentSummary:
		return a.imSummary(ctx, intent.Params["query"])
	case im.IntentStatus:
		return a.imStatus(ctx)
	case im.IntentWorkflow: // 内容工作流命名空间（/wf <id> 或 /wf-<name>）
		return a.imWorkflow(ctx, r, msg, intent)
	case im.IntentAgent:
		// 先看是否命中某个 workflow 的 im_command 前缀（站长自定义指令，如 /collect）。
		// 既有固定指令在 Layer 1 已优先命中，所以走到这里的前缀必然是空闲的，
		// 不会抢走 /post、/draft 等已有语义。
		if wf, prefix := a.wfMatchPrefix(msg.Text); wf != nil {
			return a.imRunWorkflow(ctx, r, msg, wf, wfStripTrigger(msg.Text, prefix))
		}
		return a.imAgent(ctx, msg.Text)
	default:
		return &im.Reply{Text: "我没理解你的意思，试试 /help", OK: false}
	}
}

// imPost 发博客：在博客目录建 .md 文件并发布/存草稿。
func (a *API) imPost(ctx context.Context, r *http.Request, query string, publish bool, actorID string) *im.Reply {
	query = strings.TrimSpace(query)
	if query == "" {
		return &im.Reply{Text: "要发布什么内容呢？用法：/post 正文  或  /draft 正文（存草稿）", OK: false}
	}
	if len(query) > imMaxText {
		query = query[:imMaxText]
	}
	title := firstLine(query, 60)
	name := sanitizeName(title) + ".md"
	// B7：发文身份 = 本条消息的站内身份（已绑定用户）；空则回退系统 owner
	owner := strings.TrimSpace(actorID)
	if owner == "" {
		owner = a.homeOwnerID()
	}
	space := a.homeSpaceID()
	parentID := service.BlogDirID
	doc, err := a.files.CreateDoc(ctx, owner, space, parentID, name, query, service.DefaultSiteID)
	if err != nil {
		// 重名：追加时分秒后缀重试一次
		name = sanitizeName(title) + "-" + time.Now().Format("150405") + ".md"
		doc, err = a.files.CreateDoc(ctx, owner, space, parentID, name, query, service.DefaultSiteID)
		if err != nil {
			return &im.Reply{Text: "发布失败：" + err.Error(), OK: false}
		}
	}
	status := "draft"
	if publish {
		status = "published"
	}
	if err := a.setFileStatusInternal(ctx, doc.ID, status); err != nil {
		return &im.Reply{Text: "状态设置失败：" + err.Error(), OK: false}
	}
	slug, _ := service.EnsureFileSlug(ctx, a.db, doc.ID, doc.Name)
	_, _ = a.aud.Append(ctx, owner, "im.blog_post", "files", map[string]any{
		"id": doc.ID, "slug": slug, "publish": publish, "title": title,
	})
	verb := "已存为博客草稿"
	link := ""
	if publish {
		verb = "已发布到博客"
		link = a.publicBaseURL(r) + "/" + slug
	}
	text := fmt.Sprintf("%s：%s", verb, title)
	if link != "" {
		text += "\n" + link
	}
	return &im.Reply{Text: text, OK: true, FileID: doc.ID}
}

// imSearch 在博客目录按标题检索。
func (a *API) imSearch(ctx context.Context, query string) *im.Reply {
	query = strings.TrimSpace(query)
	if query == "" {
		return &im.Reply{Text: "要搜索什么关键词？例如：/search 分布式", OK: false}
	}
	rows, err := a.db.QueryContext(ctx,
		`SELECT name FROM files WHERE parent_id=? AND kind='file' AND deleted_at IS NULL AND name LIKE ? ORDER BY updated_at DESC LIMIT 10`,
		service.BlogDirID, "%"+query+"%")
	if err != nil {
		return &im.Reply{Text: "搜索失败：" + err.Error(), OK: false}
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, "• "+strings.TrimSuffix(n, ".md"))
		}
	}
	if len(names) == 0 {
		return &im.Reply{Text: fmt.Sprintf("没有找到含「%s」的文章。", query), OK: true}
	}
	return &im.Reply{Text: "搜索结果：\n" + strings.Join(names, "\n"), OK: true}
}

// imList 最近博客文章（博客目录顶层，按更新时间倒序）。
func (a *API) imList(ctx context.Context) *im.Reply {
	rows, err := a.db.QueryContext(ctx,
		`SELECT name FROM files WHERE parent_id=? AND kind='file' AND deleted_at IS NULL ORDER BY updated_at DESC LIMIT 10`,
		service.BlogDirID)
	if err != nil {
		return &im.Reply{Text: "列表失败：" + err.Error(), OK: false}
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, "• "+strings.TrimSuffix(n, ".md"))
		}
	}
	if len(names) == 0 {
		return &im.Reply{Text: "博客还没有文章，用 /post 正文 发布第一篇吧。", OK: true}
	}
	return &im.Reply{Text: "最近文章：\n" + strings.Join(names, "\n"), OK: true}
}

// imRetrieve 按标题取回文章链接（精确/模糊匹配首个）。
func (a *API) imRetrieve(ctx context.Context, r *http.Request, query string) *im.Reply {
	query = strings.TrimSpace(query)
	if query == "" {
		return &im.Reply{Text: "要查看哪篇？例如：给我 分布式", OK: false}
	}
	name := query
	if !strings.HasSuffix(name, ".md") {
		name = name + ".md"
	}
	var id, slug string
	err := a.db.QueryRowContext(ctx,
		`SELECT id FROM files WHERE parent_id=? AND kind='file' AND deleted_at IS NULL AND name=? LIMIT 1`,
		service.BlogDirID, name).Scan(&id)
	if err != nil {
		// 模糊匹配
		err = a.db.QueryRowContext(ctx,
			`SELECT id FROM files WHERE parent_id=? AND kind='file' AND deleted_at IS NULL AND name LIKE ? ORDER BY updated_at DESC LIMIT 1`,
			service.BlogDirID, "%"+query+"%").Scan(&id)
	}
	if err != nil {
		return &im.Reply{Text: fmt.Sprintf("没找到「%s」相关的文章。", query), OK: false}
	}
	slug, _ = service.EnsureFileSlug(ctx, a.db, id, name)
	return &im.Reply{Text: fmt.Sprintf("文章链接：%s/%s", a.publicBaseURL(r), slug), OK: true}
}

// imSummary 文章摘要（优先 AI；无 AI 配置时返回前 200 字）。
func (a *API) imSummary(ctx context.Context, query string) *im.Reply {
	query = strings.TrimSpace(query)
	if query == "" {
		return &im.Reply{Text: "要总结哪篇？例如：/summary 分布式", OK: false}
	}
	name := query
	if !strings.HasSuffix(name, ".md") {
		name = name + ".md"
	}
	var id string
	err := a.db.QueryRowContext(ctx,
		`SELECT id FROM files WHERE parent_id=? AND kind='file' AND deleted_at IS NULL AND name=? LIMIT 1`,
		service.BlogDirID, name).Scan(&id)
	if err != nil {
		err = a.db.QueryRowContext(ctx,
			`SELECT id FROM files WHERE parent_id=? AND kind='file' AND deleted_at IS NULL AND name LIKE ? ORDER BY updated_at DESC LIMIT 1`,
			service.BlogDirID, "%"+query+"%").Scan(&id)
	}
	if err != nil {
		return &im.Reply{Text: fmt.Sprintf("没找到「%s」相关的文章。", query), OK: false}
	}
	rc, _, err := a.files.Content(ctx, id)
	if err != nil {
		return &im.Reply{Text: "读取正文失败：" + err.Error(), OK: false}
	}
	defer rc.Close()
	buf := make([]byte, 8000)
	n, _ := io.ReadFull(rc, buf)
	content := string(buf[:n])
	if a.agent != nil {
		msgs := []ai.Msg{{Role: "user", Content: "请用 3 句话总结以下内容，不要使用 Markdown 格式：\n\n" + content}}
		res, err := a.agent.Chat(ctx, msgs)
		if err == nil && strings.TrimSpace(res.Content) != "" {
			return &im.Reply{Text: truncate(strings.TrimSpace(res.Content), imMaxRep), OK: true}
		}
	}
	return &im.Reply{Text: truncate(firstLine(content, 600), imMaxRep), OK: true}
}

// imStatus IM 配置 + 博客状态。
func (a *API) imStatus(ctx context.Context) *im.Reply {
	var total int
	if err := a.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM files WHERE parent_id=? AND kind='file' AND deleted_at IS NULL`, service.BlogDirID).Scan(&total); err != nil {
		total = 0
	}
	tg := a.cfg.GetString("im.telegram.bot_token") != ""
	model := ""
	if a.ai != nil {
		model = a.ai.CurrentModel("llm")
	}
	if model == "" {
		model = "未配置"
	}
	return &im.Reply{Text: fmt.Sprintf("AiKlog 状态\n\n📝 博客文章：%d 篇\n🤖 模型：%s\n💬 Telegram：%s\n\n发博客：/post 正文  存草稿：/draft 正文", total, model, onOff(tg)), OK: true}
}

// imAgent 开放请求 → Agent（复用 Web 端同一 Agent）。
func (a *API) imAgent(ctx context.Context, text string) *im.Reply {
	if a.agent == nil {
		return &im.Reply{Text: "AI 助手未就绪，请稍后再试。", OK: false}
	}
	msgs := []ai.Msg{{Role: "user", Content: text}}
	res, err := a.agent.Chat(ctx, msgs)
	if err != nil {
		return &im.Reply{Text: "AI 处理失败：" + err.Error(), OK: false}
	}
	reply := strings.TrimSpace(res.Content)
	if reply == "" {
		reply = "我没能理解你的请求，试试更明确的说法，或用 /post 发博客。"
	}
	return &im.Reply{Text: truncate(reply, imMaxRep), OK: true}
}

// imConfigStatus IM 配置状态：GET /api/v1/im/status。
func (a *API) imConfigStatus(w http.ResponseWriter, r *http.Request) {
	// M10 修复：IM 渠道凭据（bot token / 企微 secret）属站点级配置，仅 owner/admin 可读写
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"telegram": map[string]any{
			"enabled": a.cfg.GetString("im.telegram.bot_token") != "",
		},
		"wecom": map[string]any{
			"enabled":     a.wecomCfg().Enabled(),
			"has_webhook": a.cfg.GetString("im.wecom.webhook_url") != "",
			"has_app":     a.cfg.GetString("im.wecom.corp_id") != "" && a.cfg.GetString("im.wecom.secret") != "",
			"has_callback": a.cfg.GetString("im.wecom.token") != "" && a.cfg.GetString("im.wecom.encoding_aes_key") != "",
		},
	})
}

// imTelegramConfig 配置 Telegram Bot Token：PUT /api/v1/im/telegram/config {bot_token?, allowed_chats?}。
func (a *API) imTelegramConfig(w http.ResponseWriter, r *http.Request) {
	// M10 修复：IM 渠道凭据（bot token / 企微 secret）属站点级配置，仅 owner/admin 可读写
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	var req struct {
		BotToken     string `json:"bot_token"`
		AllowedChats string `json:"allowed_chats"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "IM_BAD_REQ", err.Error())
		return
	}
	if req.BotToken != "" {
		if _, err := a.cfg.Set(r.Context(), "im.telegram.bot_token", strings.TrimSpace(req.BotToken), "secret", "IM 配置", "admin"); err != nil {
			writeErr(w, http.StatusInternalServerError, "IM_CFG_FAILED", err.Error())
			return
		}
	}
	if req.AllowedChats != "" {
		if _, err := a.cfg.Set(r.Context(), "im.telegram.allowed_chats", strings.TrimSpace(req.AllowedChats), "string", "IM 配置", "admin"); err != nil {
			writeErr(w, http.StatusInternalServerError, "IM_CFG_FAILED", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "telegram_enabled": a.cfg.GetString("im.telegram.bot_token") != ""})
}

// imWebhookWeCom 企微回调：GET 验证 URL（echostr），POST 解密消息 → 统一分发。
func (a *API) imWebhookWeCom(w http.ResponseWriter, r *http.Request) {
	cfg := a.wecomCfg()
	if !cfg.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "handled": false, "reason": "im_disabled"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "read body"})
		return
	}
	// GET：URL 验证（echostr 解密原样返回明文）
	if r.Method == http.MethodGet {
		plain, err := wecomAdapter.VerifyURL(cfg, r.URL.Query())
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, plain)
		return
	}
	msg, err := wecomAdapter.Parse(cfg, body, r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if msg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "handled": false, "reason": "ignored"})
		return
	}
	ctx := r.Context()
	if msg.MsgID != "" {
		var dup int
		if err := a.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM im_ingest WHERE platform=? AND msg_id=?`, msg.Platform, msg.MsgID).Scan(&dup); err == nil && dup > 0 {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "handled": true, "reason": "duplicate"})
			return
		}
	}
	intent := imRouter.Route(msg.Text)
	actorID := a.imActor(ctx, msg)
	reply := a.imDispatch(ctx, r, msg, intent, actorID)
	if reply.OK && msg.MsgID != "" {
		fid := ""
		if f, ok := reply.FileID.(string); ok {
			fid = f
		}
		_, _ = a.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO im_ingest (id, platform, msg_id, intent, file_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			newID(), msg.Platform, msg.MsgID, string(intent.Type), fid, time.Now().Unix())
	}
	if reply.Text != "" {
		go func() {
			var serr error
			if msg.UserID != "" && cfg.AgentID != "" && cfg.Secret != "" {
				serr = wecomAdapter.ReplyAppMessage(cfg.CorpID, cfg.AgentID, cfg.Secret, msg.UserID, reply.Text)
			} else if cfg.WebhookURL != "" {
				serr = wecomAdapter.SendText(cfg.WebhookURL, reply.Text)
			}
			if serr != nil {
				_, _ = a.aud.Append(context.Background(), actorID, "im.send_failed", msg.ChatID, map[string]any{
					"platform": "wecom", "err": serr.Error(),
				})
			}
		}()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "handled": reply.OK})
}

// imWeComConfig 配置企微：PUT /api/v1/im/wecom/config。
func (a *API) imWeComConfig(w http.ResponseWriter, r *http.Request) {
	// M10 修复：IM 渠道凭据（bot token / 企微 secret）属站点级配置，仅 owner/admin 可读写
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	var req struct {
		WebhookURL     string `json:"webhook_url"`
		CorpID         string `json:"corp_id"`
		AgentID        string `json:"agent_id"`
		Secret         string `json:"secret"`
		Token          string `json:"token"`
		EncodingAESKey string `json:"encoding_aes_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "IM_BAD_REQ", err.Error())
		return
	}
	type kv struct{ k, v string }
	for _, x := range []kv{
		{"im.wecom.webhook_url", req.WebhookURL},
		{"im.wecom.corp_id", req.CorpID},
		{"im.wecom.agent_id", req.AgentID},
		{"im.wecom.secret", req.Secret},
		{"im.wecom.token", req.Token},
		{"im.wecom.encoding_aes_key", req.EncodingAESKey},
	} {
		if x.v != "" {
			if _, err := a.cfg.Set(r.Context(), x.k, strings.TrimSpace(x.v), "secret", "IM 配置", "admin"); err != nil {
				writeErr(w, http.StatusInternalServerError, "IM_CFG_FAILED", err.Error())
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "wecom_enabled": a.wecomCfg().Enabled()})
}

// ---------- 工具函数 ----------

// wecomCfg 读企微配置（settings 表，key 前缀 im.wecom.*）。
func (a *API) wecomCfg() im.WeComConfig {
	return im.WeComConfig{
		WebhookURL:     a.cfg.GetString("im.wecom.webhook_url"),
		CorpID:         a.cfg.GetString("im.wecom.corp_id"),
		AgentID:        a.cfg.GetString("im.wecom.agent_id"),
		Secret:         a.cfg.GetString("im.wecom.secret"),
		Token:          a.cfg.GetString("im.wecom.token"),
		EncodingAESKey: a.cfg.GetString("im.wecom.encoding_aes_key"),
	}
}

func firstLine(s string, max int) string {
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		s = "IM 碎片"
	}
	if len([]rune(s)) > max {
		rs := []rune(s)
		s = string(rs[:max])
	}
	return s
}

func sanitizeName(s string) string {
	repl := strings.NewReplacer(`\`, "", "/", "", ":", "", "*", "", "?", "", `"`, "", "<", "", ">", "", "|", "")
	return repl.Replace(s)
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	rs := []rune(s)
	return string(rs[:n]) + "…"
}

func onOff(b bool) string {
	if b {
		return "已开启"
	}
	return "未配置"
}

// imActor 解析本条 IM 消息对应的**站内身份**：
// 已绑定 → 绑定用户；未绑定 → 系统 owner（回退，保证既有单用户部署行为完全不变）。
func (a *API) imActor(ctx context.Context, msg *im.Msg) string {
	if a.imBinds != nil && msg != nil {
		if uid, ok := a.imBinds.ResolveUser(ctx, msg.Platform, msg.UserID); ok {
			a.imBinds.Touch(ctx, msg.Platform, msg.UserID)
			return uid
		}
	}
	return a.homeOwnerID()
}

// imBindCode 消费绑定码（B7）：IM 端 `/bind <码>`。
// 码由网页端「系统 → IM 绑定」生成，一次性、默认 10 分钟有效。
func (a *API) imBindCode(ctx context.Context, msg *im.Msg, raw string) *im.Reply {
	if a.imBinds == nil {
		return &im.Reply{Text: "IM 绑定功能未启用。", OK: false}
	}
	if msg == nil || msg.UserID == "" {
		return &im.Reply{Text: "没能识别你的 IM 身份，无法绑定。", OK: false}
	}
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return &im.Reply{
			Text: "用法：/bind 绑定码\n\n绑定码获取：登录网页端 →「系统 → IM 绑定」→ 生成（10 分钟内有效）。",
			OK:   false,
		}
	}
	uid, err := a.imBinds.ConsumeCode(ctx, fields[0], msg.Platform, msg.UserID, msg.UserName, msg.ChatID)
	if err != nil {
		return &im.Reply{Text: "绑定失败：" + err.Error(), OK: false}
	}
	_, _ = a.aud.Append(ctx, uid, "im.bind", "im_bindings", map[string]any{
		"platform": msg.Platform, "platform_user_id": msg.UserID,
	})
	return &im.Reply{
		Text: fmt.Sprintf("✅ 绑定成功，当前身份：%s\n之后用这个 IM 账号发文/查询，都会记在该账号名下。\n（发送 /unbind 可解除）", a.imUserName(ctx, uid)),
		OK:   true,
	}
}

// imUnbindIM 解除当前 IM 身份与本账号的绑定（B7）：IM 端 `/unbind`。
func (a *API) imUnbindIM(ctx context.Context, msg *im.Msg) *im.Reply {
	if a.imBinds == nil {
		return &im.Reply{Text: "IM 绑定功能未启用。", OK: false}
	}
	if msg == nil || msg.UserID == "" {
		return &im.Reply{Text: "没能识别你的 IM 身份。", OK: false}
	}
	if _, ok := a.imBinds.ResolveUser(ctx, msg.Platform, msg.UserID); !ok {
		return &im.Reply{Text: "当前 IM 身份尚未绑定任何站内账号。", OK: true}
	}
	uid, err := a.imBinds.UnbindIdentity(ctx, msg.Platform, msg.UserID)
	if err != nil {
		return &im.Reply{Text: "解绑失败：" + err.Error(), OK: false}
	}
	_, _ = a.aud.Append(ctx, uid, "im.unbind", "im_bindings", map[string]any{
		"platform": msg.Platform, "platform_user_id": msg.UserID,
	})
	return &im.Reply{Text: "✅ 已解除绑定。之后消息按系统默认身份处理。\n（重新绑定：网页端生成新绑定码后发 /bind <码>）", OK: true}
}

// imUserName 取用户展示名（display_name 优先，回落 username，兜底 id）。
func (a *API) imUserName(ctx context.Context, uid string) string {
	var dn, un string
	if err := a.db.QueryRowContext(ctx,
		`SELECT COALESCE(display_name,''), COALESCE(username,'') FROM users WHERE id=?`, uid).
		Scan(&dn, &un); err != nil {
		return uid
	}
	if strings.TrimSpace(dn) != "" {
		return dn
	}
	if un != "" {
		return un
	}
	return uid
}

// imHelpText IM 助手帮助。
func imHelpText() string {
	return "🤖 AiKlog IM 助手 —— 用聊天发博客\n\n" +
		"【发博客】/post 正文 → 立即发布到博客\n" +
		"【存草稿】/draft 正文 或 保存 内容 → 存为博客草稿\n" +
		"【搜索】/search 关键词\n" +
		"【最近】/list\n" +
		"【查看】给我 文章名 / /get 文章名\n" +
		"【总结】/summary 文章名\n" +
		"【状态】/status\n" +
		"【绑定】/bind 绑定码 → 把当前 IM 账号绑到站内账号\n" +
		"【解绑】/unbind\n" +
		"【语音】直接发语音给我 → 转写回传；家族传承开启时自动归档为人生事件\n" +
		"【帮助】/help\n\n" +
		"任何复杂请求直接说即可，我会调用 AI 助手完成。"
}
