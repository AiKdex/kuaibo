// ai_providers.go 模型提供方管理：查询注册表 / 热切换能力级 provider 与模型。
// 支撑"模型后台可配置"：对话/语音/图像/向量/OCR/代码各能力独立绑定 provider+model，切换即时生效。
package handler

import (
	"encoding/json"
	"net/http"
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
	return map[string]any{
		"cap": cap, "active": active, "options": cands,
		"model": curModel, "models": models, "custom": custom,
	}
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
