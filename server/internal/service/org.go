// Package service 业务编排层。
// org.go 企业知识库组织模块（M0：组织树打标 + 岗位快照）。
// 两套正交体系：空间体系（spaces，权限边界）与标签体系（tags，语义归属）。
// 组织树节点 = tags.kind='org' + owner_id=OrgOwner（'org' 保留字），独立查询路径，不改现有 tags.List 语义。
// 岗位快照：文件上传时固化「当时岗位」组织路径（file_tags），调岗不回溯、永不回扫。
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/google/uuid"
)

// OrgOwner 组织树节点标签的统一 owner_id 保留字（不与真实用户冲突；跨用户只读可见）。
const OrgOwner = "org"

// OrgNode 组织树节点（tags 表中 kind='org' 的行）。
type OrgNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	ParentID  string `json:"parent_id,omitempty"`
	MemberCnt int    `json:"member_count"` // 在任成员数
	FileCnt   int    `json:"file_count"`   // 挂载文件数（file_tags 关联）
	CreatedAt int64  `json:"created_at"`
}

// OrgMembership 组织成员关系（岗位记录）。
type OrgMembership struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	NodeID string `json:"node_id"`
	Path   string `json:"path,omitempty"` // 冗余展示：节点路径
	Since  int64  `json:"since"`
	Until  int64  `json:"until,omitempty"`
	Role   string `json:"role"`
}

// OrgStore 组织模块服务：树管理 + 成员关系 + 上传快照打标。
type OrgStore struct {
	db  *sql.DB
	b   *bus.Bus
	aud *AuditStore
	cfg interface{ GetString(string) string }
}

// NewOrgStore 创建组织模块服务。
func NewOrgStore(db *sql.DB, b *bus.Bus, aud *AuditStore, cfg interface{ GetString(string) string }) *OrgStore {
	return &OrgStore{db: db, b: b, aud: aud, cfg: cfg}
}

// Enabled 组织模块总开关（settings org.enabled）。
func (s *OrgStore) Enabled() bool {
	return s.cfg.GetString("org.enabled") == "true"
}

// TreeEnabled 组织树打标子开关（settings org.tree；总开关关闭时视同关闭）。
func (s *OrgStore) TreeEnabled() bool {
	return s.Enabled() && s.cfg.GetString("org.tree") == "true"
}

// ---- 组织树 ----

// ListTree 组织树全量节点（按 path 排序），附在任成员数与文件数。
func (s *OrgStore) ListTree(ctx context.Context) ([]*OrgNode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.name, t.path, t.parent_id, t.created_at,
		        (SELECT COUNT(*) FROM org_memberships om WHERE om.node_id=t.id AND om.until IS NULL) AS mc,
		        (SELECT COUNT(*) FROM file_tags ft WHERE ft.tag_id=t.id) AS fc
		 FROM tags t WHERE t.owner_id=? AND t.kind='org' ORDER BY t.path COLLATE NOCASE`, OrgOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*OrgNode{}
	for rows.Next() {
		var n OrgNode
		var pid sql.NullString
		if err := rows.Scan(&n.ID, &n.Name, &n.Path, &pid, &n.CreatedAt, &n.MemberCnt, &n.FileCnt); err != nil {
			return nil, err
		}
		n.ParentID = pid.String
		out = append(out, &n)
	}
	return out, rows.Err()
}

// Node 取单个组织节点。
func (s *OrgStore) Node(ctx context.Context, id string) (*OrgNode, error) {
	var n OrgNode
	var pid sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT t.id, t.name, t.path, t.parent_id, t.created_at,
		        (SELECT COUNT(*) FROM org_memberships om WHERE om.node_id=t.id AND om.until IS NULL),
		        (SELECT COUNT(*) FROM file_tags ft WHERE ft.tag_id=t.id)
		 FROM tags t WHERE t.id=? AND t.owner_id=? AND t.kind='org'`,
		id, OrgOwner).Scan(&n.ID, &n.Name, &n.Path, &pid, &n.CreatedAt, &n.MemberCnt, &n.FileCnt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	n.ParentID = pid.String
	return &n, nil
}

// Paths 可打标组织路径列表（打标弹窗「从组织树选择路径」）。
func (s *OrgStore) Paths(ctx context.Context) ([]map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, path FROM tags WHERE owner_id=? AND kind='org' ORDER BY path COLLATE NOCASE`, OrgOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		out = append(out, map[string]string{"id": id, "path": path})
	}
	return out, rows.Err()
}

// CreateNode 建组织树节点（kind='org'，owner_id='org'；path 由父级拼接）。
func (s *OrgStore) CreateNode(ctx context.Context, name, parentID string) (*OrgNode, error) {
	if name == "" || strings.ContainsAny(name, "/") {
		return nil, errors.New("service: invalid org node name")
	}
	var path string
	if parentID != "" {
		var p string
		if err := s.db.QueryRowContext(ctx,
			`SELECT path FROM tags WHERE id=? AND owner_id=? AND kind='org'`, parentID, OrgOwner).Scan(&p); err != nil {
			return nil, ErrNotFound
		}
		path = p + "/" + name
	} else {
		path = name
	}
	var dup int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tags WHERE owner_id=? AND path=?`, OrgOwner, path).Scan(&dup)
	if dup > 0 {
		return nil, ErrConflict
	}
	id := uuid.NewString()
	ts := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO tags (id, name, path, parent_id, owner_id, kind, created_at) VALUES (?,?,?,?,?, 'org', ?)`,
		id, name, path, nullIfEmpty(parentID), OrgOwner, ts); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "org.node_created", Key: id, Data: map[string]any{"name": name, "path": path}})
	_, _ = s.aud.Append(ctx, OrgOwner, "org.node_create", id, map[string]any{"name": name, "path": path})
	return s.Node(ctx, id)
}

// RenameNode 改名组织节点，同步更新自身与全部后代 path 前缀。
func (s *OrgStore) RenameNode(ctx context.Context, id, newName string) (*OrgNode, error) {
	if newName == "" || strings.ContainsAny(newName, "/") {
		return nil, errors.New("service: invalid org node name")
	}
	n, err := s.Node(ctx, id)
	if err != nil {
		return nil, err
	}
	prefix := ""
	if n.ParentID != "" {
		var p string
		if err := s.db.QueryRowContext(ctx, `SELECT path FROM tags WHERE id=?`, n.ParentID).Scan(&p); err != nil {
			return nil, err
		}
		prefix = p + "/"
	}
	newPath := prefix + newName
	var dup int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tags WHERE owner_id=? AND path=? AND id<>?`, OrgOwner, newPath, id).Scan(&dup)
	if dup > 0 {
		return nil, ErrConflict
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE tags SET name=?, path=? WHERE id=?`, newName, newPath, id); err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE tags SET path=replace(path, ?||'/', ?||'/') WHERE owner_id=? AND kind='org' AND path LIKE ?||'/%'`,
		n.Path, newPath, OrgOwner, n.Path); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "org.node_renamed", Key: id, Data: map[string]any{"from": n.Path, "to": newPath}})
	_, _ = s.aud.Append(ctx, OrgOwner, "org.node_rename", id, map[string]any{"from": n.Path, "to": newPath})
	return s.Node(ctx, id)
}

// MoveNode 移动组织节点到新父级（含根），同步后代 path。
func (s *OrgStore) MoveNode(ctx context.Context, id, newParentID string) (*OrgNode, error) {
	n, err := s.Node(ctx, id)
	if err != nil {
		return nil, err
	}
	if newParentID != "" {
		if newParentID == id {
			return nil, errors.New("service: cannot move node into itself")
		}
		var np string
		if err := s.db.QueryRowContext(ctx,
			`SELECT path FROM tags WHERE id=? AND owner_id=? AND kind='org'`, newParentID, OrgOwner).Scan(&np); err != nil {
			return nil, ErrNotFound
		}
		// 防环：新父级不能是自己的后代（path 前缀判断）
		if np == n.Path || strings.HasPrefix(np, n.Path+"/") {
			return nil, errors.New("service: cannot move node under its own subtree")
		}
	}
	// 计算新路径（新父路径 + 自身 name）
	var np string
	if newParentID != "" {
		if err := s.db.QueryRowContext(ctx, `SELECT path FROM tags WHERE id=?`, newParentID).Scan(&np); err != nil {
			return nil, ErrNotFound
		}
		np = np + "/" + n.Name
	} else {
		np = n.Name
	}
	var dup int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tags WHERE owner_id=? AND path=? AND id<>?`, OrgOwner, np, id).Scan(&dup)
	if dup > 0 {
		return nil, ErrConflict
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE tags SET parent_id=?, path=? WHERE id=?`, nullIfEmpty(newParentID), np, id); err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE tags SET path=replace(path, ?||'/', ?||'/') WHERE owner_id=? AND kind='org' AND path LIKE ?||'/%'`,
		n.Path, np, OrgOwner, n.Path); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "org.node_moved", Key: id, Data: map[string]any{"from": n.Path, "to": np}})
	_, _ = s.aud.Append(ctx, OrgOwner, "org.node_move", id, map[string]any{"from": n.Path, "to": np, "parent": newParentID})
	return s.Node(ctx, id)
}

// DeleteNode 删除组织节点（含后代）；有文件挂载或任一在任成员时拒绝。
func (s *OrgStore) DeleteNode(ctx context.Context, id string) error {
	n, err := s.Node(ctx, id)
	if err != nil {
		return err
	}
	var files, members int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_tags ft JOIN tags t ON t.id=ft.tag_id WHERE t.owner_id=? AND t.kind='org' AND (t.id=? OR t.path LIKE ?||'/%')`,
		OrgOwner, id, n.Path).Scan(&files)
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM org_memberships om JOIN tags t ON t.id=om.node_id WHERE t.owner_id=? AND t.kind='org' AND om.until IS NULL AND (t.id=? OR t.path LIKE ?||'/%')`,
		OrgOwner, id, n.Path).Scan(&members)
	if files > 0 {
		return errors.New("service: org node has tagged files")
	}
	if members > 0 {
		return errors.New("service: org node has active members")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM org_memberships WHERE node_id IN (SELECT t2.id FROM tags t2 WHERE t2.owner_id=? AND t2.kind='org' AND (t2.id=? OR t2.path LIKE ?||'/%'))`,
		OrgOwner, id, n.Path); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM tags WHERE owner_id=? AND kind='org' AND (id=? OR path LIKE ?||'/%')`,
		OrgOwner, id, n.Path); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.b.Publish(ctx, bus.Event{Topic: "org.node_deleted", Key: id, Data: map[string]any{"path": n.Path}})
	_, _ = s.aud.Append(ctx, OrgOwner, "org.node_delete", id, map[string]any{"path": n.Path})
	return nil
}

// ---- 成员关系（岗位历史；岗位变更唯一入口） ----

// SetMembership 任职/调岗：关闭旧在任记录（until=now），写入新记录（since 缺省=now）。
// 调岗不触发任何文件标签回扫（快照原则）。同时同步部门空间成员（§3.1 接缝②）：
// 旧岗位绑定 team 空间移除成员、新岗位绑定 team 空间加入（editor），空间不存在/未绑定则跳过。
func (s *OrgStore) SetMembership(ctx context.Context, userID, nodeID string, since int64) (*OrgMembership, error) {
	if since <= 0 {
		since = time.Now().UnixMilli()
	}
	if _, err := s.Node(ctx, nodeID); err != nil {
		return nil, err
	}
	// 取旧在任节点（无旧岗位则空字符串，跳过移除）
	var oldNodeID string
	_ = s.db.QueryRowContext(ctx,
		`SELECT node_id FROM org_memberships WHERE user_id=? AND until IS NULL ORDER BY since DESC LIMIT 1`, userID).Scan(&oldNodeID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_memberships SET until=? WHERE user_id=? AND until IS NULL`, since-1, userID); err != nil {
		return nil, err
	}
	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO org_memberships (id, user_id, node_id, since, role) VALUES (?,?,?,?,'member')`,
		id, userID, nodeID, since); err != nil {
		return nil, err
	}
	if err := s.syncDeptMembershipTx(ctx, tx, userID, oldNodeID, nodeID, time.Now().Unix()); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "org.membership_changed", Key: userID, Data: map[string]any{"node_id": nodeID, "since": since}})
	_, _ = s.aud.Append(ctx, userID, "org.membership_set", nodeID, map[string]any{"since": since})
	return s.Membership(ctx, id)
}

// Membership 取单条成员关系。
func (s *OrgStore) Membership(ctx context.Context, id string) (*OrgMembership, error) {
	var m OrgMembership
	var until sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT m.id, m.user_id, m.node_id, t.path, m.since, m.until, m.role
		 FROM org_memberships m JOIN tags t ON t.id=m.node_id WHERE m.id=?`, id).
		Scan(&m.ID, &m.UserID, &m.NodeID, &m.Path, &m.Since, &until, &m.Role)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if until.Valid {
		m.Until = until.Int64
	}
	return &m, nil
}

// NodeMembers 某组织节点的在任成员（含用户名/昵称；用于管理页成员面板）。
func (s *OrgStore) NodeMembers(ctx context.Context, nodeID string) ([]map[string]any, error) {
	if _, err := s.Node(ctx, nodeID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.user_id, u.username, u.display_name, m.since
		 FROM org_memberships m JOIN users u ON u.id=m.user_id
		 WHERE m.node_id=? AND m.until IS NULL ORDER BY m.since DESC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var uid, username, display string
		var since int64
		if err := rows.Scan(&uid, &username, &display, &since); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"user_id": uid, "username": username, "display_name": display, "since": since})
	}
	return out, rows.Err()
}

// History 成员岗位历史（倒序）。
func (s *OrgStore) History(ctx context.Context, userID string) ([]*OrgMembership, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.user_id, m.node_id, t.path, m.since, m.until, m.role
		 FROM org_memberships m JOIN tags t ON t.id=m.node_id
		 WHERE m.user_id=? ORDER BY m.since DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*OrgMembership{}
	for rows.Next() {
		var m OrgMembership
		var until sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &m.NodeID, &m.Path, &m.Since, &until, &m.Role); err != nil {
			return nil, err
		}
		if until.Valid {
			m.Until = until.Int64
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// CurrentNode 取用户当前在任组织节点（无在任岗位返回 nil,nil）。
func (s *OrgStore) CurrentNode(ctx context.Context, userID string) (*OrgNode, error) {
	var nodeID string
	err := s.db.QueryRowContext(ctx,
		`SELECT node_id FROM org_memberships WHERE user_id=? AND until IS NULL ORDER BY since DESC LIMIT 1`, userID).Scan(&nodeID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.Node(ctx, nodeID)
}

// syncDeptMembershipTx 调岗时同步部门空间成员（§3.1 接缝②）：
// 旧节点绑定 team 空间移除成员、新节点绑定 team 空间加入（role=editor，可写）。
// 空间不存在/未绑定则跳过（无部门空间的组织节点不影响岗位记录）。
func (s *OrgStore) syncDeptMembershipTx(ctx context.Context, tx *sql.Tx, userID, oldNodeID, newNodeID string, now int64) error {
	if oldNodeID != "" {
		var oldSpace string
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM spaces WHERE kind='team' AND node_id=?`, oldNodeID).Scan(&oldSpace); err == nil {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM space_members WHERE space_id=? AND user_id=?`, oldSpace, userID); err != nil {
				return err
			}
		}
	}
	var newSpace string
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM spaces WHERE kind='team' AND node_id=?`, newNodeID).Scan(&newSpace); err == nil {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO space_members (space_id, user_id, role, joined_at) VALUES (?,?,'editor',?)`,
			newSpace, userID, now); err != nil {
			return err
		}
	}
	return nil
}

// ---- 部门空间（M1：spaces.kind='team' + node_id 绑定组织节点） ----

// Department 部门空间（owner=部门负责人 leader；成员 role 复用 editor|viewer）。
type Department struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	NodeID    string `json:"node_id"`
	NodePath  string `json:"node_path"`
	LeaderID  string `json:"leader_id"`
	Leader    string `json:"leader_name"`
	Role      string `json:"role"` // owner|editor|viewer（当前用户视角）
	CreatedAt int64  `json:"created_at"`
}

// Departments 当前用户可访问的部门空间列表（我拥有 + 我加入；按 node_path 排序）。
func (s *OrgStore) Departments(ctx context.Context, userID string) ([]*Department, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.name, s.node_id, t.path, s.owner_id, u.username,
		       CASE WHEN s.owner_id=? THEN 'owner' ELSE m.role END AS role, s.created_at
		FROM spaces s
		JOIN users u ON u.id=s.owner_id
		LEFT JOIN space_members m ON m.space_id=s.id AND m.user_id=?
		LEFT JOIN tags t ON t.id=s.node_id
		WHERE s.kind='team' AND (s.owner_id=? OR m.user_id IS NOT NULL)
		ORDER BY t.path COLLATE NOCASE, s.name`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Department{}
	for rows.Next() {
		var d Department
		var nodePath sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &d.NodeID, &nodePath, &d.LeaderID, &d.Leader, &d.Role, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.NodePath = nodePath.String
		out = append(out, &d)
	}
	return out, rows.Err()
}

// CreateDepartment 建部门空间（kind='team'，owner=当前用户即部门负责人 leader；绑定组织节点，唯一）。
func (s *OrgStore) CreateDepartment(ctx context.Context, nodeID, name, ownerID string) (*Department, error) {
	if name == "" || len(name) > 64 {
		return nil, errors.New("service: invalid department name")
	}
	if _, err := s.Node(ctx, nodeID); err != nil {
		return nil, err
	}
	var dup int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM spaces WHERE kind='team' AND node_id=?`, nodeID).Scan(&dup)
	if dup > 0 {
		return nil, ErrConflict
	}
	sid := uuid.NewString()
	ts := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO spaces (id, owner_id, name, kind, node_id, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		sid, ownerID, name, "team", nodeID, ts, ts); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "org.department_created", Key: sid,
		Data: map[string]any{"node_id": nodeID, "name": name, "leader_id": ownerID}})
	_, _ = s.aud.Append(ctx, ownerID, "org.department_create", sid,
		map[string]any{"node_id": nodeID, "name": name})
	d, err := s.Department(ctx, sid, ownerID)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// Department 取单个部门空间（含当前用户角色）。
func (s *OrgStore) Department(ctx context.Context, id, userID string) (*Department, error) {
	var d Department
	var nodePath sql.NullString
	var myRole sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.name, s.node_id, t.path, s.owner_id, u.username,
		       m.role, s.created_at
		FROM spaces s
		JOIN users u ON u.id=s.owner_id
		LEFT JOIN space_members m ON m.space_id=s.id AND m.user_id=?
		LEFT JOIN tags t ON t.id=s.node_id
		WHERE s.id=? AND s.kind='team'`, userID, id).
		Scan(&d.ID, &d.Name, &d.NodeID, &nodePath, &d.LeaderID, &d.Leader, &myRole, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.NodePath = nodePath.String
	if d.LeaderID == userID {
		d.Role = "owner"
	} else {
		d.Role = myRole.String
	}
	return &d, nil
}

// ---- 岗位快照打标（上传即固化当时岗位组织路径） ----

// SubscribeSnapshot 订阅 file.created：新文件上传/创建时，若上传者在任且 org 打标开启，
// 自动把「当时岗位」组织路径标签绑定到文件（快照；调岗不回扫）。
// 复制产生的 file.created 无 user_id 时跳过（保留原标签语义）。
func (s *OrgStore) SubscribeSnapshot(b *bus.Bus) func() {
	return b.Subscribe("file.created", func(ctx context.Context, e bus.Event) error {
		if e.Key == "" || e.Data == nil {
			return nil
		}
		kind, _ := e.Data["kind"].(string)
		if kind != "file" {
			return nil
		}
		uid, _ := e.Data["user_id"].(string)
		if uid == "" {
			return nil // 复制/系统创建无上传者，不打岗
		}
		if !s.TreeEnabled() {
			return nil
		}
		node, err := s.CurrentNode(ctx, uid)
		if err != nil || node == nil {
			return nil // 无在任岗位不打标（非企业场景）
		}
		// 绑定组织路径标签（EnsureOrgTagPath 逐级创建，org 域全局）
		path := strings.Trim(node.Path, "/ ")
		if path == "" {
			return nil
		}
		end, err := s.EnsureOrgTagPath(ctx, path)
		if err != nil {
			return err
		}
		return s.bindOrgTag(ctx, uid, e.Key, end.ID)
	})
}

// EnsureOrgTagPath 按组织路径逐级创建 org 标签（已存在则复用），返回末端标签 id。
func (s *OrgStore) EnsureOrgTagPath(ctx context.Context, path string) (*OrgNode, error) {
	path = strings.Trim(path, "/ ")
	if path == "" {
		return nil, errors.New("service: empty org path")
	}
	segs := strings.Split(path, "/")
	var parentID string
	cur := ""
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if cur == "" {
			cur = seg
		} else {
			cur = cur + "/" + seg
		}
		var id string
		if err := s.db.QueryRowContext(ctx,
			`SELECT id FROM tags WHERE owner_id=? AND kind='org' AND path=?`, OrgOwner, cur).Scan(&id); err == nil {
			parentID = id
			continue
		}
		t, err := s.CreateNode(ctx, seg, parentID)
		if err != nil {
			return nil, err
		}
		parentID = t.ID
	}
	return s.Node(ctx, parentID)
}

// bindOrgTag 把组织标签绑定到文件（合并写入，不覆盖用户已有标签）。
func (s *OrgStore) bindOrgTag(ctx context.Context, ownerID, fileID, tagID string) error {
	cur, err := s.fileTagIDs(ctx, fileID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range cur {
		seen[id] = true
	}
	if !seen[tagID] {
		seen[tagID] = true
		cur = append(cur, tagID)
	}
	return s.setFileTagsRaw(ctx, ownerID, fileID, cur)
}

// fileTagIDs 取文件全部标签 id。
func (s *OrgStore) fileTagIDs(ctx context.Context, fileID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tag_id FROM file_tags WHERE file_id=?`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// setFileTagsRaw 全量替换文件标签（org 域放行逻辑，与 TagStore.SetFileTags 一致）。
func (s *OrgStore) setFileTagsRaw(ctx context.Context, ownerID, fileID string, tagIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_tags WHERE file_id=?`, fileID); err != nil {
		return err
	}
	for _, tid := range tagIDs {
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT owner_id FROM tags WHERE id=?`, tid).Scan(&owner); err != nil {
			continue
		}
		if owner != ownerID && owner != OrgOwner {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO file_tags (file_id, tag_id) VALUES (?,?)`, fileID, tid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- 移交流（M2：责任人 custodian + 移交任务 预演/执行/确认/回滚/完成钩子） ----

// TransferEnabled 移交流子开关（付费；settings org.transfer；总开关关闭时视同关闭）。
func (s *OrgStore) TransferEnabled() bool {
	return s.Enabled() && s.cfg.GetString("org.transfer") == "true"
}

// Transfer 移交任务主单。
type Transfer struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"` // transfer(调岗) | resign(离职)
	FromUser    string   `json:"from_user"`
	ToUser      string   `json:"to_user"`
	FromName    string   `json:"from_name,omitempty"`
	ToName      string   `json:"to_name,omitempty"`
	SpaceID     string   `json:"space_id,omitempty"`
	NodeIDs     []string `json:"node_ids,omitempty"`
	FileIDs     []string `json:"file_ids,omitempty"`
	Status      string   `json:"status"` // draft|previewed|executed|accepted|archived|cancelled
	Note        string   `json:"note,omitempty"`
	CreatedBy   string   `json:"created_by"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
	ExecutedAt  int64    `json:"executed_at,omitempty"`
	AcceptedAt  int64    `json:"accepted_at,omitempty"`
	CompletedAt int64    `json:"completed_at,omitempty"`
}

// TransferItem 移交条目（预演/执行明细；条目级状态，失败可定位可重试）。
type TransferItem struct {
	ID           string `json:"id"`
	TransferID   string `json:"transfer_id"`
	AssetType    string `json:"asset_type"` // file|space|membership|share
	AssetID      string `json:"asset_id"`
	AssetName    string `json:"asset_name"`
	Action       string `json:"action"` // transfer|revoke|archive|remove
	TargetUserID string `json:"target_user_id,omitempty"`
	Status       string `json:"status"` // pending|done|failed|skipped
	Note         string `json:"note,omitempty"`
	DoneAt       int64  `json:"done_at,omitempty"`
}

func jsonStrings(ids []string) string {
	if ids == nil {
		ids = []string{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

func parseJSONStrings(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// CreateTransfer 发起移交（status=draft）。actor 为发起人；to_user 须存在且 active。
// 范围三选一或组合：file_ids（指定文件）/ node_ids（组织节点下文件）/ space_id（某空间全量）。
func (s *OrgStore) CreateTransfer(ctx context.Context, typ, fromUser, toUser, spaceID string, nodeIDs, fileIDs []string, note, createdBy string) (*Transfer, error) {
	if typ != "transfer" && typ != "resign" {
		return nil, errors.New("service: invalid transfer type")
	}
	var uname string
	if err := s.db.QueryRowContext(ctx,
		`SELECT username FROM users WHERE id=? AND status='active'`, toUser).Scan(&uname); err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("service: receiver not found or disabled")
		}
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id=?`, fromUser).Scan(new(string)); err != nil {
		return nil, errors.New("service: source user not found")
	}
	if len(fileIDs) == 0 && len(nodeIDs) == 0 && spaceID == "" {
		return nil, errors.New("service: specify file_ids / node_ids / space_id")
	}
	id := uuid.NewString()
	now := time.Now().Unix()
	fj := jsonStrings(fileIDs)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO org_transfers (id,type,from_user,to_user,space_id,node_ids,file_ids,status,note,created_by,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?, 'draft', ?,?,?,?)`,
		id, typ, fromUser, toUser, nullIfEmpty(spaceID), jsonStrings(nodeIDs), fj, note, createdBy, now, now); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "org.transfer.created", Key: id,
		Data: map[string]any{"type": typ, "from": fromUser, "to": toUser}})
	_, _ = s.aud.Append(ctx, createdBy, "org.transfer_create", id,
		map[string]any{"type": typ, "from": fromUser, "to": toUser})
	return s.Transfer(ctx, id)
}

// Transfer 取单个移交（含用户名）。
func (s *OrgStore) Transfer(ctx context.Context, id string) (*Transfer, error) {
	var t Transfer
	var spaceID, note sql.NullString
	var nodeIDs, fileIDs string
	var executedAt, acceptedAt, completedAt sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT t.id, t.type, t.from_user, t.to_user, t.space_id, t.node_ids, t.file_ids, t.status, t.note, t.created_by, t.created_at, t.updated_at, t.executed_at, t.accepted_at, t.completed_at,
		        fu.username, tu.username
		 FROM org_transfers t
		 JOIN users fu ON fu.id=t.from_user
		 JOIN users tu ON tu.id=t.to_user
		 WHERE t.id=?`, id).
		Scan(&t.ID, &t.Type, &t.FromUser, &t.ToUser, &spaceID, &nodeIDs, &fileIDs, &t.Status, &note, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &executedAt, &acceptedAt, &completedAt, &t.FromName, &t.ToName)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.SpaceID = spaceID.String
	t.NodeIDs = parseJSONStrings(nodeIDs)
	t.FileIDs = parseJSONStrings(fileIDs)
	t.Note = note.String
	if executedAt.Valid {
		t.ExecutedAt = executedAt.Int64
	}
	if acceptedAt.Valid {
		t.AcceptedAt = acceptedAt.Int64
	}
	if completedAt.Valid {
		t.CompletedAt = completedAt.Int64
	}
	return &t, nil
}

// ListTransfers 移交列表：scope=in → 我待接收（to_user）；scope=out → 我发起（created_by）。
func (s *OrgStore) ListTransfers(ctx context.Context, userID, scope string) ([]*Transfer, error) {
	var rows *sql.Rows
	var err error
	if scope == "in" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id FROM org_transfers WHERE to_user=? ORDER BY created_at DESC`, userID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id FROM org_transfers WHERE created_by=? ORDER BY created_at DESC`, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Transfer{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		t, err := s.Transfer(ctx, id)
		if err == nil {
			out = append(out, t)
		}
	}
	return out, rows.Err()
}

// PreviewTransfer 预演：只读生成条目清单（先清后建，幂等；可从任意状态重新预演直到执行）。
// 文件范围 = 指定范围内「属于 from 名下」的文件（custodian=from，或 custodian 为空且 owner=from）。
func (s *OrgStore) PreviewTransfer(ctx context.Context, id string) ([]*TransferItem, error) {
	t, err := s.Transfer(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.Status != "draft" && t.Status != "previewed" {
		return nil, errors.New("service: transfer cannot be previewed in status " + t.Status)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM org_transfer_items WHERE transfer_id=?`, id); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	// 1) 收集文件（file_id → name）
	files := map[string]string{}
	addFile := func(fid, fname string) {
		if _, ok := files[fid]; !ok && fname != "" {
			files[fid] = fname
		}
	}
	if len(t.FileIDs) > 0 {
		for _, fid := range t.FileIDs {
			var name string
			if err := tx.QueryRowContext(ctx,
				`SELECT name FROM files WHERE id=? AND deleted_at IS NULL`, fid).Scan(&name); err == nil {
				addFile(fid, name)
			}
		}
	}
	for _, nid := range t.NodeIDs {
		var npath string
		if err := tx.QueryRowContext(ctx,
			`SELECT path FROM tags WHERE id=? AND owner_id=? AND kind='org'`, nid, OrgOwner).Scan(&npath); err != nil {
			continue
		}
		rows, err := tx.QueryContext(ctx,
			`SELECT ft.file_id, f.name FROM file_tags ft
			 JOIN tags tg ON tg.id=ft.tag_id JOIN files f ON f.id=ft.file_id
			 WHERE tg.owner_id=? AND tg.kind='org' AND (tg.id=? OR tg.path LIKE ?||'/%') AND f.deleted_at IS NULL`,
			OrgOwner, nid, npath)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var fid, fname string
			if err := rows.Scan(&fid, &fname); err == nil {
				addFile(fid, fname)
			}
		}
		rows.Close()
	}
	if t.SpaceID != "" {
		rows, err := tx.QueryContext(ctx,
			`SELECT id, name FROM files WHERE space_id=? AND deleted_at IS NULL`, t.SpaceID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var fid, fname string
			if err := rows.Scan(&fid, &fname); err == nil {
				addFile(fid, fname)
			}
		}
		rows.Close()
	}
	// 2) 逐文件生成条目：仅 from 名下（custodian=from 或 空且 owner=from）
	fidList := make([]string, 0, len(files))
	for fid := range files {
		fidList = append(fidList, fid)
	}
	fromOwned := map[string]string{}
	for _, fid := range fidList {
		var owner, cust sql.NullString
		if err := tx.QueryRowContext(ctx,
			`SELECT owner_id, custodian_user_id FROM files WHERE id=?`, fid).
			Scan(&owner, &cust); err != nil {
			continue
		}
		if (cust.Valid && cust.String == t.FromUser) || (!cust.Valid && owner.String == t.FromUser) {
			fromOwned[fid] = files[fid]
		}
	}
	items := []*TransferItem{}
	for fid, fname := range fromOwned {
		items = append(items, &TransferItem{
			ID: uuid.NewString(), TransferID: id, AssetType: "file", AssetID: fid,
			AssetName: fname, Action: "transfer", TargetUserID: t.ToUser, Status: "pending",
		})
		// 该文件公开分享 → 吊销
		shRows, err := tx.QueryContext(ctx,
			`SELECT id FROM shares WHERE file_id=? AND revoked_at IS NULL`, fid)
		if err != nil {
			return nil, err
		}
		for shRows.Next() {
			var sid string
			if err := shRows.Scan(&sid); err == nil {
				items = append(items, &TransferItem{
					ID: uuid.NewString(), TransferID: id, AssetType: "share", AssetID: sid,
					AssetName: "分享链接", Action: "revoke", Status: "pending",
				})
			}
		}
		shRows.Close()
	}
	// 3) 空间 owner 变更（space_id 指定且 from 是 owner）
	if t.SpaceID != "" {
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT owner_id FROM spaces WHERE id=?`, t.SpaceID).Scan(&owner); err == nil && owner == t.FromUser {
			var sname string
			_ = tx.QueryRowContext(ctx, `SELECT name FROM spaces WHERE id=?`, t.SpaceID).Scan(&sname)
			items = append(items, &TransferItem{
				ID: uuid.NewString(), TransferID: id, AssetType: "space", AssetID: t.SpaceID,
				AssetName: sname, Action: "transfer", TargetUserID: t.ToUser, Status: "pending",
			})
		}
	}
	// 4) from 在任岗位 → 关闭（调岗/离职都移除岗位）
	var memID, memNode, memPath string
	if err := tx.QueryRowContext(ctx,
		`SELECT m.id, m.node_id, t.path FROM org_memberships m JOIN tags t ON t.id=m.node_id
		 WHERE m.user_id=? AND m.until IS NULL ORDER BY m.since DESC LIMIT 1`, t.FromUser).
		Scan(&memID, &memNode, &memPath); err == nil {
		items = append(items, &TransferItem{
			ID: uuid.NewString(), TransferID: id, AssetType: "membership", AssetID: memID,
			AssetName: "岗位 " + memPath, Action: "remove", Status: "pending",
		})
	}
	// 5) 落库 + 状态 previewed
	for _, it := range items {
		it.DoneAt = 0
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO org_transfer_items (id,transfer_id,asset_type,asset_id,asset_name,action,target_user_id,status,note,done_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?)`,
			it.ID, it.TransferID, it.AssetType, it.AssetID, it.AssetName, it.Action, nullIfEmpty(it.TargetUserID), it.Status, it.Note, nil); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_transfers SET status='previewed', updated_at=? WHERE id=?`, now, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

// TransferItems 取移交条目列表。
func (s *OrgStore) TransferItems(ctx context.Context, id string) ([]*TransferItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,transfer_id,asset_type,asset_id,asset_name,action,COALESCE(target_user_id,''),status,COALESCE(note,''),COALESCE(done_at,0)
		 FROM org_transfer_items WHERE transfer_id=? ORDER BY asset_type, asset_name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TransferItem{}
	for rows.Next() {
		var it TransferItem
		if err := rows.Scan(&it.ID, &it.TransferID, &it.AssetType, &it.AssetID, &it.AssetName, &it.Action, &it.TargetUserID, &it.Status, &it.Note, &it.DoneAt); err != nil {
			return nil, err
		}
		out = append(out, &it)
	}
	return out, rows.Err()
}

// SetTransferItemSkipped 排除/恢复条目（勾选排除：skip=true → skipped，执行时跳过；false → 恢复 pending）。
func (s *OrgStore) SetTransferItemSkipped(ctx context.Context, id, itemID string, skip bool) error {
	if _, err := s.Transfer(ctx, id); err != nil {
		return err
	}
	st := "pending"
	if skip {
		st = "skipped"
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE org_transfer_items SET status=? WHERE id=? AND transfer_id=? AND status IN ('pending','skipped')`, st, itemID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("service: item not editable")
	}
	return nil
}

// ExecuteTransfer 执行移交：单事务批量转移；任一失败整体回滚（条目级记录 failed 可定位），
// 全部成功则 commit 并将条目标记 done、主单置 executed。可重试（失败后修复再执行）。
func (s *OrgStore) ExecuteTransfer(ctx context.Context, id string) error {
	t, err := s.Transfer(ctx, id)
	if err != nil {
		return err
	}
	if t.Status != "draft" && t.Status != "previewed" {
		return errors.New("service: transfer cannot be executed in status " + t.Status)
	}
	items, err := s.TransferItems(ctx, id)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		items, err = s.PreviewTransfer(ctx, id)
		if err != nil {
			return err
		}
	}
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	failed := map[string]string{} // itemID -> reason
	for _, it := range items {
		if it.Status == "skipped" {
			continue
		}
		switch it.Action {
		case "transfer":
			if it.AssetType == "file" {
				res, err := tx.ExecContext(ctx,
					`UPDATE files SET custodian_user_id=? WHERE id=? AND (custodian_user_id=? OR (custodian_user_id IS NULL AND owner_id=?))`,
					t.ToUser, it.AssetID, t.FromUser, t.FromUser)
				if err != nil {
					failed[it.ID] = err.Error()
					continue
				}
				if n, _ := res.RowsAffected(); n == 0 {
					failed[it.ID] = "file not owned by source or missing"
					continue
				}
			} else if it.AssetType == "space" {
				res, err := tx.ExecContext(ctx,
					`UPDATE spaces SET owner_id=? WHERE id=? AND owner_id=?`, t.ToUser, it.AssetID, t.FromUser)
				if err != nil {
					failed[it.ID] = err.Error()
					continue
				}
				if n, _ := res.RowsAffected(); n == 0 {
					failed[it.ID] = "space owner mismatch"
					continue
				}
			}
		case "revoke":
			res, err := tx.ExecContext(ctx,
				`UPDATE shares SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, now, it.AssetID)
			if err != nil {
				failed[it.ID] = err.Error()
				continue
			}
			if n, _ := res.RowsAffected(); n == 0 {
				failed[it.ID] = "share already revoked"
				continue
			}
		case "remove":
			res, err := tx.ExecContext(ctx,
				`UPDATE org_memberships SET until=? WHERE id=? AND until IS NULL`, now-1, it.AssetID)
			if err != nil {
				failed[it.ID] = err.Error()
				continue
			}
			if n, _ := res.RowsAffected(); n == 0 {
				failed[it.ID] = "membership already closed"
				continue
			}
		default:
			failed[it.ID] = "unknown action"
		}
	}
	if len(failed) > 0 {
		// 整体回滚（事务还原）；独立事务仅记录失败条目，便于定位重试
		_ = tx.Rollback()
		tx2, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx2.Rollback()
		for iid, reason := range failed {
			_, _ = tx2.ExecContext(ctx,
				`UPDATE org_transfer_items SET status='failed', note=? WHERE id=? AND transfer_id=?`, reason, iid, id)
		}
		if err := tx2.Commit(); err != nil {
			return err
		}
		return errors.New("service: transfer execution failed, " + itoa(len(failed)) + " item(s) failed")
	}
	// 全部成功：commit 前先把条目置 done（同事务）
	for _, it := range items {
		if it.Status == "skipped" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE org_transfer_items SET status='done', done_at=? WHERE id=?`, now, it.ID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_transfers SET status='executed', executed_at=?, updated_at=? WHERE id=?`, now, now, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.b.Publish(ctx, bus.Event{Topic: "org.transfer.executed", Key: id,
		Data: map[string]any{"from": t.FromUser, "to": t.ToUser}})
	_, _ = s.aud.Append(ctx, t.FromUser, "org.transfer_execute", id, map[string]any{"to": t.ToUser})
	return nil
}

// AcceptTransfer 接收人确认（或管理员代确认）：执行完成钩子 → archived。
// type=resign：完成钩子 = users.status=disabled（登录即被冻结）+ 分享兜底吊销；
// type=transfer：完成钩子 = 岗位记录迁移（from 岗位已关闭，to 接管岗位；标签不回扫）。
func (s *OrgStore) AcceptTransfer(ctx context.Context, id, actor string, isAdmin bool) error {
	t, err := s.Transfer(ctx, id)
	if err != nil {
		return err
	}
	if t.Status != "executed" {
		return errors.New("service: transfer not in executed status")
	}
	if !isAdmin && actor != t.ToUser {
		return errors.New("service: only receiver or admin can accept")
	}
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if t.Type == "resign" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET status='disabled' WHERE id=? AND status<>'disabled'`, t.FromUser); err != nil {
			return err
		}
		// 兜底吊销 from 名下全部未吊销分享
		if _, err := tx.ExecContext(ctx,
			`UPDATE shares SET revoked_at=? WHERE owner_id=? AND revoked_at IS NULL`, now, t.FromUser); err != nil {
			return err
		}
	} else {
		// 岗位迁移：to 接管 from 刚关闭的岗位（from 的岗位在 execute 已 until）；若 to 另有在任岗位则切换
		var nodeID string
		if err := tx.QueryRowContext(ctx,
			`SELECT node_id FROM org_memberships WHERE user_id=? ORDER BY until DESC, since DESC LIMIT 1`, t.FromUser).Scan(&nodeID); err == nil && nodeID != "" {
			// 关闭 to 旧在任岗位
			if _, err := tx.ExecContext(ctx,
				`UPDATE org_memberships SET until=? WHERE user_id=? AND until IS NULL`, now-1, t.ToUser); err != nil {
				return err
			}
			mid := uuid.NewString()
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO org_memberships (id, user_id, node_id, since, role) VALUES (?,?,?,?,'member')`,
				mid, t.ToUser, nodeID, now); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_transfers SET status='archived', accepted_at=?, completed_at=?, updated_at=? WHERE id=?`,
		now, now, now, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.b.Publish(ctx, bus.Event{Topic: "org.transfer.completed", Key: id,
		Data: map[string]any{"type": t.Type, "from": t.FromUser, "to": t.ToUser}})
	_, _ = s.aud.Append(ctx, actor, "org.transfer_accept", id, map[string]any{"type": t.Type})
	return nil
}

// CancelTransfer 取消移交（仅 draft/previewed）。
func (s *OrgStore) CancelTransfer(ctx context.Context, id, actor string, isAdmin bool) error {
	t, err := s.Transfer(ctx, id)
	if err != nil {
		return err
	}
	if t.Status != "draft" && t.Status != "previewed" {
		return errors.New("service: transfer cannot be cancelled in status " + t.Status)
	}
	if !isAdmin && actor != t.CreatedBy && actor != t.FromUser {
		return errors.New("service: only creator/admin can cancel")
	}
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM org_transfer_items WHERE transfer_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_transfers SET status='cancelled', updated_at=? WHERE id=?`, now, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, _ = s.aud.Append(ctx, actor, "org.transfer_cancel", id, nil)
	return nil
}

// MyCustodianCount 当前用户名下（custodian）文件数，供移交表单提示。
func (s *OrgStore) MyCustodianCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM files WHERE deleted_at IS NULL AND (custodian_user_id=? OR (custodian_user_id IS NULL AND owner_id=?))`,
		userID, userID).Scan(&n)
	return n, err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
