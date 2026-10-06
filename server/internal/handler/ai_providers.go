// ai_providers.go 模型提供方管理：查询注册表 / 热切换能力级 provider 与模型。
// 支撑"模型后台可配置"：对话/语音/图像/向量/OCR/代码各能力独立绑定 provider+model，切换即时生效。
package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
)

// providerCaps 能力全集（含新增 ocr/code）。
var providerCaps = []string{"llm", "embedding", "asr", "tts", "image", "rerank", "ocr", "code"}

// providerCap 返回某能力的当前绑定（provider+model）与候选。
func (a *API) providerCap(cap string) map[string]any {
	active := a.ai.ActiveProvider(cap)
	// 候选 = 注册 provider 中声明支持该能力者
	cands := []string{}
	for name, d := range a.ai.Providers() {
		for _, c := range d.Caps {
			if c == cap {
				cands = append(cands, name)
				break
			}
		}
	}
	curModel := a.ai.CurrentModel(cap)
	models := map[string][]string{}
	if active != "" {
		if mc := a.ai.ModelCandidates(active, cap); len(mc) > 0 {
			models[active] = mc
		}
	}
	custom := a.ai.CustomInfo(cap)
	out := map[string]any{
		"cap": cap, "active": active, "options": cands,
		"model": curModel, "models": models, "custom": custom,
	}
	// 语音合成补充默认参数视图（family.8.1）：当前默认音色/格式/风格 + provider 声明的音色候选。
	if cap == "tts" {
		out["voice"] = map[string]any{
			"current": a.cfg.GetString("ai.tts.voice"),
			"format":  a.cfg.GetString("ai.tts.format"),
			"style":   a.cfg.GetString("ai.tts.style"),
			"cands":   a.ai.VoiceCandidates(active),
		}
	}
	return out
}

// aiProvidersList GET /api/v1/admin/ai/providers：各能力 provider 绑定状态。
func (a *API) aiProvidersList(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for _, cap := range providerCaps {
		out = append(out, a.providerCap(cap))
	}
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": out})
}

// aiProvidersSwitch POST /api/v1/admin/ai/providers/switch：热切换某能力 provider（可带模型）。
// body: {"cap":"llm","provider":"bailian","model":"glm-5.2"}
func (a *API) aiProvidersSwitch(w http.ResponseWriter, r *http.Request) {
	// H2/M10 修复：切换 provider 会改写全站 LLM 走线（含成本面），仅管理员；
	// 纵深防御：路由级 /api/v1/admin/ 前缀已统一拦截，此处再加 handler 层守卫。
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_ONLY", "仅管理员可操作")
		return
	}
	var req struct {
		Cap      string `json:"cap"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "PROV_BAD_REQ", err.Error())
		return
	}
	if err := a.ai.SetActiveProvider(r.Context(), req.Cap, req.Provider, req.Model); err != nil {
		writeErr(w, http.StatusBadRequest, "PROV_UNKNOWN", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cap": req.Cap, "provider": req.Provider, "model": req.Model})
}

// aiCustomProviderSave POST /api/v1/admin/ai/providers/custom：设置自备模型。
// body: {"cap":"llm","endpoint":"https://...","model":"gpt-x","api_key":"sk-..."}
func (a *API) aiCustomProviderSave(w http.ResponseWriter, r *http.Request) {
	// H2/M10 修复：自定义 provider 会写入 endpoint + api_key（可指向攻击者服务器并接管调用），仅管理员
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_ONLY", "仅管理员可操作")
		return
	}
	var req struct {
		Cap      string `json:"cap"`
		Endpoint string `json:"endpoint"`
		Model    string `json:"model"`
		APIKey   string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	if err := a.ai.CustomProvider(r.Context(), req.Cap, req.Endpoint, req.Model, req.APIKey); err != nil {
		writeErr(w, http.StatusBadRequest, "CUSTOM_SAVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cap": req.Cap, "provider": "custom", "model": req.Model})
}

// aiCustomProviderClear DELETE /api/v1/admin/ai/providers/custom：清除自备模型并回退平台 provider。
func (a *API) aiCustomProviderClear(w http.ResponseWriter, r *http.Request) {
	// H2/M10 修复：清空自定义 provider 属管理面操作，仅管理员
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_ONLY", "仅管理员可操作")
		return
	}
	var req struct {
		Cap string `json:"cap"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	if err := a.ai.ClearCustom(r.Context(), req.Cap); err != nil {
		writeErr(w, http.StatusBadRequest, "CUSTOM_CLEAR_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cap": req.Cap})
}

// aiProvidersManage GET /api/v1/admin/ai/providers/all：全量注册表视图（管理面板用）。
// 返回每个 provider 的 endpoint / 默认模型 / 各能力模型 / 候选 / 能力 / 密钥数 / 音色候选 / 备注 / 是否内置。
func (a *API) aiProvidersManage(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]any, 0, len(a.ai.Providers()))
	for name, d := range a.ai.Providers() {
		out = append(out, map[string]any{
			"name":         name,
			"endpoint":     d.Endpoint,
			"model":        d.Model,
			"models":       d.Models,
			"model_cands":  d.ModelCands,
			"caps":         d.Caps,
			"keys":         d.Keys,
			"keys_count":   len(d.Keys),
			"voice_cands":  d.VoiceCands,
			"note":         d.Note,
			"builtin":      d.Builtin,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) < out[j]["name"].(string) })
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

// aiProviderUpsert PUT /api/v1/admin/ai/providers：新增/更新一个 provider（即时热更网关注册表 + 持久化）。
// body 字段见 ProviderDef；keys 为数组（token 池轮替）；内置提供方允许编辑但保留 builtin 标记。
func (a *API) aiProviderUpsert(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_ONLY", "仅管理员可操作")
		return
	}
	var req struct {
		Name       string              `json:"name"`
		Endpoint   string              `json:"endpoint"`
		Model      string              `json:"model"`
		Models     map[string]string   `json:"models"`
		ModelCands map[string][]string `json:"model_cands"`
		Keys       []string            `json:"keys"`
		Caps       []string            `json:"caps"`
		VoiceCands []string            `json:"voice_cands"`
		Note       string              `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	endpoint := strings.TrimRight(strings.TrimSpace(req.Endpoint), "/")
	if name == "" || endpoint == "" {
		writeErr(w, http.StatusBadRequest, "PROV_BAD_REQ", "name 与 endpoint 均必填")
		return
	}
	def := &ai.ProviderDef{
		Endpoint:   endpoint,
		Model:      strings.TrimSpace(req.Model),
		Models:     req.Models,
		ModelCands: req.ModelCands,
		Keys:       req.Keys,
		Caps:       req.Caps,
		VoiceCands: req.VoiceCands,
		Note:       req.Note,
	}
	// 编辑内置时保留 builtin 标记（禁止经此删除内置）
	if existing, ok := a.ai.Providers()[name]; ok {
		def.Builtin = existing.Builtin
	}
	if err := ai.SaveProviderDef(a.db, name, def); err != nil {
		writeErr(w, http.StatusInternalServerError, "PROV_SAVE_FAILED", err.Error())
		return
	}
	a.ai.UpsertProvider(name, def)
	a.ai.DropProviderPools(name) // 密钥/endpoint 变更后强制重建 token 池
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "builtin": def.Builtin})
}

// aiProviderDelete DELETE /api/v1/admin/ai/providers：删除 provider（禁删内置）。
// 同时清理任何能力对该 provider 的绑定，避免悬空引用。
func (a *API) aiProviderDelete(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_ONLY", "仅管理员可操作")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "name 必填")
		return
	}
	name := strings.TrimSpace(req.Name)
	d, ok := a.ai.Providers()[name]
	if !ok {
		writeErr(w, http.StatusNotFound, "PROV_UNKNOWN", "提供方不存在")
		return
	}
	if d.Builtin {
		writeErr(w, http.StatusBadRequest, "PROV_BUILTIN", "内置提供方不可删除（可在后台编辑）")
		return
	}
	// 清理能力绑定：解除指向该 provider 的能力，回退为未配置（降级），不残留悬空引用
	for _, cap := range providerCaps {
		if a.ai.ActiveProvider(cap) == name {
			_ = a.cfg.Delete(r.Context(), "ai."+cap+".provider")
			_ = a.cfg.Delete(r.Context(), "ai."+cap+".model")
		}
	}
	if err := ai.DeleteProviderDef(a.db, name); err != nil {
		writeErr(w, http.StatusInternalServerError, "PROV_DEL_FAILED", err.Error())
		return
	}
	a.ai.RemoveProvider(name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
}

// aiProviderTest POST /api/v1/admin/ai/providers/test：最小连通性校验（不消耗真实用量）。
// body: {"name":"agnes","cap":"llm"}；name="custom" 时取该能力自备模型配置。
func (a *API) aiProviderTest(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_ONLY", "仅管理员可操作")
		return
	}
	var req struct {
		Name string `json:"name"`
		Cap  string `json:"cap"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	cap := req.Cap
	if cap == "" {
		cap = "llm"
	}
	endpoint, model, apiKey := "", "", ""
	if req.Name == "custom" {
		endpoint = a.cfg.GetString("ai." + cap + ".endpoint")
		model = a.cfg.GetString("ai." + cap + ".model")
		apiKey = a.cfg.GetString("ai." + cap + ".api_key")
	} else {
		d, ok := a.ai.Providers()[strings.TrimSpace(req.Name)]
		if !ok {
			writeErr(w, http.StatusNotFound, "PROV_UNKNOWN", "提供方不存在")
			return
		}
		endpoint = d.Endpoint
		if m, ok := d.Models[cap]; ok && m != "" {
			model = m
		} else {
			model = d.Model
		}
		if len(d.Keys) > 0 {
			apiKey = d.Keys[0]
		}
	}
	if err := a.ai.TestConnection(r.Context(), endpoint, model, apiKey, cap); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
