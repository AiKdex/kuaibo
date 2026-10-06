// Package service 业务编排层。
// impex.go 实现导入导出中心（R11/R12）：统一入口 + 异步任务队列 + 进度可见 + 失败清单可重试。
//
//	导入：ZIP 批量解压 / 剪贴板粘贴 / URL 网页抓取转 MD / Obsidian vault（双链 + frontmatter 标签）
//	导出：文件夹 ZIP 打包 / Obsidian vault 导出 / 按标签或集合批量 MD
//
// 规避 Kmap 三大槽点：无进度（进度每 5% 落库）、静默覆盖（冲突策略 rename|skip|overwrite）、
// 格式不识别就整体报错（单条失败进 fail_list，不阻断其余条目，可重试）。
// 任务持久化于 impex_jobs 表；执行用 goroutine（提交即返回 id），产物写入存储后端 _exports/。
package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/google/uuid"
)

// ixHostAllowed 防 SSRF：拒绝 loopback / 私网 / 链路本地 / 保留地址。
func ixHostAllowed(u *url.URL) bool {
	host := u.Hostname()
	if host == "" {
		return false
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return false // 解析失败一律拒绝（不 fallthrough 直连）
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			ip.IsUnspecified() || ip.IsMulticast() {
			return false
		}
	}
	return true
}

// 任务类型与来源常量。
const (
	ImpexImport = "import"
	ImpexExport = "export"
)

// ImpexFail 单条失败项（可重试）：item 为相对路径/URL，ref 记录重试所需最小上下文。
type ImpexFail struct {
	Item  string         `json:"item"`
	Error string         `json:"error"`
	Ref   map[string]any `json:"ref,omitempty"`
}

// ImpexJob 导入导出任务（进度可见 + 失败清单可重试）。
type ImpexJob struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`   // import | export
	Source    string         `json:"source"` // zip|clipboard|url|obsidian|folder|tag|collection
	Status    string         `json:"status"` // pending|running|done|partial|failed
	Percent   int            `json:"percent"`
	Total     int            `json:"total"`
	Processed int            `json:"processed"`
	Succeeded int            `json:"succeeded"`
	Failed    int            `json:"failed"`
	Payload   map[string]any `json:"payload,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
	FailList  []ImpexFail    `json:"fail_list"`
	LastError string         `json:"last_error,omitempty"`
	CreatedAt int64          `json:"created_at"`
	UpdatedAt int64          `json:"updated_at"`
}

// ImpexStore 导入导出服务：任务编排 + 内容写入（复用 FileStore / TagStore 原语）。
type ImpexStore struct {
	db    *sql.DB
	files *FileStore
	tags  *TagStore
	b     *bus.Bus
	aud   *AuditStore
}

// NewImpexStore 创建导入导出服务。
func NewImpexStore(db *sql.DB, files *FileStore, tags *TagStore, b *bus.Bus, aud *AuditStore) *ImpexStore {
	return &ImpexStore{db: db, files: files, tags: tags, b: b, aud: aud}
}

// ---- 任务读写 ----

// Get 取单个任务（含进度与失败清单）。
func (s *ImpexStore) Get(ctx context.Context, id string) (*ImpexJob, error) {
	var (
		j                        ImpexJob
		payload, result, failLis string
		lastErr                  sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, kind, source, status, percent, total, processed, payload, result, fail_list, last_error, created_at, updated_at
		 FROM impex_jobs WHERE id=?`, id).
		Scan(&j.ID, &j.Kind, &j.Source, &j.Status, &j.Percent, &j.Total, &j.Processed,
			&payload, &result, &failLis, &lastErr, &j.CreatedAt, &j.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	j.LastError = lastErr.String
	j.Payload = map[string]any{}
	j.Result = map[string]any{}
	j.FailList = []ImpexFail{}
	_ = json.Unmarshal([]byte(payload), &j.Payload)
	_ = json.Unmarshal([]byte(result), &j.Result)
	_ = json.Unmarshal([]byte(failLis), &j.FailList)
	j.Failed = len(j.FailList)
	if j.Processed >= j.Failed {
		j.Succeeded = j.Processed - j.Failed
	}
	return &j, nil
}

// OwnerOf 返回任务归属用户 ID（payload.owner_id；缺省为 SystemOwnerID）。M11 归属校验用。
func (s *ImpexStore) OwnerOf(ctx context.Context, id string) (string, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM impex_jobs WHERE id=?`, id).Scan(&raw); err != nil {
		return "", err
	}
	var p map[string]any
	if json.Unmarshal([]byte(raw), &p) == nil {
		if o := ixStr(p["owner_id"]); o != "" {
			return o, nil
		}
	}
	return SystemOwnerID, nil
}

// List 任务历史（kind 为空取全部；ownerID 非空时仅返回该用户任务——M11 归属隔离），按创建时间倒序。
func (s *ImpexStore) List(ctx context.Context, kind, ownerID string, limit int) ([]*ImpexJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id FROM impex_jobs`
	args := []any{}
	conds := []string{}
	if kind != "" {
		conds = append(conds, `kind=?`)
		args = append(args, kind)
	}
	if ownerID != "" {
		conds = append(conds, `COALESCE(json_extract(payload,'$.owner_id'),'') = ?`)
		args = append(args, ownerID)
	}
	if len(conds) > 0 {
		q += ` WHERE ` + strings.Join(conds, ` AND `)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]*ImpexJob, 0, len(ids))
	for _, id := range ids {
		if j, err := s.Get(ctx, id); err == nil {
			out = append(out, j)
		}
	}
	return out, nil
}

// insert 落库一条 pending 任务并立即异步执行。
func (s *ImpexStore) insert(ctx context.Context, id, kind, source string, payload map[string]any) (*ImpexJob, error) {
	pj, _ := json.Marshal(payload)
	ts := now()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO impex_jobs (id, kind, source, status, percent, total, processed, payload, result, fail_list, created_at, updated_at)
		 VALUES (?,?,?, 'pending', 0, 0, 0, ?, '{}', '[]', ?, ?)`,
		id, kind, source, string(pj), ts, ts); err != nil {
		return nil, err
	}
	go s.run(id, false)
	return s.Get(ctx, id)
}

// ---- 任务创建（供 handler 调用） ----

// CreateZipImport 创建 ZIP / Obsidian vault 导入任务（先把上传包落到存储，供重试复用）。
func (s *ImpexStore) CreateZipImport(ctx context.Context, ownerID, spaceID, source, parent, strategy string, r io.Reader, size int64) (*ImpexJob, error) {
	if source != "zip" && source != "obsidian" {
		return nil, errors.New("service: 仅支持 zip / obsidian 导入")
	}
	if strategy == "" {
		strategy = "rename"
	}
	id := uuid.NewString()
	key := "_imports/" + id + ".zip"
	if err := s.files.st.Put(ctx, key, r, size); err != nil {
		return nil, fmt.Errorf("service: 保存上传包失败: %w", err)
	}
	payload := map[string]any{
		"owner_id": ownerID, "space_id": spaceID, "parent": parent,
		"on_conflict": strategy, "zip_key": key,
	}
	return s.insert(ctx, id, ImpexImport, source, payload)
}

// CreateURLImport 创建 URL 网页抓取导入任务。
func (s *ImpexStore) CreateURLImport(ctx context.Context, ownerID, spaceID, parent, rawURL, ua string) (*ImpexJob, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, errors.New("service: URL 不能为空")
	}
	payload := map[string]any{"owner_id": ownerID, "space_id": spaceID, "parent": parent, "url": rawURL, "ua": ua}
	return s.insert(ctx, uuid.NewString(), ImpexImport, "url", payload)
}

// CreateClipboardImport 创建剪贴板粘贴导入任务（text 生成 Markdown；image 为 base64 图片）。
func (s *ImpexStore) CreateClipboardImport(ctx context.Context, ownerID, spaceID, parent, name, text, image string) (*ImpexJob, error) {
	if strings.TrimSpace(text) == "" && strings.TrimSpace(image) == "" {
		return nil, errors.New("service: 粘贴内容为空")
	}
	payload := map[string]any{"owner_id": ownerID, "space_id": spaceID, "parent": parent, "name": name, "text": text, "image": image}
	return s.insert(ctx, uuid.NewString(), ImpexImport, "clipboard", payload)
}

// CreateExport 创建导出任务（source: folder|obsidian|tag|collection；targetID 为对应对象 id）。
func (s *ImpexStore) CreateExport(ctx context.Context, ownerID, spaceID, source, targetID, name string) (*ImpexJob, error) {
	switch source {
	case "folder", "obsidian", "tag", "collection", "all":
	default:
		return nil, errors.New("service: 未知导出类型 " + source)
	}
	payload := map[string]any{"owner_id": ownerID, "space_id": spaceID, "target_id": targetID, "name": name}
	return s.insert(ctx, uuid.NewString(), ImpexExport, source, payload)
}

// Retry 重试失败任务：仅重跑失败项（避免重复导入已成功者）。
func (s *ImpexStore) Retry(ctx context.Context, id string) (*ImpexJob, error) {
	j, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if j.Status != "partial" && j.Status != "failed" {
		return nil, errors.New("service: 仅失败或部分完成的任务可重试")
	}
	if len(j.FailList) == 0 {
		return nil, errors.New("service: 无失败项可重试")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE impex_jobs SET status='pending', percent=0, processed=0, last_error=NULL, updated_at=? WHERE id=?`,
		now(), id); err != nil {
		return nil, err
	}
	go s.run(id, true)
	return s.Get(ctx, id)
}

// ---- 执行器 ----

func (s *ImpexStore) run(id string, onlyFailed bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	j, err := s.Get(ctx, id)
	if err != nil {
		return
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE impex_jobs SET status='running', updated_at=? WHERE id=?`, now(), id)

	// 仅重试失败项：先据 fail_list 建集合，再清空 fail_list 以便干净记录本轮结果。
	var failedSet map[string]bool
	if onlyFailed {
		failedSet = map[string]bool{}
		for _, fl := range j.FailList {
			if v, ok := fl.Ref["name"].(string); ok {
				failedSet[v] = true
			}
		}
		_, _ = s.db.ExecContext(ctx, `UPDATE impex_jobs SET fail_list='[]', processed=0 WHERE id=?`, id)
	}

	var rerr error
	switch j.Kind + ":" + j.Source {
	case "import:zip":
		rerr = s.runZipImport(ctx, j, failedSet, false)
	case "import:obsidian":
		rerr = s.runZipImport(ctx, j, failedSet, true)
	case "import:url":
		rerr = s.runURLImport(ctx, j)
	case "import:clipboard":
		rerr = s.runClipboardImport(ctx, j)
	case "export:folder":
		rerr = s.runFolderExport(ctx, j)
	case "export:all":
		// 全站导出：target 为空 → collectTree 从空间根递归（含存储层隐藏的 博客/ 目录）
		rerr = s.runFolderExport(ctx, j)
	case "export:obsidian":
		rerr = s.runObsidianExport(ctx, j)
	case "export:tag":
		rerr = s.runBatchExport(ctx, j, "tag")
	case "export:collection":
		rerr = s.runBatchExport(ctx, j, "collection")
	default:
		rerr = fmt.Errorf("未知任务类型 %s/%s", j.Kind, j.Source)
	}

	if rerr != nil {
		s.appendFail(ctx, id, ImpexFail{Item: "任务", Error: rerr.Error()})
		s.finish(ctx, id, "failed", rerr.Error())
		s.auditAction(ctx, j, "impex.failed", rerr.Error())
		return
	}
	status := "done"
	if s.failCount(ctx, id) > 0 {
		status = "partial"
	}
	s.finish(ctx, id, status, "")
	s.auditAction(ctx, j, "impex."+j.Kind, fmt.Sprintf("%s → %s（失败 %d）", j.Source, status, s.failCount(ctx, id)))
}

// zipEntryName 处理 Windows 打包 ZIP 的 GBK/CP936 文件名。
// Go archive/zip 对无 UTF-8 flag 的条目（NonUTF8=true）原样保留原始字节（不做转码），
// 因此这里直接按 GBK 解码；带 UTF-8 flag 的条目已是合法 UTF-8，原样返回。
func zipEntryName(f *zip.File) string {
	name := f.Name
	if f.NonUTF8 {
		if dec, err := simplifiedchinese.GBK.NewDecoder().Bytes([]byte(name)); err == nil {
			return string(dec)
		}
	}
	return name
}

// runZipImport 处理 ZIP / Obsidian vault 导入。failedSet 非 nil 时仅处理其中的条目。
func (s *ImpexStore) runZipImport(ctx context.Context, j *ImpexJob, failedSet map[string]bool, obsidian bool) error {
	owner := ixStr(j.Payload["owner_id"])
	if owner == "" {
		owner = SystemOwnerID
	}
	space := ixStr(j.Payload["space_id"])
	parent := ixStr(j.Payload["parent"])
	strategy := ixStr(j.Payload["on_conflict"])
	if strategy == "" {
		strategy = "rename"
	}
	key := ixStr(j.Payload["zip_key"])
	if key == "" {
		return errors.New("导入包缺失（zip_key 为空）")
	}

	rc, _, err := s.files.st.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("读取导入包失败: %w", err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("ZIP 解析失败: %w", err)
	}

	// 收集有效文件条目（跳过目录、隐藏/系统文件、路径穿越）。
	type ent struct {
		name string
		f    *zip.File
	}
	var ents []ent
	for _, f := range zr.File {
		raw := strings.ReplaceAll(zipEntryName(f), "\\", "/")
		if strings.HasSuffix(raw, "/") || f.FileInfo().IsDir() {
			continue
		}
		n := path.Clean(raw)
		if n == "." || strings.HasPrefix(n, "../") || strings.Contains(n, "/../") {
			continue
		}
		base := path.Base(n)
		if strings.EqualFold(base, ".DS_Store") {
			continue
		}
		if strings.HasPrefix(n, "__MACOSX/") {
			continue
		}
		if obsidian && (strings.HasPrefix(n, ".obsidian/") || strings.HasPrefix(base, ".")) {
			continue
		}
		ents = append(ents, ent{n, f})
	}

	targets := ents
	if failedSet != nil {
		targets = targets[:0]
		for _, e := range ents {
			if failedSet[e.name] {
				targets = append(targets, e)
			}
		}
	}

	prog := &impexProgress{s: s, id: j.ID, ctx: ctx, total: len(targets)}
	s.setTotal(ctx, j.ID, len(targets))

	var created []*File
	for _, e := range targets {
		b, err := readZipEntry(e.f)
		if err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: e.name, Error: err.Error(), Ref: map[string]any{"name": e.name}})
			prog.step()
			continue
		}
		f, err := s.importBytes(ctx, owner, space, parent, e.name, b, strategy)
		if err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: e.name, Error: err.Error(), Ref: map[string]any{"name": e.name}})
			prog.step()
			continue
		}
		if f != nil {
			created = append(created, f)
		}
		prog.step()
	}

	if obsidian && len(created) > 0 {
		prog.total += len(created)
		s.setTotal(ctx, j.ID, prog.total)
		s.obsidianPost(ctx, owner, space, created, prog)
	}
	return nil
}

// obsidianPost 导入后处理：frontmatter 标签 → file_tags；[[双链]] → 文末「引用文件」列表。
func (s *ImpexStore) obsidianPost(ctx context.Context, owner, space string, created []*File, prog *impexProgress) {
	// name（不含扩展名）→ file_id（含已存在文件），用于解析双链
	byName := map[string]string{}
	if tree, err := s.files.Tree(ctx, space); err == nil {
		for _, f := range tree {
			if f.Kind == "file" {
				byName[strings.TrimSuffix(f.Name, path.Ext(f.Name))] = f.ID
			}
		}
	}
	for _, f := range created {
		if f.Kind != "file" {
			prog.step()
			continue
		}
		byName[strings.TrimSuffix(f.Name, path.Ext(f.Name))] = f.ID
		prog.step()
	}

	for _, f := range created {
		if f.Kind != "file" {
			continue
		}
		rc, _, err := s.files.Content(ctx, f.ID)
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			continue
		}
		content := string(raw)

		// 1) frontmatter 标签映射到 file_tags
		if _, tags, _ := ixParseFrontmatter(content); len(tags) > 0 {
			var ids []string
			for _, tp := range tags {
				t, err := s.tags.EnsureTagPath(ctx, owner, tp)
				if err != nil || t == nil {
					continue
				}
				ids = append(ids, t.ID)
			}
			if len(ids) > 0 {
				_ = s.tags.MergeFileTags(ctx, owner, f.ID, ids)
			}
		}

		// 2) [[双链]] → 文末引用列表（保留原文）
		names := ixWikiLinkNames(content)
		if len(names) == 0 || strings.Contains(content, "**引用文件**") {
			continue
		}
		var lines []string
		for _, n := range names {
			if id, ok := byName[n]; ok && id != f.ID {
				lines = append(lines, "- [["+n+"]] → /read/"+id)
			} else {
				lines = append(lines, "- [["+n+"]]（未找到同名文件）")
			}
		}
		section := "\n\n---\n\n**引用文件**\n\n" + strings.Join(lines, "\n") + "\n"
		_, _ = s.files.UpdateContent(ctx, owner, f.ID, content+section)
	}
}

// importBytes 将单个条目写入文件库（含冲突策略：rename|skip|overwrite）。
func (s *ImpexStore) importBytes(ctx context.Context, owner, space, parent, rel string, b []byte, strategy string) (*File, error) {
	rel = strings.ReplaceAll(rel, "\\", "/")
	dir := path.Dir(rel)
	name := ixSafeName(path.Base(rel))
	if name == "" {
		return nil, ErrInvalidName
	}
	pid := parent
	if dir != "." && dir != "/" && dir != "" {
		p, err := s.files.EnsurePath(ctx, owner, space, parent, dir)
		if err != nil {
			return nil, err
		}
		pid = p
	}
	if strategy != "rename" {
		if existing := s.findChild(ctx, space, pid, name); existing != "" {
			switch strategy {
			case "skip":
				return nil, nil // 静默跳过（不算失败）
			case "overwrite":
				if err := s.files.Purge(ctx, owner, existing); err != nil {
					return nil, err
				}
			}
		}
	}
	// 内容级去重：同目录已有同 SHA256 文件 → 视为重复导入（同 ZIP/Obsidian 重导），跳过不产生副本。
	// 修复"Obsidian 重导产生 4 份同字节副本"根因（v2 审查 P2-11：每次导入复制 ZIP 副本）。
	if len(b) > 0 {
		sum := sha256.Sum256(b)
		sha := hex.EncodeToString(sum[:])
		if s.findChildBySHA(ctx, space, pid, sha) != "" {
			return nil, nil
		}
	}
	return s.files.Upload(ctx, owner, space, pid, name, ixMimeOf(name), bytes.NewReader(b), int64(len(b)), DefaultSiteID)
}

// findChildBySHA 同目录内按内容 SHA256 找文件（内容级去重；同名但内容不同不受影响）。
func (s *ImpexStore) findChildBySHA(ctx context.Context, space, parentID, sha string) string {
	var id string
	var err error
	if parentID == "" {
		err = s.db.QueryRowContext(ctx,
			`SELECT id FROM files WHERE space_id=? AND (parent_id IS NULL OR parent_id='') AND kind='file' AND deleted_at IS NULL AND sha256=? LIMIT 1`,
			space, sha).Scan(&id)
	} else {
		err = s.db.QueryRowContext(ctx,
			`SELECT id FROM files WHERE space_id=? AND parent_id=? AND kind='file' AND deleted_at IS NULL AND sha256=? LIMIT 1`,
			space, parentID, sha).Scan(&id)
	}
	if err != nil {
		return ""
	}
	return id
}

// runURLImport 抓取网页正文 → 转 Markdown → 入库。
func (s *ImpexStore) runURLImport(ctx context.Context, j *ImpexJob) error {
	owner := ixStr(j.Payload["owner_id"])
	if owner == "" {
		owner = SystemOwnerID
	}
	space := ixStr(j.Payload["space_id"])
	parent := ixStr(j.Payload["parent"])
	raw := ixStr(j.Payload["url"])
	ua := ixStr(j.Payload["ua"])

	prog := &impexProgress{s: s, id: j.ID, ctx: ctx, total: 1}
	s.setTotal(ctx, j.ID, 1)

	title, md, err := ixFetchURL(ctx, raw, ua)
	if err != nil {
		s.appendFail(ctx, j.ID, ImpexFail{Item: raw, Error: err.Error(), Ref: map[string]any{"url": raw}})
		prog.step()
		return nil
	}
	name := ixSafeName(title)
	if name == "" {
		if u, e := url.Parse(raw); e == nil {
			name = ixSafeName(path.Base(u.Path))
		}
	}
	if name == "" {
		name = "web-page"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		name += ".md"
	}
	body := "> 来源：" + raw + "\n\n" + md + "\n"
	if _, err := s.files.CreateDoc(ctx, owner, space, parent, name, body, DefaultSiteID); err != nil {
		s.appendFail(ctx, j.ID, ImpexFail{Item: raw, Error: err.Error(), Ref: map[string]any{"url": raw}})
	}
	prog.step()
	return nil
}

// runClipboardImport 剪贴板粘贴：文本生成 Markdown；图片二进制入库。
func (s *ImpexStore) runClipboardImport(ctx context.Context, j *ImpexJob) error {
	owner := ixStr(j.Payload["owner_id"])
	if owner == "" {
		owner = SystemOwnerID
	}
	space := ixStr(j.Payload["space_id"])
	parent := ixStr(j.Payload["parent"])
	name := ixStr(j.Payload["name"])
	text := ixStr(j.Payload["text"])
	image := ixStr(j.Payload["image"])

	prog := &impexProgress{s: s, id: j.ID, ctx: ctx, total: 1}
	s.setTotal(ctx, j.ID, 1)

	if strings.TrimSpace(image) != "" {
		raw := image
		if i := strings.Index(raw, "base64,"); i >= 0 {
			raw = raw[i+len("base64,"):]
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
		if err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: "clipboard-image", Error: "图片解码失败: " + err.Error()})
			prog.step()
			return nil
		}
		n := ixSafeName(name)
		if n == "" {
			n = "粘贴图片"
		}
		if path.Ext(n) == "" {
			n += ".png"
		}
		if _, err := s.files.Upload(ctx, owner, space, parent, n, ixMimeOf(n), bytes.NewReader(data), int64(len(data)), DefaultSiteID); err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: "clipboard-image", Error: err.Error()})
		}
		prog.step()
		return nil
	}

	n := ixSafeName(name)
	if n == "" {
		n = "剪贴板粘贴.md"
	}
	if !strings.HasSuffix(strings.ToLower(n), ".md") {
		n += ".md"
	}
	if _, err := s.files.CreateDoc(ctx, owner, space, parent, n, text, DefaultSiteID); err != nil {
		s.appendFail(ctx, j.ID, ImpexFail{Item: n, Error: err.Error()})
	}
	prog.step()
	return nil
}

// runFolderExport 目录树 → ZIP（保留子目录结构）。
func (s *ImpexStore) runFolderExport(ctx context.Context, j *ImpexJob) error {
	space := ixStr(j.Payload["space_id"])
	target := ixStr(j.Payload["target_id"])
	name := ixStr(j.Payload["name"])
	if name == "" {
		name = "export"
	}
	files, err := s.collectTree(ctx, space, target)
	if err != nil {
		return err
	}
	prog := &impexProgress{s: s, id: j.ID, ctx: ctx, total: len(files)}
	s.setTotal(ctx, j.ID, len(files))

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, rf := range files {
		if err := s.writeZipEntry(ctx, zw, rf.F, rf.Rel); err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: rf.Rel, Error: err.Error(), Ref: map[string]any{"file_id": rf.F.ID}})
		}
		prog.step()
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return s.saveArtifact(ctx, j, name+".zip", buf.Bytes())
}

// runObsidianExport 生成 Obsidian vault：frontmatter 标签 + [[双链]] 相关文件 + .obsidian 配置。
func (s *ImpexStore) runObsidianExport(ctx context.Context, j *ImpexJob) error {
	space := ixStr(j.Payload["space_id"])
	target := ixStr(j.Payload["target_id"])
	name := ixStr(j.Payload["name"])
	if name == "" {
		name = "vault"
	}
	files, err := s.collectTree(ctx, space, target)
	if err != nil {
		return err
	}
	prog := &impexProgress{s: s, id: j.ID, ctx: ctx, total: len(files)}
	s.setTotal(ctx, j.ID, len(files))

	ids := make([]string, 0, len(files))
	nameByID := map[string]string{}
	for _, rf := range files {
		ids = append(ids, rf.F.ID)
		nameByID[rf.F.ID] = strings.TrimSuffix(rf.F.Name, path.Ext(rf.F.Name))
	}
	tagMap, _ := s.tags.TagsForFiles(ctx, ids)
	fileTagIDs := map[string][]string{}
	tagFiles := map[string][]string{}
	for fid, ts := range tagMap {
		for _, t := range ts {
			fileTagIDs[fid] = append(fileTagIDs[fid], t.ID)
			tagFiles[t.ID] = append(tagFiles[t.ID], fid)
		}
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for p, c := range map[string]string{
		".obsidian/app.json":        `{"legacyEditor":false,"livePreview":true,"readableLineLength":true}`,
		".obsidian/appearance.json": `{"baseFontSize":16,"theme":"system"}`,
	} {
		if w, err := zw.Create(p); err == nil {
			_, _ = w.Write([]byte(c))
		}
	}

	for _, rf := range files {
		rc, _, err := s.files.Content(ctx, rf.F.ID)
		if err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: rf.Rel, Error: err.Error(), Ref: map[string]any{"file_id": rf.F.ID}})
			prog.step()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: rf.Rel, Error: err.Error(), Ref: map[string]any{"file_id": rf.F.ID}})
			prog.step()
			continue
		}
		if isMarkdown(rf.F.Name) {
			data = s.withObsidianMeta(rf, tagMap[rf.F.ID], fileTagIDs, tagFiles, nameByID, data)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: ixZipPath(rf.Rel), Method: zip.Deflate})
		if err == nil {
			_, err = w.Write(data)
		}
		if err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: rf.Rel, Error: err.Error(), Ref: map[string]any{"file_id": rf.F.ID}})
		}
		prog.step()
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return s.saveArtifact(ctx, j, name+".zip", buf.Bytes())
}

// withObsidianMeta 为 Markdown 补 frontmatter 标签与「相关文件」[[双链]] 区块。
func (s *ImpexStore) withObsidianMeta(rf ixRelFile, tags []*Tag, fileTagIDs, tagFiles map[string][]string, nameByID map[string]string, raw []byte) []byte {
	content := string(raw)
	if len(tags) > 0 && !strings.HasPrefix(strings.TrimLeft(content, " \t\r\n"), "---") {
		names := make([]string, 0, len(tags))
		for _, t := range tags {
			names = append(names, t.Path)
		}
		content = "---\ntags: [" + strings.Join(names, ", ") + "]\n---\n\n" + content
	}
	seen := map[string]bool{rf.F.ID: true}
	var links []string
	for _, tid := range fileTagIDs[rf.F.ID] {
		for _, other := range tagFiles[tid] {
			if seen[other] {
				continue
			}
			seen[other] = true
			if nm := nameByID[other]; nm != "" {
				links = append(links, "- [["+nm+"]]")
			}
		}
	}
	if len(links) > 0 && !strings.Contains(content, "**相关文件**") {
		sort.Strings(links)
		content = strings.TrimRight(content, "\n") + "\n\n---\n\n**相关文件**\n\n" + strings.Join(links, "\n") + "\n"
	}
	return []byte(content)
}

// runBatchExport 按标签 / 集合批量导出笔记（Markdown 打包）。
func (s *ImpexStore) runBatchExport(ctx context.Context, j *ImpexJob, kind string) error {
	space := ixStr(j.Payload["space_id"])
	target := ixStr(j.Payload["target_id"])
	name := ixStr(j.Payload["name"])
	if name == "" {
		name = "export"
	}

	var files []*File
	if kind == "tag" {
		fs, err := s.files.ListByTag(ctx, space, target)
		if err != nil {
			return err
		}
		files = fs
	} else {
		rows, err := s.db.QueryContext(ctx,
			`SELECT f.id, f.name, f.mime, f.kind FROM collection_files cf
			 JOIN files f ON f.id=cf.file_id
			 WHERE cf.collection_id=? AND f.deleted_at IS NULL`, target)
		if err != nil {
			return err
		}
		for rows.Next() {
			f := &File{}
			if err := rows.Scan(&f.ID, &f.Name, &f.Mime, &f.Kind); err != nil {
				rows.Close()
				return err
			}
			files = append(files, f)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}

	var picks []*File
	for _, f := range files {
		if f.Kind == "file" && isMarkdownOrText(f.Name) {
			picks = append(picks, f)
		}
	}
	if len(picks) == 0 {
		return errors.New("没有可导出的笔记（Markdown/文本）")
	}
	prog := &impexProgress{s: s, id: j.ID, ctx: ctx, total: len(picks)}
	s.setTotal(ctx, j.ID, len(picks))

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	used := map[string]bool{}
	for _, f := range picks {
		rc, _, err := s.files.Content(ctx, f.ID)
		if err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: f.Name, Error: err.Error(), Ref: map[string]any{"file_id": f.ID}})
			prog.step()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: f.Name, Error: err.Error(), Ref: map[string]any{"file_id": f.ID}})
			prog.step()
			continue
		}
		zn := ixUniqueName(used, ixZipPath(f.Name))
		w, err := zw.CreateHeader(&zip.FileHeader{Name: zn, Method: zip.Deflate})
		if err == nil {
			_, err = w.Write(data)
		}
		if err != nil {
			s.appendFail(ctx, j.ID, ImpexFail{Item: f.Name, Error: err.Error(), Ref: map[string]any{"file_id": f.ID}})
		}
		prog.step()
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return s.saveArtifact(ctx, j, name+".zip", buf.Bytes())
}

// ---- 内部工具 ----

// impexProgress 进度上报：每跨过 5% 或到达 100% 落库一次（避免高频写）。
type impexProgress struct {
	s     *ImpexStore
	ctx   context.Context
	id    string
	total int
	done  int
	last5 int
}

func (p *impexProgress) step() {
	p.done++
	pct := 0
	if p.total > 0 {
		pct = p.done * 100 / p.total
	}
	if pct >= 100 || pct/5 != p.last5/5 {
		p.last5 = pct
		if pct > 100 {
			pct = 100
		}
		_, _ = p.s.db.ExecContext(p.ctx,
			`UPDATE impex_jobs SET percent=?, processed=?, total=?, updated_at=? WHERE id=?`,
			pct, p.done, p.total, now(), p.id)
	}
}

func (s *ImpexStore) setTotal(ctx context.Context, id string, total int) {
	_, _ = s.db.ExecContext(ctx, `UPDATE impex_jobs SET total=?, updated_at=? WHERE id=?`, total, now(), id)
}

func (s *ImpexStore) setResult(ctx context.Context, id string, m map[string]any) error {
	b, _ := json.Marshal(m)
	_, err := s.db.ExecContext(ctx, `UPDATE impex_jobs SET result=?, updated_at=? WHERE id=?`, string(b), now(), id)
	return err
}

func (s *ImpexStore) appendFail(ctx context.Context, id string, f ImpexFail) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT fail_list FROM impex_jobs WHERE id=?`, id).Scan(&raw); err != nil {
		return
	}
	var list []ImpexFail
	_ = json.Unmarshal([]byte(raw), &list)
	list = append(list, f)
	nb, _ := json.Marshal(list)
	_, _ = s.db.ExecContext(ctx, `UPDATE impex_jobs SET fail_list=?, updated_at=? WHERE id=?`, string(nb), now(), id)
}

func (s *ImpexStore) failCount(ctx context.Context, id string) int {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT fail_list FROM impex_jobs WHERE id=?`, id).Scan(&raw); err != nil {
		return 0
	}
	var list []ImpexFail
	_ = json.Unmarshal([]byte(raw), &list)
	return len(list)
}

func (s *ImpexStore) finish(ctx context.Context, id, status, lastErr string) {
	pct := 0
	if status == "done" || status == "partial" {
		pct = 100
	} else {
		_ = s.db.QueryRowContext(ctx, `SELECT percent FROM impex_jobs WHERE id=?`, id).Scan(&pct)
	}
	_, _ = s.db.ExecContext(ctx,
		`UPDATE impex_jobs SET status=?, percent=?, last_error=?, updated_at=? WHERE id=?`,
		status, pct, nullIfEmpty(lastErr), now(), id)
	if s.b != nil {
		s.b.Publish(ctx, bus.Event{Topic: "impex." + status, Key: id, Data: map[string]any{"status": status}})
	}
}

func (s *ImpexStore) auditAction(ctx context.Context, j *ImpexJob, action, detail string) {
	owner := ixStr(j.Payload["owner_id"])
	if owner == "" {
		owner = SystemOwnerID
	}
	_, _ = s.aud.Append(ctx, owner, action, j.ID, map[string]any{"kind": j.Kind, "source": j.Source, "detail": detail})
}

// saveArtifact 把导出的 ZIP 写入存储后端（不进文件树），并记录 result 供下载。
func (s *ImpexStore) saveArtifact(ctx context.Context, j *ImpexJob, name string, data []byte) error {
	key := "_exports/" + j.ID + ".zip"
	if err := s.files.st.Put(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
		return fmt.Errorf("保存导出文件失败: %w", err)
	}
	return s.setResult(ctx, j.ID, map[string]any{"key": key, "name": ixSafeName(name), "size": len(data)})
}

// ArtifactStream 打开导出产物供下载（handler 调用）。
func (s *ImpexStore) ArtifactStream(ctx context.Context, id string) (io.ReadCloser, string, int64, error) {
	j, err := s.Get(ctx, id)
	if err != nil {
		return nil, "", 0, err
	}
	if j.Kind != ImpexExport || j.Status != "done" && j.Status != "partial" {
		return nil, "", 0, errors.New("service: 导出未完成或不可下载")
	}
	key := ixStr(j.Result["key"])
	if key == "" {
		return nil, "", 0, errors.New("service: 无导出产物")
	}
	rc, meta, err := s.files.st.Get(ctx, key)
	if err != nil {
		return nil, "", 0, err
	}
	name := ixStr(j.Result["name"])
	if name == "" {
		name = j.Source + ".zip"
	}
	return rc, name, meta.Size, nil
}

// findChild 在指定父目录下按名查未删除文件 id（冲突策略用）。
func (s *ImpexStore) findChild(ctx context.Context, spaceID, parentID, name string) string {
	var id string
	var err error
	if parentID == "" {
		err = s.db.QueryRowContext(ctx,
			`SELECT id FROM files WHERE space_id=? AND name=? AND (parent_id IS NULL OR parent_id='') AND deleted_at IS NULL LIMIT 1`,
			spaceID, name).Scan(&id)
	} else {
		err = s.db.QueryRowContext(ctx,
			`SELECT id FROM files WHERE space_id=? AND parent_id=? AND name=? AND deleted_at IS NULL LIMIT 1`,
			spaceID, parentID, name).Scan(&id)
	}
	if err != nil {
		return ""
	}
	return id
}

// ixRelFile 文件 + 相对路径（导出打包用）。
type ixRelFile struct {
	F   *File
	Rel string
}

// collectTree 递归收集目录树下所有文件（含相对路径），按路径排序保证稳定。
func (s *ImpexStore) collectTree(ctx context.Context, spaceID, rootID string) ([]ixRelFile, error) {
	out := []ixRelFile{}
	var rec func(id, prefix string) error
	rec = func(id, prefix string) error {
		items, err := s.files.ListDir(ctx, spaceID, id, DefaultSiteID)
		if err != nil {
			return err
		}
		for _, it := range items {
			rel := path.Join(prefix, it.Name)
			if it.Kind == "dir" {
				if err := rec(it.ID, rel); err != nil {
					return err
				}
				continue
			}
			out = append(out, ixRelFile{F: it, Rel: rel})
		}
		return nil
	}
	if err := rec(rootID, ""); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

// writeZipEntry 读取文件内容写入 zip（导出用）。
func (s *ImpexStore) writeZipEntry(ctx context.Context, zw *zip.Writer, f *File, rel string) error {
	rc, _, err := s.files.Content(ctx, f.ID)
	if err != nil {
		return err
	}
	defer rc.Close()
	hdr := &zip.FileHeader{Name: ixZipPath(rel), Method: zip.Deflate}
	if f.UpdatedAt > 0 {
		hdr.Modified = time.Unix(f.UpdatedAt, 0)
	}
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, rc)
	return err
}

// readZipEntry 读取 zip 内单个条目（单文件上限 128MB，防 zip bomb）。
func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 128<<20))
}

// ixFetchURL 抓取网页并转 Markdown（仅 http/https，25s 超时，正文上限 8MB；拒绝内网地址防 SSRF）。
func ixFetchURL(ctx context.Context, raw, ua string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", errors.New("仅支持 http/https URL")
	}
	if len(raw) > 2048 {
		return "", "", errors.New("URL 过长（上限 2048 字符）")
	}
	if !ixHostAllowed(u) {
		return "", "", errors.New("不允许访问内网/保留地址")
	}
	if ua == "" {
		ua = "Mozilla/5.0 (compatible; AiKlog/0.1; +https://aiklog.cn)"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := (&http.Client{Timeout: 25 * time.Second,
		Transport:     &http.Transport{DialContext: SafeDialContext}, // H6 收尾：建连前复验最终 IP
		CheckRedirect: safeCheckRedirect}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("抓取失败：HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	title, md := ixHTMLToMarkdown(string(body))
	if strings.TrimSpace(md) == "" {
		return "", "", errors.New("未提取到正文内容")
	}
	return title, md, nil
}

var (
	reHTMLTitle   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reHTMLScript  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reHTMLStyle   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reHTMLArticle = regexp.MustCompile(`(?is)<article[^>]*>(.*?)</article>`)
	reHTMLMainEl  = regexp.MustCompile(`(?is)<main[^>]*>(.*?)</main>`)
	reHTMLBody    = regexp.MustCompile(`(?is)<body[^>]*>(.*?)</body>`)
	reHTMLH       = regexp.MustCompile(`(?is)<h([1-6])[^>]*>(.*?)</h[1-6]>`)
	reHTMLA       = regexp.MustCompile(`(?is)<a[^>]*href=["']?([^"'\s>]+)["']?[^>]*>(.*?)</a>`)
	reHTMLStrong  = regexp.MustCompile(`(?is)<(?:strong|b)[^>]*>(.*?)</(?:strong|b)>`)
	reHTMLEm      = regexp.MustCompile(`(?is)<(?:em|i)[^>]*>(.*?)</(?:em|i)>`)
	reHTMLLi      = regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`)
	reHTMLP       = regexp.MustCompile(`(?is)</p\s*>`)
	reHTMLBr      = regexp.MustCompile(`(?is)<br\s*/?>`)
	reHTMLTag     = regexp.MustCompile(`(?is)<[^>]+>`)
	reMultiNL     = regexp.MustCompile(`\n{3,}`)
	reSpaces      = regexp.MustCompile(`[ \t]{2,}`)

	reFrontmatter = regexp.MustCompile(`(?s)\A---[ \t]*\r?\n(.*?)\r?\n---[ \t]*\r?\n`)
	reWikiLink    = regexp.MustCompile(`\[\[([^\[\]]+?)\]\]`)
)

// ixHTMLToMarkdown 轻量 HTML→Markdown（标题/段落/链接/加粗/列表），不引重型库。
func ixHTMLToMarkdown(raw string) (string, string) {
	s := reHTMLComment.ReplaceAllString(raw, "")
	s = reHTMLScript.ReplaceAllString(s, "")
	s = reHTMLStyle.ReplaceAllString(s, "")

	title := ""
	if m := reHTMLTitle.FindStringSubmatch(s); m != nil {
		title = strings.TrimSpace(html.UnescapeString(reHTMLTag.ReplaceAllString(m[1], "")))
	}

	content := s
	if m := reHTMLArticle.FindStringSubmatch(s); m != nil {
		content = m[1]
	} else if m := reHTMLMainEl.FindStringSubmatch(s); m != nil {
		content = m[1]
	} else if m := reHTMLBody.FindStringSubmatch(s); m != nil {
		content = m[1]
	}

	content = reHTMLH.ReplaceAllStringFunc(content, func(m string) string {
		sm := reHTMLH.FindStringSubmatch(m)
		if sm == nil {
			return m
		}
		txt := strings.TrimSpace(reHTMLTag.ReplaceAllString(sm[2], ""))
		if txt == "" {
			return "\n"
		}
		return "\n\n" + strings.Repeat("#", len(sm[1])) + " " + txt + "\n\n"
	})
	content = reHTMLA.ReplaceAllString(content, "[$2]($1)")
	content = reHTMLStrong.ReplaceAllString(content, "**$1**")
	content = reHTMLEm.ReplaceAllString(content, "*$1*")
	content = reHTMLLi.ReplaceAllString(content, "\n- $1")
	content = reHTMLP.ReplaceAllString(content, "\n\n")
	content = reHTMLBr.ReplaceAllString(content, "\n")
	content = reHTMLTag.ReplaceAllString(content, "")
	content = html.UnescapeString(content)
	content = reSpaces.ReplaceAllString(content, " ")

	lines := strings.Split(content, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t\r")
	}
	content = reMultiNL.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return title, strings.TrimSpace(content)
}

// ixParseFrontmatter 解析 Markdown frontmatter（title/tags 等）；tags 支持行内数组与列表两种写法。
func ixParseFrontmatter(content string) (map[string]string, []string, string) {
	m := reFrontmatter.FindStringSubmatch(content)
	if m == nil {
		return nil, nil, content
	}
	body := content[len(m[0]):]
	meta := map[string]string{}
	var tags []string
	curKey := ""
	for _, ln := range strings.Split(m[1], "\n") {
		t := strings.TrimSpace(ln)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "- ") && curKey == "tags" {
			v := strings.Trim(strings.TrimSpace(strings.TrimPrefix(t, "- ")), `"'`)
			if v != "" {
				tags = append(tags, v)
			}
			continue
		}
		i := strings.Index(t, ":")
		if i < 0 {
			continue
		}
		k := strings.TrimSpace(t[:i])
		v := strings.TrimSpace(t[i+1:])
		if k == "" {
			continue
		}
		curKey = k
		if k == "tags" || k == "tag" {
			if strings.HasPrefix(v, "[") {
				for _, p := range strings.Split(strings.Trim(v, "[]"), ",") {
					if p = strings.Trim(strings.TrimSpace(p), `"'`); p != "" {
						tags = append(tags, p)
					}
				}
			} else if v != "" {
				tags = append(tags, strings.Trim(v, `"'`))
			}
			continue
		}
		meta[k] = strings.Trim(v, `"'`)
	}
	return meta, tags, body
}

// ixWikiLinkNames 提取 [[双链]] 目标名（去掉 |别名 与 #标题 部分）。
func ixWikiLinkNames(content string) []string {
	var out []string
	for _, m := range reWikiLink.FindAllStringSubmatch(content, -1) {
		n := m[1]
		if i := strings.IndexAny(n, "|#"); i >= 0 {
			n = n[:i]
		}
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// ixSafeName 净化文件名（去路径/非法字符，限长）。
func ixSafeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, s)
	s = strings.Trim(s, ". ")
	if len(s) > 120 {
		s = strings.TrimSpace(s[:120])
	}
	return s
}

// ixZipPath 净化 zip 内相对路径（分段过滤 . 与 ..）。
func ixZipPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	var parts []string
	for _, seg := range strings.Split(p, "/") {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		seg = strings.Map(func(r rune) rune {
			if strings.ContainsRune(`:*?"<>|`, r) {
				return '-'
			}
			return r
		}, seg)
		parts = append(parts, seg)
	}
	return strings.Join(parts, "/")
}

// ixUniqueName zip 内去重（同名加 -2/-3 后缀）。
func ixUniqueName(used map[string]bool, name string) string {
	if !used[name] {
		used[name] = true
		return name
	}
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		n := fmt.Sprintf("%s-%d%s", base, i, ext)
		if !used[n] {
			used[n] = true
			return n
		}
	}
}

// ixMimeOf 由扩展名推断 MIME（常见文本显式映射，其余回落 mime.TypeByExtension）。
func ixMimeOf(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".txt", ".log":
		return "text/plain; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	}
	if t := mime.TypeByExtension(strings.ToLower(path.Ext(name))); t != "" {
		return t
	}
	return "application/octet-stream"
}

func isMarkdown(name string) bool {
	e := strings.ToLower(path.Ext(name))
	return e == ".md" || e == ".markdown"
}

func isMarkdownOrText(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown", ".txt", ".text", ".json", ".yaml", ".yml", ".csv", ".tsv",
		".html", ".htm", ".xml", ".ini", ".toml", ".log", ".go", ".py", ".js", ".ts",
		".java", ".c", ".cpp", ".sh", ".sql", ".vue", ".css":
		return true
	}
	return false
}

func ixStr(v any) string {
	s, _ := v.(string)
	return s
}
