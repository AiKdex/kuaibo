// derived.go 派生资产（缩略图等）的统计与批量清理（B16）。
//
// 为什么需要这一层：B15 的派生对象刻意**不建 files 行**（零污染公开列表/sitemap/配额/索引），
// 代价是生命周期只能显式管理。既有两条清理路径都依赖「源文件曾经走到过删除流程」：
//  1. FileStore.Purge 按前缀清（随源文件删除触发）；
//  2. handler 侧孤儿对账（file_media 行还在时才拿得到 thumbnail_ref）。
//
// 三类残留它们都覆盖不到：
//
//	a. 直接改库 / 迁移 / 回滚 DB 备份导致的孤儿对象（磁盘有、库里没有引用）；
//	b. 站长调小缩略图尺寸后，希望按新参数**重生成**的旧缓存；
//	c. 历史缺陷留下的脏对象（如 B15 之前 Purge 清不掉 file_media 行时的残留）。
//
// 故补一个「可观测 + 可一键清」的运维面：统计用于判断值不值得清，清完按需懒生成。
package service

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// DerivedStats 派生资产统计（运维面板展示；也是「清理前后」的判据）。
type DerivedStats struct {
	Objects     int            `json:"objects"`      // 对象总数
	Bytes       int64          `json:"bytes"`        // 占用字节
	Files       int            `json:"files"`        // 覆盖的源文件数（按 fileID 去重）
	Spaces      int            `json:"spaces"`       // 含派生对象的空间数
	Orphans     int            `json:"orphans"`      // 源文件已不存在的对象数
	OrphanBytes int64          `json:"orphan_bytes"` // 孤儿占用字节
	ByExt       map[string]int `json:"by_ext"`       // 按扩展名分布（.jpg / .mp4 ...）
	// Listable=false 表示后端不支持列举（远程对象存储可能如此），此时其余字段无意义。
	// 与 DeleteDerivedPrefix 同一口径：清不掉只是占磁盘，不应让运维接口整体报错。
	Listable bool `json:"listable"`
}

// ListDerived 列出前缀下全部派生对象（目录项已滤除）；第二返回值 = 后端是否可列举。
func (s *FileStore) ListDerived(ctx context.Context, prefix string) ([]storage.ObjectMeta, bool) {
	objs, err := s.st.List(ctx, prefix)
	if err != nil {
		return nil, false
	}
	out := make([]storage.ObjectMeta, 0, len(objs))
	for _, o := range objs {
		if !o.IsDir {
			out = append(out, o)
		}
	}
	return out, true
}

// DerivedFileIDOfKey 从派生对象 key 解析源文件 id：spaces/{sp}/.derived/{fid}/{name}。
// 与 DerivedPrefixOfKey 互补：那个回答「要删哪个目录」，这个回答「属于哪个文件」。
func DerivedFileIDOfKey(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) < 5 || parts[0] != "spaces" || parts[2] != ".derived" {
		return ""
	}
	return parts[3]
}

// derivedSpaceIDs 列出可能拥有派生对象的空间 id。
//
// 双源合并的理由：`spaces` 覆盖「空间还在但文件已清空」（否则会漏统计），
// `files.space_id` 覆盖 spaces 表缺失/未登记空间的历史数据。任一查询失败都容忍 ——
// 统计接口不该因为一张可选表而整体 500（老库/精简部署可能没有 spaces 表）。
func (s *FileStore) derivedSpaceIDs(ctx context.Context) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	if s.db != nil {
		for _, q := range []string{
			`SELECT DISTINCT space_id FROM files WHERE space_id IS NOT NULL AND space_id <> ''`,
			`SELECT id FROM spaces`,
		} {
			rows, err := s.db.QueryContext(ctx, q)
			if err != nil {
				continue
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err == nil {
					add(id)
				}
			}
			rows.Close()
		}
	}
	add(SystemHomeSpaceID) // 空库也给出确定结果
	return out
}

// DerivedStats 汇总全部空间的派生资产。
func (s *FileStore) DerivedStats(ctx context.Context) DerivedStats {
	st := DerivedStats{ByExt: map[string]int{}, Listable: true}
	type obj struct {
		fid string
		sz  int64
	}
	objs := make([]obj, 0, 16)
	for _, sp := range s.derivedSpaceIDs(ctx) {
		list, ok := s.ListDerived(ctx, DerivedSpacePrefix(sp))
		if !ok {
			return DerivedStats{ByExt: map[string]int{}, Listable: false}
		}
		if len(list) > 0 {
			st.Spaces++
		}
		for _, o := range list {
			st.Objects++
			st.Bytes += o.Size
			st.ByExt[strings.ToLower(path.Ext(o.Key))]++
			objs = append(objs, obj{DerivedFileIDOfKey(o.Key), o.Size})
		}
	}
	if len(objs) == 0 {
		return st
	}
	uniq := map[string]bool{}
	for _, o := range objs {
		if o.fid != "" {
			uniq[o.fid] = true
		}
	}
	st.Files = len(uniq)
	// 🔴 无 db 时**不做**孤儿判定：宁可漏报，也不可误报 —— 误报会让站长一键清掉正常缓存。
	if s.db != nil {
		alive := s.existingFileIDs(ctx, mapKeys(uniq))
		for _, o := range objs {
			if o.fid == "" || !alive[o.fid] {
				st.Orphans++
				st.OrphanBytes += o.sz
			}
		}
	}
	return st
}

// DeleteDerivedObject 删除单个派生对象（按完整 key）。
//
// 与 DeleteDerivedPrefix 的分工：那个删「某文件的整个派生目录」（缩略图 + 转码产物一起），
// 这个只删一个对象 —— 转码改参数后需要「写新产物 + 删旧产物」，
// 若用按前缀删会把同目录下的缩略图一起带走。
func (s *FileStore) DeleteDerivedObject(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return storage.ErrNotFound
	}
	return s.st.Delete(ctx, key)
}

// PurgeDerived 清理派生对象：fileID 非空时只清该文件，为空时清**所有**空间下的派生对象。
// 返回删除对象数与释放字节。用途：清孤儿，或改完尺寸参数后强制按新参数重生成。
//
// 为什么可以只凭 fileID 跨空间清：key 里 fid 固定在第 4 段，遍历各空间即可定位，
// 不必先查库 —— 源文件可能早就不存在了，而那正是本次要清的情形。
func (s *FileStore) PurgeDerived(ctx context.Context, fileID string) (int, int64) {
	fileID = strings.TrimSpace(fileID)
	n, freed := 0, int64(0)
	for _, sp := range s.derivedSpaceIDs(ctx) {
		prefix := DerivedSpacePrefix(sp)
		if fileID != "" {
			prefix = DerivedPrefix(sp, fileID)
		}
		list, ok := s.ListDerived(ctx, prefix)
		if !ok {
			continue // 该空间列举不可用：跳过，不中断整体清理
		}
		for _, o := range list {
			err := s.st.Delete(ctx, o.Key)
			if err == nil || errors.Is(err, storage.ErrNotFound) {
				n++
				freed += o.Size
			}
		}
	}
	return n, freed
}

// existingFileIDs 批量判定文件行是否存在（分块 IN，避开 SQLite 变量数上限）。
//
// 查询失败时把该批 id 一律视为「存在」：孤儿计数只是展示用，
// 一次查询抖动不该让站长看到「全是孤儿」而误清缓存。
func (s *FileStore) existingFileIDs(ctx context.Context, ids []string) map[string]bool {
	out := map[string]bool{}
	if s.db == nil || len(ids) == 0 {
		return out
	}
	const chunk = 400
	for i := 0; i < len(ids); i += chunk {
		end := i + chunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[i:end]
		args := make([]any, 0, len(part))
		for _, id := range part {
			args = append(args, id)
		}
		rows, err := s.db.QueryContext(ctx,
			`SELECT id FROM files WHERE id IN (`+placeholders(len(part))+`)`, args...)
		if err != nil {
			for _, id := range part {
				out[id] = true
			}
			continue
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				out[id] = true
			}
		}
		rows.Close()
	}
	return out
}

// mapKeys 取 map 的键（顺序无意义，调用方按集合用）。
func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// SweepOrphans 删除所有孤儿派生对象（源文件行已不存在的），返回删除数与释放字节。
//
// 与 DerivedStats 的孤儿判定口径**完全一致**（fid 为空或不在 files 表里即孤儿），
// 因此定时任务清掉的一定等于面板看到的 stats.orphans —— 站长在面板看到的孤儿数，兜底清理会收掉。
//
// 设计取舍（与 B16 同纪律）：
//   - 任一空间列举不可用（ok=false）就跳过该空间，不盲删 —— 避免「列举抖动」把正常缓存当孤儿误删
//     （B16 立过「宁可漏报不可误报」的规矩，误报会诱导站长清掉正常缓存）。
//   - 只删文件、不 rmdir（见 §7 #45）：.derived 下可能留空目录，不影响功能，统计目录数会虚高；
//     与 PurgeDerived 行为一致，收尾清理由运维面手动兜底。
//   - 无 db 时直接返回 (0,0,nil)：孤儿判定必须有库，宁可不动，绝不误删。
func (s *FileStore) SweepOrphans(ctx context.Context) (int, int64, error) {
	if s.db == nil {
		return 0, 0, nil
	}
	type obj struct {
		key string
		sz  int64
		fid string
	}
	objs := make([]obj, 0, 32)
	for _, sp := range s.derivedSpaceIDs(ctx) {
		list, ok := s.ListDerived(ctx, DerivedSpacePrefix(sp))
		if !ok {
			continue // 该空间列举不可用：跳过，不盲删
		}
		for _, o := range list {
			objs = append(objs, obj{key: o.Key, sz: o.Size, fid: DerivedFileIDOfKey(o.Key)})
		}
	}
	if len(objs) == 0 {
		return 0, 0, nil
	}
	uniq := map[string]bool{}
	for _, o := range objs {
		if o.fid != "" {
			uniq[o.fid] = true
		}
	}
	alive := s.existingFileIDs(ctx, mapKeys(uniq))
	n, freed := 0, int64(0)
	for _, o := range objs {
		if o.fid != "" && alive[o.fid] {
			continue // 源文件还在，保留
		}
		if err := s.st.Delete(ctx, o.key); err == nil || errors.Is(err, storage.ErrNotFound) {
			n++
			freed += o.sz
		}
	}
	return n, freed, nil
}
