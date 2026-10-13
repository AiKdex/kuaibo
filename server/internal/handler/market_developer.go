// market_developer.go 第三方开发者体系（devmarket 批次）。
//
// 设计（两条信任链分开）：
//   - 官方应用走强签名索引（开发者碰不到，见 apps_market_catalog.go）。
//   - 第三方开发者条目走 market_listings 表 + 归属校验（只能改自己的）+ 平台审核闸门。
//
// 流程：注册用户申请开发者（pending）→ 站长审核通过（approved）→ 开发者自建条目（draft）
//   → 上传资源（自动回写 download_url/sha256/icon/screenshots）→ 提交（pending）
//   → 站长审核通过（approved）→ 应用中心出现并可安装。
//
// 鉴权：开发者端点要求登录（authmw 已拦非公开 /api/v1 路径）；审核端点要求站长（blogAdminOnly）。
// 静态资源 /market/uploads/{path} 不在 /api/v1 下，默认公开（与 /market/official 同策略）。
package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// errNotOwner 条目归属校验失败（非本人条目）。
var errNotOwner = errors.New("not owner")

// rowScanner 兼容 *sql.Rows（当前行）与 *sql.Row（单行）的 Scan。
type rowScanner interface {
	Scan(dest ...any) error
}

// marketUploadsRoot 第三方上传资源根目录（生产取 exe/../data/market/uploads，与 themesRoot 同策略）。
func marketUploadsRoot() string {
	base := "data/market/uploads"
	if exe, err := os.Executable(); err == nil {
		base = filepath.Join(filepath.Dir(exe), "..", "data", "market", "uploads")
	}
	return base
}

// currentUserID 返回鉴权后的真实登录用户 id（空=未登录/无上下文）。
func (a *API) currentUserID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxUserID).(string); ok {
		return v
	}
	return ""
}

// loadDeveloper 取当前登录用户的开发者档案（含状态）。
func (a *API) loadDeveloper(r *http.Request) (id, status, displayName string, ok bool) {
	uid := a.currentUserID(r)
	if uid == "" {
		return "", "", "", false
	}
	var devID, st, dn string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT id, status, display_name FROM developers WHERE user_id=?`, uid).
		Scan(&devID, &st, &dn); err != nil {
		return "", "", "", false
	}
	return devID, st, dn, true
}

// marketListing 第三方市场条目（market_listings 行；Developer 为联表补充）。
type marketListing struct {
	ID          string `json:"id"`
	DeveloperID string `json:"developer_id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Screenshots string `json:"screenshots"`
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	PriceCents  int    `json:"price_cents"`
	Tier        string `json:"tier"`
	Status      string `json:"status"`
	ReviewNote  string `json:"review_note"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	Developer   string `json:"developer,omitempty"`
}

// scanListingRow 从行扫描 market_listings（withDev 时联表 developers.display_name）。
func scanListingRow(s rowScanner, withDev bool) (*marketListing, error) {
	m := &marketListing{}
	var dev sql.NullString
	var err error
	if withDev {
		err = s.Scan(&m.ID, &m.DeveloperID, &m.Kind, &m.Name, &m.Description, &m.Icon,
			&m.Screenshots, &m.Version, &m.DownloadURL, &m.SHA256, &m.PriceCents, &m.Tier,
			&m.Status, &m.ReviewNote, &m.CreatedAt, &m.UpdatedAt, &dev)
	} else {
		err = s.Scan(&m.ID, &m.DeveloperID, &m.Kind, &m.Name, &m.Description, &m.Icon,
			&m.Screenshots, &m.Version, &m.DownloadURL, &m.SHA256, &m.PriceCents, &m.Tier,
			&m.Status, &m.ReviewNote, &m.CreatedAt, &m.UpdatedAt)
	}
	if err != nil {
		return nil, err
	}
	if dev.Valid {
		m.Developer = dev.String
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// 开发者档案
// ---------------------------------------------------------------------------

// devApply POST /api/v1/developer/apply：申请成为开发者（pending）。已存在则按状态处理：
// pending 直接返回；rejected 允许更新资料并重新进入 pending；approved 直接返回。
func (a *API) devApply(w http.ResponseWriter, r *http.Request) {
	uid := a.currentUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "需要登录")
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
		Bio         string `json:"bio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "无效请求")
		return
	}
	dn := strings.TrimSpace(req.DisplayName)
	if dn == "" {
		writeErr(w, http.StatusBadRequest, "NAME_REQUIRED", "请填写开发者名称")
		return
	}
	var existingID, existingStatus string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT id, status FROM developers WHERE user_id=?`, uid).Scan(&existingID, &existingStatus)
	if err == nil {
		switch existingStatus {
		case "pending", "approved":
			a.writeDeveloperProfile(w, r, existingID)
			return
		case "rejected":
			_, _ = a.db.ExecContext(r.Context(),
				`UPDATE developers SET display_name=?, bio=?, status='pending', updated_at=? WHERE id=?`,
				dn, req.Bio, time.Now().Unix(), existingID)
			a.writeDeveloperProfile(w, r, existingID)
			return
		}
	}
	id := newID()
	now := time.Now().Unix()
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO developers (id, user_id, display_name, bio, status, created_at, updated_at)
		 VALUES (?,?,?,?,'pending',?,?)`, id, uid, dn, req.Bio, now, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "status": "pending", "display_name": dn})
}

// writeDeveloperProfile 输出开发者档案。
func (a *API) writeDeveloperProfile(w http.ResponseWriter, _ *http.Request, devID string) {
	var id, uid, dn, bio, st string
	var ca, ua int64
	if err := a.db.QueryRowContext(context.Background(),
		`SELECT id, user_id, display_name, bio, status, created_at, updated_at FROM developers WHERE id=?`, devID).
		Scan(&id, &uid, &dn, &bio, &st, &ca, &ua); err != nil {
		writeErr(w, http.StatusNotFound, "DEV_NOT_FOUND", "开发者档案不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "user_id": uid, "display_name": dn, "bio": bio, "status": st,
		"created_at": ca, "updated_at": ua,
	})
}

// devMe GET /api/v1/developer/me：返回当前用户开发者档案（exists=false 表示尚未申请）。
func (a *API) devMe(w http.ResponseWriter, r *http.Request) {
	uid := a.currentUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "需要登录")
		return
	}
	var id, dn, bio, st string
	var ca, ua int64
	err := a.db.QueryRowContext(r.Context(),
		`SELECT id, display_name, bio, status, created_at, updated_at FROM developers WHERE user_id=?`, uid).
		Scan(&id, &dn, &bio, &st, &ca, &ua)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"exists": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"exists": true, "id": id, "display_name": dn, "bio": bio, "status": st,
		"created_at": ca, "updated_at": ua,
	})
}

// ---------------------------------------------------------------------------
// 开发者条目 CRUD（归属校验：只能改自己的）
// ---------------------------------------------------------------------------

// devOwnedListing 取条目并校验归属；errNotOwner / sql.ErrNoRows。
func (a *API) devOwnedListing(r *http.Request, id, devID string) (*marketListing, error) {
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, developer_id, kind, name, description, icon, screenshots, version,
		        download_url, sha256, price_cents, tier, status, review_note, created_at, updated_at
		 FROM market_listings WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	m, err := scanListingRow(rows, false)
	if err != nil {
		return nil, err
	}
	if m.DeveloperID != devID {
		return nil, errNotOwner
	}
	return m, nil
}

// devListings GET /api/v1/developer/listings：列出本人全部条目。
func (a *API) devListings(w http.ResponseWriter, r *http.Request) {
	devID, st, _, ok := a.loadDeveloper(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "DEV_REQUIRED", "请先申请成为开发者")
		return
	}
	if st != "approved" {
		writeErr(w, http.StatusForbidden, "DEV_NOT_APPROVED", "开发者账户待审核或未通过")
		return
	}
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, developer_id, kind, name, description, icon, screenshots, version,
		        download_url, sha256, price_cents, tier, status, review_note, created_at, updated_at
		 FROM market_listings WHERE developer_id=? ORDER BY updated_at DESC`, devID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	defer rows.Close()
	out := []marketListing{}
	for rows.Next() {
		m, err := scanListingRow(rows, false)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
			return
		}
		out = append(out, *m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// devListingCreate POST /api/v1/developer/listings：新建草稿条目。
func (a *API) devListingCreate(w http.ResponseWriter, r *http.Request) {
	devID, st, _, ok := a.loadDeveloper(r)
	if !ok || st != "approved" {
		writeErr(w, http.StatusForbidden, "DEV_NOT_APPROVED", "仅已通过审核的开发者可发布应用")
		return
	}
	var req struct {
		Kind        string `json:"kind"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     string `json:"version"`
		Tier        string `json:"tier"`
		PriceCents  int    `json:"price_cents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "无效请求")
		return
	}
	kind := req.Kind
	if kind != "plugin" && kind != "theme" && kind != "editor" && kind != "ai" {
		writeErr(w, http.StatusBadRequest, "BAD_KIND", "kind 须为 plugin|theme|editor|ai")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "NAME_REQUIRED", "请填写应用名称")
		return
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = "0.1.0"
	}
	tier := req.Tier
	if tier != "paid" {
		tier = "free"
	}
	id := newID()
	now := time.Now().Unix()
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO market_listings
		   (id, developer_id, kind, name, description, icon, screenshots, version,
		    download_url, sha256, price_cents, tier, status, review_note, created_at, updated_at)
		 VALUES (?,?,?,?,?,'','[]',?, '', '', ?, ?, 'draft', '', ?, ?)`,
		id, devID, kind, name, req.Description, version, req.PriceCents, tier, now, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "status": "draft"})
}

// devListingGet GET /api/v1/developer/listings/{id}：查看本人条目。
func (a *API) devListingGet(w http.ResponseWriter, r *http.Request) {
	devID, _, _, ok := a.loadDeveloper(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "DEV_REQUIRED", "请先申请成为开发者")
		return
	}
	id := r.PathValue("id")
	m, err := a.devOwnedListing(r, id, devID)
	if errors.Is(err, errNotOwner) {
		writeErr(w, http.StatusForbidden, "NOT_OWNER", "只能查看自己的条目")
		return
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, "LISTING_NOT_FOUND", "条目不存在")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// devListingUpdate PUT /api/v1/developer/listings/{id}：更新本人条目（审核中不可改；已上架改后回退待审）。
func (a *API) devListingUpdate(w http.ResponseWriter, r *http.Request) {
	devID, _, _, ok := a.loadDeveloper(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "DEV_REQUIRED", "请先申请成为开发者")
		return
	}
	id := r.PathValue("id")
	m, err := a.devOwnedListing(r, id, devID)
	if errors.Is(err, errNotOwner) {
		writeErr(w, http.StatusForbidden, "NOT_OWNER", "只能修改自己的条目")
		return
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, "LISTING_NOT_FOUND", "条目不存在")
		return
	}
	if m.Status == "pending" {
		writeErr(w, http.StatusConflict, "UNDER_REVIEW", "条目审核中，暂不可编辑（撤回审核后再改）")
		return
	}
	var b struct {
		Kind        string `json:"kind"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
		Screenshots string `json:"screenshots"`
		Version     string `json:"version"`
		DownloadURL string `json:"download_url"`
		SHA256      string `json:"sha256"`
		Tier        string `json:"tier"`
		PriceCents  int    `json:"price_cents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "无效请求")
		return
	}
	// 字段归一
	if b.Kind != "" && b.Kind != "plugin" && b.Kind != "theme" && b.Kind != "editor" && b.Kind != "ai" {
		writeErr(w, http.StatusBadRequest, "BAD_KIND", "kind 须为 plugin|theme|editor|ai")
		return
	}
	kind := m.Kind
	if b.Kind != "" {
		kind = b.Kind
	}
	name := m.Name
	if strings.TrimSpace(b.Name) != "" {
		name = b.Name
	}
	version := m.Version
	if strings.TrimSpace(b.Version) != "" {
		version = b.Version
	}
	tier := m.Tier
	if b.Tier == "paid" || b.Tier == "free" {
		tier = b.Tier
	}
	// 已上架条目编辑后回退待审（重新审核）；草稿/驳回保持原状态（驳回改后仍需重新提交）
	newStatus := m.Status
	if m.Status == "approved" {
		newStatus = "pending"
	}
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE market_listings SET kind=?, name=?, description=?, icon=?, screenshots=?, version=?,
		   download_url=?, sha256=?, price_cents=?, tier=?, status=?, review_note='', updated_at=? WHERE id=?`,
		kind, name, b.Description, b.Icon, b.Screenshots, version,
		b.DownloadURL, b.SHA256, b.PriceCents, tier, newStatus, time.Now().Unix(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": newStatus})
}

// devListingDelete DELETE /api/v1/developer/listings/{id}：删除本人草稿/驳回条目（已上架不可删）。
func (a *API) devListingDelete(w http.ResponseWriter, r *http.Request) {
	devID, _, _, ok := a.loadDeveloper(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "DEV_REQUIRED", "请先申请成为开发者")
		return
	}
	id := r.PathValue("id")
	m, err := a.devOwnedListing(r, id, devID)
	if errors.Is(err, errNotOwner) {
		writeErr(w, http.StatusForbidden, "NOT_OWNER", "只能删除自己的条目")
		return
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, "LISTING_NOT_FOUND", "条目不存在")
		return
	}
	if m.Status == "approved" || m.Status == "pending" {
		writeErr(w, http.StatusConflict, "STATUS_LOCKED", "已上架或在审条目不可删除（已上架请先联系站长下架）")
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM market_listings WHERE id=? AND developer_id=?`, id, devID); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// devListingSubmit POST /api/v1/developer/listings/{id}/submit：草稿/驳回 → 提交待审（需已上传包）。
func (a *API) devListingSubmit(w http.ResponseWriter, r *http.Request) {
	devID, _, _, ok := a.loadDeveloper(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "DEV_REQUIRED", "请先申请成为开发者")
		return
	}
	id := r.PathValue("id")
	m, err := a.devOwnedListing(r, id, devID)
	if errors.Is(err, errNotOwner) {
		writeErr(w, http.StatusForbidden, "NOT_OWNER", "只能提交自己的条目")
		return
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, "LISTING_NOT_FOUND", "条目不存在")
		return
	}
	if m.Status == "pending" {
		writeErr(w, http.StatusConflict, "UNDER_REVIEW", "条目已在审核队列中")
		return
	}
	if m.Status == "approved" {
		writeErr(w, http.StatusConflict, "ALREADY_APPROVED", "条目已通过审核")
		return
	}
	if m.DownloadURL == "" || m.SHA256 == "" {
		writeErr(w, http.StatusBadRequest, "PACKAGE_REQUIRED", "请先上传应用包（含下载地址与校验和）后再提交审核")
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE market_listings SET status='pending', review_note='', updated_at=? WHERE id=?`, time.Now().Unix(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "pending"})
}

// sanitizeUploadName 过滤上传文件名（仅保留安全字符；目录遍历由上层路径拼接防护兜底）。
func sanitizeUploadName(name string) string {
	name = filepath.Base(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	s := strings.Trim(b.String(), "._")
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

// devListingUpload POST /api/v1/developer/listings/{id}/upload：上传资源（multipart field=file, query kind=package|icon|screenshot）。
// 自动回写：package→download_url+sha256；icon→icon；screenshot→screenshots(JSON 追加)。
func (a *API) devListingUpload(w http.ResponseWriter, r *http.Request) {
	devID, st, _, ok := a.loadDeveloper(r)
	if !ok || st != "approved" {
		writeErr(w, http.StatusForbidden, "DEV_NOT_APPROVED", "仅已通过审核的开发者可上传资源")
		return
	}
	id := r.PathValue("id")
	var ownerDev string
	if err := a.db.QueryRowContext(r.Context(), `SELECT developer_id FROM market_listings WHERE id=?`, id).Scan(&ownerDev); err != nil {
		writeErr(w, http.StatusNotFound, "LISTING_NOT_FOUND", "条目不存在")
		return
	}
	if ownerDev != devID {
		writeErr(w, http.StatusForbidden, "NOT_OWNER", "只能上传自己条目的资源")
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind != "package" && kind != "icon" && kind != "screenshot" {
		writeErr(w, http.StatusBadRequest, "BAD_KIND", "kind 须为 package|icon|screenshot")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_FORM", "上传表单解析失败")
		return
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NO_FILE", "缺少文件字段 file")
		return
	}
	defer f.Close()
	base := sanitizeUploadName(fh.Filename)
	if base == "" {
		base = newID()
	}
	if kind == "package" && !strings.HasSuffix(base, ".zip") {
		base += ".zip"
	}
	rel := filepath.Join(devID, id, base)
	dest := filepath.Join(marketUploadsRoot(), rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, "WRITE_FAIL", err.Error())
		return
	}
	out, err := os.Create(dest)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "WRITE_FAIL", err.Error())
		return
	}
	h := sha256.New()
	if _, err := io.Copy(out, io.TeeReader(io.LimitReader(f, 32<<20), h)); err != nil {
		out.Close()
		os.Remove(dest)
		writeErr(w, http.StatusInternalServerError, "WRITE_FAIL", "保存失败")
		return
	}
	out.Close()
	url := "/market/uploads/" + strings.ReplaceAll(rel, "\\", "/")
	now := time.Now().Unix()
	switch kind {
	case "package":
		sha := hex.EncodeToString(h.Sum(nil))
		if _, err := a.db.ExecContext(r.Context(),
			`UPDATE market_listings SET download_url=?, sha256=?, updated_at=? WHERE id=?`, url, sha, now, id); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": url, "sha256": sha})
	case "icon":
		if _, err := a.db.ExecContext(r.Context(),
			`UPDATE market_listings SET icon=?, updated_at=? WHERE id=?`, url, now, id); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": url})
	case "screenshot":
		var shots []string
		var cur string
		_ = a.db.QueryRowContext(r.Context(), `SELECT screenshots FROM market_listings WHERE id=?`, id).Scan(&cur)
		if cur != "" {
			_ = json.Unmarshal([]byte(cur), &shots)
		}
		shots = append(shots, url)
		bs, _ := json.Marshal(shots)
		if _, err := a.db.ExecContext(r.Context(),
			`UPDATE market_listings SET screenshots=?, updated_at=? WHERE id=?`, string(bs), now, id); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": url, "screenshots": shots})
	}
}

// ---------------------------------------------------------------------------
// 站长审核闸门（官方信任链之外，平台对第三方内容的最后一道关）
// ---------------------------------------------------------------------------

// adminDevelopers GET /api/v1/admin/market/developers：开发者申请列表（status=pending|all）。
func (a *API) adminDevelopers(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	var rows *sql.Rows
	var err error
	if status == "all" {
		rows, err = a.db.QueryContext(r.Context(),
			`SELECT id, user_id, display_name, bio, status, created_at, updated_at FROM developers ORDER BY created_at DESC`)
	} else {
		rows, err = a.db.QueryContext(r.Context(),
			`SELECT id, user_id, display_name, bio, status, created_at, updated_at FROM developers WHERE status=? ORDER BY created_at DESC`, status)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m, err := scanDeveloperRow(rows)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// scanDeveloperRow 扫描 developers 行。
func scanDeveloperRow(s rowScanner) (map[string]any, error) {
	var id, uid, dn, bio, st string
	var ca, ua int64
	if err := s.Scan(&id, &uid, &dn, &bio, &st, &ca, &ua); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "user_id": uid, "display_name": dn, "bio": bio,
		"status": st, "created_at": ca, "updated_at": ua,
	}, nil
}

// adminDeveloperApprove POST /api/v1/admin/market/developers/{id}/approve。
func (a *API) adminDeveloperApprove(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	res, err := a.db.ExecContext(r.Context(),
		`UPDATE developers SET status='approved', updated_at=? WHERE id=?`, time.Now().Unix(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "DEV_NOT_FOUND", "开发者档案不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "approved"})
}

// adminDeveloperReject POST /api/v1/admin/market/developers/{id}/reject。
func (a *API) adminDeveloperReject(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	res, err := a.db.ExecContext(r.Context(),
		`UPDATE developers SET status='rejected', updated_at=? WHERE id=?`, time.Now().Unix(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "DEV_NOT_FOUND", "开发者档案不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "rejected"})
}

// adminMarketListings GET /api/v1/admin/market/listings：第三方条目审核列表（status=pending|all）。
func (a *API) adminMarketListings(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	var rows *sql.Rows
	var err error
	q := `SELECT l.id, l.developer_id, l.kind, l.name, l.description, l.icon, l.screenshots, l.version,
	             l.download_url, l.sha256, l.price_cents, l.tier, l.status, l.review_note, l.created_at, l.updated_at,
	             d.display_name
	      FROM market_listings l JOIN developers d ON d.id=l.developer_id`
	if status == "all" {
		q += ` ORDER BY l.updated_at DESC`
		rows, err = a.db.QueryContext(r.Context(), q)
	} else {
		q += ` WHERE l.status=? ORDER BY l.updated_at DESC`
		rows, err = a.db.QueryContext(r.Context(), q, status)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	defer rows.Close()
	out := []marketListing{}
	for rows.Next() {
		m, err := scanListingRow(rows, true)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
			return
		}
		out = append(out, *m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// adminMarketApprove POST /api/v1/admin/market/listings/{id}/approve：通过审核（approved）。
func (a *API) adminMarketApprove(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	res, err := a.db.ExecContext(r.Context(),
		`UPDATE market_listings SET status='approved', review_note='', updated_at=? WHERE id=?`, time.Now().Unix(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "LISTING_NOT_FOUND", "条目不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "approved"})
}

// adminMarketReject POST /api/v1/admin/market/listings/{id}/reject：驳回（body {review_note}）。
func (a *API) adminMarketReject(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		ReviewNote string `json:"review_note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	res, err := a.db.ExecContext(r.Context(),
		`UPDATE market_listings SET status='rejected', review_note=?, updated_at=? WHERE id=?`, req.ReviewNote, time.Now().Unix(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_ERR", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "LISTING_NOT_FOUND", "条目不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "rejected"})
}

// ---------------------------------------------------------------------------
// 静态资源服务（公开）
// ---------------------------------------------------------------------------

// marketUploadFile GET /market/uploads/{path}：第三方上传资源公开静态服务（路径穿越防护）。
func (a *API) marketUploadFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	if name == "" || strings.Contains(name, "..") ||
		strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		writeErr(w, http.StatusBadRequest, "BAD_PATH", "非法路径")
		return
	}
	root := marketUploadsRoot()
	absRoot, _ := filepath.Abs(root)
	p := filepath.Join(root, name)
	absP, _ := filepath.Abs(p)
	if !strings.HasPrefix(absP, absRoot+string(os.PathSeparator)) {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "资源不存在")
		return
	}
	st, err := os.Stat(absP)
	if err != nil || st.IsDir() {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "资源不存在")
		return
	}
	http.ServeFile(w, r, absP)
}
