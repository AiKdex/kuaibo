// Package ai 实现 AI 网关（架构 §8.1 / 实施文档 §5.3）。
//
// 设计要点：
//   - 三个能力：Embed / Summarize / Ask（OpenAI 兼容为基线协议）；
//   - 模型可插拔：provider 可为 local（本地 llama.cpp/Ollama）或 hosted（云端/用户自带 key）；
//   - 降级链：AI 不可用时检索仍可用，摘要/问答降级为提示模式（不阻塞主流程）。
package ai

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/config"
	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
)

// Provider 是 AI 能力提供方接口。
type Provider interface {
	Name() string
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Summarize(ctx context.Context, text string) (string, error)
	Ask(ctx context.Context, messages []Msg) (string, error)
}

// Msg 对话消息。
type Msg struct {
	Role       string     `json:"role"` // system|user|assistant|tool
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // role=tool 时回填
	Name       string     `json:"name,omitempty"`         // role=tool 时工具名
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // role=assistant 时回填工具调用声明（协议要求）
	// 多模态（OCR/视觉）：非空时 content 序列化为 [{text},{image_url}...] 数组
	ImageURLs []string `json:"-"`
	ImageText string   `json:"-"`
}

// MarshalJSON 支持多模态消息：带图时 content 输出为 OpenAI 视觉格式数组。
func (m Msg) MarshalJSON() ([]byte, error) {
	if len(m.ImageURLs) == 0 {
		type plain Msg
		return json.Marshal(plain(m))
	}
	parts := []map[string]any{{"type": "text", "text": m.ImageText}}
	for _, u := range m.ImageURLs {
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": u}})
	}
	return json.Marshal(map[string]any{"role": m.Role, "content": parts})
}

// ToolCall 模型请求的工具调用。
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// MarshalJSON 按 OpenAI 线格式输出 tool_calls 元素：
// {"id":..,"type":"function","function":{"name":..,"arguments":..}}。
// 缺 type/function 包裹会被严格反序列化的上游（如 agnes/vLLM）以
// "missing field `type`" 400 拒绝，导致 agent 工具循环第二轮必挂。
func (t ToolCall) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":   t.ID,
		"type": "function",
		"function": map[string]any{
			"name":      t.Name,
			"arguments": string(t.Arguments),
		},
	})
}

// ChatResult 带工具调用的对话结果。
type ChatResult struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Error     string     `json:"error,omitempty"`
	Usage     *Usage     `json:"-"`
}

// ProviderDef 一个模型提供方的注册定义（来自 secrets/providers.json）。
// 每个能力（llm/embedding/asr/tts/image/rerank/ocr/code）可独立指定模型；
// ModelCands 提供能力级模型候选（如对话可在 qwen3.8-flash / glm-5.2 间切换）；
// Keys 用于 token 池轮替。
type ProviderDef struct {
	Endpoint   string              `json:"endpoint"`
	Model      string              `json:"model"` // 默认模型（cap 未单独指定时用）
	Models     map[string]string   `json:"models,omitempty"`
	ModelCands map[string][]string `json:"model_cands,omitempty"`
	Keys       []string            `json:"keys"`
	Caps       []string            `json:"caps"`
	// VoiceCands 语音合成音色候选（providers.json 按需声明；设置页下拉展示，
	// 不声明=不限制、设置页自由填写——音色语义由 provider 自定，内核不做枚举硬编码）。
	VoiceCands []string `json:"voice_cands,omitempty"`
	Note       string   `json:"note,omitempty"`
	// Builtin 标记来自 providers.json 的内置提供方（禁止删除；用户可在 UI 编辑但保留标记）。
	// 该字段不进入 ProviderDef 的 JSON 反序列化（name 是 map key），仅由代码/持久化层赋值。
	Builtin bool `json:"-"`
}

// Gateway 按配置路由到 provider；失败按 fallback 顺序降级。
type Gateway struct {
	cfg   *config.Store
	b     *bus.Bus
	httpc *http.Client
	pool  *TokenPool // 兼容旧逻辑（main 注入的单能力池）
	defs  map[string]*ProviderDef
	pools map[string]*TokenPool // key: providerName|cap（懒创建）
	mu    sync.Mutex            // 保护 pools 懒创建（HTTP/摘要/向量/工具多路径并发调用 pick）
	// activeDefaults: providers.json 声明的能力默认 provider（清除自定义时回退）
	activeDefaults map[string]string
	// recorder: 用量记账（A1 LLM 可观测，B27 自上游 9ddd6e7 移植）。nil=不记账；
	// pick 时透传给 OpenAICompat 统一落账。
	recorder UsageRecorder
	// meter: 用量管控（配额+增值 token 计费，meter.go）。nil=不管控。
	meter *Meter
}

// SetMeter 注入用量管控器（main 启动时传 db 即开；传 nil 关闭）。
func (g *Gateway) SetMeter(m *Meter) { g.meter = m }

// Meter 返回管控器（nil=未启用）；handler 断言做配额/余额查询。
func (g *Gateway) Meter() *Meter { return g.meter }

// SetUsageRecorder 注入用量记账器（main 启动时传 db 即开；传 nil 关闭）。
func (g *Gateway) SetUsageRecorder(r UsageRecorder) { g.recorder = r }

// UsageRecorder 返回当前记账器（nil=未注入）；handler 断言 *UsageDB 做聚合查询。
func (g *Gateway) UsageRecorder() UsageRecorder { return g.recorder }

// recordUsage 统一记账入口（OpenAICompat 内部调用）：
//  1. llm_usage 明细落账（可观测，异步）；
//  2. meter 日聚合 +（计费开启且平台模型时）按 token 扣余额（管控，meter.go）。
func (g *Gateway) recordUsage(ctx context.Context, cap, model string, u *Usage, start time.Time, err error) {
	if g.recorder == nil && g.meter == nil {
		return
	}
	rec := UsageRecord{
		Provider:  "ai." + cap,
		Cap:       cap,
		Model:     model,
		LatencyMs: time.Since(start).Milliseconds(),
		OK:        err == nil,
	}
	if err != nil {
		rec.Err = err.Error()
	}
	if u != nil {
		rec.PromptTokens = u.PromptTokens
		rec.CompletionTokens = u.CompletionTokens
	}
	if g.recorder != nil {
		g.recorder.Record(context.Background(), rec)
	}
	if g.meter != nil {
		billing := g.IsPlatform(cap) && g.QuotaPolicy().BillingEnabled
		g.meter.charge(SubjectFrom(ctx), rec.PromptTokens+rec.CompletionTokens, billing)
	}
}

// SetActiveDefaults 注入 providers.json 的能力级默认 provider（main 加载时调用）。
func (g *Gateway) SetActiveDefaults(m map[string]string) {
	if m == nil {
		return
	}
	g.activeDefaults = m
}

// NewGateway 创建网关。单次请求超时取配置 ai.llm.timeout_s（秒），
// token 池响应慢时可在设置中调大，避免请求被本地超时提前切断。
func NewGateway(cfg *config.Store, b *bus.Bus) *Gateway {
	timeout := cfg.GetInt("ai.llm.timeout_s")
	if timeout <= 0 {
		timeout = 120
	}
	return &Gateway{
		cfg:   cfg,
		b:     b,
		httpc: newHTTPClient(timeout),
		defs:  map[string]*ProviderDef{},
		pools: map[string]*TokenPool{},
	}
}

// newHTTPClient 返回一个每次请求都新建连接的 *http.Client。
//
// 根因（family.8.3.1/8.3.2 语音朗读验证时发现）：网关对所有 OpenAI 兼容
// provider 复用同一个 http.Client 的连接池；到 mimo 的 AWS ALB 的长连接会因
// ALB/防火墙的空闲回收被静默丢弃，而 TTS/对话等都是 POST（非幂等，Go 不会自动
// 重试），一旦复用到一个已死的空闲连接，请求就一直等到本地 30s 上下文超时后返回
// 502。独立的 fresh-connection 测试（每次新连接）始终 200，正印证了这一点。
// 因此直接关闭 keep-alive：每次请求走全新连接（TLS 握手约百毫秒，对本应用调用
// 频率完全可接受），从根上消除"复用死连接"导致的挂死。同时 ALPN 仅声明 http/1.1、
// 关掉 HTTP/2 自动升级，进一步规避 ALB 的 HTTP/2 复用隐患。
func newHTTPClient(timeoutSec int) *http.Client {
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{NextProtos: []string{"http/1.1"}},
			ForceAttemptHTTP2: false,
			DisableKeepAlives: true,
		},
		Timeout: time.Duration(timeoutSec) * time.Second,
	}
}

// SetPool 注入 token 池（兼容单 provider 模式；多 provider 注册后按能力懒建池）。
func (g *Gateway) SetPool(p *TokenPool) { g.pool = p }

// RegisterProviders 注册多 provider 定义（来自 secrets/providers.json）。
func (g *Gateway) RegisterProviders(defs map[string]*ProviderDef) {
	for name, d := range defs {
		g.defs[name] = d
	}
}

// ActiveProvider 返回某能力的当前 provider 名。
func (g *Gateway) ActiveProvider(cap string) string {
	name := g.cfg.GetString("ai." + cap + ".provider")
	if name == "" {
		name = g.cfg.GetString("ai." + cap + ".active_provider")
	}
	return name
}

// SetActiveProvider 热切换某能力的 provider（写配置，下次 pick 生效）。
// model 非空时同时切换该能力在此 provider 下的模型。
// provider="custom" 表示用户自备模型（endpoint/model/api_key 由设置页写入），跳过注册表校验。
func (g *Gateway) SetActiveProvider(ctx context.Context, cap, name, model string) error {
	if name != "custom" {
		if _, ok := g.defs[name]; !ok {
			return fmt.Errorf("unknown provider: %s", name)
		}
	}
	if _, err := g.cfg.Set(ctx, "ai."+cap+".provider", name, "string", "能力 provider（热切换）", "system"); err != nil {
		return err
	}
	if model != "" {
		if name != "custom" && !g.validModel(name, cap, model) {
			return fmt.Errorf("model %q not in provider %s cap %s", model, name, cap)
		}
		if _, err := g.cfg.Set(ctx, "ai."+cap+".model", model, "string", "能力模型（热切换）", "system"); err != nil {
			return err
		}
	}
	return nil
}

// CustomProvider 设置自备模型（endpoint/model/api_key 写入配置，api_key 加密存储）。
func (g *Gateway) CustomProvider(ctx context.Context, cap, endpoint, model, apiKey string) error {
	if strings.TrimSpace(endpoint) == "" || strings.TrimSpace(model) == "" {
		return fmt.Errorf("endpoint and model are required")
	}
	if _, err := g.cfg.Set(ctx, "ai."+cap+".endpoint", strings.TrimRight(strings.TrimSpace(endpoint), "/"), "string", "自备模型 endpoint（custom）", "system"); err != nil {
		return err
	}
	if _, err := g.cfg.Set(ctx, "ai."+cap+".model", model, "string", "自备模型名（custom）", "system"); err != nil {
		return err
	}
	if apiKey != "" {
		if _, err := g.cfg.Set(ctx, "ai."+cap+".api_key", apiKey, "secret", "自备模型 API Key（custom，加密存储）", "system"); err != nil {
			return err
		}
	}
	return g.SetActiveProvider(ctx, cap, "custom", model)
}

// CustomInfo 返回自备模型配置状态（脱敏视图）。
func (g *Gateway) CustomInfo(cap string) map[string]any {
	endpoint := g.cfg.GetString("ai." + cap + ".endpoint")
	model := g.cfg.GetString("ai." + cap + ".model")
	key := g.cfg.GetString("ai." + cap + ".api_key")
	out := map[string]any{
		"configured": endpoint != "" && model != "",
		"model":      model,
		"key_set":    key != "",
	}
	if endpoint != "" {
		out["endpoint"] = maskEndpoint(endpoint)
	}
	return out
}

// ClearCustom 清除自备模型配置并把 provider 切回平台注册提供方（优先 providers.json 默认，找不到再扫注册表）。
func (g *Gateway) ClearCustom(ctx context.Context, cap string) error {
	for _, k := range []string{"ai." + cap + ".endpoint", "ai." + cap + ".model", "ai." + cap + ".api_key"} {
		if err := g.cfg.Delete(ctx, k); err != nil {
			return err
		}
	}
	if name := g.activeDefaults[cap]; name != "" {
		if _, ok := g.defs[name]; ok {
			_, err := g.cfg.Set(ctx, "ai."+cap+".provider", name, "string", "能力 provider（清除自定义后回退默认）", "system")
			return err
		}
	}
	// 回退：第一个声明支持该能力的注册 provider
	for name, d := range g.defs {
		for _, c := range d.Caps {
			if c == cap {
				_, err := g.cfg.Set(ctx, "ai."+cap+".provider", name, "string", "能力 provider（清除自定义后回退）", "system")
				return err
			}
		}
	}
	return nil
}

// maskEndpoint 脱敏显示 endpoint（保留协议与域名，隐藏路径参数）。
func maskEndpoint(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	if i := strings.IndexByte(u, '/'); i >= 0 {
		u = u[:i] + "/…"
	}
	return u
}

// ModelCandidates 返回某 provider+能力 的模型候选（无候选返回空）。
func (g *Gateway) ModelCandidates(provider, cap string) []string {
	if d, ok := g.defs[provider]; ok && d.ModelCands != nil {
		return d.ModelCands[cap]
	}
	return nil
}

// VoiceCandidates 返回某 provider 声明的音色候选（providers.json voice_cands；无声明返回空）。
func (g *Gateway) VoiceCandidates(provider string) []string {
	if d, ok := g.defs[provider]; ok {
		return d.VoiceCands
	}
	return nil
}

// validModel 校验模型是否在该 provider 的候选或注册模型中。
func (g *Gateway) validModel(provider, cap, model string) bool {
	d, ok := g.defs[provider]
	if !ok {
		return false
	}
	if m, ok := d.Models[cap]; ok && m == model {
		return true
	}
	if d.Model == model {
		return true
	}
	for _, m := range d.ModelCands[cap] {
		if m == model {
			return true
		}
	}
	return false
}

// PoolNames 返回已注册 provider 名（供设置页展示）。
func (g *Gateway) PoolNames() []string {
	out := make([]string, 0, len(g.defs))
	for n := range g.defs {
		out = append(out, n)
	}
	return out
}

// Providers 返回注册表视图（供管理 API 枚举候选）。
func (g *Gateway) Providers() map[string]*ProviderDef {
	return g.defs
}

// UpsertProvider 热新增/更新一个 provider（写内存注册表，下次 pick 生效）。
// name 是 defs 的 map key（不存于 ProviderDef 自身）。并发安全（复用 mu 锁）。
func (g *Gateway) UpsertProvider(name string, def *ProviderDef) {
	if def == nil {
		return
	}
	g.mu.Lock()
	g.defs[name] = def
	g.mu.Unlock()
}

// RemoveProvider 移除一个 provider（非内置由调用方判定）。同时清理其关联 token 池，
// 避免 pick 命中已删除 provider 的残留池。
func (g *Gateway) RemoveProvider(name string) {
	g.mu.Lock()
	delete(g.defs, name)
	for k := range g.pools {
		if strings.HasPrefix(k, name+"|") {
			delete(g.pools, k)
		}
	}
	g.mu.Unlock()
}

// DropProviderPools 清理某 provider 的全部懒建 token 池（密钥变更后强制重建）。
func (g *Gateway) DropProviderPools(name string) {
	g.mu.Lock()
	for k := range g.pools {
		if strings.HasPrefix(k, name+"|") {
			delete(g.pools, k)
		}
	}
	g.mu.Unlock()
}

// TestConnection 最小连通性校验：对 endpoint 发起一次极简请求（llm=chat/completions，
// embedding=embeddings），校验 HTTP 200 + 可解析。用于后台「测试」按钮，不消耗真实用量。
func (g *Gateway) TestConnection(ctx context.Context, endpoint, model, apiKey, capName string) error {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return fmt.Errorf("endpoint 为空")
	}
	base := endpoint
	if !strings.HasSuffix(base, "/v1") {
		base = base + "/v1"
	}
	var url string
	var payload []byte
	if capName == "embedding" {
		url = base + "/embeddings"
		payload, _ = json.Marshal(map[string]any{"model": model, "input": []string{"ping"}})
	} else {
		url = base + "/chat/completions"
		payload, _ = json.Marshal(map[string]any{
			"model":       model,
			"messages":    []map[string]string{{"role": "user", "content": "ping"}},
			"max_tokens":  1,
			"stream":      false,
		})
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := g.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// CurrentModel 返回某能力的当前生效模型（provider 解析后）。
func (g *Gateway) CurrentModel(cap string) string {
	p, err := g.pick("ai." + cap)
	if err != nil || p == nil {
		return ""
	}
	return p.model
}

// EmbeddingModel 返回当前 embedding provider 的实际模型名（供向量化/检索端保持一致）。
func (g *Gateway) EmbeddingModel() string {
	name := g.ActiveProvider("embedding")
	if def, ok := g.defs[name]; ok {
		if m, ok := def.Models["embedding"]; ok && m != "" {
			return m
		}
		if def.Model != "" {
			return def.Model
		}
	}
	return g.cfg.GetString("ai.embedding.model")
}

// ChatJSON 对话（可选 tools）：返回内容与工具调用。未配置/失败时降级文本。
func (g *Gateway) ChatJSON(ctx context.Context, messages []Msg, tools []map[string]any) (*ChatResult, error) {
	p, err := g.pick("ai.llm")
	if err != nil || p == nil {
		return &ChatResult{Content: "AI 服务未配置（需在设置中填写模型 endpoint/key，或启动时注入 token 池）。当前为降级模式。"}, nil
	}
	out, err := p.chatJSON(ctx, messages, tools)
	if err != nil {
		g.poolFail(p.apiKey)
		return &ChatResult{Content: "AI 服务暂不可用，请稍后重试或检查模型配置。", Error: err.Error()}, nil
	}
	g.poolOK(p.apiKey)
	return out, nil
}

// poolFail/poolOK 与 token 池联动（未注入时为空操作）。
func (g *Gateway) poolFail(key string) {
	if g.pool != nil && key != "" {
		g.pool.ReportFailure(key)
	}
}
func (g *Gateway) poolOK(key string) {
	if g.pool != nil && key != "" {
		g.pool.ReportSuccess(key)
	}
}

// Embedding 调用 embedding 模型。provider 未配置时返回确定性伪向量（检索管线可跑、调用方据此跳过向量检索）；
// 已配置但调用失败必须返回错误，由调用方决定降级——绝不静默产出伪向量污染语义检索。
func (g *Gateway) Embedding(ctx context.Context, texts []string) ([][]float32, error) {
	p, err := g.pick("ai.embedding")
	if err != nil || p == nil {
		return degradedEmbedding(texts, g.cfg.GetInt("ai.embedding.dim")), nil
	}
	vecs, err := p.Embed(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embedding provider %s failed: %w", p.name, err)
	}
	return vecs, nil
}

// Summarize 摘要（降级：截断提示）。
func (g *Gateway) Summarize(ctx context.Context, text string) (string, error) {
	p, err := g.pick("ai.llm")
	if err != nil || p == nil {
		return degradedSummary(text), nil
	}
	out, err := p.Summarize(ctx, text)
	if err != nil {
		return degradedSummary(text), nil
	}
	return out, nil
}

// summarizeTagsSystem 摘要+标签一次输出（省 token、自动打标）。标签要求简短、可作分类。
const summarizeTagsSystem = `你是一个知识库索引助手。阅读文件内容，输出且只输出一个 JSON 对象，不要输出任何其他文字：
{"summary": "3-5 句要点摘要，覆盖主要内容与结论，不添加原文没有的信息", "tags": ["2-5 个简短标签"]}
标签要求：每个 2-6 个汉字/词，用于分类检索与知识库聚合；可含层级（用 / 分隔，如 音乐/粤语）；不包含文件扩展名；宁缺毋滥。`

// SummarizeTags 摘要 + 自动标签（一次 LLM 调用）。解析失败时回退纯摘要 + 空标签。
func (g *Gateway) SummarizeTags(ctx context.Context, text string) (string, []string, error) {
	p, err := g.pick("ai.llm")
	if err != nil || p == nil {
		return degradedSummary(text), nil, nil
	}
	out, err := p.SummarizeJSON(ctx, text)
	if err != nil {
		return degradedSummary(text), nil, nil
	}
	return out.Summary, out.Tags, nil
}

// summarizeMetaSystem 摘要+标签+元数据+实体+关系一次输出（K19 元数据 + K20 图谱建边，一次调用省 token）。
const summarizeMetaSystem = `你是一个知识库索引助手。阅读文件内容，输出且只输出一个 JSON 对象，不要输出任何其他文字：
{"summary": "3-5 句要点摘要，覆盖主要内容与结论，不添加原文没有的信息", "tags": ["2-5 个简短标签"], "meta": {"title": "文档标题（文件名已表达标题则填空字符串）", "author": "作者/创作者（未知填空字符串）", "source": "来源（如 网页/书籍/内部文档/邮件，未知填空字符串）", "keywords": ["3-6 个关键词"], "doc_type": "文档类型（如 技术方案/合同/笔记/论文/FAQ/报告/教程，不确定用 文档）", "language": "zh 或 en"}, "entities": [{"name": "实体名", "type": "人物/产品/项目/概念/组织/技术 之一"}], "relations": [{"subject": "实体名", "predicate": "关系动词（如 使用/提出/包含/开发/属于）", "object": "实体名", "weight": 0.9}]}
标签要求：每个 2-6 个汉字/词，用于分类检索与知识库聚合；可含层级（用 / 分隔，如 音乐/粤语）；不包含文件扩展名；宁缺毋滥。
meta 要求：只提取原文能支撑的信息，绝不编造；无法确定的字段填空字符串（keywords 填空数组）。
entities 要求：仅提取原文明确出现的核心实体（人物/产品/项目/概念/组织/技术），最多 8 个；宁缺毋滥，泛泛而谈的不提取。
relations 要求：仅提取 entities 之间存在明确关系的配对，最多 8 条；subject/object 必须与 entities 中名称完全一致；weight 0-1 表示置信度。`

// SummaryMetaJSON 摘要+标签+元数据结构化结果（K19）。
type SummaryMetaJSON struct {
	Summary   string         `json:"summary"`
	Tags      []string       `json:"tags"`
	Meta      MetaInfo       `json:"meta"`
	Entities  []EntityInfo   `json:"entities"`
	Relations []RelationInfo `json:"relations"`
}

// MetaInfo 文件元数据（author/source/keywords/doc_type/language，字段全可空）。
type MetaInfo struct {
	Title    string   `json:"title"`
	Author   string   `json:"author"`
	Source   string   `json:"source"`
	Keywords []string `json:"keywords"`
	DocType  string   `json:"doc_type"`
	Language string   `json:"language"`
}

// EntityInfo 抽取实体（K20：人物/产品/项目/概念/组织/技术）。
type EntityInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// RelationInfo 实体间关系三元组（K20：subject --predicate--> object，weight 置信度）。
type RelationInfo struct {
	Subject   string  `json:"subject"`
	Predicate string  `json:"predicate"`
	Object    string  `json:"object"`
	Weight    float64 `json:"weight,omitempty"`
}

// KnowledgeExtract 一次 LLM 调用的完整知识抽取结果（K19 元数据 + K20 实体/关系）。
type KnowledgeExtract struct {
	Summary   string         `json:"summary"`
	Tags      []string       `json:"tags"`
	Meta      MetaInfo       `json:"meta"`
	Entities  []EntityInfo   `json:"entities"`
	Relations []RelationInfo `json:"relations"`
}

// SummarizeMeta 摘要 + 自动标签 + 元数据 + 实体/关系（一次 LLM 调用）。解析失败时回退纯摘要 + 空其余。
func (g *Gateway) SummarizeMeta(ctx context.Context, text string) (*KnowledgeExtract, error) {
	p, err := g.pick("ai.llm")
	if err != nil || p == nil {
		return &KnowledgeExtract{Summary: degradedSummary(text)}, nil
	}
	msgs := []Msg{
		{Role: "system", Content: summarizeMetaSystem},
		{Role: "user", Content: text},
	}
	out, err := p.chat(ctx, msgs)
	if err != nil {
		return &KnowledgeExtract{Summary: degradedSummary(text)}, nil
	}
	r, err := parseSummaryMetaJSON(out)
	if err != nil {
		return &KnowledgeExtract{Summary: degradedSummary(text)}, nil
	}
	return &KnowledgeExtract{Summary: r.Summary, Tags: r.Tags, Meta: r.Meta, Entities: r.Entities, Relations: r.Relations}, nil
}

// parseSummaryMetaJSON 从模型输出提取 JSON（容忍 ```json 围栏与前后缀），解析摘要+标签+元数据。
func parseSummaryMetaJSON(out string) (*SummaryMetaJSON, error) {
	s := strings.TrimSpace(out)
	if i := strings.Index(s, "```"); i >= 0 {
		j := strings.Index(s[i+3:], "```")
		if j >= 0 {
			block := s[i+3 : i+3+j]
			if k := strings.IndexByte(block, '\n'); k >= 0 {
				block = block[k+1:]
			}
			s = strings.TrimSpace(block)
		}
	}
	start, end := strings.IndexByte(s, '{'), strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("summarize-meta: no json object in output")
	}
	var r SummaryMetaJSON
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return nil, err
	}
	if strings.TrimSpace(r.Summary) == "" {
		r.Summary = strings.TrimSpace(s)
	}
	tags := r.Tags[:0]
	for _, t := range r.Tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		tags = append(tags, t)
	}
	r.Tags = tags
	r.Meta.Author = strings.TrimSpace(r.Meta.Author)
	r.Meta.Source = strings.TrimSpace(r.Meta.Source)
	r.Meta.DocType = strings.TrimSpace(r.Meta.DocType)
	r.Meta.Language = strings.TrimSpace(r.Meta.Language)
	kws := r.Meta.Keywords[:0]
	for _, k := range r.Meta.Keywords {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		kws = append(kws, k)
	}
	r.Meta.Keywords = kws
	// 实体/关系清洗：剔除空名与超长名（≤40 rune）
	ens := r.Entities[:0]
	for _, e := range r.Entities {
		e.Name = strings.TrimSpace(e.Name)
		if e.Name == "" || len([]rune(e.Name)) > 40 {
			continue
		}
		ens = append(ens, e)
	}
	r.Entities = ens
	rls := r.Relations[:0]
	for _, rl := range r.Relations {
		rl.Subject = strings.TrimSpace(rl.Subject)
		rl.Predicate = strings.TrimSpace(rl.Predicate)
		rl.Object = strings.TrimSpace(rl.Object)
		if rl.Subject == "" || rl.Object == "" || rl.Subject == rl.Object {
			continue
		}
		if rl.Weight <= 0 || rl.Weight > 1 {
			rl.Weight = 0.9
		}
		rls = append(rls, rl)
	}
	r.Relations = rls
	return &r, nil
}

// SummaryJSON 摘要+标签结构化结果。
type SummaryJSON struct {
	Summary string   `json:"summary"`
	Tags    []string `json:"tags"`
}

// Ask 对话（降级：返回配置说明）。
func (g *Gateway) Ask(ctx context.Context, messages []Msg) (string, error) {
	p, err := g.pick("ai.llm")
	if err != nil || p == nil {
		return "AI 服务未配置（provider=local 需配置本地模型，或切换 hosted 并填写 endpoint）。当前为降级模式。", nil
	}
	out, err := p.Ask(ctx, messages)
	if err != nil {
		return "AI 服务暂不可用，已降级。请检查模型配置。", nil
	}
	return out, nil
}

// CleanText 文本整理（OCR 结果/剪藏/导入 → 干净结构化 Markdown）。
// purpose 分支：ocr（去 HTML 噪声）| raw（保留原文整理）。供 OCR 后处理与知识库入库复用。
func (g *Gateway) CleanText(ctx context.Context, text, purpose string) (string, error) {
	p, err := g.pick("ai.llm")
	if err != nil || p == nil {
		return "", fmt.Errorf("AI 服务未配置")
	}
	out, err := p.Ask(ctx, cleanMsgs(text, purpose))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// cleanMsgs 构造整理 prompt（忠实原文、只去噪声）。与 handler/ai_clean.go 共用语义。
func cleanMsgs(text, purpose string) []Msg {
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
	return []Msg{{Role: "user", Content: inst + text}}
}

// pick 根据配置构造 provider。
// 多 provider 注册表优先：provider 名 → 定义（endpoint/model/keys 懒建池）；
// 未注册时回退旧逻辑（cfg 的 endpoint/model/api_key 单配置）。
func (g *Gateway) pick(prefix string) (*OpenAICompat, error) {
	provider := g.cfg.GetString(prefix + ".provider")
	endpoint := g.cfg.GetString(prefix + ".endpoint")
	model := ""
	if g.cfg.IsExplicit(prefix + ".model") {
		model = g.cfg.GetString(prefix + ".model") // 用户显式设置（设置页/切换 API）优先
	}
	apiKey := g.cfg.GetString(prefix + ".api_key")

	capName := strings.TrimPrefix(prefix, "ai.")
	if def, ok := g.defs[provider]; ok {
		if def.Endpoint != "" {
			endpoint = def.Endpoint
		}
		if model == "" { // 未显式设置时用 provider 注册模型
			if m, ok := def.Models[capName]; ok && m != "" {
				model = m
			} else if def.Model != "" {
				model = def.Model
			}
		}
		// 该 provider+能力 的 token 池（懒创建，按注册 keys；并发安全）
		if len(def.Keys) > 0 {
			poolKey := provider + "|" + capName
			g.mu.Lock()
			p, ok := g.pools[poolKey]
			if !ok {
				p = NewTokenPool(def.Keys, 30*time.Second)
				g.pools[poolKey] = p
			}
			g.mu.Unlock()
			if k, err := p.Next(); err == nil {
				apiKey = k
			}
		}
	} else if g.pool != nil && apiKey == "" {
		// 未注册 provider（含自定义 custom）：已填 api_key 则优先，空才回退旧 token 池
		if k, err := g.pool.Next(); err == nil {
			apiKey = k
		}
	}
	if endpoint == "" {
		return nil, nil // 未配置 → 降级
	}
	return &OpenAICompat{
		name:     prefix + ":" + model,
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		apiKey:   apiKey,
		httpc:    g.httpc,
		dims:     g.cfg.GetInt(prefix + ".dim"),
		cap:      capName,
		gate:     g,
	}, nil
}

// OpenAICompat 实现 OpenAI 兼容协议（/v1/embeddings、/v1/chat/completions）。
type OpenAICompat struct {
	name     string
	endpoint string
	model    string
	apiKey   string
	httpc    *http.Client
	dims     int // >0 时 embedding 请求带 dimensions
	cap      string    // 能力名（llm/embedding/...），记账用
	gate     *Gateway  // 记账回源（recordUsage）
}

// Name 返回提供方名。
func (o *OpenAICompat) Name() string { return o.name }

// v1Base 兼容 endpoint 带/不带 /v1 后缀两种写法。
func (o *OpenAICompat) v1Base() string {
	if strings.HasSuffix(o.endpoint, "/v1") {
		return o.endpoint
	}
	return o.endpoint + "/v1"
}

// Embed 调用 embeddings。
func (o *OpenAICompat) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	start := time.Now()
	payload := map[string]any{
		"model": o.model, "input": texts,
	}
	if o.dims > 0 {
		payload["dimensions"] = o.dims
	}
	body, _ := json.Marshal(payload)
	req, err := o.newReq(ctx, o.v1Base()+"/embeddings", body)
	if err != nil {
		return nil, err
	}
	resp, err := o.httpc.Do(req)
	if err != nil {
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		err = fmt.Errorf("embedding http %d: %s", resp.StatusCode, b)
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return nil, err
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage *Usage `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return nil, err
	}
	vecs := make([][]float32, 0, len(out.Data))
	for _, d := range out.Data {
		vecs = append(vecs, d.Embedding)
	}
	if o.gate != nil {
		o.gate.recordUsage(ctx, o.cap, o.model, out.Usage, start, nil)
	}
	return vecs, nil
}

// OCR 图片文字识别（chat/completions 多模态：text + image_url）。
// 忠实转录型（适合知识库入库）：只输出图中文字，不改写不润色。
const ocrPrompt = `这是一张图片/截图/扫描件。请逐字逐句忠实转录图片中的全部文字：
1. 只输出图片中实际存在的文字，不增加、不删减、不改写、不润色；
2. 保留原文的用词和写法（包括可能的错别字、简写），不要替你纠错；
3. 保持原有的换行和段落结构；表格用 Markdown 表格呈现；
4. 确实认不出的字用【□】占位，不要臆测乱猜；
5. 标题、日期、落款、边注等文字也要转录；
6. 只输出识别出的文字，不要输出任何解释、评论或前后缀。`

// OCR 识别图片中的文字。imageData 为 base64（可带 data: 前缀），mime 如 image/png。
func (g *Gateway) OCR(ctx context.Context, imageData, mime string) (string, error) {
	p, err := g.pick("ai.ocr")
	if err != nil || p == nil {
		return "", fmt.Errorf("OCR 服务未配置（provider/模型未设置）")
	}
	return p.OCR(ctx, imageData, mime)
}

// OCR 实现（OpenAI 兼容多模态）。
func (o *OpenAICompat) OCR(ctx context.Context, imageData, mime string) (string, error) {
	if mime == "" {
		mime = "image/png"
	}
	imgURL := imageData
	if !strings.HasPrefix(imageData, "data:") {
		imgURL = "data:" + mime + ";base64," + imageData
	}
	msgs := []Msg{
		{Role: "user", Content: "", ImageURLs: []string{imgURL}, ImageText: ocrPrompt},
	}
	return o.chat(ctx, msgs)
}

// Summarize 用 chat 完成实现。
func (o *OpenAICompat) Summarize(ctx context.Context, text string) (string, error) {
	msgs := []Msg{
		{Role: "system", Content: "你是一个知识库摘要助手。用中文输出 3-5 句要点摘要，不添加原文没有的信息。"},
		{Role: "user", Content: text},
	}
	return o.chat(ctx, msgs)
}

// SummarizeJSON 摘要+标签（一次调用，JSON 输出）。输出解析失败时返回 err，由调用方降级。
func (o *OpenAICompat) SummarizeJSON(ctx context.Context, text string) (*SummaryJSON, error) {
	msgs := []Msg{
		{Role: "system", Content: summarizeTagsSystem},
		{Role: "user", Content: text},
	}
	out, err := o.chat(ctx, msgs)
	if err != nil {
		return nil, err
	}
	return parseSummaryJSON(out)
}

// parseSummaryJSON 从模型输出提取 JSON（容忍 ```json 围栏与前后缀），解析摘要+标签。
func parseSummaryJSON(out string) (*SummaryJSON, error) {
	s := strings.TrimSpace(out)
	// 提取 ```json ... ``` 块
	if i := strings.Index(s, "```"); i >= 0 {
		j := strings.Index(s[i+3:], "```")
		if j >= 0 {
			block := s[i+3 : i+3+j]
			if k := strings.IndexByte(block, '\n'); k >= 0 {
				block = block[k+1:]
			}
			s = strings.TrimSpace(block)
		}
	}
	// 提取首个 { ... } 区间
	start, end := strings.IndexByte(s, '{'), strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("summarize: no json object in output")
	}
	var r SummaryJSON
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return nil, err
	}
	if strings.TrimSpace(r.Summary) == "" {
		r.Summary = strings.TrimSpace(s)
	}
	tags := r.Tags[:0]
	for _, t := range r.Tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		tags = append(tags, t)
	}
	r.Tags = tags
	return &r, nil
}

// RerankEnabled rerank 能力是否已配置（provider/endpoint 任一就绪）。
func (g *Gateway) RerankEnabled() bool {
	return g.cfg.GetString("ai.rerank.provider") != "" || g.cfg.GetString("ai.rerank.endpoint") != ""
}

// Rerank 对检索候选文档按与 query 的相关性精排（K18：RRF 之上的二阶段精排）。
// 未配置 rerank provider 时返回 nil（调用方保持 RRF 原顺序）；已配置但调用失败返回错误（调用方降级）。
// 接口协议：OpenAI 兼容 POST {base}/rerank，body {model, query, documents, top_n}，
// 响应 {results:[{index, relevance_score}]}——本地 bge-reranker 服务与云侧 qwen3-rerank 均按此实现。
func (g *Gateway) Rerank(ctx context.Context, query string, docs []string) ([]float32, error) {
	if len(docs) == 0 || query == "" {
		return nil, nil
	}
	p, err := g.pick("ai.rerank")
	if err != nil || p == nil {
		return nil, nil // 未配置 → 跳过精排
	}
	out, err := p.Rerank(ctx, query, docs)
	if err != nil {
		g.poolFail(p.apiKey)
		return nil, fmt.Errorf("rerank provider %s failed: %w", p.name, err)
	}
	g.poolOK(p.apiKey)
	return out, nil
}

// Rerank 实现 OpenAI 兼容 /rerank 端点（模型无关：qwen3-rerank / bge-reranker 本地服务通用）。
func (o *OpenAICompat) Rerank(ctx context.Context, query string, docs []string) ([]float32, error) {
	payload := map[string]any{
		"model":            o.model,
		"query":            query,
		"documents":        docs,
		"top_n":            len(docs),
		"return_documents": false,
	}
	body, _ := json.Marshal(payload)
	req, err := o.newReq(ctx, o.v1Base()+"/rerank", body)
	if err != nil {
		return nil, err
	}
	resp, err := o.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("rerank http %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Results []struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	scores := make([]float32, len(docs))
	for _, r := range out.Results {
		if r.Index >= 0 && r.Index < len(docs) {
			scores[r.Index] = float32(r.Score)
		}
	}
	return scores, nil
}

// Ask 对话补全。
func (o *OpenAICompat) Ask(ctx context.Context, messages []Msg) (string, error) {
	return o.chat(ctx, messages)
}

// chatJSON 对话补全，支持 tools（function calling）。返回内容与工具调用。
func (o *OpenAICompat) chatJSON(ctx context.Context, messages []Msg, tools []map[string]any) (*ChatResult, error) {
	start := time.Now()
	body := map[string]any{
		"model": o.model, "messages": messages, "stream": false,
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	raw, _ := json.Marshal(body)
	req, err := o.newReq(ctx, o.v1Base()+"/chat/completions", raw)
	if err != nil {
		return nil, err
	}
	resp, err := o.httpc.Do(req)
	if err != nil {
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		err = fmt.Errorf("chat http %d: %s", resp.StatusCode, b)
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return nil, err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage *Usage `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return nil, err
	}
	if len(out.Choices) == 0 {
		err = fmt.Errorf("chat: empty choices")
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return nil, err
	}
	m := out.Choices[0].Message
	res := &ChatResult{Content: m.Content, Usage: out.Usage}
	for _, tc := range m.ToolCalls {
		res.ToolCalls = append(res.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}
	if o.gate != nil {
		o.gate.recordUsage(ctx, o.cap, o.model, out.Usage, start, nil)
	}
	return res, nil
}

func (o *OpenAICompat) chat(ctx context.Context, messages []Msg) (string, error) {
	out, _, err := o.chatWithUsage(ctx, messages)
	return out, err
}

// chatWithUsage 对话补全并解析用量（记账入口）。
func (o *OpenAICompat) chatWithUsage(ctx context.Context, messages []Msg) (string, *Usage, error) {
	start := time.Now()
	body, _ := json.Marshal(map[string]any{
		"model": o.model, "messages": messages, "stream": false,
	})
	req, err := o.newReq(ctx, o.v1Base()+"/chat/completions", body)
	if err != nil {
		return "", nil, err
	}
	resp, err := o.httpc.Do(req)
	if err != nil {
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		err = fmt.Errorf("chat http %d: %s", resp.StatusCode, b)
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return "", nil, err
	}
	var out struct {
		Choices []struct {
			Message Msg `json:"message"`
		} `json:"choices"`
		Usage *Usage `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return "", nil, err
	}
	if len(out.Choices) == 0 {
		err = fmt.Errorf("chat: empty choices")
		if o.gate != nil {
			o.gate.recordUsage(ctx, o.cap, o.model, nil, start, err)
		}
		return "", nil, err
	}
	if o.gate != nil {
		o.gate.recordUsage(ctx, o.cap, o.model, out.Usage, start, nil)
	}
	return out.Choices[0].Message.Content, out.Usage, nil
}

func (o *OpenAICompat) newReq(ctx context.Context, url string, body []byte) (*http.Request, error) {
	// 前置闸：配额/余额校验（平台模型才受限；nil meter 零开销）
	if o.gate != nil {
		if err := o.gate.checkGate(ctx, o.cap); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	return req, nil
}

// ---- 降级实现 ----

func degradedEmbedding(texts []string, dim int) [][]float32 {
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, dim)
		// 确定性伪向量：至少让检索管线可运行、可测试
		h := fnvHash(texts[i])
		v[h%uint32(dim)] = 1
		out[i] = v
	}
	return out
}

func degradedSummary(text string) string {
	r := []rune(text)
	if len(r) > 200 {
		r = r[:200]
	}
	return "（降级摘要）" + string(r)
}

func fnvHash(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
