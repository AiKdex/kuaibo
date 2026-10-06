// ai_voice.go 语音能力管理端点（TTS 合成 / ASR 转写）。
// 走能力级路由（ai.tts / ai.asr，设置 → AI 模型与路由绑定 provider+model），
// 当前消费方：管理端自测/试听；产品级消费（IM 语音、博客朗读等）为预留位，接此链路即可。
package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

// maxVoiceUpload 语音转写上传上限（25MB，覆盖常见语音消息/短音频）。
const maxVoiceUpload = 25 << 20

// aiTTS POST /api/v1/admin/ai/tts —— 语音合成自测/试听（仅管理员）。
// body: {"text":"...","style":"风格指令(可选)","voice":"茉莉(可选)","format":"mp3|wav(可选)"}
// 成功：二进制音频流（Content-Type 按格式）；失败：JSON 错误。
func (a *API) aiTTS(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_ONLY", "仅管理员可操作")
		return
	}
	var req struct {
		Text   string `json:"text"`
		Style  string `json:"style"`
		Voice  string `json:"voice"`
		Format string `json:"format"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeErr(w, http.StatusBadRequest, "BAD_TEXT", "text 不能为空")
		return
	}
	if len(req.Text) > 4000 {
		writeErr(w, http.StatusBadRequest, "TEXT_TOO_LONG", "text 超长（上限 4000 字）")
		return
	}
	// format 空值不在此处定死：交给网关回退链（调用方实参 > ai.tts.format 配置 > 内置 mp3），
	// 否则 handler 的硬编码默认会把后台配置短路（family.8.1 回归修复）。
	audio, model, err := a.ai.TTS(r.Context(), req.Text, req.Style, req.Voice, req.Format)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "TTS_ERR", err.Error())
		return
	}
	// Content-Type 按实际生效格式：显式传参 > 后台默认配置 > mp3
	effFormat := req.Format
	if effFormat == "" {
		effFormat = a.cfg.GetString("ai.tts.format")
	}
	switch effFormat {
	case "wav":
		w.Header().Set("Content-Type", "audio/wav")
	case "pcm16":
		w.Header().Set("Content-Type", "application/octet-stream")
	default:
		w.Header().Set("Content-Type", "audio/mpeg")
	}
	w.Header().Set("X-Voice-Model", model)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}

// aiASR POST /api/v1/admin/ai/asr —— 语音识别自测（仅管理员）。
// multipart：file=音频文件（mp3/wav/m4a 等，≤25MB）。
// 成功：{"text":"...","model":"..."}
func (a *API) aiASR(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_ONLY", "仅管理员可操作")
		return
	}
	if err := r.ParseMultipartForm(maxVoiceUpload); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_FORM", "需要 multipart/form-data 且 file 字段为音频文件")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NO_FILE", "缺少 file 字段")
		return
	}
	defer f.Close()
	audio, err := io.ReadAll(io.LimitReader(f, maxVoiceUpload+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "READ_FAILED", err.Error())
		return
	}
	if len(audio) == 0 {
		writeErr(w, http.StatusBadRequest, "EMPTY_FILE", "音频文件为空")
		return
	}
	if len(audio) > maxVoiceUpload {
		writeErr(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "音频超过 25MB 上限")
		return
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(hdr.Filename)), ".")
	switch format {
	case "mpeg", "mpga":
		format = "mp3"
	case "json", "":
		format = "mp3" // 无扩展名时按 mp3 试探（MiMo 支持主流格式自动识别）
	}
	text, model, err := a.ai.ASR(r.Context(), audio, format)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "ASR_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text, "model": model})
}
