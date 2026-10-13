// Package service 业务编排层。
// file.go 实现文件服务：files 表元数据 + 存储后端的受管写入（实施文档 §4.2/§4.4）。
// 阶段 1 目标：上传/列表/下载/目录/移动/删除可用，事件与审计随行，供 Web UI（拖拽上传）直接消费。
package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
	"github.com/google/uuid"
)

// File 是 files 表的行模型（阶段 1 最小字段集）。
type File struct {
	ID           string `json:"id"`
	SpaceID      string `json:"space_id"`
	OwnerID      string `json:"owner_id"`
	ParentID     string `json:"parent_id,omitempty"`
	Name         string `json:"name"`
	Kind         string `json:"kind"` // file|dir
	Mime         string `json:"mime,omitempty"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256,omitempty"`
	StorageRef   string `json:"storage_ref,omitempty"`
	Version      int    `json:"version"`
	ContentState string `json:"content_state"`
	Slug         string `json:"slug,omitempty"` // 公开稳定链接（博客文章；文件名/分类改名不碎链）
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	PinScope     string `json:"pin_scope,omitempty"` // 文章置顶范围（2.1 置顶底座）：none|category|global
	PinOrder     *int64 `json:"pin_order,omitempty"` // 置顶排序权重（同 scope 内升序；nil=未设）
}

var (
	// ErrNotFound 文件不存在。
	ErrNotFound = errors.New("service: file not found")
	// ErrConflict 同名冲突（父目录下已有同名）。
	ErrConflict = errors.New("service: name conflict")
	// ErrInvalidName 非法名称（空/斜杠/..）。
	ErrInvalidName = errors.New("service: invalid name")
)

// FileStore 文件服务：元数据在 SQLite，内容在存储后端（受管写入）。
type FileStore struct {
	db  *sql.DB
	st  storage.Backend
	b   *bus.Bus
	aud *AuditStore

	// 双链标题索引（name→id 内存映射，懒构建 + dirty 失效，避免 matchWikiTitle O(n) 全表扫）
	wikiMu    sync.Mutex
	wikiIdx   map[string]map[string]string // spaceID → { 全名/去扩展名 → fileID }
	wikiDirty bool
}

// NewFileStore 创建文件服务。
func NewFileStore(db *sql.DB, st storage.Backend, b *bus.Bus, aud *AuditStore) *FileStore {
	return &FileStore{db: db, st: st, b: b, aud: aud}
}

// validName 校验文件名：非空、不含 / 与 .. 。
func validName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return false
	}
	return true
}

// storagePath 计算存储后端内的 key：spaces/{spaceID}/{相对路径}。
func storagePath(spaceID, ref string) string {
	ref = strings.Trim(ref, "/")
	if ref == "" {
		return "spaces/" + spaceID
	}
	return "spaces/" + spaceID + "/" + ref
}

// refOf 从父目录记录计算子项 ref（父 ref 为 "" 表示空间根）。
func refOf(parentRef, name string) string {
	if parentRef == "" {
		return name
	}
	return strings.TrimSuffix(parentRef, "/") + "/" + name
}

// now 统一时间戳（毫秒）。
// 历史遗留：曾返回 time.Now().Unix()（秒），导致 files.created_at/updated_at 与博客读取端
// （按毫秒解析）单位不一致，表现为 RSS pubDate=1970、跨主题排序错乱。现统一为毫秒。
func now() int64 { return time.Now().UnixMilli() }

func scanFile(row interface{ Scan(...any) error }) (*File, error) {
	var f File
	var parentID, mime, sha, state sql.NullString
	if err := row.Scan(&f.ID, &f.SpaceID, &f.OwnerID, &parentID, &f.Name, &f.Kind, &mime, &f.Size, &sha, &f.StorageRef, &f.Version, &state, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, err
	}
	f.ParentID = parentID.String
	f.Mime = mime.String
	f.SHA256 = sha.String
	if state.Valid && state.String != "" {
		f.ContentState = state.String
	} else {
		f.ContentState = `{"visibility":"private","status":"draft"}`
	}
	return &f, nil
}

const fileCols = `id, space_id, owner_id, parent_id, name, kind, mime, size, sha256, storage_ref, version, content_state, created_at, updated_at`

// getByID 内部查询。
func (s *FileStore) getByID(ctx context.Context, id string) (*File, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+fileCols+` FROM files WHERE id=? AND deleted_at IS NULL`, id)
	f, err := scanFile(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

// splitTokens 拆词：按空白与中英文标点切分（解决中文整串 LIKE 不命中）。
func splitTokens(q string) []string {
	if q == "" {
		return nil
	}
	fields := strings.FieldsFunc(q, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', ',', ';', ':', '，', '。', '、', '；', '：', '？', '!', '！', '（', '）', '(', ')', '【', '】', '[', ']', '/', '\\':
			return true
		}
		return false
	})
	out := fields[:0]
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// Get 获取文件元数据。
func (s *FileStore) Get(ctx context.Context, id string) (*File, error) {
	return s.getByID(ctx, id)
}

// resolveParent 取父目录并校验（kind=dir 且未被删除）。
func (s *FileStore) resolveParent(ctx context.Context, parentID, spaceID string) (string, error) {
	if parentID == "" {
		return "", nil // 空间根
	}
	p, err := s.getByID(ctx, parentID)
	if err != nil {
		return "", err
	}
	if p.Kind != "dir" || p.SpaceID != spaceID {
		return "", errors.New("service: invalid parent")
	}
	return p.StorageRef, nil
}

// checkConflict 父目录下同名冲突检查。
func (s *FileStore) checkConflict(ctx context.Context, spaceID, parentID, name, exceptID string) error {
	var n int
	q := `SELECT COUNT(*) FROM files WHERE space_id=? AND parent_id IS ? AND name=? AND deleted_at IS NULL`
	args := []any{spaceID, nil, name}
	if parentID != "" {
		q = `SELECT COUNT(*) FROM files WHERE space_id=? AND parent_id=? AND name=? AND deleted_at IS NULL`
		args = []any{spaceID, parentID, name}
	}
	if exceptID != "" {
		q += ` AND id != ?`
		args = append(args, exceptID)
	}
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrConflict
	}
	return nil
}

// CreateDir 创建目录。
func (s *FileStore) CreateDir(ctx context.Context, ownerID, spaceID, parentID, name string) (*File, error) {
	if !validName(name) {
		return nil, ErrInvalidName
	}
	parentRef, err := s.resolveParent(ctx, parentID, spaceID)
	if err != nil {
		return nil, err
	}
	if err := s.checkConflict(ctx, spaceID, parentID, name, ""); err != nil {
		return nil, err
	}
	id := uuid.NewString()
	ref := refOf(parentRef, name)
	ts := now()
	// 先建磁盘目录，成功后再写 DB——磁盘失败不产生"DB 有记录、磁盘无实体"的幽灵记录。
	// 历史教训：旧实现先 INSERT 后 Mkdir，磁盘被同名软删文件占用时 Mkdir 失败但记录残留，
	// 后续上传/Move 全部基于幽灵记录操作磁盘而失败（上游纠错档案 E-2026-0916-1，已合入）。
	if err := s.st.Mkdir(ctx, storagePath(spaceID, ref)); err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO files (id, space_id, owner_id, parent_id, name, kind, storage_ref, version, content_state, created_at, updated_at)
		 VALUES (?,?,?,?,?, 'dir', ?, 1, '{"visibility":"private","status":"draft"}', ?, ?)`,
		id, spaceID, ownerID, nullIfEmpty(parentID), name, ref, ts, ts)
	if err != nil {
		// DB 写入失败：补偿删除刚建的磁盘目录，保持两侧一致（避免留下空目录实体）
		_ = s.st.Delete(ctx, storagePath(spaceID, ref))
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.created", Key: id, Data: map[string]any{"kind": "dir", "parent_id": parentID, "name": name}})
	_, _ = s.aud.Append(ctx, ownerID, "file.mkdir", id, map[string]any{"name": name, "parent_id": parentID})
	return s.getByID(ctx, id)
}

// Upload 上传文件（multipart 或流式均可；size 为已知长度，未知传 -1）。
func (s *FileStore) Upload(ctx context.Context, ownerID, spaceID, parentID, name, mime string, r io.Reader, size int64) (*File, error) {
	if !validName(name) {
		return nil, ErrInvalidName
	}
	parentRef, err := s.resolveParent(ctx, parentID, spaceID)
	if err != nil {
		return nil, err
	}
	// MIME 兜底：上传端识别失败（octet-stream）时按扩展名推断（文本类文件正确入库，索引分块模板依赖它）
	if mime == "" || mime == "application/octet-stream" {
		if t := ixMimeOf(name); t != "" && !strings.HasPrefix(t, "application/octet-stream") {
			mime = t
		}
	}
	// 同名策略：存在则加后缀（x.md → x-1.md），不覆盖（内容单一数据源，版本管理后置）
	base, ext := name, ""
	if i := strings.LastIndex(name, "."); i > 0 {
		base, ext = name[:i], name[i:]
	}
	finalName := name
	for i := 1; ; i++ {
		if err := s.checkConflict(ctx, spaceID, parentID, finalName, ""); err == nil {
			break
		} else if !errors.Is(err, ErrConflict) {
			return nil, err
		}
		finalName = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	id := uuid.NewString()
	ref := refOf(parentRef, finalName)
	// 流式算内容哈希（SHA256）：查重/一致性校验的基础
	hasher := sha256.New()
	if err := s.st.Put(ctx, storagePath(spaceID, ref), io.TeeReader(r, hasher), size); err != nil {
		return nil, err
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	ts := now()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO files (id, space_id, owner_id, parent_id, name, kind, mime, size, sha256, storage_ref, version, content_state, created_at, updated_at)
		 VALUES (?,?,?,?,?, 'file', ?, ?, ?, ?, 1, '{"visibility":"private","status":"published"}', ?, ?)`,
		id, spaceID, ownerID, nullIfEmpty(parentID), finalName, mime, size, sum, ref, ts, ts)
	if err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.created", Key: id, Data: map[string]any{"kind": "file", "parent_id": parentID, "name": finalName, "size": size, "mime": mime}})
	_, _ = s.aud.Append(ctx, ownerID, "file.upload", id, map[string]any{"name": finalName, "parent_id": parentID, "size": size})
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 文本类文件同步分块索引（"文件里有什么"检索的前提；MVP 同步，量上来切 jobs）
	if err := s.IndexFile(ctx, f); err != nil {
		s.aud.Append(ctx, ownerID, "index.failed", id, map[string]any{"name": finalName, "err": err.Error()})
	}
	s.markWikiTitleDirty()
	return f, nil
}

// ListDir 列出目录内容（按名称排序，目录在前）。
func (s *FileStore) ListDir(ctx context.Context, spaceID, parentID string) ([]*File, error) {
	var rows *sql.Rows
	var err error
	if parentID == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+fileCols+` FROM files WHERE space_id=? AND parent_id IS NULL AND deleted_at IS NULL ORDER BY (kind='dir') DESC, name COLLATE NOCASE`, spaceID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+fileCols+` FROM files WHERE space_id=? AND parent_id=? AND deleted_at IS NULL ORDER BY (kind='dir') DESC, name COLLATE NOCASE`, spaceID, parentID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*File{}
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Tree 取空间目录树（供侧栏懒加载）。
func (s *FileStore) Tree(ctx context.Context, spaceID string) ([]*File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files WHERE space_id=? AND kind='dir' AND deleted_at IS NULL ORDER BY name COLLATE NOCASE`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*File{}
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SearchOpts 检索条件（AI 工具 search_files 与全库检索共用）。
type SearchOpts struct {
	SpaceID string
	Query   string // 关键词（文件名 / 内容）
	Type    string // file | dir | ""（不限）
	Mime    string // MIME 前缀过滤，如 "image/"、"text/"
	Days    int    // 最近 N 天修改；<=0 不限
	Limit   int    // 返回条数；<=0 默认 10
	// 范围过滤（博客目录隐藏后按 scope 限定；两者互斥，同时设置时 Include 优先）
	IncludeIDs []string // 仅在这些文件 id 内搜索（scope=blog：博客子树）
	ExcludeIDs []string // 排除这些文件 id（scope=file：排除博客子树）
}

// SearchHit 检索结果项。
type SearchHit struct {
	File  *File  `json:"file"`
	Score int    `json:"score"`  // 3=文件名命中 2=内容命中 1=仅过滤命中
	HitIn string `json:"hit_in"` // name | content | filter
}

// Search 全库检索：文件名/内容关键词匹配，多词按 OR 召回、命中词数加权排序。
// 阶段 2 前置能力：无向量索引时先用关键词检索（拆词解决中文整串 LIKE 不命中），
// AI 工具层做意图理解；向量化后可叠加语义排序。
func (s *FileStore) Search(ctx context.Context, opts SearchOpts) ([]SearchHit, error) {
	q := strings.TrimSpace(opts.Query)
	if q == "" && opts.Type == "" && opts.Mime == "" && opts.Days <= 0 {
		return nil, errors.New("search: empty criteria")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	tokens := splitTokens(q)

	// 参数顺序必须与 SQL 文本中的 ? 出现顺序一致：
	// 内层 SELECT 的 score CASE（每个 token 2 个 ?）→ WHERE 的 space_id → token 匹配（每个 2 个 ?）→ [type/mime/days] → LIMIT
	var conds []string
	var whereArgs []any
	conds = append(conds, "space_id=?", "deleted_at IS NULL")
	whereArgs = append(whereArgs, opts.SpaceID)
	var scoreParts []string
	var scoreArgs []any
	if len(tokens) > 0 {
		var tokConds []string
		for _, t := range tokens {
			like := "%" + t + "%"
			tokConds = append(tokConds, `(f.name LIKE ? OR EXISTS(SELECT 1 FROM index_chunks c WHERE c.file_id=f.id AND c.content LIKE ?))`)
			whereArgs = append(whereArgs, like, like)
			scoreParts = append(scoreParts, `(CASE WHEN f.name LIKE ? THEN 3
			             WHEN EXISTS(SELECT 1 FROM index_chunks c WHERE c.file_id=f.id AND c.content LIKE ?) THEN 2
			             ELSE 0 END)`)
			scoreArgs = append(scoreArgs, like, like)
		}
		conds = append(conds, "("+strings.Join(tokConds, " OR ")+")")
	}
	if opts.Type != "" {
		conds = append(conds, "kind=?")
		whereArgs = append(whereArgs, opts.Type)
	}
	if opts.Mime != "" {
		conds = append(conds, "mime LIKE ?")
		whereArgs = append(whereArgs, opts.Mime+"%")
	}
	if opts.Days > 0 {
		conds = append(conds, "updated_at >= ?")
		whereArgs = append(whereArgs, time.Now().UnixMilli()-int64(opts.Days)*86400*1000)
	}
	if len(opts.IncludeIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(opts.IncludeIDs)), ",")
		conds = append(conds, "f.id IN ("+ph+")")
		for _, id := range opts.IncludeIDs {
			whereArgs = append(whereArgs, id)
		}
	} else if len(opts.ExcludeIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(opts.ExcludeIDs)), ",")
		conds = append(conds, "f.id NOT IN ("+ph+")")
		for _, id := range opts.ExcludeIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	scoreExpr := "0"
	if len(scoreParts) > 0 {
		scoreExpr = strings.Join(scoreParts, " + ")
	}
	args := append(scoreArgs, whereArgs...)
	args = append(args, limit)
	query := `SELECT ` + fileCols + `, (` + scoreExpr + `) AS score
	        FROM files f WHERE ` + strings.Join(conds, " AND ") + `
	        ORDER BY score DESC, (kind='dir') DESC, name COLLATE NOCASE LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchHit{}
	for rows.Next() {
		// modernc sqlite 驱动要求一次 Scan 全部列：fileCols(14) + score(1)
		var f File
		var parentID, mime, sha, state sql.NullString
		var score int
		if err := rows.Scan(&f.ID, &f.SpaceID, &f.OwnerID, &parentID, &f.Name, &f.Kind, &mime, &f.Size, &sha, &f.StorageRef, &f.Version, &state, &f.CreatedAt, &f.UpdatedAt, &score); err != nil {
			return nil, err
		}
		f.ParentID = parentID.String
		f.Mime = mime.String
		f.SHA256 = sha.String
		if state.Valid && state.String != "" {
			f.ContentState = state.String
		} else {
			f.ContentState = `{"visibility":"private","status":"draft"}`
		}
		hitIn := "filter"
		if score >= 3 {
			hitIn = "name"
		} else if score >= 2 {
			hitIn = "content"
		}
		out = append(out, SearchHit{File: &f, Score: score, HitIn: hitIn})
	}
	return out, rows.Err()
}
func (s *FileStore) Content(ctx context.Context, id string) (io.ReadCloser, *File, error) {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if f.Kind != "file" {
		return nil, nil, errors.New("service: not a file")
	}
	rc, _, err := s.st.Get(ctx, storagePath(f.SpaceID, f.StorageRef))
	if err != nil {
		return nil, nil, err
	}
	return rc, f, nil
}

// CreateDoc 新建文档（文本类文件，带初始内容；用于"新建文档/笔记"入口）。
func (s *FileStore) CreateDoc(ctx context.Context, ownerID, spaceID, parentID, name, content string) (*File, error) {
	if !validName(name) {
		return nil, ErrInvalidName
	}
	mime := "text/markdown"
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ext != "" && ext != "md" && ext != "markdown" {
		mime = "text/plain"
	}
	if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".markdown") {
		mime = "text/markdown"
	}
	f, err := s.Upload(ctx, ownerID, spaceID, parentID, name, mime, strings.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, err
	}
	// 新建文档默认草稿（进草稿箱）；发布/分享后转 published
	if _, err := s.db.ExecContext(ctx,
		`UPDATE files SET content_state=json_set(content_state, '$.status', 'draft'), updated_at=? WHERE id=?`,
		now(), f.ID); err != nil {
		return nil, err
	}
	s.markWikiTitleDirty()
	return s.getByID(ctx, f.ID)
}

// UpdateContent 更新文本类文件内容（覆盖写 + 版本自增 + 重索引；非文本文件拒绝）。
func (s *FileStore) UpdateContent(ctx context.Context, ownerID, id, content string) (*File, error) {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.Kind != "file" {
		return nil, errors.New("service: not a file")
	}
	if !isTextMime(f.Mime) {
		return nil, errors.New("service: only text files are editable")
	}
	if err := s.st.Put(ctx, storagePath(f.SpaceID, f.StorageRef), strings.NewReader(content), int64(len(content))); err != nil {
		return nil, err
	}
	// 内容变更 → 重算内容哈希
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(content))
	sum := hex.EncodeToString(hasher.Sum(nil))
	ts := now()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE files SET size=?, sha256=?, version=version+1, updated_at=? WHERE id=?`,
		len(content), sum, ts, id); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.updated", Key: id, Data: map[string]any{"kind": "file", "size": len(content), "version": f.Version + 1}})
	_, _ = s.aud.Append(ctx, ownerID, "file.update", id, map[string]any{"name": f.Name, "size": len(content), "version": f.Version + 1})
	nf, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 重索引内容（内容变了，旧 chunks/向量/摘要失效；阶段 1 同步重建）
	if err := s.IndexFile(ctx, nf); err != nil {
		s.aud.Append(ctx, ownerID, "index.failed", id, map[string]any{"name": f.Name, "err": err.Error()})
	}
	return nf, nil
}

// isTextMime 判断是否文本类文件（可在线编辑）。
func isTextMime(mime string) bool {
	if mime == "" {
		return false
	}
	return strings.HasPrefix(mime, "text/") ||
		strings.Contains(mime, "markdown") ||
		strings.Contains(mime, "json") ||
		strings.Contains(mime, "xml") ||
		strings.Contains(mime, "yaml") ||
		strings.Contains(mime, "javascript") ||
		strings.Contains(mime, "typescript") ||
		strings.Contains(mime, "html") ||
		strings.Contains(mime, "css") ||
		strings.Contains(mime, "sql") ||
		strings.Contains(mime, "csv") ||
		strings.Contains(mime, "shell")
}

// Move 移动/重命名（同空间内）。newParentID 空 = 移到根；newName 空 = 保持原名。
func (s *FileStore) Move(ctx context.Context, ownerID, id, newParentID, newName string) (*File, error) {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if newName == "" {
		newName = f.Name
	}
	if !validName(newName) {
		return nil, ErrInvalidName
	}
	// 不能移到自己/自己的后代（简单深度检查：父链不循环）
	if newParentID != "" {
		if newParentID == id {
			return nil, errors.New("service: cannot move into itself")
		}
		p, err := s.getByID(ctx, newParentID)
		if err != nil {
			return nil, err
		}
		if p.Kind != "dir" || p.SpaceID != f.SpaceID {
			return nil, errors.New("service: invalid target parent")
		}
		for cur := p.ParentID; cur != ""; {
			if cur == id {
				return nil, errors.New("service: cannot move into itself")
			}
			cur2, err := s.getByID(ctx, cur)
			if err != nil {
				return nil, err
			}
			cur = cur2.ParentID
		}
	}
	// 同名自动改名（与上传/复制一致：不覆盖，x.md → x-1.md）
	base := newName
	ext := ""
	if i := strings.LastIndex(base, "."); i > 0 {
		ext = base[i:]
		base = base[:i]
	}
	candidate := newName
	for n := 1; ; n++ {
		if err := s.checkConflict(ctx, f.SpaceID, newParentID, candidate, id); err == nil {
			break
		} else if !errors.Is(err, ErrConflict) {
			return nil, err
		}
		candidate = fmt.Sprintf("%s-%d%s", base, n, ext)
	}
	newName = candidate
	newParentRef, err := s.resolveParent(ctx, newParentID, f.SpaceID)
	if err != nil {
		return nil, err
	}
	newRef := refOf(newParentRef, newName)
	if err := s.st.Move(ctx, storagePath(f.SpaceID, f.StorageRef), storagePath(f.SpaceID, newRef)); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	// 目录移动：递归更新子树所有子项的 storage_ref（旧前缀 → 新前缀），
	// 否则子文件 ref 指向旧路径，读取/导出/索引全部失效。
	if f.Kind == "dir" && f.StorageRef != newRef {
		oldPrefix := strings.TrimSuffix(f.StorageRef, "/") + "/"
		newPrefix := strings.TrimSuffix(newRef, "/") + "/"
		// LIKE 通配符转义：目录名可能含 %/_（如 "100%done"），不转义会错误匹配
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(oldPrefix)
		// substr 按字符计，len() 是字节数：中文目录名必须用 RuneCount 定位切点
		cut := utf8.RuneCountInString(oldPrefix) + 1
		if _, err := s.db.ExecContext(ctx,
			`UPDATE files SET storage_ref = ? || substr(storage_ref, ?) WHERE space_id=? AND storage_ref LIKE ? ESCAPE '\' AND deleted_at IS NULL`,
			newPrefix, cut, f.SpaceID, esc+"%"); err != nil {
			return nil, err
		}
	}
	ts := now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE files SET parent_id=?, name=?, storage_ref=?, updated_at=? WHERE id=?`,
		nullIfEmpty(newParentID), newName, newRef, ts, id)
	if err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.moved", Key: id, Data: map[string]any{"from_parent": f.ParentID, "to_parent": newParentID, "name": newName}})
	_, _ = s.aud.Append(ctx, ownerID, "file.move", id, map[string]any{"from": f.ParentID, "to": newParentID, "name": newName})
	s.markWikiTitleDirty()
	return s.getByID(ctx, id)
}

// Copy 复制文件到目标目录（同空间）。newName 空 = 保持原名；同名自动改名（x.md→x-1.md）。
// 目录复制（含子树）阶段 3 提供，当前返回明确错误。
func (s *FileStore) Copy(ctx context.Context, ownerID, id, targetParentID, newName string) (*File, error) {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.Kind == "dir" {
		return nil, errors.New("service: 目录复制将在阶段 3 提供")
	}
	if targetParentID != "" {
		p, err := s.getByID(ctx, targetParentID)
		if err != nil {
			return nil, err
		}
		if p.Kind != "dir" || p.SpaceID != f.SpaceID {
			return nil, errors.New("service: invalid target parent")
		}
	}
	name := newName
	if name == "" {
		name = f.Name
	}
	if !validName(name) {
		return nil, ErrInvalidName
	}
	// 同名自动改名（与上传一致：不覆盖）
	base := name
	ext := ""
	if i := strings.LastIndex(base, "."); i > 0 {
		ext = base[i:]
		base = base[:i]
	}
	candidate := name
	for n := 1; ; n++ {
		if err := s.checkConflict(ctx, f.SpaceID, targetParentID, candidate, ""); err == nil {
			break
		} else if !errors.Is(err, ErrConflict) {
			return nil, err
		}
		candidate = fmt.Sprintf("%s-%d%s", base, n, ext)
	}
	targetRef, err := s.resolveParent(ctx, targetParentID, f.SpaceID)
	if err != nil {
		return nil, err
	}
	newRef := refOf(targetRef, candidate)

	// 内容复制：流式拷贝（避免大文件全读内存）
	rc, _, err := s.st.Get(ctx, storagePath(f.SpaceID, f.StorageRef))
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	if err := s.st.Put(ctx, storagePath(f.SpaceID, newRef), rc, f.Size); err != nil {
		return nil, err
	}

	ts := now()
	newID := uuid.NewString()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO files (id, space_id, owner_id, parent_id, name, kind, mime, size, sha256, storage_ref, version, content_state, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,1,?,?,?)`,
		newID, f.SpaceID, ownerID, nullIfEmpty(targetParentID), candidate, "file", f.Mime, f.Size, f.SHA256, newRef, f.ContentState, ts, ts)
	if err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.created", Key: newID, Data: map[string]any{"parent": targetParentID, "name": candidate, "from": id}})
	_, _ = s.aud.Append(ctx, ownerID, "file.copy", newID, map[string]any{"from": id, "to": targetParentID, "name": candidate})
	s.markWikiTitleDirty()
	return s.getByID(ctx, newID)
}

// ListDrafts 全空间草稿文件（content_state.status=draft，草稿箱视图数据源）。
func (s *FileStore) ListDrafts(ctx context.Context, spaceID string) ([]*File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files WHERE space_id=? AND kind='file' AND content_state LIKE '%"status":"draft"%' AND deleted_at IS NULL ORDER BY updated_at DESC`,
		spaceID)
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
	return out, rows.Err()
}

// ListByTag 按标签跨目录筛选文件（标签筛选 / 文件库 tag 视图共用）。
func (s *FileStore) ListByTag(ctx context.Context, spaceID, tagID string) ([]*File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files f
		 JOIN file_tags ft ON ft.file_id = f.id
		 WHERE f.space_id=? AND ft.tag_id=? AND f.deleted_at IS NULL
		 ORDER BY f.updated_at DESC`, spaceID, tagID)
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
	return out, rows.Err()
}

// SetStatus 更新内容状态（draft/published），草稿箱与发布闭环。
func (s *FileStore) SetStatus(ctx context.Context, ownerID, id, status string) (*File, error) {
	if status != "draft" && status != "published" {
		return nil, errors.New("service: invalid status")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE files SET content_state=json_set(content_state, '$.status', ?), updated_at=? WHERE id=? AND deleted_at IS NULL`,
		status, now(), id); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.status", Key: id, Data: map[string]any{"status": status}})
	_, _ = s.aud.Append(ctx, ownerID, "file.status", id, map[string]any{"status": status})
	return s.getByID(ctx, id)
}

// Delete 软删除（回收站语义；目录递归软删整棵子树，文件同时删后端内容；事务保证一致）。
func (s *FileStore) Delete(ctx context.Context, ownerID, id string) error {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return err
	}
	ts := now()
	// 纯软删：只标记 deleted_at（子树递归），blob 与子表数据全部保留——
	// 回收站恢复后内容/标签/摘要/分享完整可用。彻底删除走 Purge（硬删 blob + 清子表）。
	// 修复历史 bug：旧实现软删 DB 记录却硬删 blob，恢复后读内容报 "storage: not found"（纠错档案 E22）。
	sub := `
		WITH RECURSIVE sub(id) AS (
			SELECT id FROM files WHERE id=?
			UNION ALL
			SELECT f.id FROM files f JOIN sub ON f.parent_id = sub.id
		)`
	if f.Kind == "dir" {
		if _, err := s.db.ExecContext(ctx, sub+` UPDATE files SET deleted_at=? WHERE id IN (SELECT id FROM sub)`, id, ts); err != nil {
			return err
		}
	} else {
		if _, err := s.db.ExecContext(ctx, `UPDATE files SET deleted_at=? WHERE id=?`, ts, id); err != nil {
			return err
		}
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.deleted", Key: id, Data: map[string]any{"kind": f.Kind}})
	_, _ = s.aud.Append(ctx, ownerID, "file.delete", id, map[string]any{"kind": f.Kind, "name": f.Name})
	s.markWikiTitleDirty()
	return nil
}

// EnsureRoot 确保空间根目录占位存在（创建空间时调用，阶段 1 由 seed 处理）。
func (s *FileStore) EnsureRoot(ctx context.Context, spaceID string) error {
	_ = s.st.Put(ctx, storagePath(spaceID, ""), strings.NewReader(""), 0)
	return nil
}

// EnsurePath 按相对目录链逐级查找/创建目录，返回末级目录 id（供文件夹拖拽上传使用）。
// relDir 形如 "a/b/c"（正斜杠），空返回 parentID。
func (s *FileStore) EnsurePath(ctx context.Context, ownerID, spaceID, parentID, relDir string) (string, error) {
	relDir = strings.Trim(strings.ReplaceAll(relDir, "\\", "/"), "/")
	if relDir == "" {
		return parentID, nil
	}
	cur := parentID
	for _, seg := range strings.Split(relDir, "/") {
		if seg == "" || seg == "." {
			continue
		}
		var id string
		var err error
		if cur == "" {
			err = s.db.QueryRowContext(ctx,
				`SELECT id FROM files WHERE parent_id IS NULL AND name=? AND kind='dir' AND deleted_at IS NULL`,
				seg).Scan(&id)
		} else {
			err = s.db.QueryRowContext(ctx,
				`SELECT id FROM files WHERE parent_id=? AND name=? AND kind='dir' AND deleted_at IS NULL`,
				cur, seg).Scan(&id)
		}
		if err == sql.ErrNoRows {
			d, err := s.CreateDir(ctx, ownerID, spaceID, cur, seg)
			if err != nil {
				return "", err
			}
			cur = d.ID
			continue
		}
		if err != nil {
			return "", err
		}
		cur = id
	}
	return cur, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// MarshalContentState 供前端读取 content_state 用。
func (f *File) State() (vis, status string) {
	var st struct {
		Visibility string `json:"visibility"`
		Status     string `json:"status"`
	}
	_ = json.Unmarshal([]byte(f.ContentState), &st)
	if st.Visibility == "" {
		st.Visibility = "private"
	}
	if st.Status == "" {
		st.Status = "draft"
	}
	return st.Visibility, st.Status
}

// ---- 回收站（软删视图 / 恢复 / 彻底删除 / 清空） ----

// getByIDIncludingDeleted 查询含已软删记录（回收站操作需要拿到已删文件的父级/名字）。
func (s *FileStore) getByIDIncludingDeleted(ctx context.Context, id string) (*File, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+fileCols+` FROM files WHERE id=?`, id)
	f, err := scanFile(row)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// ListTrash 回收站视图：全空间软删文件（含目录），按删除时间倒序。
func (s *FileStore) ListTrash(ctx context.Context, spaceID string) ([]*File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files WHERE space_id=? AND deleted_at IS NOT NULL ORDER BY deleted_at DESC`,
		spaceID)
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
	return out, rows.Err()
}

// trashSubtree 递归收集 id 的整棵子树 id（含自身；含已软删项）。
func (s *FileStore) trashSubtree(ctx context.Context, id string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE sub(id) AS (
			SELECT ?
			UNION ALL
			SELECT f.id FROM files f JOIN sub ON f.parent_id = sub.id
		) SELECT id FROM sub`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		out = append(out, cid)
	}
	return out, rows.Err()
}

// Restore 从回收站恢复。目录整棵子树一起恢复；目标位置同名自动重命名（x.md→x-1.md，与移动一致）。
func (s *FileStore) Restore(ctx context.Context, ownerID, id string) (*File, error) {
	f, err := s.getByIDIncludingDeleted(ctx, id)
	if err != nil {
		return nil, err
	}
	newName := f.Name
	base := newName
	ext := ""
	if i := strings.LastIndex(base, "."); i > 0 {
		ext = base[i:]
		base = base[:i]
	}
	candidate := newName
	for n := 1; ; n++ {
		if err := s.checkConflict(ctx, f.SpaceID, f.ParentID, candidate, id); err == nil {
			break
		} else if !errors.Is(err, ErrConflict) {
			return nil, err
		}
		candidate = fmt.Sprintf("%s-%d%s", base, n, ext)
	}
	if candidate != newName {
		// 文件/目录改名：文件需同步存储 key
		if f.Kind == "file" {
			parentRef, err := s.resolveParent(ctx, f.ParentID, f.SpaceID)
			if err != nil {
				return nil, err
			}
			newRef := refOf(parentRef, candidate)
			if err := s.st.Move(ctx, storagePath(f.SpaceID, f.StorageRef), storagePath(f.SpaceID, newRef)); err != nil && !errors.Is(err, storage.ErrNotFound) {
				return nil, err
			}
			if _, err := s.db.ExecContext(ctx,
				`UPDATE files SET deleted_at=NULL, name=?, storage_ref=?, updated_at=? WHERE id=?`,
				candidate, newRef, now(), id); err != nil {
				return nil, err
			}
		} else {
			if _, err := s.db.ExecContext(ctx,
				`UPDATE files SET deleted_at=NULL, name=?, updated_at=? WHERE id=?`,
				candidate, now(), id); err != nil {
				return nil, err
			}
		}
	} else {
		// 无同名冲突：恢复整棵子树（目录的子树一并回来）
		if _, err := s.db.ExecContext(ctx,
			`UPDATE files SET deleted_at=NULL, updated_at=? WHERE id IN (SELECT id FROM (`+
				`WITH RECURSIVE sub(id) AS (SELECT ? UNION ALL SELECT f.id FROM files f JOIN sub ON f.parent_id = sub.id) SELECT id FROM sub))`,
			now(), id); err != nil {
			return nil, err
		}
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.restored", Key: id, Data: map[string]any{"name": f.Name}})
	_, _ = s.aud.Append(ctx, ownerID, "file.restore", id, map[string]any{"name": f.Name})
	s.markWikiTitleDirty()
	return s.getByID(ctx, id)
}

// Purge 彻底删除（不可恢复）：行 + 关联子表 + 存储内容。子树递归。
func (s *FileStore) Purge(ctx context.Context, ownerID, id string) error {
	f, err := s.getByIDIncludingDeleted(ctx, id)
	if err != nil {
		return err
	}
	ids, err := s.trashSubtree(ctx, id)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	// 先删引用子表（vectors.chunk_id 引用 index_chunks，须在 chunks 之前）
	for _, t := range []string{"vectors", "file_ai_summaries", "file_tags", "collection_files", "shares", "comments"} {
		q := `DELETE FROM ` + t + ` WHERE file_id IN (` + placeholders(len(ids)) + `)`
		args := make([]any, len(ids))
		for i, v := range ids {
			args[i] = v
		}
		if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
			return err
		}
	}
	q := `DELETE FROM index_chunks WHERE file_id IN (` + placeholders(len(ids)) + `)`
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return err
	}
	// 删存储内容（子树中所有文件）
	rows, err := s.db.QueryContext(ctx, `SELECT storage_ref, space_id FROM files WHERE id IN (`+placeholders(len(ids))+`) AND kind='file' AND storage_ref<>''`, args...)
	if err == nil {
		var refs []struct{ ref, space string }
		for rows.Next() {
			var r, sp string
			_ = rows.Scan(&r, &sp)
			refs = append(refs, struct{ ref, space string }{r, sp})
		}
		rows.Close()
		for _, rf := range refs {
			_ = s.st.Delete(ctx, storagePath(rf.space, rf.ref))
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE id IN (`+placeholders(len(ids))+`)`, args...); err != nil {
		return err
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.purged", Key: id, Data: map[string]any{"name": f.Name}})
	_, _ = s.aud.Append(ctx, ownerID, "file.purge", id, map[string]any{"name": f.Name})
	s.markWikiTitleDirty()
	return nil
}

// EmptyTrash 清空回收站（全部彻底删除）。单个失败不中断。
func (s *FileStore) EmptyTrash(ctx context.Context, ownerID, spaceID string) (int, error) {
	list, err := s.ListTrash(ctx, spaceID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range list {
		if err := s.Purge(ctx, ownerID, f.ID); err != nil {
			continue // 已被父级 Purge 连带删除的跳过
		}
		n++
	}
	return n, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}
