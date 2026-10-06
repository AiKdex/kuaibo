// skill.go 技能包端点（B9）。
//
//	GET  /api/v1/skills                    已上架技能包目录（可按 kind 过滤）
//	GET  /api/v1/skills/grants              我的授权（当前用户维度）
//	POST /api/v1/skills/{id}/claim          自助领取免费包
//	POST /api/v1/admin/skills               上架/更新技能包（owner/admin）
//	POST /api/v1/admin/skills/{id}/grant    手工发放授权（owner/admin）
//
// 与「应用中心」的边界：应用中心管站点能力（主题/插件/站点授权，落 site_licenses），
// 技能包管可授权能力资产（落 skill_grants，grantee 可为 user/space/instance）。
// 支付通道与结算**不在本批**（商业化运营动作）。
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// skillsList GET /api/v1/skills?kind=&limit=
func (a *API) skillsList(w http.ResponseWriter, r *http.Request) {
	if a.skills == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "kinds": service.SkillKinds})
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	if kind != "" && !service.ValidSkillKind(kind) {
		writeErr(w, http.StatusBadRequest, "SKILL_BAD_KIND", "kind 仅支持 tool / skill / workflow / extension")
		return
	}
	// 与既有端点同惯例：解析失败/越界由 store 侧兜底为默认值
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := a.skills.ListPackages(r.Context(), kind, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SKILL_LIST_FAILED", err.Error())
		return
	}
	uid := a.curUserID(r)
	// 标注「我是否已持有」（前端据此显示"已领取"）
	type itemWithGrant struct {
		service.SkillPackage
		Granted bool `json:"granted"`
	}
	out := make([]itemWithGrant, 0, len(items))
	for _, p := range items {
		out = append(out, itemWithGrant{SkillPackage: p, Granted: a.skills.HasGrant(r.Context(), p.ID, "user", uid)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "kinds": service.SkillKinds})
}

// skillsGrants GET /api/v1/skills/grants
func (a *API) skillsGrants(w http.ResponseWriter, r *http.Request) {
	if a.skills == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	items, err := a.skills.ListGrants(r.Context(), "user", a.curUserID(r))
	if err != nil {
		if errors.Is(err, service.ErrSkillBadGrantee) {
			writeErr(w, http.StatusBadRequest, "SKILL_BAD_GRANTEE", err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "SKILL_GRANT_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// skillsClaim POST /api/v1/skills/{id}/claim —— 仅免费包可自助领取。
func (a *API) skillsClaim(w http.ResponseWriter, r *http.Request) {
	if a.skills == nil {
		writeErr(w, http.StatusServiceUnavailable, "SKILLS_DISABLED", "技能包模块未启用")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeErr(w, http.StatusBadRequest, "SKILL_BAD_ID", "缺少技能包 id")
		return
	}
	uid := a.curUserID(r)
	if err := a.skills.Claim(r.Context(), id, "user", uid); err != nil {
		switch {
		case errors.Is(err, service.ErrSkillNotFound):
			writeErr(w, http.StatusNotFound, "SKILL_NOT_FOUND", err.Error())
		case errors.Is(err, service.ErrSkillNotPub):
			writeErr(w, http.StatusForbidden, "SKILL_NOT_PUBLISHED", err.Error())
		case errors.Is(err, service.ErrSkillNotFree):
			writeErr(w, http.StatusPaymentRequired, "SKILL_NOT_FREE", err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, "SKILL_CLAIM_FAILED", err.Error())
		}
		return
	}
	_, _ = a.aud.Append(r.Context(), uid, "skill.claim", "skill_packages", map[string]any{"package_id": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "package_id": id})
}

// adminSkillUpsert POST /api/v1/admin/skills —— 上架/更新（id 为空则新建）。
func (a *API) adminSkillUpsert(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "FORBIDDEN", "仅 owner/admin 可上架技能包")
		return
	}
	if a.skills == nil {
		writeErr(w, http.StatusServiceUnavailable, "SKILLS_DISABLED", "技能包模块未启用")
		return
	}
	var req struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Version     string `json:"version"`
		Kind        string `json:"kind"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Manifest    string `json:"manifest"`
		Status      string `json:"status"`
		PriceCents  int64  `json:"price_cents"`
		License     string `json:"license"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "SKILL_BAD_BODY", "请求体格式错误")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "SKILL_BAD_NAME", "name 必填")
		return
	}
	if !service.ValidSkillKind(req.Kind) {
		writeErr(w, http.StatusBadRequest, "SKILL_BAD_KIND", "kind 仅支持 tool / skill / workflow / extension")
		return
	}
	p := &service.SkillPackage{
		ID: req.ID, Name: req.Name, Version: req.Version, Kind: req.Kind,
		Title: req.Title, Description: req.Description, Manifest: req.Manifest,
		AuthorID: a.curUserID(r), Status: req.Status,
		PriceCents: req.PriceCents, License: req.License,
	}
	if err := a.skills.UpsertPackage(r.Context(), p); err != nil {
		if errors.Is(err, service.ErrSkillNotFound) {
			writeErr(w, http.StatusNotFound, "SKILL_NOT_FOUND", err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "SKILL_UPSERT_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "skill.upsert", "skill_packages", map[string]any{
		"package_id": p.ID, "status": p.Status, "price_cents": p.PriceCents,
	})
	writeJSON(w, http.StatusOK, p)
}

// adminSkillGrant POST /api/v1/admin/skills/{id}/grant {grantee_type?,grantee_id?,source?,expires_at?}
func (a *API) adminSkillGrant(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "FORBIDDEN", "仅 owner/admin 可发放授权")
		return
	}
	if a.skills == nil {
		writeErr(w, http.StatusServiceUnavailable, "SKILLS_DISABLED", "技能包模块未启用")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeErr(w, http.StatusBadRequest, "SKILL_BAD_ID", "缺少技能包 id")
		return
	}
	var req struct {
		GranteeType string `json:"grantee_type"`
		GranteeID   string `json:"grantee_id"`
		Source      string `json:"source"`
		ExpiresAt   int64  `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "SKILL_BAD_BODY", "请求体格式错误")
		return
	}
	if req.GranteeType == "" {
		req.GranteeType = "user"
	}
	if req.GranteeID == "" {
		req.GranteeID = a.curUserID(r)
	}
	if err := a.skills.Grant(r.Context(), id, req.GranteeType, req.GranteeID, req.Source, req.ExpiresAt); err != nil {
		switch {
		case errors.Is(err, service.ErrSkillNotFound):
			writeErr(w, http.StatusNotFound, "SKILL_NOT_FOUND", err.Error())
		case errors.Is(err, service.ErrSkillBadGrantee):
			writeErr(w, http.StatusBadRequest, "SKILL_BAD_GRANTEE", err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, "SKILL_GRANT_FAILED", err.Error())
		}
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "skill.grant", "skill_grants", map[string]any{
		"package_id": id, "grantee_type": req.GranteeType, "grantee_id": req.GranteeID,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
