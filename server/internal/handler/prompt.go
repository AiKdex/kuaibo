package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// ---- B5 写作增强：AI 写作提示词模板 端点 ----
//
// 端点：GET /prompts、POST /prompts、GET|PUT|DELETE /prompts/{id}、POST /prompts/{id}/render
// 判权统一交给 service.PromptActor（作者本人 或 管理员），handler 只做 I/O 与错误码映射。

// promptActor 组装操作者。
func (a *API) promptActor(r *http.Request) service.PromptActor {
	return service.PromptActor{UID: a.curUserID(r), IsAdmin: a.isAdmin(r)}
}

// promptsReady 模块门控（nil = 未装配，返回 404 而非 panic）。
func (a *API) promptsReady(w http.ResponseWriter) bool {
	if a.prompts == nil {
		writeErr(w, http.StatusNotFound, "PROMPTS_DISABLED", "提示词模板模块未启用")
		return false
	}
	return true
}

// promptBody 请求体。variables 同时接受「JSON 数组」与「JSON 字符串」两种形态：
// 前端表单直传数组最自然，而 curl/脚本传字符串更省事，两种都收。
type promptBody struct {
	Name      string          `json:"name"`
	Category  string          `json:"category"`
	Content   string          `json:"content"`
	Variables json.RawMessage `json:"variables"`
	IsPublic  *bool           `json:"is_public"` // 省略时默认 true（模板默认共享）
}

// varsRaw 把 variables 归一为「JSON 文本」，交给 service 校验。
func (b promptBody) varsRaw() string {
	s := strings.TrimSpace(string(b.Variables))
	if s == "" || s == "null" {
		return ""
	}
	if strings.HasPrefix(s, `"`) {
		var str string
		if json.Unmarshal(b.Variables, &str) == nil {
			return str
		}
	}
	return s
}

// toInput 转 service 入参（is_public 缺省 true）。
func (b promptBody) toInput() service.PromptInput {
	pub := true
	if b.IsPublic != nil {
		pub = *b.IsPublic
	}
	return service.PromptInput{
		Name:      b.Name,
		Category:  b.Category,
		Content:   b.Content,
		Variables: b.varsRaw(),
		IsPublic:  pub,
	}
}

// writePromptErr 统一错误映射（避免每个端点重复 if 链）。
func writePromptErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrPromptInvalid):
		writeErr(w, http.StatusBadRequest, "PROMPT_INVALID", err.Error())
	case errors.Is(err, service.ErrPromptNotFound):
		writeErr(w, http.StatusNotFound, "PROMPT_NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrPromptForbidden):
		writeErr(w, http.StatusForbidden, "PROMPT_FORBIDDEN", err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "PROMPT_FAILED", err.Error())
	}
}

// promptsList GET /api/v1/prompts?category=
func (a *API) promptsList(w http.ResponseWriter, r *http.Request) {
	if !a.promptsReady(w) {
		return
	}
	list, err := a.prompts.List(r.Context(), a.promptActor(r), r.URL.Query().Get("category"))
	if err != nil {
		writePromptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": list})
}

// promptsCreate POST /api/v1/prompts
func (a *API) promptsCreate(w http.ResponseWriter, r *http.Request) {
	if !a.promptsReady(w) {
		return
	}
	var body promptBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	t, err := a.prompts.Create(r.Context(), a.promptActor(r), body.toInput())
	if err != nil {
		writePromptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// promptsGet GET /api/v1/prompts/{id}
func (a *API) promptsGet(w http.ResponseWriter, r *http.Request) {
	if !a.promptsReady(w) {
		return
	}
	t, err := a.prompts.Get(r.Context(), a.promptActor(r), r.PathValue("id"))
	if err != nil {
		writePromptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// promptsUpdate PUT /api/v1/prompts/{id}
func (a *API) promptsUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.promptsReady(w) {
		return
	}
	var body promptBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	t, err := a.prompts.Update(r.Context(), a.promptActor(r), r.PathValue("id"), body.toInput())
	if err != nil {
		writePromptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// promptsDelete DELETE /api/v1/prompts/{id}
func (a *API) promptsDelete(w http.ResponseWriter, r *http.Request) {
	if !a.promptsReady(w) {
		return
	}
	if err := a.prompts.Delete(r.Context(), a.promptActor(r), r.PathValue("id")); err != nil {
		writePromptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// promptsRender POST /api/v1/prompts/{id}/render
// body: {"values":{"标题":"…"}} → 返回填充后的文本 + 未解析/未声明的变量清单。
func (a *API) promptsRender(w http.ResponseWriter, r *http.Request) {
	if !a.promptsReady(w) {
		return
	}
	var body struct {
		Values map[string]string `json:"values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	res, err := a.prompts.Render(r.Context(), a.promptActor(r), r.PathValue("id"), body.Values)
	if err != nil {
		writePromptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
