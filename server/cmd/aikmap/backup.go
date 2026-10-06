package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// backupOnce 一致性备份：VACUUM INTO 快照（SQLite 在线一致性拷贝）→ storage 后端推远端。
// 本地保留 keepLocal 份、远端保留 keepRemote 份（按文件名时间戳清理）。
func backupOnce(ctx context.Context, db *sql.DB, st storage.Backend, dataDir string, keepLocal, keepRemote int) error {
	ts := time.Now().Format("20060102-150405")
	localDir := filepath.Join(dataDir, "backup")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}
	snap := filepath.Join(localDir, "aiklog-"+ts+".sqlite")
	// VACUUM INTO 需要字符串字面量（不支持参数绑定）；路径来自本机 dataDir 配置，转义单引号防注入
	esc := strings.ReplaceAll(snap, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+esc+"'"); err != nil {
		return err
	}
	fi, err := os.Stat(snap)
	if err != nil {
		return err
	}
	f, err := os.Open(snap)
	if err != nil {
		return err
	}
	remoteKey := "backup/" + filepath.Base(snap)
	if err := st.Put(ctx, remoteKey, f, fi.Size()); err != nil {
		f.Close()
		return err
	}
	f.Close()
	log.Printf("backup: %s -> %s (%d bytes)", snap, remoteKey, fi.Size())

	// 本地保留 keepLocal 份
	prune(localDir, "aiklog-*.sqlite", keepLocal)
	// 远端保留 keepRemote 份（按对象名时间戳清理，backup/aiklog-YYYYMMDD-HHMMSS.sqlite）
	pruneRemote(ctx, st, "backup/", keepRemote)
	return nil
}

// pruneRemote 按对象名（含时间戳）保留最近 keep 个 backup/ 对象，其余删除。
// 对象名形如 backup/aiklog-20060102-150405.sqlite，按字典序即时间序。
func pruneRemote(ctx context.Context, st storage.Backend, prefix string, keep int) {
	metas, err := st.List(ctx, prefix)
	if err != nil {
		log.Printf("backup remote prune: list failed: %v", err)
		return
	}
	keys := make([]string, 0, len(metas))
	for _, m := range metas {
		base := m.Key
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		if strings.HasPrefix(base, "aiklog-") && strings.HasSuffix(base, ".sqlite") {
			keys = append(keys, m.Key)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys))) // 新→旧
	if len(keys) <= keep {
		return
	}
	for _, k := range keys[keep:] {
		if err := st.Delete(ctx, k); err != nil {
			log.Printf("backup remote prune: delete %s: %v", k, err)
		}
	}
}

// prune 按修改时间保留最近 keep 个匹配 glob 的文件，其余删除。
func prune(dir, pattern string, keep int) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return
	}
	sort.Slice(matches, func(i, j int) bool {
		a, _ := os.Stat(matches[i])
		b, _ := os.Stat(matches[j])
		return a.ModTime().After(b.ModTime())
	})
	if len(matches) <= keep {
		return
	}
	for _, p := range matches[keep:] {
		if err := os.Remove(p); err != nil {
			log.Printf("backup prune: %v", err)
		}
	}
}

// backupDue 判断当前时刻是否到达每日备份调度时刻（HH:MM，容错前后 1 分钟窗口）。
// 非法格式返回 false（保持不触发，避免误备份）。
func backupDue(schedule string) bool {
	hhmm := strings.TrimSpace(schedule)
	if hhmm == "" {
		return false
	}
	var h, m int
	if _, err := fmt.Sscanf(hhmm, "%d:%d", &h, &m); err != nil {
		return false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return false
	}
	now := time.Now()
	target := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	delta := now.Sub(target)
	return delta >= -time.Minute && delta < time.Minute // 到点前后 1 分钟窗口内触发
}
