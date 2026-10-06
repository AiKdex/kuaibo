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
	ViewCount    int64  `json:"view_count"`          // 累计阅读量（files.view_count 列；WebDAV/Org 列表展示用）
	SiteID       string `json:"site_id,omitempty"`   // 多站点归属（SPEC-MS-001）：内容隔离核心；缺省 'default'
}

var (
	// ErrNotFound 文件不存在。
	ErrNotFound = errors.New("service: file not found")
	// ErrConflict 同名冲突（父目录下已有同名）。
	ErrConflict = errors.New("service: name conflict")
	// ErrInvalidName 非法名称（空/斜杠/..）。
	ErrInvalidName = errors.New("service: invalid name")
	// ErrForbidden 无写权限（C3：调用者对目标文件所在空间无写权限）。
	ErrForbidden = errors.New("service: forbidden")
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
	if err := row.Scan(&f.ID, &f.SpaceID, &f.OwnerID, &parentID, &f.Name, &f.Kind, &mime, &f.Size, &sha, &f.StorageRef, &f.Version, &state, &f.CreatedAt, &f.UpdatedAt, &f.SiteID); err != nil {
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

const fileCols = `id, space_id, owner_id, parent_id, name, kind, mime, size, sha256, storage_ref, version, content_state, created_at, updated_at, site_id`

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

// requireSpaceWrite 校验调用者对目标空间有写权限（C3：文件写操作归属校验，
// 防任意登录用户删改他人文件；服务层兜底——即使某端点漏加 handler 守卫也不会绕过）。
// 放行条件（任一）：
//   - ownerID 为空或系统 owner（SystemOwnerID）：内部/系统调用（桥接器、定时任务、服务令牌、AI 编排）
//   - spaces.owner_id = ownerID：空间所有者（全权）
//   - space_members.role = 'editor'：协作空间成员（可写；viewer 只读，拒绝）
//
// AiKlog 适配：本 fork 的博客内容统一落在站长 home 空间，handler 侧由 writeActor 把
// 「站长/管理员/作者白名单成员」归一为站长主体，因此作者与内部调用均命中第 1/2 条；
// 未被授权的一般登录用户其真实 uid 不命中任何一条 → ErrForbidden。
func (s *FileStore) requireSpaceWrite(ctx context.Context, ownerID, spaceID string) error {
	if ownerID == "" || ownerID == SystemOwnerID {
		return nil
	}
	var oid string
	if err := s.db.QueryRowContext(ctx, `SELECT owner_id FROM spaces WHERE id=?`, spaceID).Scan(&oid); err != nil {
		return ErrNotFound
	}
	if oid == ownerID {
		return nil
	}
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM space_members WHERE space_id=? AND user_id=?`, spaceID, ownerID).Scan(&role)
	if err == nil && role == "editor" {
		return nil
	}
	return ErrForbidden
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
func (s *FileStore) CreateDir(ctx context.Context, ownerID, spaceID, parentID, name, siteID string) (*File, error) {
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
		`INSERT INTO files (id, space_id, owner_id, parent_id, name, kind, storage_ref, version, content_state, created_at, updated_at, site_id)
		 VALUES (?,?,?,?,?, 'dir', ?, 1, '{"visibility":"private","status":"draft"}', ?, ?, ?)`,
		id, spaceID, ownerID, nullIfEmpty(parentID), name, ref, ts, ts, siteID)
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
func (s *FileStore) Upload(ctx context.Context, ownerID, spaceID, parentID, name, mime string, r io.Reader, size int64, siteID string) (*File, error) {
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
		`INSERT INTO files (id, space_id, owner_id, parent_id, name, kind, mime, size, sha256, storage_ref, version, content_state, created_at, updated_at, site_id)
		 VALUES (?,?,?,?,?, 'file', ?, ?, ?, ?, 1, '{"visibility":"private","status":"published"}', ?, ?, ?)`,
		id, spaceID, ownerID, nullIfEmpty(parentID), finalName, mime, size, sum, ref, ts, ts, siteID)
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
func (s *FileStore) ListDir(ctx context.Context, spaceID, parentID, siteID string) ([]*File, error) {
	var rows *sql.Rows
	var err error
	if parentID == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+fileCols+` FROM files WHERE space_id=? AND parent_id IS NULL AND site_id=? AND deleted_at IS NULL ORDER BY (kind='dir') DESC, name COLLATE NOCASE`, spaceID, siteID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+fileCols+` FROM files WHERE space_id=? AND parent_id=? AND site_id=? AND deleted_at IS NULL ORDER BY (kind='dir') DESC, name COLLATE NOCASE`, spaceID, parentID, siteID)
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
		// modernc sqlite 驱动要求一次 Scan 全部列：fileCols(15) + score(1)
		var f File
		var parentID, mime, sha, state sql.NullString
		var score int
		if err := rows.Scan(&f.ID, &f.SpaceID, &f.OwnerID, &parentID, &f.Name, &f.Kind, &mime, &f.Size, &sha, &f.StorageRef, &f.Version, &state, &f.CreatedAt, &f.UpdatedAt, &f.SiteID, &score); err != nil {
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

// LocalPath 返回文件在**本地磁盘后端**上的宿主机绝对路径。
//
// 仅当存储后端实现 storage.PathLocator（本地盘 / 本地挂载点）时可用；
// 远程后端（S3 / WebDAV）一律 ok=false，调用方据此降级为
// probe_status='unsupported' —— 不要为此把整个对象拉回本地（音视频动辄数百 MB，
// 代价远大于「这次探测不到」）。
func (s *FileStore) LocalPath(ctx context.Context, id string) (string, bool) {
	f, err := s.getByID(ctx, id)
	if err != nil || f.Kind != "file" {
		return "", false
	}
	loc, ok := s.st.(storage.PathLocator)
	if !ok {
		return "", false
	}
	return loc.LocalPath(storagePath(f.SpaceID, f.StorageRef))
}

// CreateDoc 新建文档（文本类文件，带初始内容；用于"新建文档/笔记"入口）。
func (s *FileStore) CreateDoc(ctx context.Context, ownerID, spaceID, parentID, name, content, siteID string) (*File, error) {
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
	f, err := s.Upload(ctx, ownerID, spaceID, parentID, name, mime, strings.NewReader(content), int64(len(content)), siteID)
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
	if err := s.requireSpaceWrite(ctx, ownerID, f.SpaceID); err != nil {
		return nil, err
	}
	if f.Kind != "file" {
		return nil, errors.New("service: not a file")
	}
	if !isTextMime(f.Mime) {
		return nil, errors.New("service: only text files are editable")
	}
	// B5 版本历史：覆盖写前先给「旧内容」留快照（v=f.Version，必须在 Put 之前）。
	// 快照失败只记审计、不阻断本次保存 —— 版本历史是增值能力，不该连累用户这次编辑。
	if err := s.SaveVersion(ctx, ownerID, id, f.Version, "编辑前快照"); err != nil {
		_, _ = s.aud.Append(ctx, ownerID, "version.snapshot_failed", id, map[string]any{"version": f.Version, "err": err.Error()})
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

// ReplaceContentBinary 二进制覆盖写：保持文件名与位置不变，仅替换内容与哈希（版本自增）。
// 与 Upload 的区别：Upload 遇同名会加后缀另存，本方法是「原地覆盖」——WebDAV PUT 已存在文件的
// 语义要求（否则每次保存都会复制出新文件）。任意 mime 均可（不限文本）。
func (s *FileStore) ReplaceContentBinary(ctx context.Context, ownerID, id string, r io.Reader, size int64) (*File, error) {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireSpaceWrite(ctx, ownerID, f.SpaceID); err != nil {
		return nil, err
	}
	if f.Kind != "file" {
		return nil, errors.New("service: not a file")
	}
	// 调用方常复用同一个 *os.File（写完未回绕），此处统一回绕，避免写入空内容
	if sk, ok := r.(io.Seeker); ok {
		if _, err := sk.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("service: 内容流回绕失败: %w", err)
		}
	}
	// B5 版本历史：二进制覆盖前快照旧内容（同样失败不阻断）。
	if err := s.SaveVersion(ctx, ownerID, id, f.Version, "二进制覆盖前快照"); err != nil {
		_, _ = s.aud.Append(ctx, ownerID, "version.snapshot_failed", id, map[string]any{"version": f.Version, "err": err.Error()})
	}
	if err := s.st.Put(ctx, storagePath(f.SpaceID, f.StorageRef), r, size); err != nil {
		return nil, err
	}
	ts := now()
	// 哈希留空：二进制大文件不做全量重算（内容以存储后端为准，sha256 由上传侧提供/可选）
	if _, err := s.db.ExecContext(ctx,
		`UPDATE files SET size=?, version=version+1, updated_at=? WHERE id=?`, size, ts, id); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.updated", Key: id, Data: map[string]any{"kind": "file", "size": size, "version": f.Version + 1, "via": "replace_binary"}})
	_, _ = s.aud.Append(ctx, ownerID, "file.replace_binary", id, map[string]any{"name": f.Name, "size": size, "version": f.Version + 1})
	nf, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 文本文件覆盖后重索引（内容变了，旧 chunks/向量/摘要失效）
	if isTextMime(nf.Mime) {
		if err := s.IndexFile(ctx, nf); err != nil {
			_, _ = s.aud.Append(ctx, ownerID, "index.failed", id, map[string]any{"name": nf.Name, "err": err.Error()})
		}
	}
	return nf, nil
}

// isTextMime 判断是否文本类文件（可在线编辑）。
//
// 结构化白名单：前缀 text/、后缀 +json/+xml，或下方显式枚举的 application/* 文本类型。
// 历史实现用 strings.Contains 做子串匹配，会把
// "application/vnd.openxmlformats-officedocument.wordprocessingml.document"（.docx/.xlsx/.pptx）
// 里的 "xml" 误判为文本 —— 于是二进制被当文本读出、再写回即损坏文件，故收紧为白名单。
func isTextMime(mime string) bool {
	if mime == "" {
		return false
	}
	m := strings.ToLower(strings.TrimSpace(mime))
	if i := strings.IndexByte(m, ';'); i >= 0 { // 去掉 charset 等参数
		m = strings.TrimSpace(m[:i])
	}
	if strings.HasPrefix(m, "text/") {
		return true
	}
	if strings.HasSuffix(m, "+json") || strings.HasSuffix(m, "+xml") {
		return true
	}
	switch m {
	case "application/json",
		"application/xml",
		"application/yaml",
		"application/x-yaml",
		"application/toml",
		"application/x-toml",
		"application/javascript",
		"application/x-javascript",
		"application/ecmascript",
		"application/typescript",
		"application/sql",
		"application/x-sql",
		"application/x-sh",
		"application/x-shellscript",
		"application/x-markdown",
		"application/rtf":
		return true
	}
	return false
}

// Move 移动/重命名（同空间内）。newParentID 空 = 移到根；newName 空 = 保持原名。
func (s *FileStore) Move(ctx context.Context, ownerID, id, newParentID, newName string) (*File, error) {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireSpaceWrite(ctx, ownerID, f.SpaceID); err != nil {
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
	if err := s.requireSpaceWrite(ctx, ownerID, f.SpaceID); err != nil {
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
	// C3 修复：写操作前置归属校验（原实现直接按 id UPDATE，任意登录用户可改他人文章状态）
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireSpaceWrite(ctx, ownerID, f.SpaceID); err != nil {
		return nil, err
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
	if err := s.requireSpaceWrite(ctx, ownerID, f.SpaceID); err != nil {
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
			d, err := s.CreateDir(ctx, ownerID, spaceID, cur, seg, DefaultSiteID)
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

// NodeType 读取内容类型（A-G 契约：content_state.node_type）。
// 缺省空串，约定为普通文章（post）；资源站/导航站等生产场景由主题按此差异化渲染。
func (f *File) NodeType() string {
	var st struct {
		NodeType string `json:"node_type"`
	}
	_ = json.Unmarshal([]byte(f.ContentState), &st)
	return st.NodeType
}

// Fields 读取自定义业务字段（A-G 契约：content_state.fields）。
// 例如资源条目 {"file_id":"...","download_price":990}、导航条目 {"url":"...","logo":"..."}。
func (f *File) Fields() map[string]any {
	var st struct {
		Fields map[string]any `json:"fields"`
	}
	_ = json.Unmarshal([]byte(f.ContentState), &st)
	return st.Fields
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
	if err := s.requireSpaceWrite(ctx, ownerID, f.SpaceID); err != nil {
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
	if err := s.requireSpaceWrite(ctx, ownerID, f.SpaceID); err != nil {
		return err
	}
	ids, err := s.trashSubtree(ctx, id)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	// B5 版本历史：先把快照位置取出来（file_versions 行马上会随子表一起删除），稍后删 blob。
	type verRef struct {
		space string
		fid   string
		ver   int
	}
	var vrefs []verRef
	if vrows, verr := s.db.QueryContext(ctx,
		`SELECT f.space_id, fv.file_id, fv.version FROM file_versions fv
		 JOIN files f ON f.id = fv.file_id WHERE fv.file_id IN (`+placeholders(len(ids))+`)`, args...); verr == nil {
		for vrows.Next() {
			var r verRef
			if err := vrows.Scan(&r.space, &r.fid, &r.ver); err == nil {
				vrefs = append(vrefs, r)
			}
		}
		vrows.Close()
	}
	// B6：子树下的评论 id 先收集 —— comments 行马上会被删，而它们的 @提及 也要一起清。
	var commentIDs []string
	if crows, cerr := s.db.QueryContext(ctx,
		`SELECT id FROM comments WHERE file_id IN (`+placeholders(len(ids))+`)`, args...); cerr == nil {
		for crows.Next() {
			var cid string
			if crows.Scan(&cid) == nil {
				commentIDs = append(commentIDs, cid)
			}
		}
		crows.Close()
	}
	// B15 派生资产（缩略图等）：key 形态 spaces/{space}/.derived/{fid}/{name}，与 files 行**没有**
	// 外键关系、子表清扫也覆不到 → 必须像 vrefs 一样在删行前把 (space, fid) 取出来，稍后按前缀清。
	// 🔴 别指望「事后靠 file_media.thumbnail_ref 对账」：本函数下面的子表循环**已经**把 file_media
	// 行删掉了（B8 起就如此），等对账跑到时行已不在 —— B15 实测踩到，缩略图会永久残留。
	type derRef struct{ space, fid string }
	var drefs []derRef
	if drows, derr := s.db.QueryContext(ctx,
		`SELECT space_id, id FROM files WHERE id IN (`+placeholders(len(ids))+`)`, args...); derr == nil {
		for drows.Next() {
			var r derRef
			if err := drows.Scan(&r.space, &r.fid); err == nil && r.space != "" {
				drefs = append(drefs, r)
			}
		}
		drows.Close()
	}
	// 先删引用子表（vectors.chunk_id 引用 index_chunks，须在 chunks 之前）
	// file_media（B8）同样以 file_id 为主键列，随子树一并清理，避免遗留孤儿元信息
	for _, t := range []string{"vectors", "file_ai_summaries", "file_tags", "collection_files", "shares", "comments", "file_versions", "file_media"} {
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
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return err
	}
	// B6：订阅行用 target_id 列（不是 file_id），故不能并入上面的子表循环。
	// 指向被删文件/目录的订阅一并失效（否则会留下指向幽灵目标的订阅）。
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM doc_subscriptions WHERE target_id IN (`+placeholders(len(ids))+`)`, args...); err != nil {
		return err
	}
	// B6：提及行用 source_id 列，且同时承载 post(文件 id) 与 comment(评论 id) 两类，故合并一起清。
	if mIDs := append(append([]string{}, ids...), commentIDs...); len(mIDs) > 0 {
		margs := make([]any, len(mIDs))
		for i, v := range mIDs {
			margs[i] = v
		}
		if _, err := s.db.ExecContext(ctx,
			`DELETE FROM mentions WHERE source_id IN (`+placeholders(len(mIDs))+`)`, margs...); err != nil {
			return err
		}
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
	// B5 版本历史：快照 blob 也要清（否则回收站清空后仍留有孤儿快照）
	for _, r := range vrefs {
		_ = s.st.Delete(ctx, versionKey(r.space, r.fid, r.ver))
	}
	// B15 派生资产：按 (space, fid) 整目录清。对象不存在时 List 安静返回空 → 天然幂等。
	for _, r := range drefs {
		s.DeleteDerivedPrefix(ctx, DerivedPrefix(r.space, r.fid))
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

// ---- 派生资产（缩略图等）存储：B15 ----
//
// 设计取舍：派生对象**不建 files 行**——它们是媒体文件的实现细节，不是用户文件。
// 好处是公开列表 / sitemap / 回收站 / 配额 / 搜索索引一律不受影响（零污染）；
// 代价是生命周期要显式管理：随源文件 Purge 时按前缀整目录清理（见 DeleteDerivedPrefix）。
//
// key 形态：spaces/{spaceID}/.derived/{fileID}/{name}
//   前缀里带 fileID 是为了「按文件一次清干净」；目录名以 . 开头，
//   与用户可见路径（files.storage_ref）不会撞名。

// DerivedPrefix 某文件的派生对象 key 前缀。
func DerivedPrefix(spaceID, fileID string) string {
	return "spaces/" + spaceID + "/.derived/" + fileID
}

// DerivedKey 派生对象完整 key。
func DerivedKey(spaceID, fileID, name string) string {
	return DerivedPrefix(spaceID, fileID) + "/" + name
}

// DerivedSpacePrefix 整个空间的派生对象前缀（对账兜底用）。
func DerivedSpacePrefix(spaceID string) string {
	return "spaces/" + spaceID + "/.derived"
}

// PutDerived 写入派生对象：不建 files 行、不发事件、不做文本索引（有意为之）。
func (s *FileStore) PutDerived(ctx context.Context, key string, r io.Reader, size int64) error {
	return s.st.Put(ctx, key, r, size)
}

// DerivedReader 读取派生对象。
func (s *FileStore) DerivedReader(ctx context.Context, key string) (io.ReadCloser, error) {
	rc, _, err := s.st.Get(ctx, key)
	return rc, err
}

// DerivedStat 派生对象元信息（判存在与取大小）。
func (s *FileStore) DerivedStat(ctx context.Context, key string) (storage.ObjectMeta, bool) {
	om, err := s.st.Stat(ctx, key)
	if err != nil || om.IsDir {
		return storage.ObjectMeta{}, false
	}
	return om, true
}

// DeleteDerived 删除单个派生对象；对象不存在视为成功（幂等清理）。
func (s *FileStore) DeleteDerived(ctx context.Context, key string) error {
	if err := s.st.Delete(ctx, key); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	return nil
}

// DeleteDerivedPrefix 删除某前缀下全部派生对象，返回删除个数。
// 后端不支持 List（远程对象存储实现可能返回错误）时安静返回 0 —— 清不掉派生对象
// 只影响磁盘占用，不应让文件删除整体失败。
func (s *FileStore) DeleteDerivedPrefix(ctx context.Context, prefix string) int {
	objs, err := s.st.List(ctx, prefix)
	if err != nil {
		return 0
	}
	n := 0
	for _, o := range objs {
		if o.IsDir {
			continue
		}
		if s.st.Delete(ctx, o.Key) == nil {
			n++
		}
	}
	return n
}
