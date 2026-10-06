// webdavmount.go 外部 WebDAV 文件库挂载服务。
//
// 与 storage/webdav.go 的 WebDAVBackend 不同：那是"本系统文件存哪里"；
// 这里是"用户挂载的外部网盘作为内容源"，每个挂载独立 URL/凭据，多实例。
// 网络层用 net/http 直发标准 WebDAV 方法（PROPFIND/GET/PUT/MOVE），零第三方依赖；
// PROPFIND 统一 depth=1 逐层浏览（兼容 KodBox 等不支持 infinity 的实现）。
package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ---- 可导入/可读的扩展名 ----

var (
	wmTextExts = map[string]bool{
		".txt": true, ".md": true, ".markdown": true, ".csv": true, ".json": true,
		".html": true, ".htm": true, ".log": true, ".yaml": true, ".yml": true, ".xml": true,
	}
	wmImportExts = map[string]bool{
		".txt": true, ".md": true, ".markdown": true, ".text": true, ".csv": true,
		".json": true, ".html": true, ".htm": true, ".docx": true, ".pdf": true,
		".doc": true, ".xlsx": true, ".xls": true, ".ods": true, ".png": true,
		".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".mp3": true,
		".mp4": true, ".webm": true,
	}
)

// WebDAVMount 挂载记录（对外 JSON 形态；password 仅写时接受，读出脱敏）。
type WebDAVMount struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Username  string `json:"username,omitempty"`
	HasPass   bool   `json:"has_pass"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// WebDAVStore 挂载存储。
type WebDAVStore struct {
	db *sql.DB
}

// NewWebDAVStore 创建挂载存储。
func NewWebDAVStore(db *sql.DB) *WebDAVStore { return &WebDAVStore{db: db} }

// ---- 密码加密（AES-256-GCM；密钥 env AIKMAP_ENC_KEY，缺省派生） ----

func wmEncKey() []byte {
	raw := os.Getenv("AIKMAP_ENC_KEY")
	if raw == "" {
		raw = "aikmap-webdav-mounts-v1"
	}
	h := sha256.Sum256([]byte(raw))
	return h[:]
}

func wmEncrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(wmEncKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func wmDecrypt(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(wmEncKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("密文过短")
	}
	plain, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// ---- CRUD ----

// Create 新增挂载（密码加密存储）。
func (s *WebDAVStore) Create(ctx context.Context, name, rawURL, username, password string) (*WebDAVMount, error) {
	enc, err := wmEncrypt(password)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO webdav_mounts (id, name, url, username, password_enc, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?)`,
		id, name, rawURL, username, enc, now, now); err != nil {
		return nil, err
	}
	return &WebDAVMount{ID: id, Name: name, URL: rawURL, Username: username, HasPass: password != "", CreatedAt: now, UpdatedAt: now}, nil
}

// Update 更新挂载（password 为空保留原密码）。
func (s *WebDAVStore) Update(ctx context.Context, id, name, rawURL, username, password string) (*WebDAVMount, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	enc := m.passEnc
	if password != "" {
		enc, err = wmEncrypt(password)
		if err != nil {
			return nil, err
		}
	}
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE webdav_mounts SET name=?, url=?, username=?, password_enc=?, updated_at=? WHERE id=?`,
		name, rawURL, username, enc, now, id); err != nil {
		return nil, err
	}
	m.Name, m.URL, m.Username = name, rawURL, username
	m.HasPass = enc != ""
	m.UpdatedAt = now
	return m.WebDAVMount, nil
}

// Delete 删除挂载。
func (s *WebDAVStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM webdav_mounts WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// List 挂载列表（密码脱敏）。
func (s *WebDAVStore) List(ctx context.Context) ([]*WebDAVMount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, url, COALESCE(username,''), COALESCE(password_enc,''), created_at, updated_at
		 FROM webdav_mounts ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WebDAVMount{}
	for rows.Next() {
		var m WebDAVMount
		var user, enc string
		if err := rows.Scan(&m.ID, &m.Name, &m.URL, &user, &enc, &m.CreatedAt, &m.UpdatedAt); err != nil {
			continue
		}
		m.Username = user
		m.HasPass = enc != ""
		out = append(out, &m)
	}
	return out, nil
}

type webdavMountRow struct {
	*WebDAVMount
	passEnc string
}

// Get 单条（含密文）。
func (s *WebDAVStore) Get(ctx context.Context, id string) (*webdavMountRow, error) {
	var m WebDAVMount
	var user, enc string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, url, COALESCE(username,''), COALESCE(password_enc,''), created_at, updated_at
		 FROM webdav_mounts WHERE id=?`, id).
		Scan(&m.ID, &m.Name, &m.URL, &user, &enc, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m.Username = user
	m.HasPass = enc != ""
	return &webdavMountRow{WebDAVMount: &m, passEnc: enc}, nil
}

// Password 取明文密码。
func (s *WebDAVStore) Password(ctx context.Context, id string) (string, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return wmDecrypt(m.passEnc)
}

// ---- WebDAV 客户端 ----

// WmItem 远端目录项。
type WmItem struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"` // directory|file
	Size       int64  `json:"size"`
	SizeH      string `json:"size_h,omitempty"`
	Mtime      string `json:"mtime,omitempty"`
	Text       bool   `json:"text"`
	Importable bool   `json:"importable"`
}

// WmClient 单个挂载的 WebDAV 操作封装。
type WmClient struct {
	base     string
	username string
	password string
	client   *http.Client
}

// NewWmClient 构造客户端。
func NewWmClient(rawURL, username, password string) (*WmClient, error) {
	u := strings.TrimSpace(rawURL)
	u = strings.TrimRight(u, "/")
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return nil, errors.New("WebDAV 地址需以 http:// 或 https:// 开头")
	}
	return &WmClient{
		base:     u,
		username: username,
		password: password,
		client:   &http.Client{Timeout: 30 * time.Second, CheckRedirect: safeCheckRedirect},
	}, nil
}

func (c *WmClient) do(method, relPath string, body io.Reader, hdr map[string]string) (*http.Response, error) {
	if strings.HasPrefix(relPath, "/") || strings.Contains(relPath, "://") {
		return nil, errors.New("非法路径")
	}
	u := c.base + "/" + relPath
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接失败：%w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		return nil, errors.New("认证失败：账号/密码错误或无权访问")
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, errors.New("路径不存在")
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("WebDAV 请求失败（HTTP %d）", resp.StatusCode)
	}
	return resp, nil
}

// Test 连通测试。
func (c *WmClient) Test() error {
	resp, err := c.do("PROPFIND", "", nil, map[string]string{"Depth": "1"})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

type davMultistatus struct {
	XMLName xml.Name      `xml:"multistatus"`
	Resp    []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string `xml:"href"`
	Propstat struct {
		Prop struct {
			ResourceType *struct {
				Collection *struct{} `xml:"collection"`
			} `xml:"resourcetype"`
			GetContentLength string `xml:"getcontentlength"`
			GetLastModified  string `xml:"getlastmodified"`
		} `xml:"prop"`
	} `xml:"propstat"`
}

func wmExt(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	return strings.ToLower(name[i:])
}

func wmHumanSize(n int64) string {
	f := float64(n)
	for _, unit := range []string{"B", "KB", "MB", "GB"} {
		if f < 1024 || unit == "GB" {
			if unit == "B" {
				return fmt.Sprintf("%.0f %s", f, unit)
			}
			return fmt.Sprintf("%.1f %s", f, unit)
		}
		f /= 1024
	}
	return fmt.Sprintf("%.1f GB", f)
}

// wmEscapePath 转义路径段但保留 / 分隔符。
func wmEscapePath(rel string) string {
	u := &url.URL{Path: rel}
	return u.EscapedPath()
}

// ListDir 目录浏览（PROPFIND depth=1）。
func (c *WmClient) ListDir(relPath string) ([]WmItem, error) {
	rel := strings.Trim(strings.TrimSpace(relPath), "/")
	resp, err := c.do("PROPFIND", wmEscapePath(rel), nil, map[string]string{"Depth": "1"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var ms davMultistatus
	if err := xml.Unmarshal(raw, &ms); err != nil {
		return nil, fmt.Errorf("目录解析失败：%w", err)
	}
	basePath := ""
	if pu, err := url.Parse(c.base); err == nil {
		basePath = strings.TrimRight(pu.Path, "/")
	}
	items := []WmItem{}
	for _, r := range ms.Resp {
		href := r.Href
		if href == "" {
			continue
		}
		// 跳过当前目录自身（href 以 / 结尾且与请求路径相同）
		dec, err := url.PathUnescape(href)
		if err != nil {
			dec = href
		}
		p := dec
		if strings.Contains(href, "://") {
			if pu, err := url.Parse(href); err == nil {
				p = pu.Path
			}
		} else {
			p = strings.SplitN(href, "?", 2)[0]
		}
		rel := strings.TrimLeft(p, "/")
		if basePath != "" {
			rel = strings.TrimPrefix(rel, strings.TrimLeft(basePath, "/")+"/")
		}
		// KodBox 类前缀
		rel = strings.TrimPrefix(rel, "index.php/dav/")
		rel = strings.TrimLeft(rel, "/")
		if rel == "" {
			continue
		}
		name := rel
		if i := strings.LastIndex(strings.TrimRight(rel, "/"), "/"); i >= 0 {
			name = rel[i+1:]
		}
		if name == "" || name == "." || name == ".." {
			continue
		}
		isDir := r.Propstat.Prop.ResourceType != nil &&
			r.Propstat.Prop.ResourceType.Collection != nil
		it := WmItem{
			Name: name, Path: rel,
			Type:  "file",
			Size:  0,
			Mtime: r.Propstat.Prop.GetLastModified,
		}
		if isDir {
			it.Type = "directory"
		} else {
			var sz int64
			fmt.Sscanf(r.Propstat.Prop.GetContentLength, "%d", &sz)
			it.Size = sz
			ext := wmExt(name)
			it.Text = wmTextExts[ext]
			it.Importable = wmImportExts[ext]
		}
		it.SizeH = wmHumanSize(it.Size)
		items = append(items, it)
	}
	// 目录在前，名称排序
	sortItems(items)
	return items, nil
}

func sortItems(items []WmItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0; j-- {
			a, b := items[j-1], items[j]
			ad, bd := a.Type == "directory", b.Type == "directory"
			if ad == bd {
				if strings.ToLower(a.Name) <= strings.ToLower(b.Name) {
					break
				}
			} else if ad {
				break
			}
			items[j-1], items[j] = items[j], items[j-1]
		}
	}
}

// Read 直读文件字节。
func (c *WmClient) Read(relPath string) ([]byte, error) {
	rel := strings.TrimLeft(strings.TrimSpace(relPath), "/")
	resp, err := c.do("GET", wmEscapePath(rel), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 上限 32MB
}

// ReadStream 直读文件流（导入用）。
func (c *WmClient) ReadStream(relPath string) (io.ReadCloser, error) {
	rel := strings.TrimLeft(strings.TrimSpace(relPath), "/")
	resp, err := c.do("GET", wmEscapePath(rel), nil, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Write 直写（PUT，单文件 <= 32MB）。
func (c *WmClient) Write(relPath string, data []byte) error {
	rel := strings.TrimLeft(strings.TrimSpace(relPath), "/")
	resp, err := c.do("PUT", wmEscapePath(rel), bytes.NewReader(data), map[string]string{
		"Content-Type": "application/octet-stream",
	})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
