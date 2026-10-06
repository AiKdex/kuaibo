// ai_clean.go 文本整理模块：OCR 结果/剪藏/导入文本 → 干净结构化 Markdown。
// 组件化设计——OCR 弹窗、知识库入库、剪藏链路均可复用。
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
)

// aiClean POST /api/v1/ai/clean：把原始文本（如 OCR 输出的 HTML 噪声）整理为干净 Markdown。
// body: {"text":"...","purpose":"ocr"}  purpose 可选：ocr|raw
func (a *API) aiClean(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text    string `json:"text"`
		Purpose string `json:"purpose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	if req.Text == "" {
		writeErr(w, http.StatusBadRequest, "EMPTY_TEXT", "text 不能为空")
		return
	}
	out, err := a.ai.Ask(r.Context(), cleanMsgs(req.Text, req.Purpose))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "CLEAN_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "text": out, "chars": len([]rune(out)),
	})
}

// cleanMsgs 构造整理 prompt（忠实原文、只去噪声）。
func cleanMsgs(text, purpose string) []ai.Msg {
	inst := "将以下 OCR 识别结果整理为干净、结构化的 Markdown 文档。要求：\n" +
		"1. 去掉 HTML 标签、源码标记、无意义的代码噪声，只保留有实际意义的文字内容；\n" +
		"2. 按语义分段，合理使用 #/##/### 标题层级；\n" +
		"3. 表格用 Markdown 表格呈现；\n" +
		"4. 不添加原文没有的信息，不编造、不补全；原文不明确的用【□】占位；\n" +
		"5. 只输出整理后的 Markdown，不要输出任何解释、评论或前后缀。\n\n原始文本：\n"
	if purpose == "raw" {
		inst = "将以下文本整理为干净、结构化的 Markdown 文档。要求：\n" +
			"1. 保留原文全部信息，去掉明显噪声；\n" +
			"2. 合理分段与标题层级；\n" +
			"3. 不添加原文没有的信息；\n" +
			"4. 只输出整理后的 Markdown。\n\n原始文本：\n"
	}
	return []ai.Msg{{Role: "user", Content: inst + text}}
}
