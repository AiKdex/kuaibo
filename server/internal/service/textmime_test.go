// textmime_test.go 回归：文本类判定必须是「结构化白名单」，不得用子串匹配。
//
// 背景：历史实现用 strings.Contains 做子串匹配，其中 Contains(mime,"xml") 会命中
// application/vnd.openxmlformats-officedocument.*（"openxmlformats" 里含 "xml"），
// 于是 .docx / .xlsx / .pptx 被判为「可在线编辑的文本」——
// 读出来是二进制乱码，再经 UpdateContent 写回就把文件彻底损坏。
package service

import (
	"strings"
	"testing"
)

func TestIsTextMimeWhitelist(t *testing.T) {
	yes := []string{
		"text/plain",
		"text/markdown",
		"text/csv",
		"text/html",
		"text/plain; charset=utf-8",
		"application/json",
		"application/json; charset=utf-8",
		"APPLICATION/JSON", // 大小写不敏感
		"application/xml",
		"application/x-yaml",
		"application/toml",
		"application/javascript",
		"application/typescript",
		"application/sql",
		"application/x-shellscript",
		"application/rtf",
		"application/xhtml+xml", // RFC 6839 +xml 后缀
		"application/ld+json",   // RFC 6839 +json 后缀
	}
	for _, m := range yes {
		if !isTextMime(m) {
			t.Errorf("isTextMime(%q) = false，期望 true", m)
		}
	}

	no := []string{
		"",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",   // .docx
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",        // .xlsx
		"application/vnd.openxmlformats-officedocument.presentationml.presentation", // .pptx
		"application/vnd.oasis.opendocument.text",                                   // .odt
		"application/msword",                                                        // .doc（二进制）
		"application/pdf",
		"image/png",
		"image/jpeg",
		"video/mp4",
		"audio/mpeg",
		"application/zip",
		"application/gzip",
		"application/octet-stream",
		"font/woff2",
		"application/wasm",
	}
	for _, m := range no {
		if isTextMime(m) {
			t.Errorf("isTextMime(%q) = true，期望 false（二进制/容器格式不得当文本编辑）", m)
		}
	}
}

// TestIsTextMimeDocxRegression 固化最典型的回归点：.docx 的 mime 串里含 "xml" 字样。
func TestIsTextMimeDocxRegression(t *testing.T) {
	const docx = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	if !strings.Contains(docx, "xml") {
		t.Fatal("前提失效：docx 的 mime 已不含 xml 字样，本回归用例可回收")
	}
	if isTextMime(docx) {
		t.Fatal(".docx 被判为文本 → UpdateContent 会以文本覆盖写，损坏文件")
	}
}
