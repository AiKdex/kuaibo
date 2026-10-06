// Package service 业务层：知识库聚合（KB 聚合层，设计需求文档 A05/A06/A09）。
// 聚合 tags/collections/chunks/AI 摘要/图谱边，为知识库总览、概念页、图谱视图提供数据；
// 集合（collection）为手动/智能分组的组织原语（FR3）。
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	// SummarySource 摘要来源：ai=概念页 AI 独立生成（缓存）；fallback=取标签下最长文件解读；空=无摘要
	SummarySource    string `json:"summary_source,omitempty"`
	SummaryUpdatedAt int64  `json:"summary_updated_at,omitempty"` // ai 摘要生成时间（ms）
	// SummarySources 摘要来源文档引用链（A5）：[{"file_id":"...","name":"..."}]
	SummarySources []map[string]string `json:"summary_sources,omitempty"`
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
	// A09+A5：概念页摘要——优先 AI 独立生成缓存（kb_concept_summaries）；无缓存回退取标签下最长文件解读
	var aiSum, aiSrcs string
	var aiUpdMs int64
	_ = s.db.QueryRowContext(ctx,
		`SELECT summary, sources, updated_at FROM kb_concept_summaries WHERE tag_id=?`, tagID).Scan(&aiSum, &aiSrcs, &aiUpdMs)
	if strings.TrimSpace(aiSum) != "" {
		agg.Summary = aiSum
		agg.SummarySource = "ai"
		agg.SummaryUpdatedAt = aiUpdMs
		if strings.TrimSpace(aiSrcs) != "" && aiSrcs != "[]" {
			var srcs []map[string]string
			if json.Unmarshal([]byte(aiSrcs), &srcs) == nil {
				agg.SummarySources = srcs
			}
		}
	} else {
		_ = s.db.QueryRowContext(ctx,
			`SELECT summary FROM file_ai_summaries s
			 JOIN file_tags ft ON ft.file_id = s.file_id
			 WHERE ft.tag_id=? AND s.status='done' AND s.summary<>'' ORDER BY length(s.summary) DESC LIMIT 1`, tagID).Scan(&agg.Summary)
		if agg.Summary != "" {
			agg.SummarySource = "fallback"
		}
	}

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

// ---------------- FAQ 知识库类型（WeKnora 借鉴 A2，B27 自上游 bbd6764 移植） ----------------
// 独立于 files 的检索单元：标准问/相似问/反例问/答案。SearchFAQ 供 Agent 与问答注入。

// FAQ 条目。
type FAQ struct {
	ID         string   `json:"id"`
	SpaceID    string   `json:"space_id"`
	StandardQ  string   `json:"standard_q"`
	SimilarQs  []string `json:"similar_qs"`
	CounterQs  []string `json:"counter_qs"`
	Answer     string   `json:"answer"`
	Tags       []string `json:"tags"`
	Source     string   `json:"source"`
	CreatedBy  string   `json:"created_by"`
	CreatedAt  int64    `json:"created_at"`
	UpdatedAt  int64    `json:"updated_at"`
}

// CreateFAQ 新建 FAQ 条目。
func (s *KBStore) CreateFAQ(ctx context.Context, f *FAQ) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	if f.SpaceID == "" {
		return fmt.Errorf("faq: space_id required")
	}
	if strings.TrimSpace(f.StandardQ) == "" {
		return fmt.Errorf("faq: standard_q required")
	}
	now := time.Now().UnixMilli()
	f.CreatedAt, f.UpdatedAt = now, now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO kb_faq(id, space_id, standard_q, similar_qs, counter_qs, answer, tags, created_by, created_at, updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		f.ID, f.SpaceID, f.StandardQ, jsonArray(f.SimilarQs), jsonArray(f.CounterQs), f.Answer,
		jsonArray(f.Tags), f.CreatedBy, f.CreatedAt, f.UpdatedAt)
	return err
}

// UpdateFAQ 更新 FAQ 条目（仅本空间可改）。
func (s *KBStore) UpdateFAQ(ctx context.Context, id, spaceID string, f *FAQ) error {
	f.UpdatedAt = time.Now().UnixMilli()
	res, err := s.db.ExecContext(ctx,
		`UPDATE kb_faq SET standard_q=?, similar_qs=?, counter_qs=?, answer=?, tags=?, updated_at=?
		 WHERE id=? AND space_id=?`,
		f.StandardQ, jsonArray(f.SimilarQs), jsonArray(f.CounterQs), f.Answer, jsonArray(f.Tags),
		f.UpdatedAt, id, spaceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("faq: not found or no permission")
	}
	return nil
}

// DeleteFAQ 删除 FAQ 条目（仅本空间可删）。
func (s *KBStore) DeleteFAQ(ctx context.Context, id, spaceID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM kb_faq WHERE id=? AND space_id=?`, id, spaceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("faq: not found or no permission")
	}
	return nil
}

// ListFAQ 按空间列出（可选标准问/标签过滤）。
func (s *KBStore) ListFAQ(ctx context.Context, spaceID, q string) ([]*FAQ, error) {
	where, args := []string{"space_id=?"}, []any{spaceID}
	if q != "" {
		like := "%" + escapeLike(q) + "%"
		where = append(where, `(standard_q LIKE ? ESCAPE '\' OR similar_qs LIKE ? ESCAPE '\' OR answer LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, space_id, standard_q, similar_qs, counter_qs, answer, tags, source, created_by, created_at, updated_at
		 FROM kb_faq WHERE `+strings.Join(where, " AND ")+` ORDER BY updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*FAQ, 0, 16)
	for rows.Next() {
		f, err := scanFAQ(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SearchFAQ 问题检索：问题即检索单元。
// 两级匹配：① LIKE 子串命中（standard_q/similar_qs/answer，标准问优先）；② 中文 bigram 覆盖率
// （查询与条目共享≥50% 连续二元组即候选，对"如何开启…"vs"…怎么开启"这类同义语序鲁棒）。
// 反例问（counter_qs）命中会降权（排歧）。返回 top N。
func (s *KBStore) SearchFAQ(ctx context.Context, spaceID, q string, limit int) ([]*FAQ, error) {
	if strings.TrimSpace(q) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 8
	}
	// 粗取本空间条目（FAQ 量级小，内存打分；上限 200 防失控）
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, space_id, standard_q, similar_qs, counter_qs, answer, tags, source, created_by, created_at, updated_at
		 FROM kb_faq WHERE space_id=? ORDER BY updated_at DESC LIMIT 200`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type scored struct {
		f    *FAQ
		like int // 0=未命中 LIKE 1=命中相似问/答案 2=命中标准问
		cov  float64
		cntr bool // 命中反例问（降权）
	}
	items := make([]*scored, 0, 64)
	for rows.Next() {
		f, err := scanFAQ(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, &scored{f: f})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	qb := bigrams(q)
	if len(qb) == 0 {
		return nil, nil
	}
	for _, it := range items {
		// LIKE 层
		for _, text := range []string{it.f.StandardQ, strings.Join(it.f.SimilarQs, " "), it.f.Answer} {
			if strings.Contains(text, q) {
				if text == it.f.StandardQ {
					it.like = 2
				} else if it.like < 1 {
					it.like = 1
				}
			}
		}
		// bigram 覆盖率层：查询二元组在 标准问+相似问（+答案）中命中占比
		body := it.f.StandardQ + " " + strings.Join(it.f.SimilarQs, " ") + " " + it.f.Answer
		hit := 0
		for _, b := range qb {
			if strings.Contains(body, b) {
				hit++
			}
		}
		it.cov = float64(hit) / float64(len(qb))
		// 反例问命中标记（排歧降权）
		for _, c := range it.f.CounterQs {
			if strings.Contains(c, q) || bigramCover(c, qb) >= 0.5 {
				it.cntr = true
				break
			}
		}
	}
	// 过滤：LIKE 命中 或 bigram 覆盖率≥0.5
	kept := make([]*scored, 0, len(items))
	for _, it := range items {
		if it.like > 0 || it.cov >= 0.5 {
			kept = append(kept, it)
		}
	}
	// 排序：LIKE 命中（标准问>相似问）> bigram 覆盖率降序；反例问整体后置
	sort.Slice(kept, func(i, j int) bool {
		a, b := kept[i], kept[j]
		if a.cntr != b.cntr {
			return !a.cntr
		}
		sa, sb := a.like*10+int(a.cov*9), b.like*10+int(b.cov*9)
		if sa != sb {
			return sa > sb
		}
		return a.f.UpdatedAt > b.f.UpdatedAt
	})
	if len(kept) > limit {
		kept = kept[:limit]
	}
	out := make([]*FAQ, 0, len(kept))
	for _, it := range kept {
		out = append(out, it.f)
	}
	return out, nil
}

// bigrams 提取字符串中的连续汉字二元组（去重；非中文跳过）。
func bigrams(s string) []string {
	runes := []rune(s)
	var out []string
	seen := map[string]bool{}
	var prev rune
	hasPrev := false
	for _, r := range runes {
		if isHan(r) {
			if hasPrev {
				b := string(prev) + string(r)
				if !seen[b] {
					seen[b] = true
					out = append(out, b)
				}
			}
			prev = r
			hasPrev = true
		} else {
			hasPrev = false
		}
	}
	return out
}

// bigramCover 目标串对给定二元组的覆盖率（用于反例问判定）。
func bigramCover(s string, qb []string) float64 {
	if len(qb) == 0 {
		return 0
	}
	hit := 0
	for _, b := range qb {
		if strings.Contains(s, b) {
			hit++
		}
	}
	return float64(hit) / float64(len(qb))
}

// isHan 汉字（CJK 统一表意文字）判定。
func isHan(r rune) bool {
	return r >= 0x4E00 && r <= 0x9FFF
}

func scanFAQ(rows interface{ Scan(...any) error }) (*FAQ, error) {
	var f FAQ
	var sq, cq, tags, src string
	var by string
	if err := rows.Scan(&f.ID, &f.SpaceID, &f.StandardQ, &sq, &cq, &f.Answer, &tags, &src, &by, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, err
	}
	f.SimilarQs = parseStringArray(sq)
	f.CounterQs = parseStringArray(cq)
	f.Tags = parseStringArray(tags)
	f.CreatedBy = by
	f.Source = src
	return &f, nil
}

// jsonArray 编码字符串数组为 JSON 字符串（空数组兜底）。
func jsonArray(items []string) string {
	if items == nil {
		items = []string{}
	}
	b, _ := json.Marshal(items)
	return string(b)
}

// parseStringArray 解析 JSON 字符串数组（坏数据兜底空数组）。
func parseStringArray(s string) []string {
	var out []string
	if s == "" {
		return []string{}
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

// ---------------- 空间级长期记忆（WeKnora 借鉴 A3 + 云雇工叙事，B27 自上游 55683ab 移植） ----------------
// 按 space 存资料/偏好/事实/事项/兴趣；Agent 与云雇工任务可读写。

// Memory 空间记忆条目。
type Memory struct {
	ID        string `json:"id"`
	SpaceID   string `json:"space_id"`
	Kind      string `json:"kind"`
	Key       string `json:"key"`
	Content   string `json:"content"`
	Source    string `json:"source"`
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// MemorySet 写入/覆盖记忆（同空间同 key 幂等更新）。
func (s *KBStore) MemorySet(ctx context.Context, m *Memory) error {
	if m.SpaceID == "" || strings.TrimSpace(m.Key) == "" {
		return fmt.Errorf("memory: space_id and key required")
	}
	if m.Kind == "" {
		m.Kind = "note"
	}
	now := time.Now().UnixMilli()
	var exists int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM space_memory WHERE space_id=? AND key=?`, m.SpaceID, m.Key).Scan(&exists)
	if exists > 0 {
		_, err := s.db.ExecContext(ctx,
			`UPDATE space_memory SET kind=?, content=?, source=?, updated_at=? WHERE space_id=? AND key=?`,
			m.Kind, m.Content, m.Source, now, m.SpaceID, m.Key)
		return err
	}
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	m.CreatedAt, m.UpdatedAt = now, now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO space_memory(id, space_id, kind, key, content, source, created_by, created_at, updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		m.ID, m.SpaceID, m.Kind, m.Key, m.Content, m.Source, m.CreatedBy, m.CreatedAt, m.UpdatedAt)
	return err
}

// MemoryGet 按 key 取单条。
func (s *KBStore) MemoryGet(ctx context.Context, spaceID, key string) (*Memory, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, space_id, kind, key, content, source, created_by, created_at, updated_at
		 FROM space_memory WHERE space_id=? AND key=?`, spaceID, key)
	return scanMemory(row)
}

// MemoryList 按空间列出（可选 kind 过滤）。
func (s *KBStore) MemoryList(ctx context.Context, spaceID, kind string) ([]*Memory, error) {
	where, args := []string{"space_id=?"}, []any{spaceID}
	if kind != "" {
		where = append(where, "kind=?")
		args = append(args, kind)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, space_id, kind, key, content, source, created_by, created_at, updated_at
		 FROM space_memory WHERE `+strings.Join(where, " AND ")+` ORDER BY updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Memory, 0, 32)
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MemoryDelete 删除记忆（同空间同 key）。
func (s *KBStore) MemoryDelete(ctx context.Context, spaceID, key string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM space_memory WHERE space_id=? AND key=?`, spaceID, key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("memory: not found")
	}
	return nil
}

// MemoryContext 汇总空间记忆为注入上下文文本（Agent 问答/云雇工任务开场合用）。
// 按 kind 分组，每行 "- [kind] key: content"；limit 条以内。
func (s *KBStore) MemoryContext(ctx context.Context, spaceID string, limit int) (string, error) {
	items, err := s.MemoryList(ctx, spaceID, "")
	if err != nil {
		return "", err
	}
	if limit <= 0 {
		limit = 40
	}
	if len(items) > limit {
		items = items[:limit]
	}
	if len(items) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("【空间长期记忆】\n")
	for _, m := range items {
		b.WriteString("- [" + m.Kind + "] " + m.Key + ": " + m.Content + "\n")
	}
	return b.String(), nil
}


func scanMemory(r rowScanner) (*Memory, error) {
	var m Memory
	if err := r.Scan(&m.ID, &m.SpaceID, &m.Kind, &m.Key, &m.Content, &m.Source, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}
