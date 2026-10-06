// ocr_postprocess.go OCR 结果后处理管线（任务包 03 / 需求 K06 增强）。
//
// 背景：图片/扫描件 OCR 结果常夹带大段 HTML 结构（网页截图、前端源码），直接入库可读性与检索都差。
// 策略：识别结果分两类处理——
//   - 纯文本结果：直接入库（不调用 AI，省 token）；
//   - 结构结果（HTML 标签占比 > 30%）：自动复用 AI（providers 对话能力）整理为干净 Markdown 后入库；
//   - 失败回退：AI 整理失败时保留原始识别文本（绝不丢内容），并记录错误以便追溯。
//
// 开关（settings 表，config.Defaults 内置默认）：
//   - ocr.auto_clean  （默认 true）  自动后处理总开关；
//   - ocr.clean_on_ai （默认 true）  结构结果是否调用 AI 整理（关闭则省 token、原样入库）。
package handler

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// ocrHTMLTagRe 匹配 HTML 标签（含闭合/自闭合/带属性）。
var ocrHTMLTagRe = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9-]*(?:\s[^<>]*)?/?>`)

// ocrCleanThreshold HTML 标签占比阈值：超过则判为「结构结果」。
const ocrCleanThreshold = 0.30

// htmlTagRatio 计算 HTML 标签字符占全文字符的比例（0~1）。空文本返回 0。
func htmlTagRatio(s string) float64 {
	total := len([]rune(s))
	if total == 0 {
		return 0
	}
	tagChars := 0
	for _, loc := range ocrHTMLTagRe.FindAllStringIndex(s, -1) {
		tagChars += len([]rune(s[loc[0]:loc[1]]))
	}
	return float64(tagChars) / float64(total)
}

// OCRPostReport 后处理报告（回传前端，便于提示「已 AI 整理 / 已保留原文」）。
type OCRPostReport struct {
	Cleaned bool    `json:"cleaned"`         // 是否经 AI 整理
	Reason  string  `json:"reason"`          // disabled|empty|plain|clean_off|ai|ai_failed
	Ratio   float64 `json:"html_ratio"`      // HTML 标签占比
	Error   string  `json:"error,omitempty"` // AI 整理失败原因（cleaned=false 时）
}

var errEmptyClean = errors.New("ai clean returned empty")

// postProcessOCR 对 OCR 原始文本执行后处理，返回入库文本与报告。
func (a *API) postProcessOCR(ctx context.Context, text string) (string, OCRPostReport) {
	rep := OCRPostReport{Ratio: htmlTagRatio(text)}

	if strings.TrimSpace(text) == "" {
		rep.Reason = "empty"
		return text, rep
	}
	if !a.cfg.GetBool("ocr.auto_clean") {
		rep.Reason = "disabled"
		return text, rep
	}
	if rep.Ratio <= ocrCleanThreshold {
		rep.Reason = "plain"
		return text, rep
	}
	if !a.cfg.GetBool("ocr.clean_on_ai") {
		rep.Reason = "clean_off"
		return text, rep
	}

	cleaned, err := a.ai.CleanText(ctx, text, "ocr")
	if err != nil {
		rep.Reason = "ai_failed"
		rep.Error = err.Error()
		a.recordOCRCleanError(ctx, err)
		return text, rep // 回退：保留原始识别文本，不丢内容
	}
	cleaned = stripCodeFence(cleaned)
	if strings.TrimSpace(cleaned) == "" {
		rep.Reason = "ai_failed"
		rep.Error = errEmptyClean.Error()
		a.recordOCRCleanError(ctx, errEmptyClean)
		return text, rep
	}

	rep.Cleaned = true
	rep.Reason = "ai"
	return cleaned, rep
}

// recordOCRCleanError 记录 AI 整理失败。
//
// 说明：OCR 发生在「文件入库之前」，此刻尚无 file_id，无法直接写 file_ai_summaries.last_error；
// 统一写入审计日志（action=ai.ocr_clean_failed）以便追溯。文件入库后其 AI 解读（摘要）失败
// 由 Summarizer 独立记录到 file_ai_summaries.last_error（模块 18），两条链路互不影响。
func (a *API) recordOCRCleanError(ctx context.Context, err error) {
	if a.aud == nil || err == nil {
		return
	}
	_, _ = a.aud.Append(ctx, "", "ai.ocr_clean_failed", "ocr",
		map[string]any{"error": err.Error()})
}
