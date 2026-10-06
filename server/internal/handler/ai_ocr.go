// ai_ocr.go OCR 识别模块：图片/截图/扫描件文字提取。
// 组件化设计——知识库入库、IM 图片链路、阅读器均可复用本 service。
package handler

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// aiOCR POST /api/v1/ai/ocr：识别图片中的文字。
// body 两种形态：
//   - {"file_id":"<文件id>"}             —— 从文件库读图
//   - {"image_base64":"...","mime":"image/png"} —— 直接传图（base64）
//
// 返回：{"ok":true,"text":"...","chars":N,"cleaned":bool,"clean_reason":"...","clean_error":"..."}
// text 已按 ocr.auto_clean / ocr.clean_on_ai 设置做过后处理（结构结果自动整理为干净 Markdown）。
func (a *API) aiOCR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileID      string `json:"file_id"`
		ImageBase64 string `json:"image_base64"`
		Mime        string `json:"mime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}

	data := req.ImageBase64
	mime := req.Mime
	if req.FileID != "" {
		rc, f, err := a.files.Content(r.Context(), req.FileID)
		if err != nil || rc == nil {
			writeErr(w, http.StatusBadRequest, "FILE_NOT_FOUND", "文件不存在或不是文本/图片")
			return
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "READ_FAILED", err.Error())
			return
		}
		data = base64.StdEncoding.EncodeToString(b)
		if mime == "" && f != nil && f.Mime != "" {
			mime = f.Mime
		}
		if mime == "" {
			mime = "image/png"
		}
	}
	if data == "" {
		writeErr(w, http.StatusBadRequest, "NO_IMAGE", "缺少图片（file_id 或 image_base64）")
		return
	}

	text, err := a.ai.OCR(r.Context(), data, mime)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "OCR_FAILED", err.Error())
		return
	}
	text = stripCodeFence(text)

	// 后处理：结构结果（HTML 占比高）自动 AI 整理为干净 Markdown；纯文本原样；失败回退保留原文
	out, rep := a.postProcessOCR(r.Context(), text)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"text":         out,
		"chars":        len([]rune(out)),
		"cleaned":      rep.Cleaned,
		"clean_reason": rep.Reason,
		"clean_error":  rep.Error,
	})
}

// stripCodeFence 去除模型输出整体的 Markdown 代码围栏（```lang ... ```）。
// OCR 对网页/界面截图常输出 ```html 包裹，入库前去掉围栏保留原文（保真、不耗 token）。
func stripCodeFence(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "```") {
		return s
	}
	if i := strings.IndexByte(t, '\n'); i >= 0 {
		t = t[i+1:]
	}
	t = strings.TrimSpace(t)
	if strings.HasSuffix(t, "```") {
		t = strings.TrimRight(t, "`")
		t = strings.TrimSpace(t)
	}
	return t
}
