// org.go 企业知识库组织模块 HTTP 入口（M0：组织树打标 + 岗位快照）。
// 路由经 CoreModules.Org 门控注册（org 为 nil 时 404=能力未启用，不 panic）。
// 权限：树/路径/设置读=登录用户；树管理/成员关系写=admin（组织负责人）。
package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// orgSettings GET /api/v1/org/settings —— 模块与子开关状态。
func (a *API) orgSettings(w http.ResponseWriter, r *http.Request) {
	enabled := a.cfg.GetString("org.enabled") == "true"
	writeJSON(w, http.StatusOK, map[string]any{
		"org": map[string]bool{
			"enabled":    enabled,
			"tree":       enabled && a.cfg.GetString("org.tree") == "true",
			"department": enabled && a.cfg.GetString("org.department") == "true",
			"transfer":   enabled && a.cfg.GetString("org.transfer") == "true",
		},
	})
}

// orgBlocked 组织模块总开关拦截（org.enabled）。返回 true 表示已写完 403 响应。
// 全部 org 读写端点（除 GET /org/settings 这个门控查询端点本身）都要先过此闸；
// 否则「总开关」只是摆设——实测 B14 前组织树建/改/删在总开关关闭时仍可写。
func (a *API) orgBlocked(w http.ResponseWriter) bool {
	if a.cfg != nil && a.cfg.GetString("org.enabled") == "true" {
		return false
	}
	writeErr(w, http.StatusForbidden, "ORG_DISABLED", "组织模块总开关未开启（org.enabled）")
	return true
}

// orgTreeEnabled 组织树子开关（需 org.enabled 同时开启）：管组织树维护（建/改/删）
// 与自动打标（service 层 EnsureOrgTag 链路同用此判定）。
func (a *API) orgTreeEnabled() bool {
	return a.cfg.GetString("org.enabled") == "true" && a.cfg.GetString("org.tree") == "true"
}

// orgDepartmentEnabled 部门空间子开关（需 org.enabled 同时开启）。
func (a *API) orgDepartmentEnabled() bool {
	return a.cfg.GetString("org.enabled") == "true" && a.cfg.GetString("org.department") == "true"
}

// orgPaidGate 付费门禁：放行条件（任一）——
//   (1) 站点管理员（owner/admin，内部豁免）；
//   (2) 站点级 feature:org 授权（Entitlement v0.1：license.key 含 feature:org scope，上游 REQ-008 口径，scope=feature:org / tier=pro）；
//   (3) 站点级 pro 授权（license.key 验签通过 → licenseEdition()=="pro"，对应上游 users.tier=pro 的站点级落地）。
// 三项均不满足 → 403 PAYMENT_REQUIRED。
//
// 模型说明：本 fork 采用站点级许可证模型，users 表无 per-user tier 列（原 users.tier 查询为潜伏 bug，已移除）；
// 上游口径的「users.tier=pro」在本 fork 落地为站点级 pro 授权（licenseEdition），org 付费能力以「站点授权」为粒度开放给全体登录用户。
func (a *API) orgPaidGate(w http.ResponseWriter, r *http.Request) bool {
	if a.isAdmin(r) {
		return true
	}
	// (2) 站点级 feature:org 授权（license 门禁接入点；上游 REQ-008 已确认 scope=feature:org、tier=pro）。
	if p, err := a.currentLicense(); err == nil && p.HasFeature("org") {
		return true
	}
	// (3) 站点级 pro 授权（对应上游 users.tier=pro；本 fork 无 per-user tier 列，按站点授权粒度落地）。
	if a.licenseEdition() == "pro" {
		return true
	}
	writeErr(w, http.StatusForbidden, "PAYMENT_REQUIRED", "该能力属于付费版（Pro），请升级后使用")
	return false
}

// ---- 部门空间（M1：付费；spaces.kind='team' + 组织节点绑定） ----

// orgDepartmentsCreate POST /api/v1/org/departments {node_id, name}：建部门空间（leader=当前用户）。
func (a *API) orgDepartmentsCreate(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgDepartmentEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_DEPT_DISABLED", "部门空间子开关未开启（org.department）")
		return
	}
	if !a.orgPaidGate(w, r) {
		return
	}
	var req struct {
		NodeID string `json:"node_id"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_BAD_REQ", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.NodeID == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "ORG_BAD_REQ", "node_id 与 name 必填")
		return
	}
	d, err := a.org.CreateDepartment(r.Context(), req.NodeID, req.Name, a.curUserID(r))
	if err != nil {
		if err == service.ErrNotFound {
			writeErr(w, http.StatusNotFound, "ORG_NODE_NOT_FOUND", "组织节点不存在")
			return
		}
		if err == service.ErrConflict {
			writeErr(w, http.StatusConflict, "ORG_DEPT_CONFLICT", "该组织节点已绑定部门空间")
			return
		}
		writeErr(w, http.StatusBadRequest, "ORG_DEPT_INVALID", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "department": d})
}

// orgDepartmentsList GET /api/v1/org/departments：当前用户可访问的部门空间列表。
func (a *API) orgDepartmentsList(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgDepartmentEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_DEPT_DISABLED", "部门空间子开关未开启（org.department）")
		return
	}
	list, err := a.org.Departments(r.Context(), a.curUserID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ORG_DEPT_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

// orgDepartmentsMembers POST /api/v1/org/departments/{id}/members {user_id|username, role}：
// 部门加成员（leader/editor 可加；空间级授权，不写岗位历史）。
func (a *API) orgDepartmentsMembers(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgDepartmentEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_DEPT_DISABLED", "部门空间子开关未开启（org.department）")
		return
	}
	if !a.orgPaidGate(w, r) {
		return
	}
	sid := r.PathValue("id")
	uid := a.curUserID(r)
	myRole, ok := a.spaceRole(r.Context(), uid, sid)
	if !ok {
		writeErr(w, http.StatusForbidden, "SPACE_FORBIDDEN", "无权访问该空间")
		return
	}
	if myRole != "owner" && myRole != "editor" {
		writeErr(w, http.StatusForbidden, "SPACE_INVITE_FORBIDDEN", "仅部门负责人（leader）/成员可邀请")
		return
	}
	var req struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
		Role     string `json:"role"` // editor|viewer
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	if req.Role != "editor" && req.Role != "viewer" {
		writeErr(w, http.StatusBadRequest, "BAD_MEMBER_ROLE", "成员角色仅支持 editor|viewer")
		return
	}
	target := strings.TrimSpace(req.UserID)
	if target == "" {
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" {
			writeErr(w, http.StatusBadRequest, "MEMBER_TARGET_MISSING", "请提供 user_id 或 username")
			return
		}
		if err := a.db.QueryRowContext(r.Context(),
			`SELECT id FROM users WHERE username=? AND status='active'`, req.Username).Scan(&target); err != nil {
			writeErr(w, http.StatusNotFound, "USER_NOT_FOUND", "用户不存在或未激活："+req.Username)
			return
		}
	}
	if target == a.homeOwnerID() {
		writeErr(w, http.StatusBadRequest, "INVITE_OWNER_SELF", "不能邀请站点 owner（owner 已拥有全站权限）")
		return
	}
	if target == uid {
		writeErr(w, http.StatusBadRequest, "INVITE_SELF", "不能邀请自己（你是空间 owner）")
		return
	}
	now := time.Now().Unix()
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO space_members (space_id, user_id, role, joined_at) VALUES (?,?,?,?)
		 ON CONFLICT(space_id, user_id) DO UPDATE SET role=excluded.role`,
		sid, target, req.Role, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "INVITE_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), uid, "org.department.member_add", sid,
		map[string]any{"user": target, "role": req.Role})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user_id": target, "role": req.Role})
}

// orgDepartmentsFiles GET /api/v1/org/departments/{id}/files：部门文件（按空间 + 组织节点聚合，成员可读）。
func (a *API) orgDepartmentsFiles(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgDepartmentEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_DEPT_DISABLED", "部门空间子开关未开启（org.department）")
		return
	}
	sid := r.PathValue("id")
	if _, ok := a.spaceRole(r.Context(), a.curUserID(r), sid); !ok {
		writeErr(w, http.StatusForbidden, "SPACE_FORBIDDEN", "无权访问该空间")
		return
	}
	// 复用文件列表：注入 ?space= 后走 filesList（成员可见，viewer 只读由写入侧守卫保障）
	r.URL.Query().Set("space", sid)
	a.filesList(w, r)
}

// orgTreeList GET /api/v1/org/tree —— 组织树全量节点（跨用户只读可见）。
func (a *API) orgTreeList(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	tree, err := a.org.ListTree(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ORG_TREE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tree})
}

// orgTreeCreate POST /api/v1/org/tree —— 建节点（admin）。
func (a *API) orgTreeCreate(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTreeEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TREE_DISABLED", "组织树子开关未开启（org.tree）")
		return
	}
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ORG_ADMIN_REQUIRED", "仅组织管理员可维护组织树")
		return
	}
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_BAD_REQ", err.Error())
		return
	}
	n, err := a.org.CreateNode(r.Context(), req.Name, req.ParentID)
	if err != nil {
		if err == service.ErrConflict {
			writeErr(w, http.StatusConflict, "ORG_NODE_CONFLICT", "同名组织节点已存在")
			return
		}
		if err == service.ErrNotFound {
			writeErr(w, http.StatusNotFound, "ORG_PARENT_NOT_FOUND", "父节点不存在")
			return
		}
		writeErr(w, http.StatusBadRequest, "ORG_NODE_INVALID", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "node": n})
}

// orgTreeUpdate PUT /api/v1/org/tree/{id} —— 改名/移动（admin；name 与 parent_id 均可选，至少一项）。
func (a *API) orgTreeUpdate(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTreeEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TREE_DISABLED", "组织树子开关未开启（org.tree）")
		return
	}
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ORG_ADMIN_REQUIRED", "仅组织管理员可维护组织树")
		return
	}
	id := r.PathValue("id")
	var req struct {
		Name     *string `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_BAD_REQ", err.Error())
		return
	}
	var n *service.OrgNode
	var err error
	if req.Name != nil && *req.Name != "" {
		n, err = a.org.RenameNode(r.Context(), id, *req.Name)
	} else if req.ParentID != nil {
		n, err = a.org.MoveNode(r.Context(), id, *req.ParentID)
	} else {
		writeErr(w, http.StatusBadRequest, "ORG_BAD_REQ", "至少提供 name 或 parent_id 之一")
		return
	}
	if err != nil {
		if err == service.ErrNotFound {
			writeErr(w, http.StatusNotFound, "ORG_NODE_NOT_FOUND", "组织节点不存在")
			return
		}
		if err == service.ErrConflict {
			writeErr(w, http.StatusConflict, "ORG_NODE_CONFLICT", "目标路径已存在")
			return
		}
		writeErr(w, http.StatusBadRequest, "ORG_NODE_INVALID", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "node": n})
}

// orgTreeDelete DELETE /api/v1/org/tree/{id} —— 删除节点（admin；有文件/成员时拒绝）。
func (a *API) orgTreeDelete(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTreeEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TREE_DISABLED", "组织树子开关未开启（org.tree）")
		return
	}
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ORG_ADMIN_REQUIRED", "仅组织管理员可维护组织树")
		return
	}
	if err := a.org.DeleteNode(r.Context(), r.PathValue("id")); err != nil {
		if err == service.ErrNotFound {
			writeErr(w, http.StatusNotFound, "ORG_NODE_NOT_FOUND", "组织节点不存在")
			return
		}
		writeErr(w, http.StatusConflict, "ORG_NODE_IN_USE", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// orgTreePaths GET /api/v1/org/tree/paths —— 可打标组织路径列表（打标弹窗）。
func (a *API) orgTreePaths(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	paths, err := a.org.Paths(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ORG_PATHS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": paths})
}

// orgMembershipsSet POST /api/v1/org/memberships —— 任职/调岗（admin；岗位变更唯一入口）。
func (a *API) orgMembershipsSet(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ORG_ADMIN_REQUIRED", "仅组织管理员可设置岗位")
		return
	}
	var req struct {
		UserID string `json:"user_id"`
		NodeID string `json:"node_id"`
		Since  int64  `json:"since"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_BAD_REQ", err.Error())
		return
	}
	if req.UserID == "" || req.NodeID == "" {
		writeErr(w, http.StatusBadRequest, "ORG_BAD_REQ", "user_id 与 node_id 必填")
		return
	}
	m, err := a.org.SetMembership(r.Context(), req.UserID, req.NodeID, req.Since)
	if err != nil {
		if err == service.ErrNotFound {
			writeErr(w, http.StatusNotFound, "ORG_NODE_NOT_FOUND", "组织节点不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "ORG_MEMBERSHIP_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "membership": m})
}

// orgNodeMembers GET /api/v1/org/members?node_id= —— 节点在任成员列表（登录用户可读）。
func (a *API) orgNodeMembers(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	nodeID := r.URL.Query().Get("node_id")
	if nodeID == "" {
		writeErr(w, http.StatusBadRequest, "ORG_BAD_REQ", "node_id 必填")
		return
	}
	members, err := a.org.NodeMembers(r.Context(), nodeID)
	if err != nil {
		if err == service.ErrNotFound {
			writeErr(w, http.StatusNotFound, "ORG_NODE_NOT_FOUND", "组织节点不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "ORG_MEMBERS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": members})
}

// orgMembershipsHistory GET /api/v1/org/memberships?user_id= —— 成员岗位历史。
// 权限：本人可查自己的；admin 可查任意用户。
func (a *API) orgMembershipsHistory(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	uid := r.URL.Query().Get("user_id")
	if uid == "" {
		uid = a.curUserID(r)
	}
	if uid != a.curUserID(r) && !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ORG_FORBIDDEN", "无权查看他人岗位历史")
		return
	}
	hist, err := a.org.History(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ORG_HISTORY_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": hist})
}

// ---- 移交流（M2：付费；责任人 custodian + 移交任务 预演/执行/确认/取消/归档） ----

// orgTransferEnabled 移交流子开关（需 org.enabled 同时开启）。
func (a *API) orgTransferEnabled() bool {
	return a.cfg.GetString("org.enabled") == "true" && a.cfg.GetString("org.transfer") == "true"
}

// orgTransfersCreate POST /api/v1/org/transfers {type, from_user, to_user, space_id?, node_ids?, file_ids?, note?}
// 发起移交（draft）。发起人=from 本人（admin 可代发起）。
func (a *API) orgTransfersCreate(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTransferEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TRANSFER_DISABLED", "移交流子开关未开启（org.transfer）")
		return
	}
	if !a.orgPaidGate(w, r) {
		return
	}
	var req struct {
		Type     string   `json:"type"`
		FromUser string   `json:"from_user"`
		ToUser   string   `json:"to_user"`
		SpaceID  string   `json:"space_id"`
		NodeIDs  []string `json:"node_ids"`
		FileIDs  []string `json:"file_ids"`
		Note     string   `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_BAD_REQ", err.Error())
		return
	}
	uid := a.curUserID(r)
	from := strings.TrimSpace(req.FromUser)
	if from == "" {
		from = uid
	}
	if from != uid && !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ORG_TRANSFER_FORBIDDEN", "仅可发起本人名下文件的移交（管理员可代发起）")
		return
	}
	if req.ToUser == "" {
		writeErr(w, http.StatusBadRequest, "ORG_BAD_REQ", "to_user 必填（接手人）")
		return
	}
	if req.Type == "" {
		req.Type = "transfer"
	}
	t, err := a.org.CreateTransfer(r.Context(), req.Type, from, req.ToUser, req.SpaceID, req.NodeIDs, req.FileIDs, req.Note, uid)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_TRANSFER_INVALID", err.Error())
		return
	}
	// 通知接手人
	_ = a.notify.AddUser(r.Context(), req.ToUser, "org.transfer", map[string]any{
		"title": "收到移交流", "message": "有文件移交待您处理（" + t.Type + "）", "link": "/#/org",
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "transfer": t})
}

// orgTransfersList GET /api/v1/org/transfers?scope=in|out
func (a *API) orgTransfersList(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTransferEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TRANSFER_DISABLED", "移交流子开关未开启（org.transfer）")
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope != "in" && scope != "out" {
		scope = "out"
	}
	list, err := a.org.ListTransfers(r.Context(), a.curUserID(r), scope)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ORG_TRANSFER_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

// orgTransferItems GET /api/v1/org/transfers/{id}/items
func (a *API) orgTransferItems(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTransferEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TRANSFER_DISABLED", "移交流子开关未开启（org.transfer）")
		return
	}
	items, err := a.org.TransferItems(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "ORG_TRANSFER_NOT_FOUND", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// orgTransferPreview POST /api/v1/org/transfers/{id}/preview：预演生成清单。
func (a *API) orgTransferPreview(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTransferEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TRANSFER_DISABLED", "移交流子开关未开启（org.transfer）")
		return
	}
	items, err := a.org.PreviewTransfer(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_TRANSFER_PREVIEW_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}

// orgTransferItemSkip POST /api/v1/org/transfers/{id}/items/{itemId} {skip:true|false}：勾选排除/恢复。
func (a *API) orgTransferItemSkip(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTransferEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TRANSFER_DISABLED", "移交流子开关未开启（org.transfer）")
		return
	}
	var req struct {
		Skip bool `json:"skip"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := a.org.SetTransferItemSkipped(r.Context(), r.PathValue("id"), r.PathValue("itemId"), req.Skip); err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_TRANSFER_ITEM_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skipped": req.Skip})
}

// orgTransferExecute POST /api/v1/org/transfers/{id}/execute
func (a *API) orgTransferExecute(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTransferEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TRANSFER_DISABLED", "移交流子开关未开启（org.transfer）")
		return
	}
	id := r.PathValue("id")
	if err := a.org.ExecuteTransfer(r.Context(), id); err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_TRANSFER_EXECUTE_FAILED", err.Error())
		return
	}
	// 通知接手人待确认
	t, _ := a.org.Transfer(r.Context(), id)
	if t != nil {
		_ = a.notify.AddUser(r.Context(), t.ToUser, "org.transfer", map[string]any{
			"title": "移交待确认", "message": "文件移交已执行，请确认接收", "link": "/#/org",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// orgTransferAccept PUT /api/v1/org/transfers/{id}/accept：接收人确认（或 admin）。
func (a *API) orgTransferAccept(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTransferEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TRANSFER_DISABLED", "移交流子开关未开启（org.transfer）")
		return
	}
	id := r.PathValue("id")
	if err := a.org.AcceptTransfer(r.Context(), id, a.curUserID(r), a.isAdmin(r)); err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_TRANSFER_ACCEPT_FAILED", err.Error())
		return
	}
	t, _ := a.org.Transfer(r.Context(), id)
	if t != nil {
		_ = a.notify.AddUser(r.Context(), t.FromUser, "org.transfer", map[string]any{
			"title": "移交已完成", "message": "移交（" + t.Type + "）已被接收人确认归档", "link": "/#/org",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// orgTransferCancel POST /api/v1/org/transfers/{id}/cancel
func (a *API) orgTransferCancel(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	if !a.orgTransferEnabled() {
		writeErr(w, http.StatusForbidden, "ORG_TRANSFER_DISABLED", "移交流子开关未开启（org.transfer）")
		return
	}
	id := r.PathValue("id")
	if err := a.org.CancelTransfer(r.Context(), id, a.curUserID(r), a.isAdmin(r)); err != nil {
		writeErr(w, http.StatusBadRequest, "ORG_TRANSFER_CANCEL_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// orgCustodianCount GET /api/v1/org/custodian-count：当前用户名下文件数（发起移交表单提示）。
func (a *API) orgCustodianCount(w http.ResponseWriter, r *http.Request) {
	if a.orgBlocked(w) {
		return
	}
	n, err := a.org.MyCustodianCount(r.Context(), a.curUserID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ORG_COUNT_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": n})
}
