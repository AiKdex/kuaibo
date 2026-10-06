// Package handler 的 ai.go 实现 AI 对话端点：POST /api/v1/ai/chat。
// 请求带消息列表与可选浏览上下文；Agent 循环执行工具调用后返回最终回答 + 工具记录。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/google/uuid"
)

type chatReq struct {
	Messages       []ai.Msg `json:"messages"`
	Context        *chatCtx `json:"context,omitempty"`
	ConversationID string   `json:"conversation_id,omitempty"`
}

// chatCtx 前端传入的浏览上下文（当前阅读文件/所在目录），注入 system 供 Agent 理解。
type chatCtx struct {
	FileID string `json:"file_id,omitempty"`
	DirID  string `json:"dir_id,omitempty"`
}

type chatResp struct {
	Reply          string            `json:"reply"`
	Tools          []ai.ToolCallInfo `json:"tools,omitempty"`
	Model          string            `json:"model,omitempty"`
	Degraded       bool              `json:"degraded,omitempty"`
	Error          string            `json:"error,omitempty"`
	ConversationID string            `json:"conversation_id"`
}

// aiChat 处理对话请求。单用户阶段：owner/space 固定为配置默认值。
// 多话题模式：conversation_id 续接历史会话；缺省则自动新建会话并落库。
func (a *API) aiChat(w http.ResponseWriter, r *http.Request) {
	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "AI_BAD_REQ", err.Error())
		return
	}
	if len(req.Messages) == 0 {
		writeErr(w, http.StatusBadRequest, "AI_NO_MESSAGES", "messages required")
		return
	}
	// 末条必须是用户消息（防注入侧构造）
	last := req.Messages[len(req.Messages)-1]
	if last.Role != "user" {
		writeErr(w, http.StatusBadRequest, "AI_LAST_NOT_USER", "last message must be user")
		return
	}
	ctx := r.Context()
	owner := a.homeOwnerID()

	// ---- 会话定位：续接 or 新建 ----
	convID := req.ConversationID
	if convID != "" {
		var o string
		if err := a.db.QueryRowContext(ctx, `SELECT owner_id FROM ai_conversations WHERE id=?`, convID).Scan(&o); err != nil {
			writeErr(w, http.StatusNotFound, "CONV_NOT_FOUND", "conversation not found")
			return
		}
		if o != owner {
			writeErr(w, http.StatusForbidden, "CONV_FORBIDDEN", "not your conversation")
			return
		}
	} else {
		id := uuid.NewString()
		title := shortTitle(last.Content)
		now := time.Now().Unix()
		if _, err := a.db.ExecContext(ctx,
			`INSERT INTO ai_conversations (id, owner_id, title, created_at, updated_at) VALUES (?,?,?,?,?)`,
			id, owner, title, now, now); err != nil {
			writeErr(w, http.StatusInternalServerError, "CONV_CREATE_FAILED", err.Error())
			return
		}
		convID = id
	}

	// ---- 载历史（user/assistant 文本）+ 拼接当前消息 ----
	hist, err := a.loadConvMessages(ctx, convID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CONV_MSGS_FAILED", err.Error())
		return
	}
	msgs := append(hist, req.Messages...)
	msgs = injectContext(ctx, a, msgs, req.Context)
	res, err := a.agent.Chat(ctx, msgs)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "AI_CHAT_FAILED", err.Error())
		return
	}

	// ---- 落库：用户消息 + 助手回复 ----
	now := time.Now().Unix()
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO ai_messages (id, conversation_id, role, content, created_at) VALUES (?,?,?,?,?)`,
		uuid.NewString(), convID, "user", last.Content, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "AI_SAVE_FAILED", err.Error())
		return
	}
	if res.Content != "" {
		if _, err := a.db.ExecContext(ctx,
			`INSERT INTO ai_messages (id, conversation_id, role, content, created_at) VALUES (?,?,?,?,?)`,
			uuid.NewString(), convID, "assistant", res.Content, now+1); err != nil {
			writeErr(w, http.StatusInternalServerError, "AI_SAVE_FAILED", err.Error())
			return
		}
	}
	// 标题未定（新对话）时用首条用户消息
	if _, err := a.db.ExecContext(ctx,
		`UPDATE ai_conversations SET updated_at=? WHERE id=?`, now, convID); err != nil {
		writeErr(w, http.StatusInternalServerError, "AI_SAVE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, chatResp{
		Reply:          res.Content,
		Tools:          res.Tools,
		Error:          res.Error,
		ConversationID: convID,
	})
}

// loadConvMessages 载入会话历史消息（无记录返回空切片）。
func (a *API) loadConvMessages(ctx context.Context, convID string) ([]ai.Msg, error) {
	rows, err := a.db.QueryContext(ctx,
		`SELECT role, content FROM ai_messages WHERE conversation_id=? ORDER BY created_at`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ai.Msg{}
	for rows.Next() {
		var m ai.Msg
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// shortTitle 由首条用户消息生成会话标题（截断 20 字符，去换行）。
func shortTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "新对话"
	}
	runes := []rune(s)
	if len(runes) > 20 {
		runes = runes[:20]
	}
	return strings.ReplaceAll(string(runes), "\n", " ")
}

// injectContext 组装 system 注入层（插在消息序列之前），顺序：角色提示词 → 站点身份 → 浏览上下文。
// 角色提示词须在此显式前置：agent.Chat 见到任何 system 消息即跳过自身默认提示词（覆盖语义），
// 原实现"有浏览上下文才注入 system"会连带丢掉角色设定——此处统一兜住。
// 站点身份始终注入：AI 助手须知道自己挂在哪个站点，才能直接回答「介绍一下这个站点」类问题，
// 而不是反问用户"您指的是哪个站点"。
func injectContext(ctx context.Context, a *API, msgs []ai.Msg, c *chatCtx) []ai.Msg {
	head := []ai.Msg{{Role: "system", Content: ai.SystemPrompt}}
	var site []string
	if v := strings.TrimSpace(a.cfg.GetString("blog.title")); v != "" {
		site = append(site, "站点名称："+v)
	}
	if v := strings.TrimSpace(a.cfg.GetString("blog.description")); v != "" {
		site = append(site, "站点简介："+v)
	}
	if v := strings.TrimSpace(a.cfg.GetString("blog.base_url")); v != "" {
		site = append(site, "站点地址："+v)
	}
	if len(site) > 0 {
		head = append(head, ai.Msg{Role: "system", Content:
			"你所在的站点信息（回答「这是什么网站 / 介绍一下这个站点」时据此作答，不要反问用户）：" + strings.Join(site, "；")})
	}
	if c == nil || (c.FileID == "" && c.DirID == "") {
		return append(head, msgs...)
	}
	var parts []string
	if c.FileID != "" {
		if f, err := a.files.Get(ctx, c.FileID); err == nil && f != nil {
			parts = append(parts, fmt.Sprintf("当前正在查看的文件：%s（类型 %s，%d 字节）", f.Name, f.Mime, f.Size))
		}
	}
	if c.DirID != "" {
		if d, err := a.files.Get(ctx, c.DirID); err == nil && d != nil {
			parts = append(parts, fmt.Sprintf("当前所在的目录：%s", d.Name))
		}
	}
	if len(parts) > 0 {
		head = append(head, ai.Msg{Role: "system", Content: "用户当前的操作上下文：" + strings.Join(parts, "；")})
	}
	return append(head, msgs...)
}
