// sourcepack_test.go 采集源包（SPEC-SP-001）解析/校验/导入回归。
//
// 覆盖点：
//  1. zip 解析：manifest + sources/*.json 外置覆盖内嵌；
//  2. 拒绝路径：kind 非 source-pack、target 不含本壳、compliance 缺、schema 越界、
//     可执行物、模板缺 urls/fields、SSRF 内网地址；
//  3. ImportSources 事务：首次 created，重装 updated 且保留用户改过的 enabled；
//  4. DeleteSourcesByPack 只删该包的源，不动自建源。
package service

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/repo"
)

// buildPack 构造源包 zip（entries: 文件名 → 内容）。
func buildPack(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

const validManifest = `{
  "id": "source-pack-test",
  "name": "测试源包",
  "version": "1.0.0",
  "min_core_version": "1.0.0",
  "kind": "source-pack",
  "target": ["aiklog", "aikmap"],
  "compliance": {"target_site": "示例招聘网", "usage": "岗位聚合展示", "robots_allowed": true, "fetch_frequency": "6h"},
  "sources": [
    {"id":"job-bengbu","city":"bengbu","name":"蚌埠站","kind":"job","channel_type":"job_position","fetch_mode":"http","url":"https://example.com/list","template":{"list":{"urls":["https://example.com/list?page=1"],"item_selector":"div.item","fields":{"title":"h3 a","link":"h3 a@href"}}}}
  ]
}`

const externalSource = `{"id":"job-bengbu","city":"bengbu","name":"外置覆盖版","kind":"job","url":"https://example.com/v2","template":{"list":{"urls":["https://example.com/v2"],"item_selector":"li.row","fields":{"title":".t","link":"a@href"}}}}`

func newSourceDB(t *testing.T) *SourceStore {
	t.Helper()
	db, err := repo.Open(filepath.Join(t.TempDir(), "pack.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewSourceStore(db)
}

func TestParseSourcePackZipExternalOverride(t *testing.T) {
	raw := buildPack(t, map[string]string{
		"manifest.json":          validManifest,
		"sources/job-bengbu.json": externalSource,
		"README.md":              "说明",
	})
	p, err := ParseSourcePackZip(raw)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(p.Manifest.Sources) != 1 {
		t.Fatalf("外置条目应覆盖内嵌同名 id，实际 %d 条", len(p.Manifest.Sources))
	}
	if p.Manifest.Sources[0].Name != "外置覆盖版" {
		t.Errorf("外置应覆盖内嵌：name=%s", p.Manifest.Sources[0].Name)
	}
	if len(p.Files) != 3 {
		t.Errorf("包内文件清单应为 3，实际 %v", p.Files)
	}
}

func TestValidateSourcePackRejects(t *testing.T) {
	cases := []struct {
		name    string
		files   map[string]string
		wantCode string
	}{
		{"kind 非 source-pack", map[string]string{"manifest.json": strings.Replace(validManifest, `"kind": "source-pack"`, `"kind": "plugin"`, 1)}, "SOURCE_PACK_BAD_KIND"},
		{"target 不含本壳", map[string]string{"manifest.json": strings.Replace(validManifest, `["aiklog", "aikmap"]`, `["aikbox"]`, 1)}, "MARKET_TARGET_MISMATCH"},
		{"compliance 缺失", map[string]string{"manifest.json": strings.Replace(validManifest, `"compliance": {"target_site": "示例招聘网", "usage": "岗位聚合展示", "robots_allowed": true, "fetch_frequency": "6h"},`, "", 1)}, "SOURCE_PACK_COMPLIANCE_REQUIRED"},
		{"schema 越下界", map[string]string{"manifest.json": strings.Replace(validManifest, `"min_core_version": "1.0.0",`, `"min_core_version": "1.0.0", "min_schema": 2,`, 1)}, "MARKET_SCHEMA_MISMATCH"},
		{"宿主版本过低", map[string]string{"manifest.json": strings.Replace(validManifest, `"min_core_version": "1.0.0"`, `"min_core_version": "9.9.0"`, 1)}, "PLUGIN_CORE_TOO_OLD"},
		{"含可执行物", map[string]string{"manifest.json": validManifest, "install.sh": "#!/bin/sh"}, "SOURCE_PACK_EXECUTABLE_DENIED"},
		{"模板缺 urls", map[string]string{"manifest.json": strings.Replace(validManifest, `"urls":["https://example.com/list?page=1"]`, `"urls":[]`, 1)}, "SOURCE_PACK_BAD_TEMPLATE"},
		{"模板缺 link", map[string]string{"manifest.json": strings.Replace(validManifest, `,"link":"h3 a@href"`, ``, 1)}, "SOURCE_PACK_BAD_TEMPLATE"},
		{"job 缺 city", map[string]string{"manifest.json": strings.Replace(validManifest, `"city":"bengbu",`, ``, 1)}, "SOURCE_PACK_BAD_ENTRY"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := ParseSourcePackZip(buildPack(t, c.files))
			if err != nil {
				// 可执行物在解析阶段即拒
				var pe *SourcePackErr
				if ok := asPackErr(err, &pe); ok && pe.Code == c.wantCode {
					return
				}
				t.Fatalf("解析阶段意外失败：%v", err)
			}
			err = ValidateSourcePack(p, ValidateSourcePackOptions{ShellID: "aiklog", HostVersion: "1.0.0", SkipSSRF: true})
			if err == nil {
				t.Fatalf("应被拒绝（期望 %s），但校验通过", c.wantCode)
			}
			var pe *SourcePackErr
			if !asPackErr(err, &pe) {
				t.Fatalf("错误类型不是 SourcePackErr：%v", err)
			}
			if pe.Code != c.wantCode {
				t.Errorf("错误码：期望 %s，实际 %s（%s）", c.wantCode, pe.Code, pe.Msg)
			}
		})
	}
}

// asPackErr 提取 SourcePackErr（errors.As 的测试内简写）。
func asPackErr(err error, out **SourcePackErr) bool {
	if e, ok := err.(*SourcePackErr); ok {
		*out = e
		return true
	}
	return false
}

func TestValidateSourcePackSSRF(t *testing.T) {
	m := `{"id":"sp","name":"n","version":"1.0.0","kind":"source-pack","compliance":{"target_site":"x","usage":"y"},"sources":[{"id":"s1","city":"c","name":"内网源","url":"http://127.0.0.1:8080/a","template":{"list":{"urls":["http://127.0.0.1:8080/a"],"item_selector":"div","fields":{"title":"a","link":"b"}}}}]}`
	p, err := ParseSourcePackZip(buildPack(t, map[string]string{"manifest.json": m}))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	err = ValidateSourcePack(p, ValidateSourcePackOptions{ShellID: "aiklog", HostVersion: "1.0.0"})
	if err == nil {
		t.Fatal("内网 URL 应被 SSRF 拒绝")
	}
	var pe *SourcePackErr
	if !asPackErr(err, &pe) || pe.Code != "SOURCE_PACK_SSRF_BLOCKED" {
		t.Errorf("期望 SOURCE_PACK_SSRF_BLOCKED，实际 %v", err)
	}
}

func TestSourcePackImportUpsertAndUninstall(t *testing.T) {
	store := newSourceDB(t)
	ctx := context.Background()
	p, err := ParseSourcePackZip(buildPack(t, map[string]string{"manifest.json": validManifest}))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if err := ValidateSourcePack(p, ValidateSourcePackOptions{ShellID: "aiklog", HostVersion: "1.0.0", SkipSSRF: true}); err != nil {
		t.Fatalf("合法包被拒：%v", err)
	}
	// 自建源（pack_id 空）——卸载源包时不应被牵连
	if _, err := store.CreateSource(ctx, &Source{ID: "manual-1", City: "hefei", Name: "自建源", URL: "https://example.com/m", Enabled: true}); err != nil {
		t.Fatalf("创建自建源失败：%v", err)
	}
	items := SourcePackToSources(p.Manifest)
	created, updated, err := store.ImportSources(ctx, items)
	if err != nil {
		t.Fatalf("导入失败：%v", err)
	}
	if created != 1 || updated != 0 {
		t.Errorf("首次导入：期望 created=1 updated=0，实际 %d/%d", created, updated)
	}
	// 用户手动停用
	if _, err := store.UpdateSource(ctx, "job-bengbu", map[string]any{"enabled": false}); err != nil {
		t.Fatalf("停用失败：%v", err)
	}
	// 重装：应覆盖配置但保留 enabled=false
	items2 := SourcePackToSources(p.Manifest)
	items2[0].Name = "改过的名字"
	created2, updated2, err := store.ImportSources(ctx, items2)
	if err != nil {
		t.Fatalf("重装失败：%v", err)
	}
	if created2 != 0 || updated2 != 1 {
		t.Errorf("重装：期望 created=0 updated=1，实际 %d/%d", created2, updated2)
	}
	got, err := store.GetSource(ctx, "job-bengbu")
	if err != nil {
		t.Fatalf("回读失败：%v", err)
	}
	if got.Name != "改过的名字" {
		t.Errorf("配置应被覆盖：name=%s", got.Name)
	}
	if got.Enabled {
		t.Error("重装应保留用户手动停用的 enabled=false")
	}
	if got.PackID != "source-pack-test" {
		t.Errorf("pack_id 应为 source-pack-test，实际 %q", got.PackID)
	}
	// 卸载：只删该包的源
	n, err := store.DeleteSourcesByPack(ctx, "source-pack-test")
	if err != nil {
		t.Fatalf("卸载失败：%v", err)
	}
	if n != 1 {
		t.Errorf("卸载应删 1 条，实际 %d", n)
	}
	if _, err := store.GetSource(ctx, "manual-1"); err == sql.ErrNoRows {
		t.Error("自建源不应被源包卸载牵连")
	}
}
