// blog_voice_read.go 读者侧博客文章语音朗读端点。
// 复用 family.8 / 8.1 已配置的 ai.tts 能力级路由（voice/format/style 在设置页可调），
// 面向公开已发布文章：GET /api/v1/public/blog/tts?slug=...[&voice=...&format=...]
// 命中磁盘缓存（按 slug + 音色 + 格式 + 正文内容哈希）即返回，避免重复合成计费；
// 正文变更自动失效缓存。锁定态（密码/付费）不合成正文（与 SSR/分享轨闸门同源）。
package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	blogTTSCacheSubdir = "tts_cache"
	blogTTSMaxChars    = 4000 // 与 admin TTS 上限一致（MiMo 单次合成安全区）
)

// publicBlogTTS GET /api/v1/public/blog/tts —— 文章语音朗读（公开、仅已发布、走缓存）。
func (a *API) publicBlogTTS(w http.ResponseWriter, r *http.Request) {
	if !a.blogOpen(r) {
		writeErr(w, http.StatusNotFound, "BLOG_CLOSED", "博客已关闭")
		return
	}
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	if slug == "" {
		writeErr(w, http.StatusBadRequest, "TTS_NO_SLUG", "slug 必填")
		return
	}
	// 仅公开轨可见（已发布、内容隔离按站点）：复用与 SSR 同源的公开文章收集，
	// 不会读到草稿/附件/未授权内容（公开列表之外的内容不得合成）。
	posts, err := a.blogPostsHTML(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, "TTS_POST_NOT_FOUND", "文章不存在")
		return
	}
	var hit *dirFile
	for i := range posts {
		if slugOf(posts[i].f, posts[i].path) == slug {
			hit = &posts[i]
			break
		}
	}
	if hit == nil {
		writeErr(w, http.StatusNotFound, "TTS_POST_NOT_FOUND", "文章不存在")
		return
	}
	// 锁定态（密码/付费）不合成正文（与 SSR/分享轨闸门同源，防止绕过）
	if a.articleLocked(r, hit.f.ID) || a.paidLocked(r, hit.f.ID) {
		writeErr(w, http.StatusForbidden, "TTS_LOCKED", "该文章已加密或付费，暂不支持朗读")
		return
	}
	// 读正文
	rc, _, cerr := a.files.Content(r.Context(), hit.f.ID)
	if cerr != nil {
		writeErr(w, http.StatusNotFound, "TTS_READ_FAILED", "读取正文失败")
		return
	}
	defer rc.Close()
	raw, _ := io.ReadAll(io.LimitReader(rc, 2<<20))
	text := markdownToPlainText(string(raw))
	text = strings.TrimSpace(text)
	if text == "" {
		writeErr(w, http.StatusBadRequest, "TTS_EMPTY", "正文无可朗读文本")
		return
	}
	truncated := false
	if n := len([]rune(text)); n > blogTTSMaxChars {
		text = string([]rune(text)[:blogTTSMaxChars])
		truncated = true
	}
	// 音色/格式：调用方实参 > 后台配置 > 内置默认（与 admin TTS 同源回退链）
	voice := strings.TrimSpace(r.URL.Query().Get("voice"))
	format := strings.TrimSpace(r.URL.Query().Get("format"))
	if voice == "" {
		voice = a.cfg.GetString("ai.tts.voice")
	}
	if format == "" {
		format = a.cfg.GetString("ai.tts.format")
	}
	if format == "" {
		format = "mp3"
	}
	// 缓存键：slug + 音色 + 格式 + 正文内容哈希（正文变更自动失效）
	contentHash := sha256.Sum256(raw)
	cacheKey := fmt.Sprintf("%s|%s|%s|%x", slug, voice, format, contentHash)
	sum := sha256.Sum256([]byte(cacheKey))
	cacheFile := filepath.Join(a.ttsCacheDir(), hex.EncodeToString(sum[:])+"."+cacheExt(format))
	if data, ok := readFileIfExist(cacheFile); ok {
		serveAudio(w, format, data, truncated)
		return
	}
	// 公开端点：长文合成可达分钟级（实测 4000 字≈5.9MB/134s），采用分块流式合成——
	// 每块小请求（≤400 字）独立合成、边合成边推送给客户端并落盘，既避免单次大请求触发
	// 网关/代理超时，又让客户端 2 秒内听到首段、播放连续（每块音频交付快于播放）。
	// 整体宽预算（30min）覆盖长文，命中磁盘缓存后二次访问瞬时返回。
	overallCtx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	w.Header().Set("Content-Type", ttsContentType(format))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if truncated {
		w.Header().Set("X-TTS-Truncated", "1")
	}
	flusher, _ := w.(http.Flusher)
	var buf bytes.Buffer
	// 延迟提交响应头：只有真的拿到第一块**非空**音频字节才 WriteHeader(200)。
	// 否则 TTS 未配置/上游不可达（运维高发态）时，TTSStream 会在任何字节之前返回错误，
	// 若此时已提交 200，客户端只会拿到「200 + 空 body」，前端 <audio> 报「无源可播」，
	// 拿不到真正的 TTS_ERR。推迟提交即可在零字节时回一个干净的 502。
	started := false
	wrote := 0
	terr := a.ai.TTSStream(overallCtx, text, "", voice, format, func(chunk []byte) error {
		if len(chunk) == 0 {
			return nil // 空块不算成功，避免把 200 头提交出去
		}
		if !started {
			w.WriteHeader(http.StatusOK)
			started = true
		}
		n, err := w.Write(chunk)
		wrote += n
		buf.Write(chunk)
		if flusher != nil {
			flusher.Flush()
		}
		return err
	})
	// 合成「成功」但一段音频都没拿到（如上游返回空音频）：等同失败，
	// 否则会回 200 空 body 并把 0 字节文件写进缓存，污染后续命中。
	if terr == nil && wrote == 0 {
		terr = errors.New("语音合成未返回音频数据")
	}
	if terr != nil {
		if !started {
			// 尚未推送任何字节 → 状态码仍可改写，给出可诊断的错误。
			writeErr(w, http.StatusBadGateway, "TTS_ERR", "语音合成失败："+terr.Error())
		}
		// 已开始推送则状态码无法改写；不完整合成不落盘缓存。
		return
	}
	if started {
		if dir := a.ttsCacheDir(); dir != "" {
			_ = os.MkdirAll(dir, 0o755)
			_ = os.WriteFile(cacheFile, buf.Bytes(), 0o644)
		}
	}
}

// ttsCacheDir 朗读音频缓存目录：随 storage.local.root 同基（默认 ./data/tts_cache）。
func (a *API) ttsCacheDir() string {
	root := a.cfg.GetString("storage.local.root")
	if root == "" {
		root = "./data/files"
	}
	return filepath.Join(filepath.Dir(root), blogTTSCacheSubdir)
}

// ttsContentType 朗读音频的 Content-Type（流式与缓存命中两路共用）。
func ttsContentType(format string) string {
	switch format {
	case "wav":
		return "audio/wav"
	case "pcm16":
		return "application/octet-stream"
	default:
		return "audio/mpeg"
	}
}

// serveAudio 写出音频流并设置可缓存头。
func serveAudio(w http.ResponseWriter, format string, data []byte, truncated bool) {
	switch format {
	case "wav":
		w.Header().Set("Content-Type", "audio/wav")
	case "pcm16":
		w.Header().Set("Content-Type", "application/octet-stream")
	default:
		w.Header().Set("Content-Type", "audio/mpeg")
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if truncated {
		w.Header().Set("X-TTS-Truncated", "1")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// readFileIfExist 读取已存在的缓存文件，不存在返回 (nil,false)。
func readFileIfExist(p string) ([]byte, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	return b, true
}

// cacheExt 缓存文件扩展名。
func cacheExt(format string) string {
	switch format {
	case "wav":
		return "wav"
	case "pcm16":
		return "pcm"
	default:
		return "mp3"
	}
}

// markdownToPlainText 将博客正文（markdown）抽取为可朗读纯文本：
// 去 frontmatter / 代码块 / HTML / 图片；链接保留文本；去强调/标题/列表/引用标记；折叠空白。
var (
	reTTSFence      = regexp.MustCompile("```[\\s\\S]*?```")
	reTTSFrontMatter = regexp.MustCompile("(?s)^---[\\s\\S]*?---\\s*")
	reTTSHTML       = regexp.MustCompile("<[^>]+>")
	reTTSImg        = regexp.MustCompile("!\\[[^\\]]*\\]\\([^)]*\\)")
	reTTSLink       = regexp.MustCompile("\\[([^\\]]*)\\]\\([^)]*\\)")
	reTTSBold       = regexp.MustCompile("\\*\\*([^*]+)\\*\\*")
	reTTSEmph       = regexp.MustCompile("__([^_]+)__")
	reTTSItalic     = regexp.MustCompile("(?m)\\*([^*\\n]+)\\*")
	reTTSHeading    = regexp.MustCompile("(?m)^#{1,6}\\s*")
	reTTSListMark   = regexp.MustCompile("(?m)^\\s*[-*+]\\s+")
	reTTSOrdered    = regexp.MustCompile("(?m)^\\s*\\d+[.)]\\s+")
	reTTSQuote      = regexp.MustCompile("(?m)^>\\s?")
	reTTSRule       = regexp.MustCompile("(?m)^((\\*{3,})|(-{3,})|(_{3,}))\\s*$")
	reTTSWhitespace = regexp.MustCompile("[ \\t]+")
	reTTSBlankLines = regexp.MustCompile("\\n{3,}")
)

func markdownToPlainText(md string) string {
	md = reTTSFrontMatter.ReplaceAllString(md, "")
	md = reTTSFence.ReplaceAllString(md, " ")
	md = reTTSHTML.ReplaceAllString(md, " ")
	md = reTTSImg.ReplaceAllString(md, " ")
	md = reTTSLink.ReplaceAllString(md, "$1")
	md = reTTSBold.ReplaceAllString(md, "$1")
	md = reTTSEmph.ReplaceAllString(md, "$1")
	md = reTTSItalic.ReplaceAllString(md, "$1")
	md = reTTSHeading.ReplaceAllString(md, "")
	md = reTTSListMark.ReplaceAllString(md, "")
	md = reTTSOrdered.ReplaceAllString(md, "")
	md = reTTSQuote.ReplaceAllString(md, "")
	md = reTTSRule.ReplaceAllString(md, " ")
	md = reTTSWhitespace.ReplaceAllString(md, " ")
	md = reTTSBlankLines.ReplaceAllString(md, "\n\n")
	return strings.TrimSpace(md)
}
