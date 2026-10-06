// voice.go 语音能力（TTS/ASR）：MiMo chat-audio 契约实现。
// 小米 MiMo（token-plan / 按量）的语音不走 /v1/audio/*，而是 chat/completions：
//   TTS：目标文本放 role=assistant 消息（风格指令放可选 user 消息），audio={format,voice}，
//        响应 choices[0].message.audio.data 为 base64 音频；
//   ASR：user 消息 content 为 [{type:input_audio,input_audio:{data,format}}]，响应 content 即转写文本。
// 路由沿用能力级 pick("ai.tts"/"ai.asr")：provider/模型/密钥池/记账与其它能力同模型。
package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// TTS 语音合成：text=待合成文本，style=风格指令（可空），voice=音色（如 茉莉/冰糖/Mia），
// format=mp3|wav|pcm16。返回音频字节与实际模型名。
// 空参回退链：调用方实参 > 后台配置（ai.tts.voice / ai.tts.style / ai.tts.format，设置页可改）> 内置默认。
func (g *Gateway) TTS(ctx context.Context, text, style, voice, format string) ([]byte, string, error) {
	p, err := g.pick("ai.tts")
	if err != nil {
		return nil, "", err
	}
	if p == nil {
		return nil, "", fmt.Errorf("tts 未配置：请先在设置 → AI 模型与路由绑定语音合成 provider")
	}
	if voice == "" {
		voice = g.cfg.GetString("ai.tts.voice")
	}
	if style == "" {
		style = g.cfg.GetString("ai.tts.style")
	}
	if format == "" {
		format = g.cfg.GetString("ai.tts.format")
	}
	audio, err := p.speak(ctx, text, style, voice, format)
	return audio, p.model, err
}

// TTSStream 分块合成并流式回调每个文本块的音频。长文朗读时按 blogTTSChunkRunes 切分为
// 多个小请求，每块独立调用 provider（各自 blogTTSChunkTimeout 超时、失败重试一次），
// 避免单次超大合成触发网关/代理超时——长文 mimo 合成+返回可达分钟级（实测 4000 字≈5.9MB/134s）。
// onChunk 收到每块的音频字节；调用方通常边合成边写给客户端并落盘缓存。返回首个失败块的错误。
func (g *Gateway) TTSStream(ctx context.Context, text, style, voice, format string, onChunk func([]byte) error) error {
	if onChunk == nil {
		return fmt.Errorf("tts: onChunk 回调为空")
	}
	if voice == "" {
		voice = g.cfg.GetString("ai.tts.voice")
	}
	if style == "" {
		style = g.cfg.GetString("ai.tts.style")
	}
	if format == "" {
		format = g.cfg.GetString("ai.tts.format")
	}
	p, err := g.pick("ai.tts")
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("tts 未配置：请先在设置 → AI 模型与路由绑定语音合成 provider")
	}
	chunks := splitTTSChunks(text, blogTTSChunkRunes)
	for i, ch := range chunks {
		var audio []byte
		var e error
		for attempt := 0; attempt < 2; attempt++ {
			chunkCtx, cancel := context.WithTimeout(ctx, blogTTSChunkTimeout)
			audio, e = p.speak(chunkCtx, ch, style, voice, format)
			cancel()
			if e == nil {
				break
			}
		}
		if e != nil {
			return fmt.Errorf("tts 第 %d/%d 块合成失败: %w", i+1, len(chunks), e)
		}
		if err := onChunk(audio); err != nil {
			return err
		}
	}
	return nil
}

const (
	blogTTSChunkRunes   = 400 // 每块约 400 字：合成快、响应小（几百 KB），远离超时
	blogTTSChunkTimeout = 120 * time.Second
)

// splitTTSChunks 将长文本按句末标点/换行优先断句，切成 ≤ maxRunes 的块，
// 避免把词语斩断；超长无标点段落按 maxRunes 硬切。返回非空块切片。
func splitTTSChunks(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	chunks := make([]string, 0, len(runes)/maxRunes+1)
	cur := make([]rune, 0, maxRunes)
	flush := func() {
		if len(cur) > 0 {
			chunks = append(chunks, strings.TrimSpace(string(cur)))
			cur = cur[:0]
		}
	}
	for _, r := range runes {
		cur = append(cur, r)
		if len(cur) >= maxRunes {
			flush()
			continue
		}
		if (r == '。' || r == '！' || r == '？' || r == '；' || r == '\n' || r == '，' || r == '、') && len(cur) >= maxRunes/2 {
			flush()
		}
	}
	flush()
	if len(chunks) == 0 {
		return []string{text}
	}
	return chunks
}

// ASR 语音识别：audioBytes=音频字节，format=mp3|wav|…。返回转写文本与实际模型名。
func (g *Gateway) ASR(ctx context.Context, audioBytes []byte, format string) (string, string, error) {
	p, err := g.pick("ai.asr")
	if err != nil {
		return "", "", err
	}
	if p == nil {
		return "", "", fmt.Errorf("asr 未配置：请先在设置 → AI 模型与路由绑定语音识别 provider")
	}
	text, err := p.transcribe(ctx, audioBytes, format)
	return text, p.model, err
}

// speak MiMo chat-audio 语音合成。
func (o *OpenAICompat) speak(ctx context.Context, text, style, voice, format string) ([]byte, error) {
	start := time.Now()
	if format == "" {
		format = "mp3"
	}
	msgs := []map[string]any{}
	if style != "" {
		msgs = append(msgs, map[string]any{"role": "user", "content": style})
	}
	msgs = append(msgs, map[string]any{"role": "assistant", "content": text})
	payload := map[string]any{
		"model":    o.model,
		"messages": msgs,
		"audio":    map[string]any{"format": format, "voice": voice},
		"stream":   false,
	}
	body, _ := json.Marshal(payload)
	req, err := o.newReq(ctx, o.v1Base()+"/chat/completions", body)
	if err != nil {
		return nil, err
	}
	resp, err := o.httpc.Do(req)
	if err != nil {
		o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		err = fmt.Errorf("tts http %d: %s", resp.StatusCode, b)
		o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		return nil, err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Audio *struct {
					Data string `json:"data"`
				} `json:"audio"`
			} `json:"message"`
		} `json:"choices"`
		Usage *Usage `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		return nil, err
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Audio == nil || out.Choices[0].Message.Audio.Data == "" {
		err = fmt.Errorf("tts: 响应不含音频（检查模型是否为语音合成型号，如 mimo-v2.5-tts）")
		o.gate.recordUsage(ctx, o.cap, o.model, out.Usage, start, err)
		return nil, err
	}
	audio, err := base64.StdEncoding.DecodeString(out.Choices[0].Message.Audio.Data)
	if err != nil {
		o.gate.recordUsage(ctx, o.cap, o.model, out.Usage, start, err)
		return nil, err
	}
	o.gate.recordUsage(ctx, o.cap, o.model, out.Usage, start, nil)
	return audio, nil
}

// transcribe MiMo chat-audio 语音识别。
func (o *OpenAICompat) transcribe(ctx context.Context, audioBytes []byte, format string) (string, error) {
	start := time.Now()
	if format == "" {
		format = "mp3"
	}
	payload := map[string]any{
		"model": o.model,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type":        "input_audio",
				"input_audio": map[string]any{"data": base64.StdEncoding.EncodeToString(audioBytes), "format": format},
			}},
		}},
		"stream": false,
	}
	body, _ := json.Marshal(payload)
	req, err := o.newReq(ctx, o.v1Base()+"/chat/completions", body)
	if err != nil {
		return "", err
	}
	resp, err := o.httpc.Do(req)
	if err != nil {
		o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		err = fmt.Errorf("asr http %d: %s", resp.StatusCode, b)
		o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *Usage `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		return "", err
	}
	if len(out.Choices) == 0 {
		err = fmt.Errorf("asr: empty choices")
		o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		return "", err
	}
	o.gate.recordUsage(ctx, o.cap, o.model, out.Usage, start, nil)
	return out.Choices[0].Message.Content, nil
}
