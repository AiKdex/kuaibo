package service

import (
	"context"
	"io"
	"strings"
	"testing"
)

// B5 版本历史：快照 → 列表 → 读旧版 → 恢复 → 可逆（恢复本身也能被恢复）。
func TestFileVersionsSnapshotListRestore(t *testing.T) {
	fs, ctx, owner, space := newTestFS(t)

	f, err := fs.Upload(ctx, owner, space, "", "a.md", "text/markdown", strings.NewReader("v1"), -1, DefaultSiteID)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if f.Version != 1 {
		t.Fatalf("初始版本应为 1，实际 %d", f.Version)
	}

	// 编辑前无任何版本
	if vs, err := fs.Versions(ctx, f.ID); err != nil || len(vs) != 0 {
		t.Fatalf("初始版本列表应为空，得到 %d 条 err=%v", len(vs), err)
	}

	// 第一次编辑：应给「旧内容 v1」留快照
	if _, err := fs.UpdateContent(ctx, owner, f.ID, "v2"); err != nil {
		t.Fatalf("update1: %v", err)
	}
	vs, err := fs.Versions(ctx, f.ID)
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	if len(vs) != 1 || vs[0].Version != 1 {
		t.Fatalf("期望 1 条且版本号 1，得到 %+v", vs)
	}

	// 第二次编辑 → 快照 v2
	if _, err := fs.UpdateContent(ctx, owner, f.ID, "v3"); err != nil {
		t.Fatalf("update2: %v", err)
	}
	vs, _ = fs.Versions(ctx, f.ID)
	if len(vs) != 2 || vs[0].Version != 2 || vs[1].Version != 1 {
		t.Fatalf("期望倒序 [2 1]，得到 %+v", vs)
	}

	// 读旧版正文
	rc, _, err := fs.VersionContent(ctx, f.ID, 1)
	if err != nil {
		t.Fatalf("version content: %v", err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if string(body) != "v1" {
		t.Fatalf("v1 正文应为 v1，得到 %q", string(body))
	}

	// 恢复 v1
	nf, err := fs.RestoreVersion(ctx, owner, f.ID, 1)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	cur := readAll(t, fs, ctx, f.ID)
	if cur != "v1" {
		t.Fatalf("恢复后正文应为 v1，得到 %q", cur)
	}
	// 恢复 = 产生新版本（不回退版本号），且不低于原版本+1
	if nf.Version <= 3 {
		t.Fatalf("恢复后版本号应递增（>3），得到 %d", nf.Version)
	}
	vs, _ = fs.Versions(ctx, f.ID)
	if len(vs) != 4 || vs[0].Version != nf.Version {
		t.Fatalf("恢复后应有 4 条快照且首条为 %d，得到 %+v", nf.Version, vs)
	}

	// 回归：「列出来的版本一定下载得到」——恢复产生的新版本也必须真有快照
	// （上游只写 DB 行不落盘，会出现点下载即 404 的幽灵版本）
	for _, v := range vs {
		rc, _, err := fs.VersionContent(ctx, f.ID, v.Version)
		if err != nil {
			t.Fatalf("版本 v%d 已登记却读不到快照: %v", v.Version, err)
		}
		rc.Close()
	}

	// 可逆性：恢复前的内容（v3）也被快照了，故可以恢复回去
	if _, err := fs.RestoreVersion(ctx, owner, f.ID, 3); err != nil {
		t.Fatalf("re-restore: %v", err)
	}
	if got := readAll(t, fs, ctx, f.ID); got != "v3" {
		t.Fatalf("再次恢复 v3 后正文应为 v3，得到 %q", got)
	}
}

// 不存在的版本：列表为空、读取报错、恢复报错（不静默成功）。
func TestFileVersionsUnknownVersion(t *testing.T) {
	fs, ctx, owner, space := newTestFS(t)
	f, err := fs.Upload(ctx, owner, space, "", "b.md", "text/markdown", strings.NewReader("x"), -1, DefaultSiteID)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, _, err := fs.VersionContent(ctx, f.ID, 99); err == nil {
		t.Fatal("读取不存在版本应报错")
	}
	if _, err := fs.RestoreVersion(ctx, owner, f.ID, 99); err == nil {
		t.Fatal("恢复不存在版本应报错")
	}
}

// 二进制覆盖也要留快照（ReplaceContentBinary 路径）。
func TestReplaceBinarySnapshotsVersion(t *testing.T) {
	fs, ctx, owner, space := newTestFS(t)
	f, err := fs.Upload(ctx, owner, space, "", "c.bin", "application/octet-stream", strings.NewReader("bin-1"), -1, DefaultSiteID)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := fs.ReplaceContentBinary(ctx, owner, f.ID, strings.NewReader("bin-2"), int64(len("bin-2"))); err != nil {
		t.Fatalf("replace: %v", err)
	}
	vs, err := fs.Versions(ctx, f.ID)
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	if len(vs) != 1 {
		t.Fatalf("二进制覆盖后应有 1 条快照，得到 %d", len(vs))
	}
	rc, _, err := fs.VersionContent(ctx, f.ID, vs[0].Version)
	if err != nil {
		t.Fatalf("version content: %v", err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if string(body) != "bin-1" {
		t.Fatalf("快照内容应为 bin-1，得到 %q", string(body))
	}
}

func readAll(t *testing.T, fs *FileStore, ctx context.Context, id string) string {
	t.Helper()
	rc, _, err := fs.Content(ctx, id)
	if err != nil {
		t.Fatalf("content: %v", err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	return string(b)
}
