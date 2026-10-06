// Package handler 的 webdav.go：对外 WebDAV 服务端（aikbox 挂载用）。
//
// 能力：把用户的 home space 文件树通过 WebDAV 协议暴露，任意 WebDAV 客户端
// （rclone / Windows 映射网络驱动器 / 坚果云 WebDAV 等）可直接挂载。
//
// 鉴权：HTTP Basic（WebDAV 客户端普遍支持）。
//   - 密码可以是账号密码、会话 token、或服务令牌（全量/只读/采集）；
//   - 认证通过后以该用户身份操作其 home space（多用户隔离天然成立）；
//   - viewer 角色只读（写方法返回 403）。
//
// 语义映射：
//   - PUT 覆盖写（文件已存在走 ReplaceContentBinary，避免 Upload 的同名加后缀）；
//   - DELETE 软删（进回收站，与 Web UI 一致）；
//   - MKCOL/目录解析复用 FileStore.EnsurePath / ResolveNode；
//   - COPY 通过 webdav.CopyFile 接口实现。
package handler

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
	"golang.org/x/net/webdav"
)

// davEnabled 开关：dav.enabled（默认开启）。
func (a *API) davEnabled() bool {
	return a.cfg.GetBool("dav.enabled")
}

// davHandler 组装对外 WebDAV 服务端。
func (a *API) davHandler() http.Handler {
	h := &webdav.Handler{
		Prefix:     "/api/v1/dav",
		FileSystem: davFS{a: a},
		LockSystem: webdav.NewMemLS(),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.davEnabled() {
			writeErr(w, http.StatusForbidden, "DAV_DISABLED", "WebDAV 未启用（settings dav.enabled）")
			return
		}
		ctx, ok := a.davAuth(w, r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="AiKmap WebDAV"`)
			writeErr(w, http.StatusUnauthorized, "DAV_AUTH_REQUIRED", "需要 WebDAV 登录（账号密码 / 会话 token / 服务令牌）")
			return
		}
		// viewer 角色只读
		if a.davViewer(ctx) && davIsWrite(r.Method) {
			writeErr(w, http.StatusForbidden, "DAV_READONLY", "当前账号为只读角色，不允许写入")
			return
		}
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// davIsWrite 判定 WebDAV 写方法。
func davIsWrite(m string) bool {
	switch m {
	case http.MethodPut, http.MethodDelete, "MKCOL", "MOVE", "COPY", "PROPPATCH", "LOCK", "UNLOCK":
		return true
	}
	return false
}

// davAuth 校验 Basic 凭据并注入 ctxUserID。
func (a *API) davAuth(w http.ResponseWriter, r *http.Request) (context.Context, bool) {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return nil, false
	}
	// 1) 服务令牌（read/ingest 任一）→ 系统 owner
	if a.serviceTokenOK(pass, "read") || a.serviceTokenOK(pass, "ingest") {
		return context.WithValue(r.Context(), ctxUserID, service.SystemOwnerID), true
	}
	// 2) 会话 token
	var uid string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT user_id FROM sessions WHERE token=? AND expires_at>?`, pass, time.Now().Unix()).Scan(&uid); err == nil && uid != "" {
		return context.WithValue(r.Context(), ctxUserID, uid), true
	}
	// 3) 账号 + 密码
	if user != "" {
		var hid, role, st string
		if err := a.db.QueryRowContext(r.Context(),
			`SELECT id, pass_hash, role, status FROM users WHERE username=?`, user).Scan(&uid, &hid, &role, &st); err == nil &&
			st == "active" && service.VerifyPassword(hid, pass) {
			return context.WithValue(r.Context(), ctxUserID, uid), true
		}
	}
	return nil, false
}

// davViewer 判断当前 ctx 用户是否 viewer 只读角色。
func (a *API) davViewer(ctx context.Context) bool {
	uid, _ := ctx.Value(ctxUserID).(string)
	if uid == "" {
		return true
	}
	var role string
	if err := a.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id=?`, uid).Scan(&role); err != nil {
		return true
	}
	return role == "viewer"
}

// davUserCtx 从 ctx 解析 (uid, homeSpaceID)。
func (a *API) davUserCtx(ctx context.Context) (string, string, error) {
	uid, _ := ctx.Value(ctxUserID).(string)
	if uid == "" {
		return "", "", errors.New("webdav: no user in ctx")
	}
	if uid == a.homeOwnerID() {
		return uid, a.homeSpaceID(), nil
	}
	var sid string
	err := a.db.QueryRowContext(ctx, `SELECT id FROM spaces WHERE owner_id=? AND kind='home'`, uid).Scan(&sid)
	if err != nil {
		return uid, "", err
	}
	return uid, sid, nil
}

// davFS 实现 webdav.FileSystem，把 WebDAV 路径映射到用户 home space 文件树。
type davFS struct {
	a *API
}

// cleanPath 规范化 WebDAV 路径（去前缀斜杠、防穿越）。
func cleanPath(name string) string {
	name = path.Clean("/" + strings.TrimPrefix(name, "/"))
	return strings.TrimPrefix(name, "/")
}

// davInfo 包装 *service.File 实现 os.FileInfo。
type davInfo struct {
	f *service.File
}

func (d davInfo) Name() string {
	if d.f.Name == "" {
		return "/"
	}
	return d.f.Name
}
func (d davInfo) Size() int64 {
	if d.f.Kind == "dir" {
		return 0
	}
	return d.f.Size
}
func (d davInfo) Mode() os.FileMode {
	if d.f.Kind == "dir" {
		return os.ModeDir | 0o755
	}
	return 0o644
}
func (d davInfo) ModTime() time.Time { return time.Unix(d.f.UpdatedAt, 0) }
func (d davInfo) IsDir() bool        { return d.f.Kind == "dir" }
func (d davInfo) Sys() any           { return nil }

// lookup 严格解析路径（只查不建）。
// 返回 (parentID, node, err)：
//   - name 为空/根 → parentID="", node=虚拟空间根（Kind=dir），err=nil；
//   - 目录命中 → parentID=目录自身 ID（写子项时即父目录），node=目录；
//   - 文件命中 → parentID=父目录 ID，node=文件；
//   - 任一段缺失（含父目录缺失）→ parentID=最近存在目录 ID，node=nil，err=os.ErrNotExist。
func (d davFS) lookup(ctx context.Context, name string) (string, *service.File, error) {
	_, spaceID, err := d.a.davUserCtx(ctx)
	if err != nil {
		return "", nil, err
	}
	rel := cleanPath(name)
	if rel == "" {
		return "", &service.File{SpaceID: spaceID, Kind: "dir", Name: ""}, nil
	}
	segs := strings.Split(rel, "/")
	cur := ""
	var last *service.File
	for idx, seg := range segs {
		if seg == "" || seg == "." {
			continue
		}
		if !validDavName(seg) {
			return cur, nil, os.ErrNotExist
		}
		var f service.File
		var parentID, mime, sha256, slug sql.NullString
		var size, viewCount, createdAt, updatedAt sql.NullInt64
		var q string
		var args []any
		if cur == "" {
			q = `SELECT id, space_id, owner_id, parent_id, name, kind, mime, size, sha256, storage_ref, version, content_state, slug, view_count, created_at, updated_at FROM files WHERE space_id=? AND parent_id IS NULL AND name=? AND deleted_at IS NULL`
			args = []any{spaceID, seg}
		} else {
			q = `SELECT id, space_id, owner_id, parent_id, name, kind, mime, size, sha256, storage_ref, version, content_state, slug, view_count, created_at, updated_at FROM files WHERE space_id=? AND parent_id=? AND name=? AND deleted_at IS NULL`
			args = []any{spaceID, cur, seg}
		}
		err := d.a.db.QueryRowContext(ctx, q, args...).Scan(&f.ID, &f.SpaceID, &f.OwnerID, &parentID, &f.Name, &f.Kind, &mime, &size, &sha256, &f.StorageRef, &f.Version, &f.ContentState, &slug, &viewCount, &createdAt, &updatedAt)
		if err != nil {
			log.Printf("dav lookup err: rel=%s seg=%s space=%s cur=%s err=%v", rel, seg, spaceID, cur, err)
			return cur, nil, os.ErrNotExist
		}
		f.ParentID, f.Mime, f.SHA256, f.Slug = parentID.String, mime.String, sha256.String, slug.String
		f.Size, f.ViewCount = size.Int64, viewCount.Int64
		if createdAt.Valid {
			f.CreatedAt = createdAt.Int64
		}
		if updatedAt.Valid {
			f.UpdatedAt = updatedAt.Int64
		}
		if f.Kind == "dir" {
			cur = f.ID
		} else if idx < len(segs)-1 {
			return cur, nil, os.ErrNotExist
		}
		last = &f
	}
	if last == nil {
		return cur, nil, os.ErrNotExist
	}
	return cur, last, nil
}

// resolve 兼容包装：返回 (父目录ID, 节点, err)。
func (d davFS) resolve(ctx context.Context, name string) (string, *service.File, error) {
	return d.lookup(ctx, name)
}

// Mkdir 创建目录（父目录必须已存在）。
func (d davFS) Mkdir(ctx context.Context, name string, _ os.FileMode) error {
	uid, spaceID, err := d.a.davUserCtx(ctx)
	if err != nil {
		return err
	}
	rel := cleanPath(name)
	if rel == "" {
		return os.ErrExist
	}
	dir, base := path.Split(rel)
	parentID, _, err := d.resolve(ctx, dir)
	if err != nil {
		return err
	}
	if !validDavName(base) {
		return os.ErrInvalid
	}
	if _, exist, _ := d.resolve(ctx, rel); exist != nil {
		return os.ErrExist
	}
	_, err = d.a.files.CreateDir(ctx, uid, spaceID, parentID, base, service.DefaultSiteID)
	return err
}

// OpenFile 打开/创建节点。
func (d davFS) OpenFile(ctx context.Context, name string, flag int, _ os.FileMode) (webdav.File, error) {
	uid, spaceID, err := d.a.davUserCtx(ctx)
	if err != nil {
		return nil, err
	}
	rel := cleanPath(name)
	if rel == "" {
		// 根路径：返回虚拟空间根（只读目录，Readdir 列根条目）
		return &davFile{a: d.a, uid: uid, spaceID: spaceID, name: "", node: &service.File{SpaceID: spaceID, Kind: "dir", Name: ""}, mode: 'r'}, nil
	}
	dir, base := path.Split(rel)
	parentID, _, err := d.resolve(ctx, dir)
	if err != nil {
		return nil, err // 父目录缺失 → 标准库 PUT 转 409 / GET 转 404
	}
	if !validDavName(base) {
		return nil, os.ErrInvalid
	}
	df := &davFile{a: d.a, uid: uid, spaceID: spaceID, parentID: parentID, name: base}
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC) != 0 {
		// 写模式：父目录已确认存在；目标可新建或覆盖
		_, node, err := d.resolve(ctx, rel)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if node != nil && node.Kind == "dir" {
			return nil, os.ErrExist
		}
		df.node = node // 可能 nil（新建）
		tmp, err := os.CreateTemp("", "aikmap-dav-*")
		if err != nil {
			return nil, err
		}
		df.tmp = tmp
		df.mode = 'w'
		return df, nil
	}
	// 读模式
	_, node, err := d.resolve(ctx, rel)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, os.ErrNotExist
	}
	if node.Kind == "dir" {
		df.node = node
		df.mode = 'r'
		return df, nil // Readdir 用；不读内容
	}
	rc, _, err := d.a.files.Content(ctx, node.ID)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	tmp, err := os.CreateTemp("", "aikmap-dav-*")
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, err
	}
	df.node = node
	df.tmp = tmp
	df.mode = 'r'
	return df, nil
}

// RemoveAll 删除（软删进回收站，与 Web UI 一致）。
func (d davFS) RemoveAll(ctx context.Context, name string) error {
	uid, _, err := d.a.davUserCtx(ctx)
	if err != nil {
		return err
	}
	_, node, err := d.resolve(ctx, name)
	if err != nil {
		return err
	}
	if node == nil {
		return os.ErrNotExist
	}
	return d.a.files.Delete(ctx, uid, node.ID)
}

// Rename 移动/重命名（MOVE）。
func (d davFS) Rename(ctx context.Context, oldName, newName string) error {
	uid, _, err := d.a.davUserCtx(ctx)
	if err != nil {
		return err
	}
	_, src, err := d.resolve(ctx, oldName)
	if err != nil {
		return err
	}
	if src == nil {
		return os.ErrNotExist
	}
	dir, base := path.Split(cleanPath(newName))
	dstParentID, _, err := d.resolve(ctx, dir) // 目标父目录必须存在
	if err != nil {
		return err
	}
	// 目标自身已存在检查（WebDAV 不静默覆盖；Overwrite=T 由标准库先删后移）
	if _, dstExist, _ := d.resolve(ctx, cleanPath(newName)); dstExist != nil && dstExist.ID != src.ID {
		return os.ErrExist
	}
	if !validDavName(base) {
		return os.ErrInvalid
	}
	_, err = d.a.files.Move(ctx, uid, src.ID, dstParentID, base)
	return err
}
// CopyFile 实现 webdav.CopyFile（COPY 请求直通服务层子树复制，语义对齐标准库 copyFiles）：
//   404=源不存在；412=目标已存在且 Overwrite:F；201=新建；204=覆盖；
//   403=目标为源自身或源之子孙（RFC 4918 §9.8.3 无限递归防护）；
//   Depth:0 仅复制条目本身（目录=建同名空目录，不含子项）；infinity=整棵子树。
// 服务层 FileStore.Copy：文件=存储后端原生复制（S3/WebDAV 服务端复制、Local io.Copy），
// 目录=递归含子树；覆盖时先软删目标再复制（与 Web UI 回收站语义一致）；冲突名自动改名。
func (d davFS) CopyFile(ctx context.Context, srcName, destName string, overwrite bool, depth int, _ int) (int, error) {
	uid, spaceID, err := d.a.davUserCtx(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	_, src, err := d.resolve(ctx, srcName)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if src == nil {
		return http.StatusNotFound, os.ErrNotExist
	}
	// 目标为源自身或源之子孙 → 403（防无限递归）
	if src.Kind == "dir" {
		srcRel := cleanPath(srcName)
		dstRel := cleanPath(destName)
		if dstRel == srcRel || strings.HasPrefix(dstRel, srcRel+"/") {
			return http.StatusForbidden, errors.New("webdav: cannot copy a directory into itself")
		}
	}
	dir, base := path.Split(cleanPath(destName))
	if base == "" {
		return http.StatusForbidden, errors.New("webdav: invalid destination")
	}
	dstParentID, _, err := d.resolve(ctx, dir)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !validDavName(base) {
		return http.StatusBadRequest, os.ErrInvalid
	}
	// 目标已存在：Overwrite 语义（F=412；T=先软删再复制，与 Web UI 一致）
	created := false
	if _, dstExist, _ := d.resolve(ctx, cleanPath(destName)); dstExist != nil {
		if !overwrite {
			return http.StatusPreconditionFailed, os.ErrExist
		}
		if err := d.a.files.Delete(ctx, uid, dstExist.ID); err != nil {
			return http.StatusInternalServerError, err
		}
	} else {
		created = true
	}
	// Depth:0 —— 目录仅建同名空目录（不含子项）
	if depth == 0 && src.Kind == "dir" {
		if _, err := d.a.files.CreateDir(ctx, uid, spaceID, dstParentID, base, service.DefaultSiteID); err != nil {
			return http.StatusInternalServerError, err
		}
		if created {
			return http.StatusCreated, nil
		}
		return http.StatusNoContent, nil
	}
	// 服务层子树复制（文件=存储原生 Copy；目录=递归含子树）
	if _, err := d.a.files.Copy(ctx, uid, src.ID, dstParentID, base); err != nil {
		return http.StatusInternalServerError, err
	}
	if created {
		return http.StatusCreated, nil
	}
	return http.StatusNoContent, nil
}

// Stat 节点元信息。
func (d davFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	_, node, err := d.resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, os.ErrNotExist
	}
	return davInfo{f: node}, nil
}

// validDavName 校验 WebDAV 段名（与 FileStore.validName 一致，杜绝穿越）。
func validDavName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return false
	}
	return true
}

// davFile 实现 webdav.File。
// 读写都经临时文件缓冲：读=内容落临时文件支持 Seek/Range；写=Close 时落库（覆盖或新建）。
type davFile struct {
	a        *API
	uid      string
	spaceID  string
	parentID string
	name     string
	node     *service.File // 已存在节点；写模式 nil=新建
	tmp      *os.File
	mode     byte // 'r' | 'w'
	closed   bool
}

func (f *davFile) Read(p []byte) (int, error) {
	if f.mode != 'r' || f.tmp == nil {
		return 0, errors.New("webdav: not readable")
	}
	return f.tmp.Read(p)
}

func (f *davFile) Write(p []byte) (int, error) {
	if f.mode != 'w' || f.tmp == nil {
		return 0, errors.New("webdav: not writable")
	}
	return f.tmp.Write(p)
}

func (f *davFile) Seek(offset int64, whence int) (int64, error) {
	if f.tmp == nil {
		return 0, errors.New("webdav: no seekable buffer")
	}
	return f.tmp.Seek(offset, whence)
}

// Close 写模式落库（覆盖或新建），计入上传配额。
func (f *davFile) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	if f.mode == 'w' && f.tmp != nil {
		defer func() {
			f.tmp.Close()
			os.Remove(f.tmp.Name())
		}()
		if _, err := f.tmp.Seek(0, io.SeekStart); err != nil {
			return err
		}
		st, err := f.tmp.Stat()
		if err != nil {
			return err
		}
		mb := (st.Size() + (1 << 20) - 1) / (1 << 20)
		if ok, _ := f.a.quotaUse(f.uid, quotaUploadMB, mb); !ok {
			return errors.New("webdav: upload quota exceeded")
		}
		if f.node != nil && f.node.Kind == "file" {
			_, err = f.a.files.ReplaceContentBinary(context.Background(), f.uid, f.node.ID, f.tmp, st.Size())
			return err
		}
		_, err = f.a.files.Upload(context.Background(), f.uid, f.spaceID, f.parentID, f.name, "", f.tmp, st.Size(), service.DefaultSiteID)
		return err
	}
	if f.tmp != nil {
		f.tmp.Close()
		os.Remove(f.tmp.Name())
	}
	return nil
}

// Readdir 列目录子项。
func (f *davFile) Readdir(count int) ([]os.FileInfo, error) {
	if f.node == nil || f.node.Kind != "dir" {
		return nil, errors.New("webdav: not a directory")
	}
	items, err := f.a.files.ListDir(context.Background(), f.spaceID, f.node.ID, service.DefaultSiteID)
	if err != nil {
		return nil, err
	}
	out := make([]os.FileInfo, 0, len(items))
	for _, it := range items {
		out = append(out, davInfo{f: it})
	}
	if count > 0 && count < len(out) {
		out = out[:count]
	}
	return out, nil
}

// Stat 返回当前节点信息。
func (f *davFile) Stat() (os.FileInfo, error) {
	if f.node == nil {
		if f.tmp != nil {
			st, err := f.tmp.Stat()
			if err != nil {
				return nil, err
			}
			return davInfo{f: &service.File{Name: f.name, Kind: "file", Size: st.Size(), UpdatedAt: time.Now().Unix()}}, nil
		}
		return nil, os.ErrNotExist
	}
	return davInfo{f: f.node}, nil
}

var _ webdav.File = (*davFile)(nil)
