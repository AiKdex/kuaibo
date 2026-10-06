// family.go 家族传承记录模块（传家365 同族能力，模块化内核组件——不装不用）：
//   - 一生时间轴：复用知识库文件（content_state.node_type='life_event' + fields{occurred_at,stage,people}），
//     本模块只做读取聚合（时间轴/阶段分组），写入走既有 blog/files 管线 → md 真理源、双链、评论、版本全继承。
//   - 家族关系树：family_members（成员）+ family_relations（typed edges：parent_of/child_of/spouse_of/
//     sibling_of/other，有向边）——独立表，模块卸载零残留，不污染知识图谱。
//   - 纪念日提醒：family_anniversaries（MM-DD 每年循环 + remind_days 提前量）→ NotifyStore 通知中心。
//   - 双人/家族共同空间：复用 spaces 协作（成员/邀请），前端引导页承接。
// 归属：所有数据按 user_id 隔离；开关 settings family.enabled（默认关=不装不用）。
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// FamilyMember 家族成员。
type FamilyMember struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Gender    string `json:"gender,omitempty"`    // male|female|''
	BirthDate string `json:"birth_date,omitempty"` // YYYY-MM-DD（可选）
	Note      string `json:"note,omitempty"`
	AvatarID  string `json:"avatar_id,omitempty"` // 头像文件 id（可选）
	CreatedAt int64  `json:"created_at"`
	RelCount  int    `json:"relation_count,omitempty"` // 关联关系数（列表展示）
	AnnivCnt  int    `json:"anniversary_count,omitempty"`
}

// FamilyRelation 家族关系（typed edge，有向）。
type FamilyRelation struct {
	ID        string `json:"id"`
	FromID    string `json:"from_id"`
	ToID      string `json:"to_id"`
	Relation  string `json:"relation"` // parent_of|child_of|spouse_of|sibling_of|other
	Note      string `json:"note,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// FamilyAnniversary 纪念日（MM-DD 每年循环）。
type FamilyAnniversary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Kind        string `json:"kind"` // birthday|anniversary|custom
	Date        string `json:"date"` // MM-DD
	RefMemberID string `json:"ref_member_id,omitempty"`
	RemindDays  int    `json:"remind_days"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   int64  `json:"created_at"`
	// 计算字段（列表返回）：下次日期
	NextDate string `json:"next_date,omitempty"`
}

// FamilyTreeMember 家族树节点（轻量）。
type FamilyTreeMember struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Gender    string `json:"gender,omitempty"`
	BirthDate string `json:"birth_date,omitempty"`
}

// FamilyTreeData 家族树/图数据（前端渲染）。
type FamilyTreeData struct {
	Members   []*FamilyTreeMember `json:"members"`
	Relations []*FamilyRelation   `json:"relations"`
}

// FamilyTimelineItem 一生时间轴条目（life_event 文件聚合）。
type FamilyTimelineItem struct {
	FileID     string `json:"file_id"`
	Title      string `json:"title"`
	Stage      string `json:"stage,omitempty"`
	OccurredAt int64  `json:"occurred_at"`
	Preview    string `json:"preview,omitempty"`
	UpdatedAt  int64  `json:"updated_at"`
	Link       string `json:"link"` // 前端阅读路由
}

// FamilyStore 家族记录模块服务。
type FamilyStore struct {
	db     *sql.DB
	cfg    interface{ GetString(string) string }
	notify *NotifyStore
}

// NewFamilyStore 创建家族记录模块服务。
func NewFamilyStore(db *sql.DB, cfg interface{ GetString(string) string }, notify *NotifyStore) *FamilyStore {
	return &FamilyStore{db: db, cfg: cfg, notify: notify}
}

// Enabled 模块总开关（settings family.enabled；默认关=不装不用）。
func (s *FamilyStore) Enabled() bool {
	return s.cfg.GetString("family.enabled") == "true"
}

// ErrFamilyNotFound 家族数据不存在。
var ErrFamilyNotFound = errors.New("family: not found")

// ---- 成员 ----

// ListMembers 成员列表（含关系数/纪念日数）。
func (s *FamilyStore) ListMembers(ctx context.Context, userID string) ([]*FamilyMember, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.name, COALESCE(m.gender,''), COALESCE(m.birth_date,''), COALESCE(m.note,''), COALESCE(m.avatar_id,''), m.created_at,
		        (SELECT COUNT(*) FROM family_relations r WHERE r.user_id=m.user_id AND (r.from_id=m.id OR r.to_id=m.id)) AS rc,
		        (SELECT COUNT(*) FROM family_anniversaries a WHERE a.user_id=m.user_id AND a.ref_member_id=m.id) AS ac
		 FROM family_members m WHERE m.user_id=? ORDER BY m.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*FamilyMember{}
	for rows.Next() {
		m := &FamilyMember{}
		if err := rows.Scan(&m.ID, &m.Name, &m.Gender, &m.BirthDate, &m.Note, &m.AvatarID, &m.CreatedAt, &m.RelCount, &m.AnnivCnt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CreateMember 新增成员。
func (s *FamilyStore) CreateMember(ctx context.Context, userID string, m *FamilyMember) (*FamilyMember, error) {
	m.ID = uuid.NewString()
	m.CreatedAt = time.Now().UnixMilli()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO family_members (id, user_id, name, gender, birth_date, note, avatar_id, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		m.ID, userID, strings.TrimSpace(m.Name), m.Gender, m.BirthDate, m.Note, m.AvatarID, m.CreatedAt); err != nil {
		return nil, err
	}
	return m, nil
}

// UpdateMember 更新成员（按归属校验）。
func (s *FamilyStore) UpdateMember(ctx context.Context, userID, id string, m *FamilyMember) (*FamilyMember, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE family_members SET name=?, gender=?, birth_date=?, note=?, avatar_id=? WHERE id=? AND user_id=?`,
		strings.TrimSpace(m.Name), m.Gender, m.BirthDate, m.Note, m.AvatarID, id, userID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrFamilyNotFound
	}
	m.ID = id
	return m, nil
}

// DeleteMember 删除成员（级联清理其关系与关联纪念日）。
func (s *FamilyStore) DeleteMember(ctx context.Context, userID, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM family_members WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFamilyNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM family_relations WHERE user_id=? AND (from_id=? OR to_id=?)`, userID, id, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM family_anniversaries WHERE user_id=? AND ref_member_id=?`, userID, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- 关系 ----

// ListRelations 关系列表。
func (s *FamilyStore) ListRelations(ctx context.Context, userID string) ([]*FamilyRelation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, from_id, to_id, relation, COALESCE(note,''), created_at FROM family_relations WHERE user_id=? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*FamilyRelation{}
	for rows.Next() {
		r := &FamilyRelation{}
		if err := rows.Scan(&r.ID, &r.FromID, &r.ToID, &r.Relation, &r.Note, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateRelation 新增关系（两端成员必须存在且同属 user）。
func (s *FamilyStore) CreateRelation(ctx context.Context, userID string, r *FamilyRelation) (*FamilyRelation, error) {
	if r.FromID == r.ToID {
		return nil, errors.New("family: 不能与自己建立关系")
	}
	var cnt int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM family_members WHERE user_id=? AND id IN (?,?)`, userID, r.FromID, r.ToID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt != 2 {
		return nil, errors.New("family: 关系两端成员不存在")
	}
	r.ID = uuid.NewString()
	r.CreatedAt = time.Now().UnixMilli()
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO family_relations (id, user_id, from_id, to_id, relation, note, created_at) VALUES (?,?,?,?,?,?,?)`,
		r.ID, userID, r.FromID, r.ToID, r.Relation, r.Note, r.CreatedAt); err != nil {
		return nil, err
	}
	return r, nil
}

// DeleteRelation 删除关系（按归属校验）。
func (s *FamilyStore) DeleteRelation(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM family_relations WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFamilyNotFound
	}
	return nil
}

// Tree 家族树/图数据。
func (s *FamilyStore) Tree(ctx context.Context, userID string) (*FamilyTreeData, error) {
	members, err := s.ListMembers(ctx, userID)
	if err != nil {
		return nil, err
	}
	rels, err := s.ListRelations(ctx, userID)
	if err != nil {
		return nil, err
	}
	tm := make([]*FamilyTreeMember, 0, len(members))
	for _, m := range members {
		tm = append(tm, &FamilyTreeMember{ID: m.ID, Name: m.Name, Gender: m.Gender, BirthDate: m.BirthDate})
	}
	return &FamilyTreeData{Members: tm, Relations: rels}, nil
}

// ---- 一生时间轴（life_event 文件聚合） ----

// Timeline 按 occurred_at 升序返回用户 home space 下全部 life_event 文件。
func (s *FamilyStore) Timeline(ctx context.Context, userID string) ([]*FamilyTimelineItem, error) {
	var spaceID string
	if err := s.db.QueryRowContext(ctx,
		`SELECT id FROM spaces WHERE owner_id=? AND kind='home'`, userID).Scan(&spaceID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, COALESCE(content_state,''), updated_at FROM files
		 WHERE space_id=? AND deleted_at IS NULL AND content_state LIKE '%"node_type":"life_event"%'
		 ORDER BY updated_at DESC LIMIT 500`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*FamilyTimelineItem{}
	for rows.Next() {
		var id, name, cs string
		var updatedAt int64
		if err := rows.Scan(&id, &name, &cs, &updatedAt); err != nil {
			return nil, err
		}
		item := &FamilyTimelineItem{
			FileID:    id,
			Title:     strings.TrimSuffix(name, ".md"),
			OccurredAt: updatedAt,
			UpdatedAt: updatedAt,
			Link:      "#/read/" + id,
		}
		// 解析 content_state.fields{occurred_at, stage}
		var csObj struct {
			Fields map[string]any `json:"fields"`
		}
		if json.Unmarshal([]byte(cs), &csObj) == nil && csObj.Fields != nil {
			if v, ok := csObj.Fields["occurred_at"].(float64); ok && v > 0 {
				item.OccurredAt = int64(v)
			}
			if v, ok := csObj.Fields["stage"].(string); ok {
				item.Stage = v
			}
			if v, ok := csObj.Fields["preview"].(string); ok {
				item.Preview = v
			}
		}
		out = append(out, item)
	}
	// occurred_at 升序（时间轴由远及近）
	// 稳定排序：occurred_at 相等时按 updated_at 降序
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].OccurredAt < out[j-1].OccurredAt ||
			(out[j].OccurredAt == out[j-1].OccurredAt && out[j].UpdatedAt > out[j-1].UpdatedAt)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// ---- 纪念日 ----

// ListAnniversaries 纪念日列表（附下次日期计算）。
func (s *FamilyStore) ListAnniversaries(ctx context.Context, userID string) ([]*FamilyAnniversary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, kind, date, COALESCE(ref_member_id,''), remind_days, enabled, created_at
		 FROM family_anniversaries WHERE user_id=? ORDER BY date`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*FamilyAnniversary{}
	now := time.Now()
	for rows.Next() {
		a := &FamilyAnniversary{}
		var en int
		if err := rows.Scan(&a.ID, &a.Title, &a.Kind, &a.Date, &a.RefMemberID, &a.RemindDays, &en, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Enabled = en == 1
		a.NextDate = nextAnnivDate(now, a.Date)
		out = append(out, a)
	}
	return out, rows.Err()
}

// nextAnnivDate 计算 MM-DD 纪念日的下一次发生日期（YYYY-MM-DD；今天不算"下次"）。
func nextAnnivDate(now time.Time, mmdd string) string {
	parts := strings.SplitN(mmdd, "-", 2)
	if len(parts) != 2 {
		return ""
	}
	target := time.Date(now.Year(), monthOf(parts[0]), dayOf(parts[1]), 0, 0, 0, 0, now.Location())
	if !target.After(now) {
		target = time.Date(now.Year()+1, monthOf(parts[0]), dayOf(parts[1]), 0, 0, 0, 0, now.Location())
	}
	return target.Format("2006-01-02")
}

func monthOf(s string) time.Month {
	m, _ := time.Parse("01", s)
	return m.Month()
}

func dayOf(s string) int {
	d, _ := time.Parse("02", s)
	return d.Day()
}

// CreateAnniversary 新增纪念日。
func (s *FamilyStore) CreateAnniversary(ctx context.Context, userID string, a *FamilyAnniversary) (*FamilyAnniversary, error) {
	a.ID = uuid.NewString()
	a.CreatedAt = time.Now().UnixMilli()
	en := 0
	if a.Enabled {
		en = 1
	}
	if a.RefMemberID != "" {
		var cnt int
		_ = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM family_members WHERE user_id=? AND id=?`, userID, a.RefMemberID).Scan(&cnt)
		if cnt == 0 {
			return nil, errors.New("family: 关联成员不存在")
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO family_anniversaries (id, user_id, title, kind, date, ref_member_id, remind_days, enabled, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		a.ID, userID, strings.TrimSpace(a.Title), a.Kind, a.Date, a.RefMemberID, a.RemindDays, en, a.CreatedAt); err != nil {
		return nil, err
	}
	a.NextDate = nextAnnivDate(time.Now(), a.Date)
	return a, nil
}

// UpdateAnniversary 更新纪念日（按归属校验）。
func (s *FamilyStore) UpdateAnniversary(ctx context.Context, userID, id string, a *FamilyAnniversary) (*FamilyAnniversary, error) {
	en := 0
	if a.Enabled {
		en = 1
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE family_anniversaries SET title=?, kind=?, date=?, ref_member_id=?, remind_days=?, enabled=? WHERE id=? AND user_id=?`,
		strings.TrimSpace(a.Title), a.Kind, a.Date, a.RefMemberID, a.RemindDays, en, id, userID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrFamilyNotFound
	}
	a.ID = id
	a.NextDate = nextAnnivDate(time.Now(), a.Date)
	return a, nil
}

// DeleteAnniversary 删除纪念日（按归属校验）。
func (s *FamilyStore) DeleteAnniversary(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM family_anniversaries WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFamilyNotFound
	}
	return nil
}

// DailyReminders 纪念日提醒扫描：对"今日"与"今日+remind_days 内"的纪念日向归属用户推送通知。
// 每小时调用幂等：同日同条以 anniv_key 去重（notifications.payload.anniv_key）。
func (s *FamilyStore) DailyReminders(ctx context.Context, now time.Time) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, title, date, COALESCE(ref_member_id,''), remind_days FROM family_anniversaries WHERE enabled=1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		id, uid, title, date, ref string
		remind                    int
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.uid, &r.title, &r.date, &r.ref, &r.remind); err != nil {
			return err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	today := now.Format("2006-01-02")
	for _, r := range list {
		// 目标日 = 今日 + remind_days；命中 date（MM-DD）即推送
		target := now.AddDate(0, 0, r.remind).Format("01-02")
		if target != r.date {
			continue
		}
		key := today + ":" + r.id
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM notifications WHERE user_id=? AND type='family' AND json_extract(payload,'$.anniv_key')=?`,
			r.uid, key).Scan(&n); err != nil || n > 0 {
			continue
		}
		msg := "今天" + r.title
		if r.remind > 0 {
			msg = r.title + " 将在 " + strconv.Itoa(r.remind) + " 天后到来"
		}
		if r.ref != "" {
			var nm string
			_ = s.db.QueryRowContext(ctx,
				`SELECT name FROM family_members WHERE id=? AND user_id=?`, r.ref, r.uid).Scan(&nm)
			if nm != "" {
				msg = nm + " · " + msg
			}
		}
		_ = s.notify.AddUser(ctx, r.uid, "family", map[string]any{
			"title":   "家族纪念日提醒",
			"message": msg,
			"link":    "#/family",
			"extra":   map[string]any{"anniv_key": key, "anniv_id": r.id},
		})
	}
	return nil
}
