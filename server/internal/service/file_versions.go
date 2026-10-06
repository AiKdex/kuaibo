package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/google/uuid"
)

// ---- B5 写作增强：文章版本历史与回滚 ----
//
// 文本编辑（UpdateContent）与二进制覆盖（ReplaceContentBinary）在**覆盖写之前**，
// 把「即将被覆盖的旧内容」快照到 storage 的 versions/<file_id>/v<n>，并在 file_versions 登记元数据。
//
// 硬边界（三条，都有理由）：
//  1. SaveVersion 必须在 Put 之前调用，且**只读主存储不写**——版本历史绝不改正文。
//  2. 快照失败**不阻断**主流程：版本历史是增值能力，不能因为它丢了用户这一次保存。
//  3. 恢复**不回退版本号**（历史只增不减）：以历史版本为内容产生一个**新版本**，
//     于是「恢复」这个动作本身也可被再次恢复（可撤销、可审计），版本序列保持线性。
//     这与 B3 收录的「只 append 不 mutate」是同一套哲学。

// FileVersion 版本记录（只含元数据；正文在 storage 的 versions/ 下）。
type FileVersion struct {
	Version   int    `json:"version"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256,omitempty"`
	Note      string `json:"note,omitempty"`
	CreatedAt int64  `json:"created_at"`
	CreatedBy string `json:"created_by,omitempty"`
}

// versionKey 版本快照存储键（同一 storage 后端；按 file_id 分子目录防同目录膨胀）。
func versionKey(spaceID, fileID string, version int) string {
	return storagePath(spaceID, fmt.Sprintf("versions/%s/v%d", fileID, version))
}

// SaveVersion 把「当前磁盘内容」存为 v{version} 快照。**必须在覆盖写之前调用**。
// 使用 INSERT OR IGNORE：同一版本号重复快照是幂等操作（已有登记则保留原记录，
// 磁盘快照可能被重写但字节一致），不会因主键/唯一约束冲突而报错。
func (s *FileStore) SaveVersion(ctx context.Context, ownerID, id string, version int, note string) error {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return err
	}
	rc, _, err := s.st.Get(ctx, storagePath(f.SpaceID, f.StorageRef))
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := s.st.Put(ctx, versionKey(f.SpaceID, id, version), rc, f.Size); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO file_versions (id, file_id, version, sha256, size, note, created_at, created_by)
		 VALUES (?,?,?,?,?,?,?,?)`,
		uuid.NewString(), id, version, f.SHA256, f.Size, note, now(), ownerID)
	return err
}

// Versions 版本列表（倒序：新 → 旧）。无版本时返回空切片而非 nil（前端可直接 .length）。
func (s *FileStore) Versions(ctx context.Context, id string) ([]FileVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT version, sha256, size, note, created_at, created_by FROM file_versions
		 WHERE file_id=? ORDER BY version DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FileVersion{}
	for rows.Next() {
		var v FileVersion
		var note, by sql.NullString
		if err := rows.Scan(&v.Version, &v.SHA256, &v.Size, &note, &v.CreatedAt, &by); err != nil {
			return nil, err
		}
		v.Note = note.String
		v.CreatedBy = by.String
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// VersionContent 读取指定历史版本正文（只读）。返回 (*File) 供 handler 取 mime/name 做下载头。
func (s *FileStore) VersionContent(ctx context.Context, id string, version int) (io.ReadCloser, *File, error) {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if !s.hasVersion(ctx, id, version) {
		return nil, nil, errors.New("version not found")
	}
	rc, _, err := s.st.Get(ctx, versionKey(f.SpaceID, id, version))
	if err != nil {
		return nil, nil, err
	}
	return rc, f, nil
}

// RestoreVersion 以历史版本 v{version} 为内容产生**新版本**（version+1），不删任何历史。
// 步骤：①当前内容先入历史（让「恢复」本身可撤销）→ ②快照写回主存储 →
// ③从主存储回读重算 size+sha256（不信任旧元数据，保证与落盘字节一致）→
// ④files.version 自增到 max(file_versions.version)+1 → ⑤新版本落快照并登记 + 事件 + 审计 + 重索引。
func (s *FileStore) RestoreVersion(ctx context.Context, ownerID, id string, version int) (*File, error) {
	f, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.hasVersion(ctx, id, version) {
		return nil, errors.New("version not found")
	}
	// ① 当前内容入历史（若尚无同号记录）——恢复动作自身也可被再次恢复
	if !s.hasVersion(ctx, id, f.Version) {
		if err := s.SaveVersion(ctx, ownerID, id, f.Version, "恢复前快照"); err != nil {
			return nil, err
		}
	}
	// 快照大小：优先用版本登记值（快照当时的真实字节数），异常时回落当前大小
	var vsize int64
	_ = s.db.QueryRowContext(ctx,
		`SELECT size FROM file_versions WHERE file_id=? AND version=?`, id, version).Scan(&vsize)
	if vsize <= 0 {
		vsize = f.Size
	}
	// ② 快照写回主存储
	rc, _, err := s.st.Get(ctx, versionKey(f.SpaceID, id, version))
	if err != nil {
		return nil, err
	}
	if err := s.st.Put(ctx, storagePath(f.SpaceID, f.StorageRef), rc, vsize); err != nil {
		rc.Close()
		return nil, err
	}
	rc.Close()
	// ③ 回读重算哈希与大小
	rc2, _, err := s.st.Get(ctx, storagePath(f.SpaceID, f.StorageRef))
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	n, cerr := io.Copy(h, rc2)
	rc2.Close()
	if cerr != nil {
		return nil, cerr
	}
	sum := hex.EncodeToString(h.Sum(nil))
	// ④ 新版本号：max(file_versions)+1，且不低于 f.Version+1（单调递增，防历史缺号）
	var nv int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version),0)+1 FROM file_versions WHERE file_id=?`, id).Scan(&nv)
	if nv <= f.Version {
		nv = f.Version + 1
	}
	ts := now()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE files SET version=?, size=?, sha256=?, updated_at=? WHERE id=?`,
		nv, n, sum, ts, id); err != nil {
		return nil, err
	}
	// ⑤ 落一份新版本快照，再登记记录 —— 保证「列表里出现的版本一定下载得到」。
	// 上游只写 DB 行不落盘，于是列表里会出现一条点下载就 404 的「幽灵版本」；这里补上。
	// 快照写失败则**不登记**该行：宁可不显示，也不显示一个点不开的版本。
	snapOK := false
	if rc3, _, gerr := s.st.Get(ctx, storagePath(f.SpaceID, f.StorageRef)); gerr == nil {
		perr := s.st.Put(ctx, versionKey(f.SpaceID, id, nv), rc3, n)
		rc3.Close()
		snapOK = perr == nil
	}
	if snapOK {
		_, _ = s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO file_versions (id, file_id, version, sha256, size, note, created_at, created_by)
			 VALUES (?,?,?,?,?,?,?,?)`,
			uuid.NewString(), id, nv, sum, n, fmt.Sprintf("恢复自 v%d", version), ts, ownerID)
	}

	s.b.Publish(ctx, bus.Event{Topic: "file.updated", Key: id, Data: map[string]any{"kind": "file", "version": nv}})
	_, _ = s.aud.Append(ctx, ownerID, "file.restore", id, map[string]any{"from": version, "to": nv})

	nf, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 内容变了 → 旧 chunks/向量/摘要失效，同步重建
	if err := s.IndexFile(ctx, nf); err != nil {
		s.aud.Append(ctx, ownerID, "index.failed", id, map[string]any{"name": f.Name, "err": err.Error()})
	}
	return nf, nil
}

// hasVersion 版本是否已登记。
func (s *FileStore) hasVersion(ctx context.Context, id string, version int) bool {
	var n int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_versions WHERE file_id=? AND version=?`, id, version).Scan(&n)
	return n > 0
}
