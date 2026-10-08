# WebDAV / Org 待接线最小实现（主系统现成代码片段）

> 交付方：上游（AiKmap 主系统）｜收件：AiKlog
> 关联：回执《AiKlog回执-CoreModules已接入-R2R4落地-v1.0.md》§一·4（4 个缺口）
> 用途：补齐本壳核心层后注入 WebDAV / Org 两模块（当前保持 nil=路由 404）

---

## 缺口 1 · `service.File` 缺 `ViewCount` 字段

主系统 `server/internal/service/file.go:41`（File 结构体字段，仅读展示，不影响存储布局）：

```go
	ViewCount    int64  `json:"view_count"`     // 公开阅读次数（热门文章数据源）
```

合入提示：在 File 结构体（约 updated_at 附近）补该字段即可；列表/详情 SQL 已含 `view_count` 列（主系统 `handler/shares.go:511` SELECT 已扫），你方确认 SELECT 列集包含 `view_count` 并 Scan 到该字段。

---

## 缺口 2 · `FileStore` 缺 `ReplaceContentBinary`（WebDAV 写回）

主系统 `server/internal/service/file.go:576-617` 全文：

```go
func (s *FileStore) ReplaceContentBinary(ctx context.Context, ownerID, id string, r io.Reader, size int64) (*File, error) {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.Kind != "file" {
		return nil, errors.New("service: not a file")
	}
	// P1-4 版本历史：覆盖写前先快照旧版本
	_ = s.SaveVersion(ctx, ownerID, id, f.Version, "二进制覆盖前快照")
	if err := s.st.Put(ctx, storagePath(f.SpaceID, f.StorageRef), r, size); err != nil {
		return nil, err
	}
	// 内容变更 → 重算内容哈希（从存储回读，保证与落盘一致）
	rc, _, err := s.st.Get(ctx, storagePath(f.SpaceID, f.StorageRef))
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	hasher := sha256.New()
	written, _ := io.Copy(hasher, rc)
	sum := hex.EncodeToString(hasher.Sum(nil))
	if written != size {
		size = written
	}
	ts := now()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE files SET size=?, sha256=?, version=version+1, updated_at=? WHERE id=?`,
		size, sum, ts, id); err != nil {
		return nil, err
	}
	s.b.Publish(ctx, bus.Event{Topic: "file.updated", Key: id, Data: map[string]any{"kind": "file", "size": size, "version": f.Version + 1}})
	_, _ = s.aud.Append(ctx, ownerID, "file.update", id, map[string]any{"name": f.Name, "size": size, "version": f.Version + 1})
	nf, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.IndexFile(ctx, nf); err != nil {
		s.aud.Append(ctx, ownerID, "index.failed", id, map[string]any{"name": f.Name, "err": err.Error()})
	}
	return nf, nil
}
```

依赖：`s.st`（storage.Backend）、`SaveVersion`（版本快照，主系统 P1-4 已有）、`IndexFile`（索引重建，你方已有）。

---

## 缺口 3 · `NotifyStore` 缺 `AddUser`（Org 成员变更通知）

主系统 `server/internal/service/notify.go:41-74` 全文：

```go
// AddUser 向指定用户写入一条通知（payload 支持 title/message/link/extra 结构化字段）。
// 多用户协作（评论/@/订阅）通知按目标 user_id 落库；单用户阶段 userID 传 owner 即可。
func (s *NotifyStore) AddUser(ctx context.Context, userID, ntype string, payload map[string]any) error {
	id := uuid.NewString()
	title, _ := payload["title"].(string)
	if title == "" {
		title = ntype
	}
	msg, _ := payload["message"].(string)
	link, _ := payload["link"].(string)
	extra := map[string]any{}
	if v, ok := payload["extra"].(map[string]any); ok {
		extra = v
	}
	raw, _ := json.Marshal(extra)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (id, user_id, type, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, userID, ntype, raw, time.Now().Unix())
	if err != nil {
		return err
	}
	// 冗余字段落 payload（除 extra 外平铺），供列表直接读出；保留完整结构便于扩展。
	flat := map[string]any{}
	_ = json.Unmarshal(raw, &flat)
	if title != "" {
		flat["title"] = title
	}
	if msg != "" {
		flat["message"] = msg
	}
	if link != "" {
		flat["link"] = link
	}
	flat2, _ := json.Marshal(flat)
	_, _ = s.db.ExecContext(ctx, `UPDATE notifications SET payload=? WHERE id=?`, string(flat2), id)
	return nil
}
```

依赖：`notifications` 表（id/user_id/type/payload/created_at）。

---

## 缺口 4 · handler 侧共享辅助（spaceRole / quota）

### 4a. `spaceRole`（主系统 `handler/spaces.go:24-41` 全文）

```go
func (a *API) spaceRole(ctx context.Context, uid, spaceID string) (string, bool) {
	if uid == "" || spaceID == "" {
		return "", false
	}
	var owner string
	if err := a.db.QueryRowContext(ctx, `SELECT owner_id FROM spaces WHERE id=?`, spaceID).Scan(&owner); err != nil {
		return "", false
	}
	if owner == uid {
		return "owner", true
	}
	var role string
	if err := a.db.QueryRowContext(ctx,
		`SELECT role FROM space_members WHERE space_id=? AND user_id=?`, spaceID, uid).Scan(&role); err == nil {
		return role, true
	}
	return "", false
}
```

依赖：`space_members` 表（space_id/user_id/role）——你方 Org 模块建表时若已含该表则直接可用；未含则补建（主系统建表见 repo/schema.go）。

### 4b. quota 体系（主系统 `handler/quota.go` 关键段）

```go
type quotaKind int
const (
	quotaRounds   quotaKind = iota // AI 对话轮数
	quotaSearch                    // 检索次数
	quotaUploadMB                  // 上传累计 MB
)

// quotaLimits 返回当前用户配额上限与是否 pro 档
func (a *API) quotaLimits(uid string) (rounds, searches, uploadMB int64, pro bool) {
	if a.quotaPro(uid) {
		return int64(a.cfg.GetInt("quota.rounds_pro")), int64(a.cfg.GetInt("quota.search_pro")), int64(a.cfg.GetInt("quota.upload_mb_pro")), true
	}
	return int64(a.cfg.GetInt("quota.rounds_free")), int64(a.cfg.GetInt("quota.search_free")), int64(a.cfg.GetInt("quota.upload_mb_free")), false
}

// quotaPro 判定用户是否享受 pro 档配额：owner/admin 或全局 PRO license 已激活。
func (a *API) quotaPro(uid string) bool {
	if uid == a.homeOwnerID() || a.isAdminByID(uid) {
		return true
	}
	return a.licenseEdition() == "pro"
}

// quotaUse 使用一次配额：返回 (allowed, 剩余提示)。超限返回 false。
func (a *API) quotaUse(uid string, kind quotaKind, amount int64) (bool, string) {
	rounds, searches, uploadMB, _ := a.quotaLimits(uid)
	now := todayStr()
	v, _ := quotaState.LoadOrStore(uid, &quotaDay{day: now})
	d := v.(*quotaDay)
	if d.day != now {
		d.day = now
		d.rounds, d.searches, d.uploadMB = 0, 0, 0
	}
	// 0 = 不限；按 kind 扣减，超限返回 false
	switch kind {
	case quotaRounds:
		if rounds > 0 && d.rounds >= rounds {
			return false, "今日 AI 配额已用尽"
		}
		d.rounds += amount
	case quotaSearch:
		if searches > 0 && d.searches >= searches {
			return false, "今日检索配额已用尽"
		}
		d.searches += amount
	case quotaUploadMB:
		if uploadMB > 0 && d.uploadMB+amount > uploadMB {
			return false, "上传配额已超限"
		}
		d.uploadMB += amount
	}
	return true, ""
}
```

依赖：`quota.rounds_free/search_free/upload_mb_free` 与 `*.pro` 设置键（settings 白名单）、`licenseEdition()`、`homeOwnerID()`、`isAdminByID()`。WebDAV 上传前调 `quotaUse(uid, quotaUploadMB, (size+1048575)/1048576)` 即可（主系统 `handler/files.go:334` 同款用法）。

---

*上游 · AiKmap 主系统 · 2026-09-20*
