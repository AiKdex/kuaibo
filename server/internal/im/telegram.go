// telegram.go Telegram 平台适配器：Webhook 解析 + 文件下载 + 回复发送。
//
// 移植自 Kmap services/im/telegram.py（FastAPI/httpx）→ Go net/http。
// 关键经验沿用：
//   - 发送用纯文本（不开 parse_mode），避免用户内容含 Markdown 特殊字符时
//     Telegram 返回 400 can't parse entities 而静默丢消息；
//   - 文件下载走 getFile → file_path 两步；
//   - 适配器不持有业务逻辑，只做 HTTP 出入。
package im

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const tgAPIBase = "https://api.telegram.org/bot%s"

// TgUpdate Telegram webhook 顶层结构（只解析需要的字段）。
type TgUpdate struct {
	Message *TgMessage `json:"message"`
}

type TgMessage struct {
	MessageID int        `json:"message_id"`
	From      *TgUser    `json:"from"`
	Chat      *TgChat    `json:"chat"`
	Text      string     `json:"text"`
	Voice     *TgMedia   `json:"voice"`
	Audio     *TgMedia   `json:"audio"`
	VideoNote *TgMedia   `json:"video_note"`
	Photo     []TgPhoto  `json:"photo"`
	Document  *TgDoc     `json:"document"`
	Entities  []TgEntity `json:"entities"`
	Caption   string     `json:"caption"`
}

type TgUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
}

type TgChat struct {
	ID int64 `json:"id"`
}

type TgMedia struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type"`
}

type TgPhoto struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
}

type TgDoc struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
}

type TgEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

// tgClient 共享 HTTP 客户端（含超时）。
var tgClient = &http.Client{Timeout: 90 * time.Second}

// TelegramAdapter 平台适配器（无状态，可并发）。
type TelegramAdapter struct{}

// Parse 解析 webhook body 为统一消息；无法识别返回 nil。
func (a *TelegramAdapter) Parse(ctx context.Context, body []byte) (*Msg, error) {
	var upd TgUpdate
	if err := json.Unmarshal(body, &upd); err != nil {
		return nil, fmt.Errorf("parse telegram update: %w", err)
	}
	msg := upd.Message
	if msg == nil {
		return nil, nil
	}
	// From/Chat 可能为 nil（频道 sender_chat、伪造报文）——先判空再取值，防 panic
	if msg.Chat == nil {
		return nil, nil
	}
	out := &Msg{
		Platform: "telegram",
		MsgID:    fmt.Sprintf("%d", msg.MessageID),
		ChatID:   fmt.Sprintf("%d", msg.Chat.ID),
		Caption:  msg.Caption,
	}
	if msg.From != nil {
		out.UserName = msg.From.FirstName
		out.UserID = fmt.Sprintf("%d", msg.From.ID)
	}
	// 语音/音频/视频
	if msg.Voice != nil || msg.Audio != nil || msg.VideoNote != nil {
		m := msg.Voice
		if m == nil {
			m = msg.Audio
		}
		if m == nil {
			m = msg.VideoNote
		}
		out.MsgType = "voice"
		out.FileURL = m.FileID
		if strings.Contains(m.MimeType, "ogg") {
			out.FileName = fmt.Sprintf("voice_%d.ogg", msg.MessageID)
		} else {
			out.FileName = fmt.Sprintf("voice_%d.wav", msg.MessageID)
		}
		return out, nil
	}
	// 图片
	if len(msg.Photo) > 0 {
		largest := msg.Photo[len(msg.Photo)-1]
		out.MsgType = "image"
		out.FileURL = largest.FileID
		out.FileName = fmt.Sprintf("photo_%s.jpg", largest.FileUniqueID)
		return out, nil
	}
	// 文档
	if msg.Document != nil {
		out.MsgType = "file"
		out.FileURL = msg.Document.FileID
		out.FileName = msg.Document.FileName
		if out.FileName == "" {
			out.FileName = "file"
		}
		return out, nil
	}
	// 文本 + 链接识别
	out.MsgType = "text"
	out.Text = msg.Text
	for _, e := range msg.Entities {
		if e.Type == "url" && msg.Text != "" && e.Offset+e.Length <= len(msg.Text) {
			out.LinkURL = msg.Text[e.Offset : e.Offset+e.Length]
			break
		}
	}
	return out, nil
}

// DownloadFile 通过 getFile 下载平台文件到 localPath。返回 localPath 或空串。
func (a *TelegramAdapter) DownloadFile(ctx context.Context, token, fileID, localPath string) (string, error) {
	base := fmt.Sprintf(tgAPIBase, token)
	// 1. getFile
	var gf struct {
		OK     bool `json:"ok"`
		Result *struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	req1, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/getFile?file_id="+urlQueryEscape(fileID), nil)
	if err != nil {
		return "", err
	}
	resp1, err := tgClient.Do(req1)
	if err != nil {
		return "", err
	}
	defer resp1.Body.Close()
	if err := json.NewDecoder(resp1.Body).Decode(&gf); err != nil || !gf.OK || gf.Result == nil {
		return "", fmt.Errorf("telegram getFile failed")
	}
	// 2. 下载
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", token, gf.Result.FilePath)
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return "", err
	}
	resp2, err := tgClient.Do(req2)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		return "", fmt.Errorf("telegram file download status=%d", resp2.StatusCode)
	}
	f, err := os.Create(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp2.Body); err != nil {
		return "", err
	}
	return localPath, nil
}

// Send 发送纯文本回复（禁用 parse_mode，防 Markdown 特殊字符 400）。
func (a *TelegramAdapter) Send(ctx context.Context, token, chatID, text string) error {
	body, _ := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf(tgAPIBase, token)+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := tgClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("telegram sendMessage status=%d body=%s", resp.StatusCode, string(b))
	}
	return nil
}

// urlQueryEscape 对 file_id 做查询参数转义（防御性处理，file_id 本身多为安全字符）。
func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}
