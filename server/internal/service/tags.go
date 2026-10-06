// Package service 业务编排层。
// tags.go 实现标签/分类服务：tags 树状表（path 层级 + parent_id）+ file_tags 多对多。
// 网盘文件打标签 → 知识库聚合 / 博客分类 / 检索过滤 共用同一套标签体系。
package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/google/uuid"
)

// Tag 标签/分类（树状：path 表示完整层级路径，parent_id 指向父标签）。
type Tag struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	ParentID  string `json:"parent_id,omitempty"`
	Count     int    `json:"count"`
	CreatedAt int64  `json:"created_at"`
}

// TagStore 标签服务：标签树管理 + 文件关联（file_tags）。
type TagStore struct {
	db  *sql.DB
	b   *bus.Bus
	aud *AuditStore
}

// NewTagStore 创建标签服务。
func NewTagStore(db *sql.DB, b *bus.Bus, aud *AuditStore) *TagStore {
	return &TagStore{db: db, b: b, aud: aud}
}

func tagNow() int64 { return time.Now().Unix() }

func (s *TagStore) get(ctx context.Context, ownerID, id string) (*Tag, error) {
	var t Tag
	var pid sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, path, parent_id, created_at FROM tags WHERE id=? AND owner_id=?`,
		id, ownerID).Scan(&t.ID, &t.Name, &t.Path, &pid, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.ParentID = pid.String
	return &t, nil
}

// List 标签树（按 path 排序），附带每个标签关联的文件数。
func (s *TagStore) List(ctx context.Context, ownerID string) ([]*Tag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.name, t.path, t.parent_id, t.created_at,
		        (SELECT COUNT(*) FROM file_tags ft WHERE ft.tag_id = t.id) AS cnt
		 FROM tags t WHERE t.owner_id=? ORDER BY t.path COLLATE NOCASE`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Tag{}
	for rows.Next() {
		var t Tag
		var pid sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Path, &pid, &t.CreatedAt, &t.Count); err != nil {
			return nil, err
		}
		t.ParentID = pid.String
		out = append(out, &t)
	}
	return out, rows.Err()
}

// Create 创建标签。parent_id 为空建在根；path 由父级路径拼接。
func (s *TagStore) Create(ctx context.Context, ownerID, name, parentID string) (*Tag, error) {
	if name == "" || strings.ContainsAny(name, "/") {
		return nil, errors.New("service: invalid tag name")
	}
	var path string
	if parentID != "" {
		var p string
		if err := s.db.QueryRowContext(ctx, `SELECT path FROM tags WHERE id=? AND owner_id=?`, parentID, ownerID).Scan(&p); err != nil {
			return nil, ErrNotFound
		}
		path = p + "/" + name
	} else {
		path = name
	}
	var dup int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE owner_id=? AND path=?`, ownerID, path).Scan(&dup)
	if dup > 0 {
		return nil, ErrConflict
	}
	id := uuid.NewString()
	ts := tagNow()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO tags (id, name, path, parent_id, owner_id, created_at) VALUES (?,?,?,?,?,?)`,
		id, name, path, nullIfEmpty(parentID), ownerID, ts); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "tag.created", Key: id, Data: map[string]any{"name": name, "path": path}})
	_, _ = s.aud.Append(ctx, ownerID, "tag.create", id, map[string]any{"name": name, "path": path})
	return s.get(ctx, ownerID, id)
}

// Rename 重命名标签，同步更新自身与全部后代的 path 前缀。
func (s *TagStore) Rename(ctx context.Context, ownerID, id, newName string) (*Tag, error) {
	if newName == "" || strings.ContainsAny(newName, "/") {
		return nil, errors.New("service: invalid tag name")
	}
	t, err := s.get(ctx, ownerID, id)
	if err != nil {
		return nil, err
	}
	prefix := ""
	if t.ParentID != "" {
		var p string
		if err := s.db.QueryRowContext(ctx, `SELECT path FROM tags WHERE id=?`, t.ParentID).Scan(&p); err != nil {
			return nil, err
		}
		prefix = p + "/"
	}
	newPath := prefix + newName
	var dup int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE owner_id=? AND path=? AND id<>?`, ownerID, newPath, id).Scan(&dup)
	if dup > 0 {
		return nil, ErrConflict
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE tags SET name=?, path=? WHERE id=?`, newName, newPath, id); err != nil {
		return nil, err
	}
	// 后代 path 前缀更新（旧前缀 → 新前缀）
	if _, err := s.db.ExecContext(ctx,
		`UPDATE tags SET path=replace(path, ?||'/', ?||'/') WHERE path LIKE ?||'/%'`,
		t.Path, newPath, t.Path); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "tag.renamed", Key: id, Data: map[string]any{"from": t.Path, "to": newPath}})
	_, _ = s.aud.Append(ctx, ownerID, "tag.rename", id, map[string]any{"from": t.Path, "to": newPath})
	return s.get(ctx, ownerID, id)
}

// Delete 删除标签并解除全部文件关联（后代一并删除）。
func (s *TagStore) Delete(ctx context.Context, ownerID, id string) error {
	t, err := s.get(ctx, ownerID, id)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM tags WHERE owner_id=? AND (id=? OR path LIKE ?||'/%')`, ownerID, id, t.Path)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var tid string
		_ = rows.Scan(&tid)
		ids = append(ids, tid)
	}
	rows.Close()
	if len(ids) == 0 {
		return ErrNotFound
	}
	q := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_tags WHERE tag_id IN (`+q+`)`, args...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id IN (`+q+`)`, args...); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.b.Publish(ctx, bus.Event{Topic: "tag.deleted", Key: id, Data: map[string]any{"path": t.Path}})
	_, _ = s.aud.Append(ctx, ownerID, "tag.delete", id, map[string]any{"path": t.Path, "children": len(ids) - 1})
	return nil
}

// SetFileTags 全量替换文件的标签集合（file_tags 覆盖写）。
func (s *TagStore) SetFileTags(ctx context.Context, ownerID, fileID string, tagIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_tags WHERE file_id=?`, fileID); err != nil {
		return err
	}
	for _, tid := range tagIDs {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE id=? AND owner_id=?`, tid, ownerID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			continue // 非本用户标签忽略
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO file_tags (file_id, tag_id) VALUES (?,?)`, fileID, tid); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.tagged", Key: fileID, Data: map[string]any{"tags": tagIDs}})
	_, _ = s.aud.Append(ctx, ownerID, "file.tag", fileID, map[string]any{"tags": tagIDs})
	return nil
}

// FileTags 取单个文件的标签。
func (s *TagStore) FileTags(ctx context.Context, fileID string) ([]*Tag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.name, t.path, t.parent_id, t.created_at, 0
		 FROM file_tags ft JOIN tags t ON t.id=ft.tag_id WHERE ft.file_id=? ORDER BY t.path`,
		fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Tag{}
	for rows.Next() {
		var t Tag
		var pid sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Path, &pid, &t.CreatedAt, &t.Count); err != nil {
			return nil, err
		}
		t.ParentID = pid.String
		out = append(out, &t)
	}
	return out, rows.Err()
}

// TagsForFiles 批量取多个文件的标签：map[fileID][]*Tag（文件列表徽章用）。
func (s *TagStore) TagsForFiles(ctx context.Context, fileIDs []string) (map[string][]*Tag, error) {
	out := map[string][]*Tag{}
	if len(fileIDs) == 0 {
		return out, nil
	}
	q := strings.TrimRight(strings.Repeat("?,", len(fileIDs)), ",")
	args := make([]any, len(fileIDs))
	for i, v := range fileIDs {
		args[i] = v
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT ft.file_id, t.id, t.name, t.path
		 FROM file_tags ft JOIN tags t ON t.id=ft.tag_id
		 WHERE ft.file_id IN (`+q+`) ORDER BY t.path COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var fid, id, name, path string
		if err := rows.Scan(&fid, &id, &name, &path); err != nil {
			return nil, err
		}
		out[fid] = append(out[fid], &Tag{ID: id, Name: name, Path: path})
	}
	return out, rows.Err()
}

// ---- 规则打标：扩展名 → 标签（基础规则层，上传即生效；AI 标签是增强层） ----

// extTagRules 扩展名自动标签默认规则（小写含点）。类型/二级 结构，与 AI 语义标签自然区分。
var extTagRules = map[string]string{
	// 音频
	".mp3": "类型/音频", ".wav": "类型/音频", ".flac": "类型/音频", ".aac": "类型/音频",
	".ogg": "类型/音频", ".m4a": "类型/音频", ".opus": "类型/音频",
	// 视频
	".mp4": "类型/视频", ".mkv": "类型/视频", ".avi": "类型/视频", ".mov": "类型/视频",
	".wmv": "类型/视频", ".flv": "类型/视频", ".webm": "类型/视频",
	// 图片
	".jpg": "类型/图片", ".jpeg": "类型/图片", ".png": "类型/图片", ".gif": "类型/图片",
	".webp": "类型/图片", ".bmp": "类型/图片", ".svg": "类型/图片", ".heic": "类型/图片",
	// 文档
	".pdf": "类型/文档/PDF",
	".doc": "类型/文档/Word", ".docx": "类型/文档/Word",
	".xls": "类型/文档/表格", ".xlsx": "类型/文档/表格", ".csv": "类型/文档/表格",
	".ppt": "类型/文档/演示", ".pptx": "类型/文档/演示",
	".md": "类型/文档/笔记", ".markdown": "类型/文档/笔记", ".txt": "类型/文档/笔记",
	// 压缩包
	".zip": "类型/压缩包", ".rar": "类型/压缩包", ".7z": "类型/压缩包",
	".tar": "类型/压缩包", ".gz": "类型/压缩包",
	// 网页
	".html": "类型/网页", ".htm": "类型/网页",
	// 代码/配置
	".json": "类型/代码/配置", ".yaml": "类型/代码/配置", ".yml": "类型/代码/配置",
	".xml": "类型/代码/配置", ".toml": "类型/代码/配置", ".ini": "类型/代码/配置",
	".go": "类型/代码/Go", ".py": "类型/代码/Python", ".js": "类型/代码/JS",
	".ts": "类型/代码/TS", ".java": "类型/代码/Java", ".c": "类型/代码/C",
	".cpp": "类型/代码/C++", ".vue": "类型/代码/前端", ".sql": "类型/代码/SQL",
	".sh": "类型/代码/脚本", ".ps1": "类型/代码/脚本",
}

// ExtTagRule 返回扩展名对应的标签路径（无规则返回 ""）。name 可为完整文件名。
func ExtTagRule(name string) string {
	ext := ""
	if i := strings.LastIndexByte(name, '.'); i >= 0 && i < len(name)-1 {
		ext = strings.ToLower(name[i:])
	}
	if ext == "" {
		return ""
	}
	return extTagRules[ext]
}

// EnsureTagPath 按路径逐级创建标签（如 类型/音频：先建 类型 再建 音频），返回末端标签。
// 已存在则直接返回；任一级失败返回错误。
func (s *TagStore) EnsureTagPath(ctx context.Context, ownerID, path string) (*Tag, error) {
	path = strings.Trim(path, "/ ")
	if path == "" {
		return nil, errors.New("service: empty tag path")
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
			`SELECT id FROM tags WHERE owner_id=? AND path=?`, ownerID, cur).Scan(&id); err == nil {
			parentID = id
			continue
		}
		t, err := s.Create(ctx, ownerID, seg, parentID)
		if err != nil {
			return nil, err
		}
		parentID = t.ID
	}
	var end Tag // 值类型：end 为 nil 时 Scan(&end.ID) 会对 nil 指针取字段地址 → panic
	var pid sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, path, parent_id, created_at FROM tags WHERE id=? AND owner_id=?`,
		parentID, ownerID).Scan(&end.ID, &end.Name, &end.Path, &pid, &end.CreatedAt)
	if err != nil {
		return nil, err
	}
	end.ParentID = pid.String
	return &end, nil
}

// MergeFileTags 把额外标签合并挂到文件（保留已有标签，并集写入）。
func (s *TagStore) MergeFileTags(ctx context.Context, ownerID, fileID string, tagIDs []string) error {
	cur, err := s.FileTags(ctx, fileID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, t := range cur {
		seen[t.ID] = true
	}
	for _, id := range tagIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			cur = append(cur, &Tag{ID: id})
		}
	}
	ids := make([]string, 0, len(cur))
	for _, t := range cur {
		ids = append(ids, t.ID)
	}
	return s.SetFileTags(ctx, ownerID, fileID, ids)
}

// ApplyExtRules 按扩展名规则为文件自动打标（上传即生效，无需 AI）。返回是否命中规则。
func (s *TagStore) ApplyExtRules(ctx context.Context, ownerID, fileID, name string) (bool, error) {
	path := ExtTagRule(name)
	if path == "" {
		return false, nil
	}
	t, err := s.EnsureTagPath(ctx, ownerID, path)
	if err != nil {
		return false, err
	}
	return true, s.MergeFileTags(ctx, ownerID, fileID, []string{t.ID})
}

// SubscribeExtRules 订阅 file.created：非文本二进制/任意文件都按扩展名打规则标签。
func (s *TagStore) SubscribeExtRules(b *bus.Bus) func() {
	return b.Subscribe("file.created", func(_ context.Context, e bus.Event) error {
		if e.Key == "" {
			return nil
		}
		name, _ := e.Data["name"].(string)
		if name == "" {
			return nil
		}
		_, _ = s.ApplyExtRules(context.Background(), SystemOwnerID, e.Key, name)
		return nil
	})
}
