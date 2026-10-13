// Package service 业务层：知识库聚合（KB 聚合层，设计需求文档 A05/A06/A09）。
// 聚合 tags/collections/chunks/AI 摘要/图谱边，为知识库总览、概念页、图谱视图提供数据；
// 集合（collection）为手动/智能分组的组织原语（FR3）。
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// KBOverview 知识库总览统计。
type KBOverview struct {
	Files          int64         `json:"files"`
	Dirs           int64         `json:"dirs"`
	Tags           int64         `json:"tags"`
	Collections    int64         `json:"collections"`
	Summaries      int64         `json:"summaries"`
	SummaryPending int64         `json:"summary_pending"` // 解读队列待处理（含 running）
	SummaryError   int64         `json:"summary_error"`   // 解读失败（超限标记 error）
	Chunks         int64         `json:"chunks"`
	Edges          int64         `json:"edges"`
	RecentFiles    []*File       `json:"recent_files"`
	RecentSummary  []SummaryItem `json:"recent_summaries"`
}

// SummaryItem 摘要条目（概念页/列表展示）。
type SummaryItem struct {
	FileID    string `json:"file_id"`
	FileName  string `json:"file_name"`
	Summary   string `json:"summary"`
	Tags      string `json:"tags,omitempty"`
	UpdatedAt int64  `json:"updated_at"`
}

// TagAggregate 标签聚合（标签 + 其下文件 + 摘要）。
type TagAggregate struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Path    string  `json:"path"`
	Count   int64   `json:"count"`
	Files   []*File `json:"files"`
	Summary string  `json:"summary,omitempty"`
}

// Concept 概念页（标签维度聚合视图 A09 轻量形态）。
type Concept struct {
	Tag          *TagAggregate   `json:"tag"`
	RelatedTags  []*TagAggregate `json:"related_tags"`
	Inbound      []*File         `json:"inbound,omitempty"`       // 反向引用：引用了本概念下文件的外部文件
	RelatedFiles []*File         `json:"related_files,omitempty"` // 相关文件推荐：共现标签下的文件（按共现度）
	Edges        []*KbEdge       `json:"edges,omitempty"`
}

// KbEdge 图谱边（节点用 id+type 定位）。
type KbEdge struct {
	SourceType string  `json:"source_type"`
	SourceID   string  `json:"source_id"`
	TargetType string  `json:"target_type"`
	TargetID   string  `json:"target_id"`
	Relation   string  `json:"relation"`
	Weight     float64 `json:"weight"`
}

// GraphNode / GraphLink 图谱视图数据（文件-标签二部图 + 知识边）。
type GraphNode struct {
	ID    string `json:"id"`
	Type  string `json:"type"` // file | tag
	Label string `json:"label"`
	Size  int64  `json:"size"`
}
type GraphLink struct {
	Source   string  `json:"source"`
	Target   string  `json:"target"`
	Relation string  `json:"relation"`
	Weight   float64 `json:"weight"`
}
type GraphData struct {
	Nodes []*GraphNode `json:"nodes"`
	Links []*GraphLink `json:"links"`
}

// Collection 集合。
type Collection struct {
	ID        string `json:"id"`
	OwnerID   string `json:"owner_id"`
	SpaceID   string `json:"space_id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // manual | smart
	Query     string `json:"query,omitempty"`
	FileCount int64  `json:"file_count"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// KBStore 知识库聚合存取。
type KBStore struct {
	db *sql.DB
}

func NewKBStore(db *sql.DB) *KBStore { return &KBStore{db: db} }

// Overview 总览统计（ownerID 用于 tags 过滤——tags 表无 space_id，仅按 owner 隔离）。
func (s *KBStore) Overview(ctx context.Context, ownerID, spaceID string) (*KBOverview, error) {
	out := &KBOverview{}
	one := func(q string, dst *int64, args ...any) error {
		return s.db.QueryRowContext(ctx, q, args...).Scan(dst)
	}
	if err := one(`SELECT COUNT(*) FROM files WHERE space_id=? AND deleted_at IS NULL`, &out.Files, spaceID); err != nil {
		return nil, err
	}
	if err := one(`SELECT COUNT(*) FROM files WHERE space_id=? AND kind='dir' AND deleted_at IS NULL`, &out.Dirs, spaceID); err != nil {
		return nil, err
	}
	if err := one(`SELECT COUNT(*) FROM tags WHERE owner_id=?`, &out.Tags, ownerID); err != nil {
		return nil, err
	}
	if err := one(`SELECT COUNT(*) FROM collections WHERE space_id=?`, &out.Collections, spaceID); err != nil {
		return nil, err
	}
	if err := one(`SELECT COUNT(*) FROM file_ai_summaries s JOIN files f ON s.file_id=f.id WHERE s.status='done' AND f.space_id=?`, &out.Summaries, spaceID); err != nil {
		return nil, err
	}
	if err := one(`SELECT COUNT(*) FROM file_ai_summaries s JOIN files f ON s.file_id=f.id WHERE s.status IN ('pending','running') AND f.space_id=?`, &out.SummaryPending, spaceID); err != nil {
		return nil, err
	}
	if err := one(`SELECT COUNT(*) FROM file_ai_summaries s JOIN files f ON s.file_id=f.id WHERE s.status='error' AND f.space_id=?`, &out.SummaryError, spaceID); err != nil {
		return nil, err
	}
	if err := one(`SELECT COUNT(*) FROM index_chunks WHERE space_id=?`, &out.Chunks, spaceID); err != nil {
		return nil, err
	}
	if err := one(`SELECT COUNT(*) FROM knowledge_edges WHERE space_id=?`, &out.Edges, spaceID); err != nil {
		return nil, err
	}
	// 最近文件（含目录，按更新时间）
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files WHERE space_id=? AND deleted_at IS NULL ORDER BY updated_at DESC LIMIT 8`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out.RecentFiles = append(out.RecentFiles, f)
	}
	// 最近摘要
	srows, err := s.db.QueryContext(ctx,
		`SELECT f.id, f.name, s.summary, s.tags, s.updated_at
		 FROM file_ai_summaries s JOIN files f ON f.id = s.file_id
		 WHERE s.status='done' ORDER BY s.updated_at DESC LIMIT 6`)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var it SummaryItem
		if err := srows.Scan(&it.FileID, &it.FileName, &it.Summary, &it.Tags, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out.RecentSummary = append(out.RecentSummary, it)
	}
	return out, nil
}

// TagsAggregate 标签聚合（每个标签下文件列表，带摘要）。
func (s *KBStore) TagsAggregate(ctx context.Context, spaceID string) ([]*TagAggregate, error) {
	tags, err := s.listTagsAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*TagAggregate, 0, len(tags))
	for _, t := range tags {
		agg := &TagAggregate{ID: t.ID, Name: t.Name, Path: t.Path}
		frows, err := s.db.QueryContext(ctx,
			`SELECT `+fileCols+` FROM files f
			 JOIN file_tags ft ON ft.file_id = f.id
			 WHERE ft.tag_id=? AND f.deleted_at IS NULL ORDER BY f.updated_at DESC LIMIT 12`, t.ID)
		if err != nil {
			return nil, err
		}
		var files []*File
		for frows.Next() {
			f, err := scanFile(frows)
			if err != nil {
				frows.Close()
				return nil, err
			}
			files = append(files, f)
		}
		frows.Close()
		agg.Count = int64(len(files))
		agg.Files = files
		// 该标签下已生成摘要的聚合（取一条最全的）
		_ = s.db.QueryRowContext(ctx,
			`SELECT summary FROM file_ai_summaries s
			 JOIN file_tags ft ON ft.file_id = s.file_id
			 WHERE ft.tag_id=? AND s.status='done' AND s.summary<>'' ORDER BY length(s.summary) DESC LIMIT 1`, t.ID).Scan(&agg.Summary)
		if len(files) > 0 {
			out = append(out, agg)
		}
	}
	return out, nil
}

// Concept 概念页：标签 + 文件 + 关联标签 + 知识边。
func (s *KBStore) Concept(ctx context.Context, spaceID, tagID string) (*Concept, error) {
	var t struct {
		ID   string
		Name string
		Path string
	}
	if err := s.db.QueryRowContext(ctx, `SELECT id, name, path FROM tags WHERE id=?`, tagID).Scan(&t.ID, &t.Name, &t.Path); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	agg := &TagAggregate{ID: t.ID, Name: t.Name, Path: t.Path}
	frows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files f
		 JOIN file_tags ft ON ft.file_id = f.id
		 WHERE ft.tag_id=? AND f.deleted_at IS NULL ORDER BY f.updated_at DESC`, tagID)
	if err != nil {
		return nil, err
	}
	defer frows.Close()
	for frows.Next() {
		f, err := scanFile(frows)
		if err != nil {
			return nil, err
		}
		agg.Files = append(agg.Files, f)
	}
	agg.Count = int64(len(agg.Files))
	_ = s.db.QueryRowContext(ctx,
		`SELECT summary FROM file_ai_summaries s
		 JOIN file_tags ft ON ft.file_id = s.file_id
		 WHERE ft.tag_id=? AND s.status='done' AND s.summary<>'' ORDER BY length(s.summary) DESC LIMIT 1`, tagID).Scan(&agg.Summary)

	// 关联标签：与该标签共现于同一文件的其它标签
	rel, err := s.db.QueryContext(ctx,
		`SELECT t2.id, t2.name, t2.path, COUNT(DISTINCT ft1.file_id) c
		 FROM file_tags ft1
		 JOIN file_tags ft2 ON ft2.file_id = ft1.file_id AND ft2.tag_id <> ft1.tag_id
		 JOIN tags t2 ON t2.id = ft2.tag_id
		 WHERE ft1.tag_id=?
		 GROUP BY t2.id ORDER BY c DESC LIMIT 10`, tagID)
	if err != nil {
		return nil, err
	}
	defer rel.Close()
	var rels []*TagAggregate = []*TagAggregate{}
	for rel.Next() {
		var r TagAggregate
		if err := rel.Scan(&r.ID, &r.Name, &r.Path, &r.Count); err != nil {
			return nil, err
		}
		rels = append(rels, &r)
	}
	// 知识边
	edges, err := s.edgesForTag(ctx, spaceID, tagID)
	if err != nil {
		return nil, err
	}
	if edges == nil {
		edges = []*KbEdge{}
	}

	// 反向引用：cites 边指向本概念文件的外部文件（排除本概念内文件）
	inbound, err := s.conceptInbound(ctx, spaceID, tagID)
	if err != nil {
		return nil, err
	}
	// 相关文件推荐：与本概念文件共现标签的文件（排除本概念内文件，按共现标签数排序）
	relFiles, err := s.conceptRelatedFiles(ctx, spaceID, tagID)
	if err != nil {
		return nil, err
	}
	return &Concept{Tag: agg, RelatedTags: rels, Inbound: inbound, RelatedFiles: relFiles, Edges: edges}, nil
}

// conceptInbound 引用本概念下文件的外部文件（双链 cites 边；排除本概念内文件）。
func (s *KBStore) conceptInbound(ctx context.Context, spaceID, tagID string) ([]*File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files f
		 WHERE f.id IN (
		   SELECT DISTINCT e.source_id FROM knowledge_edges e
		   JOIN file_tags ft ON ft.file_id = e.target_id
		   WHERE e.space_id=? AND e.source_type='file' AND e.target_type='file'
		     AND e.relation='cites' AND ft.tag_id=?
		     AND e.source_id NOT IN (SELECT file_id FROM file_tags WHERE tag_id=?)
		 )
		 AND f.space_id=? AND f.deleted_at IS NULL
		 ORDER BY f.updated_at DESC LIMIT 10`, spaceID, tagID, tagID, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// conceptRelatedFiles 相关文件推荐：与本概念文件共享"共现标签"最多的文件（排除本概念内文件，按共享数排序）。
func (s *KBStore) conceptRelatedFiles(ctx context.Context, spaceID, tagID string) ([]*File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files f
		 JOIN (
		   SELECT ft2.file_id, COUNT(DISTINCT ft2.tag_id) co FROM file_tags ft2
		   WHERE ft2.tag_id IN (
		     SELECT DISTINCT ft1.tag_id FROM file_tags ft1
		     WHERE ft1.file_id IN (SELECT file_id FROM file_tags WHERE tag_id=?))
		   GROUP BY ft2.file_id
		 ) x ON x.file_id = f.id
		 WHERE f.space_id=? AND f.deleted_at IS NULL
		   AND f.id NOT IN (SELECT file_id FROM file_tags WHERE tag_id=?)
		 ORDER BY x.co DESC, f.updated_at DESC LIMIT 10`, tagID, spaceID, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (s *KBStore) edgesForTag(ctx context.Context, spaceID, tagID string) ([]*KbEdge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT source_type, source_id, target_type, target_id, relation, weight
		 FROM knowledge_edges WHERE space_id=? AND (source_id=? OR target_id=?) LIMIT 50`,
		spaceID, tagID, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KbEdge
	for rows.Next() {
		var e KbEdge
		if err := rows.Scan(&e.SourceType, &e.SourceID, &e.TargetType, &e.TargetID, &e.Relation, &e.Weight); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, nil
}

// Graph 图谱数据：文件-标签二部图（默认） + 知识边。limit 控制节点规模（默认 200 文件）。
func (s *KBStore) Graph(ctx context.Context, spaceID string) (*GraphData, error) {
	g := &GraphData{Nodes: []*GraphNode{}, Links: []*GraphLink{}}
	// 标签节点（带文件数）
	trows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.name, COUNT(ft.file_id)
		 FROM tags t LEFT JOIN file_tags ft ON ft.tag_id = t.id
		 GROUP BY t.id ORDER BY COUNT(ft.file_id) DESC`)
	if err != nil {
		return nil, err
	}
	tagCount := map[string]int64{}
	for trows.Next() {
		var id, name string
		var c int64
		if err := trows.Scan(&id, &name, &c); err != nil {
			trows.Close()
			return nil, err
		}
		tagCount[id] = c
		g.Nodes = append(g.Nodes, &GraphNode{ID: "tag:" + id, Type: "tag", Label: name, Size: c})
	}
	trows.Close()
	// 文件节点（带标签的文件；limit 控制）
	frows, err := s.db.QueryContext(ctx,
		`SELECT f.id, f.name, f.kind FROM files f
		 JOIN file_tags ft ON ft.file_id = f.id
		 WHERE f.deleted_at IS NULL AND f.space_id=?
		 GROUP BY f.id ORDER BY f.updated_at DESC LIMIT 200`, spaceID)
	if err != nil {
		return nil, err
	}
	fileIDs := map[string]bool{}
	for frows.Next() {
		var id, name, kind string
		if err := frows.Scan(&id, &name, &kind); err != nil {
			frows.Close()
			return nil, err
		}
		fileIDs[id] = true
		g.Nodes = append(g.Nodes, &GraphNode{ID: "file:" + id, Type: "file", Label: name, Size: 2})
	}
	frows.Close()
	// 文件-标签边
	erows, err := s.db.QueryContext(ctx,
		`SELECT ft.file_id, ft.tag_id FROM file_tags ft WHERE ft.file_id IN (SELECT id FROM files WHERE deleted_at IS NULL AND space_id=?)`, spaceID)
	if err != nil {
		return nil, err
	}
	for erows.Next() {
		var fid, tid string
		if err := erows.Scan(&fid, &tid); err != nil {
			erows.Close()
			return nil, err
		}
		g.Links = append(g.Links, &GraphLink{Source: "file:" + fid, Target: "tag:" + tid, Relation: "tagged_as", Weight: 1})
	}
	erows.Close()
	// 知识边（可选补充）
	krows, err := s.db.QueryContext(ctx,
		`SELECT source_type, source_id, target_type, target_id, relation, weight FROM knowledge_edges WHERE space_id=? LIMIT 200`, spaceID)
	if err != nil {
		return nil, err
	}
	for krows.Next() {
		var e KbEdge
		if err := krows.Scan(&e.SourceType, &e.SourceID, &e.TargetType, &e.TargetID, &e.Relation, &e.Weight); err != nil {
			krows.Close()
			return nil, err
		}
		// 只保留 source 端在图中（文件 id 或标签类型）的边；target 端不设限制
		if fileIDs[e.SourceID] || e.SourceType == "tag" {
			g.Links = append(g.Links, &GraphLink{
				Source:   e.SourceType + ":" + e.SourceID,
				Target:   e.TargetType + ":" + e.TargetID,
				Relation: e.Relation,
				Weight:   e.Weight,
			})
		}
	}
	krows.Close()
	return g, nil
}

// AddEdge 手动/自动加知识边。
func (s *KBStore) AddEdge(ctx context.Context, spaceID, st, sid, tt, tid, relation string, weight float64, src string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO knowledge_edges (space_id, source_type, source_id, target_type, target_id, relation, weight, source, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		spaceID, st, sid, tt, tid, relation, weight, src, time.Now().Unix())
	return err
}

// ---- 集合 CRUD ----

// ListCollections 集合列表（带文件数；smart 集合为动态匹配数）。
func (s *KBStore) ListCollections(ctx context.Context, spaceID string) ([]*Collection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.owner_id, c.space_id, c.name, c.kind, c.query, c.created_at, c.updated_at,
		        (SELECT COUNT(*) FROM collection_files cf WHERE cf.collection_id = c.id) cnt
		 FROM collections c WHERE c.space_id=? ORDER BY c.updated_at DESC`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.SpaceID, &c.Name, &c.Kind, &c.Query, &c.CreatedAt, &c.UpdatedAt, &c.FileCount); err != nil {
			return nil, err
		}
		if c.Kind == "smart" {
			n, cerr := s.CountSmartCollectionFiles(ctx, spaceID, c.ID)
			if cerr == nil {
				c.FileCount = n
			}
		}
		out = append(out, &c)
	}
	return out, nil
}

// CreateCollection 创建集合。
func (s *KBStore) CreateCollection(ctx context.Context, ownerID, spaceID, name, kind, query string) (*Collection, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("service: name required")
	}
	if kind == "" {
		kind = "manual"
	}
	ts := time.Now().Unix()
	id := uuid.NewString()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO collections (id, owner_id, space_id, name, kind, query, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		id, ownerID, spaceID, name, kind, query, ts, ts); err != nil {
		return nil, err
	}
	return &Collection{ID: id, OwnerID: ownerID, SpaceID: spaceID, Name: name, Kind: kind, Query: query, CreatedAt: ts, UpdatedAt: ts}, nil
}

// DeleteCollection 删除集合（连带集合-文件关联）。
func (s *KBStore) DeleteCollection(ctx context.Context, spaceID, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM collection_files WHERE collection_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM collections WHERE id=? AND space_id=?`, id, spaceID)
	return err
}

// AddToCollection 加入集合（幂等）。
func (s *KBStore) AddToCollection(ctx context.Context, spaceID, collectionID, fileID string) error {
	var exists int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM collection_files WHERE collection_id=? AND file_id=?`, collectionID, fileID).Scan(&exists)
	if exists > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO collection_files (collection_id, file_id, added_at) VALUES (?,?,?)`,
		collectionID, fileID, time.Now().Unix()); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE collections SET updated_at=? WHERE id=?`, time.Now().Unix(), collectionID)
	return err
}

// RemoveFromCollection 移出集合。
func (s *KBStore) RemoveFromCollection(ctx context.Context, spaceID, collectionID, fileID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM collection_files WHERE collection_id=? AND file_id=?`, collectionID, fileID)
	return err
}

// CollectionFiles 集合内文件（手动集合：collection_files 关联表）。
func (s *KBStore) CollectionFiles(ctx context.Context, spaceID, collectionID string) ([]*File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files f
		 JOIN collection_files cf ON cf.file_id = f.id
		 WHERE cf.collection_id=? AND f.deleted_at IS NULL ORDER BY cf.added_at DESC`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// SmartQuery 智能集合查询定义（JSON，条件间 AND；空条件=全部文件）。
// tag：标签路径（含其全部子标签）；kind：file|dir；q：文件名关键词。
type SmartQuery struct {
	Tag  string `json:"tag"`
	Kind string `json:"kind"`
	Q    string `json:"q"`
}

// smartWhere 解析智能集合 query → WHERE 片段与参数（不含 WHERE 关键字）。
func smartWhere(spaceID, query string) (string, []any, error) {
	var cond SmartQuery
	if strings.TrimSpace(query) != "" {
		if err := json.Unmarshal([]byte(query), &cond); err != nil {
			return "", nil, errors.New("service: bad smart collection query: " + err.Error())
		}
	}
	where := []string{`f.space_id=?`, `f.deleted_at IS NULL`}
	args := []any{spaceID}
	if cond.Kind != "" {
		where = append(where, `f.kind=?`)
		args = append(args, cond.Kind)
	}
	if cond.Q != "" {
		where = append(where, `f.name LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(cond.Q)+"%")
	}
	if cond.Tag != "" {
		// 命中该标签或其任意子标签
		where = append(where, `EXISTS (SELECT 1 FROM file_tags ft JOIN tags t ON t.id=ft.tag_id
			 WHERE ft.file_id=f.id AND (t.path=? OR t.path LIKE ? ESCAPE '\'))`)
		args = append(args, cond.Tag, cond.Tag+"/%")
	}
	return strings.Join(where, ` AND `), args, nil
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// SmartCollectionFiles 智能集合：动态执行 query 匹配文件（P1-2 闭环）。
func (s *KBStore) SmartCollectionFiles(ctx context.Context, spaceID, collectionID string) ([]*File, error) {
	var query string
	if err := s.db.QueryRowContext(ctx, `SELECT query FROM collections WHERE id=? AND space_id=?`, collectionID, spaceID).Scan(&query); err != nil {
		return nil, err
	}
	where, args, err := smartWhere(spaceID, query)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files f WHERE `+where+` GROUP BY f.id ORDER BY f.updated_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// CountSmartCollectionFiles 智能集合动态匹配数（列表角标用）。
func (s *KBStore) CountSmartCollectionFiles(ctx context.Context, spaceID, collectionID string) (int64, error) {
	var query string
	if err := s.db.QueryRowContext(ctx, `SELECT query FROM collections WHERE id=? AND space_id=?`, collectionID, spaceID).Scan(&query); err != nil {
		return 0, err
	}
	where, args, err := smartWhere(spaceID, query)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT f.id) FROM files f WHERE `+where, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// CollectionFilesByKind 按集合类型取文件：smart 动态匹配 / manual 关联表。
func (s *KBStore) CollectionFilesByKind(ctx context.Context, spaceID, collectionID string) ([]*File, error) {
	var kind string
	if err := s.db.QueryRowContext(ctx, `SELECT kind FROM collections WHERE id=? AND space_id=?`, collectionID, spaceID).Scan(&kind); err != nil {
		return nil, err
	}
	if kind == "smart" {
		return s.SmartCollectionFiles(ctx, spaceID, collectionID)
	}
	return s.CollectionFiles(ctx, spaceID, collectionID)
}

// ---- helpers ----

// listTagsAll 全部标签（含空标签），供聚合查询。
func (s *KBStore) listTagsAll(ctx context.Context) ([]*Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, path, parent_id, created_at FROM tags ORDER BY path COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Tag
	for rows.Next() {
		var t Tag
		var parent sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Path, &parent, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.ParentID = parent.String
		out = append(out, &t)
	}
	return out, nil
}
