// im_voice.go IM 语音消息处理（family.8.2，按上游 d819c12 同名能力最小移植）。
// 链路：Telegram 语音消息 → 下载音频 → 网关 ASR（ai.asr 能力路由，MiMo chat-audio 契约）→
// 把转写文本主动回传到对话；family.enabled 时同时把转写归档为人生事件
// （life_event 文档，content_state 打标 node_type=life_event，进「一生时间轴」）。
// 同步段只受理并回「正在转写」，下载/识别/归档全程异步（webhook 立即返回，
// Telegram 超时会重推消息，靠 im_ingest msg_id 幂等键去重）。
// 企业微信适配器暂不解析语音消息（wecom.Parse 只出 text/event），后续按需扩展。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/im"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// imVoiceMaxDownload IM 语音下载上限（Telegram 语音消息 ≤20MB；贴 25MB ASR 上限）。
const imVoiceMaxDownload = 25 << 20

// imVoiceIncoming 语音消息受理（同步段）：立即回执 + 派发异步转写。
func (a *API) imVoiceIncoming(msg *im.Msg, actorID string) *im.Reply {
	if msg.FileURL == "" {
		return &im.Reply{Text: "语音消息缺少文件 ID，无法转写", OK: false}
	}
	if a.ai == nil || a.ai.ActiveProvider("asr") == "" {
		return &im.Reply{Text: "语音识别未配置：请在设置 → AI 模型为「语音识别」绑定 provider（如 mimo）", OK: true}
	}
	owner := strings.TrimSpace(actorID)
	go a.imVoiceProcess(owner, msg.Platform, msg.ChatID, msg.FileURL, msg.FileName)
	return &im.Reply{Text: "🎤 收到语音，正在转写，请稍候…", OK: true}
}

// imVoiceProcess 异步：下载音频 → ASR → 回传转写 →（可选）归档人生事件。
func (a *API) imVoiceProcess(actorID, platform, chatID, fileID, fileName string) {
	bctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	send := func(text string) { a.imVoiceReply(bctx, platform, chatID, text) }

	format := "mp3"
	switch {
	case strings.HasSuffix(strings.ToLower(fileName), ".ogg"), strings.HasSuffix(strings.ToLower(fileName), ".oga"):
		format = "ogg"
	case strings.HasSuffix(strings.ToLower(fileName), ".wav"):
		format = "wav"
	}

	// 下载到临时文件（Telegram 语音一般 <1MB，上限对齐 ASR 25MB）
	tmp, err := os.CreateTemp("", "im_voice_*."+format)
	if err != nil {
		send("语音处理失败：" + err.Error())
		return
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	token := a.cfg.GetString("im.telegram.bot_token")
	if platform == "telegram" && token == "" {
		send("语音处理失败：Telegram bot token 未配置")
		return
	}
	if _, err := tgAdapter.DownloadFile(bctx, token, fileID, tmpPath); err != nil {
		send("语音下载失败：" + err.Error())
		return
	}
	audio, err := os.ReadFile(tmpPath)
	if err != nil {
		send("语音读取失败：" + err.Error())
		return
	}
	if len(audio) == 0 {
		send("语音文件为空")
		return
	}

	text, _, err := a.ai.ASR(bctx, audio, format)
	if err != nil {
		send("语音识别失败：" + err.Error())
		return
	}
	msgText := "【语音转写】\n" + text
	// 家族传承（上游二期② 同名能力）：family.enabled 时把转写归档为人生事件
	if a.family != nil && a.family.Enabled() {
		if docID, err := a.imFamilyVoiceArchive(bctx, actorID, text); err == nil && docID != "" {
			if base := a.imPublicBaseURL(); base != "" {
				msgText += "\n\n已归档到家族传承时间轴：\n" + base + "/#/read/" + docID
			} else {
				msgText += "\n\n已归档到家族传承时间轴（文档 " + docID + "）"
			}
		}
	}
	send(msgText)
}

// imVoiceReply 转写结果回传（当前仅 Telegram 有语音入口；wecom 语音待适配器扩展）。
func (a *API) imVoiceReply(ctx context.Context, platform, chatID, text string) {
	switch platform {
	case "telegram":
		if tok := a.cfg.GetString("im.telegram.bot_token"); tok != "" {
			if err := tgAdapter.Send(ctx, tok, chatID, text); err != nil {
				_, _ = a.aud.Append(context.Background(), a.homeOwnerID(), "im.send_failed", chatID, map[string]any{
					"platform": "telegram", "err": err.Error(),
				})
			}
		}
	default:
		// 无出站通道的平台：仅审计留痕
		_, _ = a.aud.Append(context.Background(), a.homeOwnerID(), "im.voice_reply_dropped", chatID, map[string]any{
			"platform": platform, "len": len(text),
		})
	}
}

// imFamilyVoiceArchive 把 IM 语音转写文本归档为家族人生事件（life_event 文档）。
// 归属：语音消息用户（已绑定站内账号）的 home space；正文含转写全文，
// content_state 打标 node_type=life_event + fields{occurred_at,stage,preview,source:im_voice}，
// 家族「一生时间轴」按 content_state 聚合（service/family.go Timeline），目录位置不限。
func (a *API) imFamilyVoiceArchive(ctx context.Context, actorID, transcript string) (string, error) {
	owner := strings.TrimSpace(actorID)
	if owner == "" {
		owner = a.homeOwnerID()
	}
	space := a.homeSpaceID()
	now := time.Now()
	title := firstLine(transcript, 30)
	if title == "" {
		title = "语音人生记录"
	}
	content := fmt.Sprintf("# %s\n\n%s\n\n---\n> 语音记录 · %s（IM 语音转写）", title, transcript, now.Format("2006-01-02 15:04"))
	f, err := a.files.CreateDoc(ctx, owner, space, "", sanitizeName(title)+".md", content, service.DefaultSiteID)
	if err != nil {
		// 重名：加月日-时分秒后缀重试一次
		f, err = a.files.CreateDoc(ctx, owner, space, "", sanitizeName(title)+"-"+now.Format("0102-1504")+".md", content, service.DefaultSiteID)
		if err != nil {
			return "", err
		}
	}
	// content_state 合并打标（保留既有键）
	var cur string
	_ = a.db.QueryRowContext(ctx, `SELECT COALESCE(content_state,'{}') FROM files WHERE id=?`, f.ID).Scan(&cur)
	var m map[string]any
	if json.Unmarshal([]byte(cur), &m) != nil || m == nil {
		m = map[string]any{}
	}
	m["node_type"] = "life_event"
	m["fields"] = map[string]any{
		"occurred_at": now.UnixMilli(),
		"stage":       "other",
		"preview":     truncate(transcript, 60),
		"source":      "im_voice",
	}
	b, _ := json.Marshal(m)
	// files.*_at 与 life_event.fields.occurred_at 单位铁律 = 毫秒（service/file.go:now()，
	// 前端 FamilyView.fmtDate 亦按 ms 解析）。此处曾用 now.Unix()（秒），导致
	// updated_at 单位不一致（读取端按毫秒解析 → 1970 / 排序错乱）且时间轴日期显示 1970。
	if _, err := a.db.ExecContext(ctx,
		`UPDATE files SET content_state=?, updated_at=? WHERE id=?`, string(b), now.UnixMilli(), f.ID); err != nil {
		return "", err
	}
	return f.ID, nil
}

// imPublicBaseURL 站点对外地址（blog.base_url，两实例已配置）；未配置返回空串（回执降级为文档 ID）。
func (a *API) imPublicBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(a.cfg.GetString("blog.base_url")), "/")
}
