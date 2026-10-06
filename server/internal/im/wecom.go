// wecom.go 企业微信（WeCom）平台适配器：回调加解密 + 验签 + 发送。
//
// 移植自 Kmap services/im/wecom.py → Go（crypto/aes + crypto/sha1 标准库）。
// 回调协议（官方）：
//   - GET  ?msg_signature&timestamp&nonce&echostr  验证 URL，解密 echostr 原样返回明文；
//   - POST ?msg_signature&timestamp&nonce          加密 XML 回调，解密后取文本/事件消息。
// 发送：
//   - 群机器人 webhook（im.wecom.webhook_url）——通知/推送；
//   - 应用消息（im.wecom.corp_id/agent_id/secret）——回复用户。
package im

import (
	"bytes"
	"encoding/json"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const wecomAPIBase = "https://qyapi.weixin.qq.com/cgi-bin"

// WeComConfig 企微适配器配置（settings 表，key 前缀 im.wecom.*）。
type WeComConfig struct {
	WebhookURL    string // 群机器人 webhook（发送用）
	CorpID        string // 企业 ID
	AgentID       string // 应用 AgentId
	Secret        string // 应用 Secret
	Token         string // 回调 Token
	EncodingAESKey string // 回调 EncodingAESKey（43 位）
}

func (c WeComConfig) Enabled() bool {
	return c.WebhookURL != "" || c.CorpID != ""
}

// WeComCrypt 企微回调加解密（AES-256-CBC + PKCS7 + base64；官方算法复刻）。
type WeComCrypt struct {
	token   []byte
	aesKey  []byte
	corpID  string
}

func mustDecodeAESKey(encodingAESKey string) []byte {
	key, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return nil
	}
	return key
}

func NewWeComCrypt(token, encodingAESKey, corpID string) *WeComCrypt {
	return &WeComCrypt{
		token:  []byte(token),
		aesKey: mustDecodeAESKey(encodingAESKey),
		corpID: corpID,
	}
}

func pkcs7Pad(data []byte) []byte {
	pad := 32 - (len(data) % 32)
	return append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	pad := int(data[len(data)-1])
	if pad < 1 || pad > 32 || pad > len(data) {
		return nil, fmt.Errorf("bad padding")
	}
	return data[:len(data)-pad], nil
}

// Encrypt 加密明文（随机 16 字节 + 4 字节长度 + 明文 + corpid，PKCS7 填充）。
func (c *WeComCrypt) Encrypt(plain string) (string, error) {
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(plain)))
	raw := append(randBytes, lenBuf[:]...)
	raw = append(raw, plain...)
	raw = append(raw, c.corpID...)
	raw = pkcs7Pad(raw)
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return "", err
	}
	enc := make([]byte, len(raw))
	cipher.NewCBCEncrypter(block, c.aesKey[:16]).CryptBlocks(enc, raw)
	return base64.StdEncoding.EncodeToString(enc), nil
}

// Decrypt 解密回调密文。
func (c *WeComCrypt) Decrypt(encrypt string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encrypt)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return "", err
	}
	raw := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, c.aesKey[:16]).CryptBlocks(raw, data)
	raw, err = pkcs7Unpad(raw)
	if err != nil || len(raw) < 20 {
		return "", fmt.Errorf("decrypt: bad payload")
	}
	msgLen := binary.BigEndian.Uint32(raw[16:20])
	if int(msgLen) > len(raw)-20 {
		return "", fmt.Errorf("decrypt: length overflow")
	}
	return string(raw[20 : 20+msgLen]), nil
}

// Signature 回调签名（token/timestamp/nonce/encrypt/echostr 字典序 sha1）。
func (c *WeComCrypt) Signature(timestamp, nonce, encrypt, echostr string) string {
	items := []string{string(c.token), timestamp, nonce, encrypt}
	if echostr != "" {
		items = append(items, echostr)
	}
	sort.Strings(items)
	h := sha1.Sum([]byte(strings.Join(items, "")))
	return hex.EncodeToString(h[:])
}

// wecomCallbackXML 企微回调 XML 结构。
type wecomCallbackXML struct {
	XMLName      xml.Name `xml:"xml"`
	Encrypt      string   `xml:"Encrypt"`
	MsgType      string   `xml:"MsgType"`
	FromUserName string   `xml:"FromUserName"`
	Content      string   `xml:"Content"`
	MsgID        string   `xml:"MsgId"`
	Event        string   `xml:"Event"`
	EventKey     string   `xml:"EventKey"`
}

// WeComAdapter 企微平台适配器（无状态，可并发）。
type WeComAdapter struct{}

// VerifyURL GET 回调验证：验签并解密 echostr，返回明文供企微校验。
func (a *WeComAdapter) VerifyURL(cfg WeComConfig, q url.Values) (string, error) {
	if cfg.Token == "" || cfg.EncodingAESKey == "" || cfg.CorpID == "" {
		return "", fmt.Errorf("wecom: not configured")
	}
	crypt := NewWeComCrypt(cfg.Token, cfg.EncodingAESKey, cfg.CorpID)
	ts, nonce, echostr := q.Get("timestamp"), q.Get("nonce"), q.Get("echostr")
	if crypt.Signature(ts, nonce, echostr, echostr) != q.Get("msg_signature") {
		return "", fmt.Errorf("wecom: signature mismatch")
	}
	return crypt.Decrypt(echostr)
}

// Parse POST 回调：验签 + 解密 XML → 统一 Msg。事件/非文本返回 nil。
func (a *WeComAdapter) Parse(cfg WeComConfig, body []byte, q url.Values) (*Msg, error) {
	if cfg.Token == "" || cfg.EncodingAESKey == "" || cfg.CorpID == "" {
		return nil, fmt.Errorf("wecom: not configured")
	}
	crypt := NewWeComCrypt(cfg.Token, cfg.EncodingAESKey, cfg.CorpID)
	ts, nonce := q.Get("timestamp"), q.Get("nonce")
	var outer wecomCallbackXML
	if err := xml.Unmarshal(body, &outer); err != nil {
		return nil, fmt.Errorf("wecom: parse xml: %w", err)
	}
	if outer.Encrypt == "" {
		return nil, fmt.Errorf("wecom: no encrypt field")
	}
	if crypt.Signature(ts, nonce, outer.Encrypt, "") != q.Get("msg_signature") {
		return nil, fmt.Errorf("wecom: signature mismatch")
	}
	plain, err := crypt.Decrypt(outer.Encrypt)
	if err != nil {
		return nil, fmt.Errorf("wecom: decrypt: %w", err)
	}
	var inner wecomCallbackXML
	if err := xml.Unmarshal([]byte(plain), &inner); err != nil {
		return nil, fmt.Errorf("wecom: parse inner xml: %w", err)
	}
	if inner.MsgType == "event" {
		if inner.Event == "subscribe" {
			return &Msg{Platform: "wecom", MsgID: inner.MsgID, MsgType: "text", UserID: inner.FromUserName, Text: "/start"}, nil
		}
		return nil, nil
	}
	if inner.MsgType == "text" && strings.TrimSpace(inner.Content) != "" {
		return &Msg{
			Platform: "wecom",
			MsgID:    inner.MsgID,
			MsgType:  "text",
			UserID:   inner.FromUserName,
			Text:     strings.TrimSpace(inner.Content),
		}, nil
	}
	return nil, nil
}

// SendText 群机器人 webhook 发送（通知/推送）。
func (a *WeComAdapter) SendText(webhookURL, text string) error {
	if webhookURL == "" {
		return fmt.Errorf("wecom: webhook_url not configured")
	}
	payload := map[string]any{"msgtype": "text", "text": map[string]any{"content": truncateBytes(text, 4000)}}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	cli := &http.Client{Timeout: 10 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wecom: webhook http %d", resp.StatusCode)
	}
	var d struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(b, &d); err == nil && d.ErrCode != 0 {
		return fmt.Errorf("wecom: errcode %d %s", d.ErrCode, d.ErrMsg)
	}
	return nil
}

// wecomTokenCache 应用 access_token 进程内缓存。
var wecomTokenCache struct {
	token string
	exp   int64
}

// AccessToken 应用 access_token（corp_id + secret，带进程内缓存）。
func (a *WeComAdapter) AccessToken(corpID, secret string) (string, error) {
	if corpID == "" || secret == "" {
		return "", fmt.Errorf("wecom: corp_id/secret not configured")
	}
	if wecomTokenCache.token != "" && time.Now().Unix() < wecomTokenCache.exp-60 {
		return wecomTokenCache.token, nil
	}
	u := fmt.Sprintf("%s/gettoken?corpid=%s&corpsecret=%s", wecomAPIBase, url.QueryEscape(corpID), url.QueryEscape(secret))
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	var d struct {
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn  int    `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return "", fmt.Errorf("wecom: gettoken parse: %w", err)
	}
	if d.ErrCode != 0 {
		return "", fmt.Errorf("wecom: gettoken errcode %d %s", d.ErrCode, d.ErrMsg)
	}
	exp := int64(d.ExpiresIn)
	if exp <= 0 {
		exp = 7200
	}
	wecomTokenCache = struct {
		token string
		exp   int64
	}{token: d.AccessToken, exp: time.Now().Unix() + exp}
	return d.AccessToken, nil
}

// ReplyAppMessage 应用消息回复（touser + agent_id + secret）。
func (a *WeComAdapter) ReplyAppMessage(corpID, agentID, secret, userID, text string) error {
	at, err := a.AccessToken(corpID, secret)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"touser":  userID,
		"msgtype": "text",
		"agentid": agentID,
		"text":    map[string]any{"content": truncateBytes(text, 4000)},
	}
	body, _ := json.Marshal(payload)
	u := fmt.Sprintf("%s/message/send?access_token=%s", wecomAPIBase, url.QueryEscape(at))
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Post(u, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	var d struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(b, &d); err == nil && d.ErrCode != 0 {
		return fmt.Errorf("wecom: app message errcode %d %s", d.ErrCode, d.ErrMsg)
	}
	return nil
}

func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
