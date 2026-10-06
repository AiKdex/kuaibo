// family.go 家族传承记录模块 HTTP 层（路由见 routes.go：a.family != nil 时注册；模块未启用 404）。
// 归属校验：写操作均按 curUserID 隔离；admin 可访问任意用户数据（管理场景）。
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// familyPaidGate 家族模块付费门禁（复用 orgPaidGate 模型，REQ-010 口径 scope=feature:family）：放行条件（任一）——
//   - 站长/管理员（isAdmin）；
//   - 站点级 feature:family 授权（license 门禁接入点）；
//   - 站点级 pro 授权（本 fork 站点级授权粒度）。
//
// 路由层以包装器方式对除 /family/settings 外的全部 family 端点生效。
//
// 总开关（master switch）：settings family.enabled=false 时整模块对所有人关闭（含 admin/持 license 用户），
// 仅 /family/settings 仍可读状态（供前端入口判定与后台重新开启）。该检查先于付费门禁——
// 顺序即"关闭即不装不用"，与 service.FamilyStore.Enabled() 注释口径一致；否则 family.enabled 只是个
// 前端假开关，后端数据端点仍可被 admin/license 用户访问（安全错觉）。
func (a *API) familyPaidGate(w http.ResponseWriter, r *http.Request) bool {
	if a.family != nil && !a.family.Enabled() {
		writeErr(w, http.StatusForbidden, "FAMILY_DISABLED", "家族传承功能未启用（请在后台开启 family.enabled）")
		return false
	}
	if a.isAdmin(r) {
		return true
	}
	if p, err := a.currentLicense(); err == nil && p.HasFeature("family") {
		return true
	}
	if a.licenseEdition() == "pro" {
		return true
	}
	writeErr(w, http.StatusForbidden, "PAYMENT_REQUIRED", "该能力属于付费版（Pro），请升级后使用")
	return false
}

// familySettings GET /api/v1/family/settings —— 模块开关状态（前端门控入口显示）。
func (a *API) familySettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"enabled": a.family.Enabled()})
}

// familyMembersList GET /api/v1/family/members
func (a *API) familyMembersList(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	list, err := a.family.ListMembers(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": list})
}

// familyMembersCreate POST /api/v1/family/members
func (a *API) familyMembersCreate(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	var m service.FamilyMember
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	if m.Name == "" {
		writeErr(w, http.StatusBadRequest, "BAD_NAME", "成员姓名不能为空")
		return
	}
	created, err := a.family.CreateMember(r.Context(), uid, &m)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"member": created})
}

// familyMembersUpdate PUT /api/v1/family/members/{id}
func (a *API) familyMembersUpdate(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	var m service.FamilyMember
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	if m.Name == "" {
		writeErr(w, http.StatusBadRequest, "BAD_NAME", "成员姓名不能为空")
		return
	}
	updated, err := a.family.UpdateMember(r.Context(), uid, r.PathValue("id"), &m)
	if err != nil {
		if err == service.ErrFamilyNotFound {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "成员不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"member": updated})
}

// familyMembersDelete DELETE /api/v1/family/members/{id} —— 级联清理关系与关联纪念日。
func (a *API) familyMembersDelete(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	if err := a.family.DeleteMember(r.Context(), uid, r.PathValue("id")); err != nil {
		if err == service.ErrFamilyNotFound {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "成员不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// familyRelationsList GET /api/v1/family/relations
func (a *API) familyRelationsList(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	list, err := a.family.ListRelations(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"relations": list})
}

// familyRelationsCreate POST /api/v1/family/relations
func (a *API) familyRelationsCreate(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	var rel service.FamilyRelation
	if err := json.NewDecoder(r.Body).Decode(&rel); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	created, err := a.family.CreateRelation(r.Context(), uid, &rel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"relation": created})
}

// familyRelationsDelete DELETE /api/v1/family/relations/{id}
func (a *API) familyRelationsDelete(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	if err := a.family.DeleteRelation(r.Context(), uid, r.PathValue("id")); err != nil {
		if err == service.ErrFamilyNotFound {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "关系不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// familyTree GET /api/v1/family/tree —— 家族树/图数据。
func (a *API) familyTree(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	tree, err := a.family.Tree(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tree)
}

// familyTimeline GET /api/v1/family/timeline —— 一生时间轴（life_event 文件按 occurred_at 升序）。
func (a *API) familyTimeline(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	list, err := a.family.Timeline(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

// familyAnniversariesList GET /api/v1/family/anniversaries
func (a *API) familyAnniversariesList(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	list, err := a.family.ListAnniversaries(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"anniversaries": list})
}

// familyAnniversariesCreate POST /api/v1/family/anniversaries
func (a *API) familyAnniversariesCreate(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	var an service.FamilyAnniversary
	if err := json.NewDecoder(r.Body).Decode(&an); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	if an.Title == "" || an.Date == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "标题与日期不能为空")
		return
	}
	created, err := a.family.CreateAnniversary(r.Context(), uid, &an)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"anniversary": created})
}

// familyAnniversariesUpdate PUT /api/v1/family/anniversaries/{id}
func (a *API) familyAnniversariesUpdate(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	var an service.FamilyAnniversary
	if err := json.NewDecoder(r.Body).Decode(&an); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	updated, err := a.family.UpdateAnniversary(r.Context(), uid, r.PathValue("id"), &an)
	if err != nil {
		if err == service.ErrFamilyNotFound {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "纪念日不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"anniversary": updated})
}

// familyAnniversariesDelete DELETE /api/v1/family/anniversaries/{id}
func (a *API) familyAnniversariesDelete(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	if err := a.family.DeleteAnniversary(r.Context(), uid, r.PathValue("id")); err != nil {
		if err == service.ErrFamilyNotFound {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "纪念日不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}


// familyAdvise POST /api/v1/family/advise —— AI 人生参谋（二期①）。
// 入参 {question}：基于当前用户家族档案（成员/关系/纪念日/时间轴），
// 由 LLM 生成人生规划/纪念日策划/家族历史整理等参谋建议（markdown）。
func (a *API) familyAdvise(w http.ResponseWriter, r *http.Request) {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	var req struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", err.Error())
		return
	}
	if req.Question == "" {
		writeErr(w, http.StatusBadRequest, "BAD_QUESTION", "请描述你想咨询的人生/家族问题")
		return
	}
	ctx := r.Context()
	contextText, err := ai.BuildFamilyContext(ctx, a.family, uid, 20)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FAMILY_ERR", err.Error())
		return
	}
	const sys = "你是家族人生参谋，熟悉人生规划、家族传承、纪念日策划与家庭关系梳理。请基于提供的家族档案与用户问题，给出务实、可执行的建议：先给一句结论，再分点给建议与理由，最后给出可立即执行的第一步。语气温暖克制，不编造档案中不存在的成员或事件；档案信息不足时明确说明并给出引导性的提问建议。输出为 Markdown。"
	msgs := []ai.Msg{
		{Role: "system", Content: sys},
		{Role: "user", Content: "【家族档案】\n" + contextText + "\n\n【用户问题】\n" + req.Question},
	}
	res, err := a.ai.ChatJSON(ctx, msgs, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "AI_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"advice":  res.Content,
		"context": contextText,
	})
}
