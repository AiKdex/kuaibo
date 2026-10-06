// im_voice_test.go IM 语音归档（family.8.2）回归。
//
// 覆盖点：
//  1. imFamilyVoiceArchive：转写文本 → life_event 文档（content_state 打标 node_type/fields{occurred_at,stage,preview,source:im_voice}）；
//  2. 与 service.FamilyStore.Timeline 的聚合契约：归档后语音记录出现在「一生时间轴」；
//  3. 重名兜底：同名标题连续归档不失败（时间戳后缀重试）。
//
// 下载/ASR/回传段依赖平台出网，不在单测范围（见 im_voice.go 注释）。
package handler

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/config"
	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/service"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// newVoiceArchiveAPI 建「db+cfg+files+family 都在位」的最小环境。
func newVoiceArchiveAPI(t *testing.T) *API {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "voice.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	b := bus.New()
	cfg := config.New(db, b)
	aud := service.New(db)
	ctx := context.Background()
	if err := service.EnsureSystem(ctx, db, aud); err != nil {
		t.Fatalf("ensure system: %v", err)
	}
	st, err := storage.NewLocal(filepath.Join(dir, "files"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	return &API{
		db: db, b: b, cfg: cfg, aud: aud,
		files:  service.NewFileStore(db, st, b, aud),
		family: service.NewFamilyStore(db, cfg, service.NewNotifyStore(db)),
	}
}

func TestImFamilyVoiceArchiveTimeline(t *testing.T) {
	a := newVoiceArchiveAPI(t)
	ctx := context.Background()
	transcript := "今天带小满去了动物园，她第一次看到大象，兴奋得不行，回家路上一直在学大象叫。"
	docID, err := a.imFamilyVoiceArchive(ctx, "", transcript)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if docID == "" {
		t.Fatal("archive returned empty docID")
	}
	// content_state 打标校验
	var state string
	if err := a.db.QueryRowContext(ctx, `SELECT content_state FROM files WHERE id=?`, docID).Scan(&state); err != nil {
		t.Fatalf("read content_state: %v", err)
	}
	for _, want := range []string{`"node_type":"life_event"`, `"source":"im_voice"`, `"stage":"other"`, `"occurred_at":`} {
		if !strings.Contains(state, want) {
			t.Errorf("content_state 缺 %s：%s", want, state)
		}
	}
	// 正文含转写全文（正文在 storage 后端，走 FileStore.Content 读）
	rc, _, err := a.files.Content(ctx, docID)
	if err != nil {
		t.Fatalf("read content: %v", err)
	}
	var sb strings.Builder
	if _, err := io.Copy(&sb, rc); err != nil {
		t.Fatalf("copy content: %v", err)
	}
	_ = rc.Close()
	if !strings.Contains(sb.String(), transcript) {
		t.Errorf("正文缺转写全文：%s", sb.String())
	}
	// 家族时间轴聚合
	fam := service.NewFamilyStore(a.db, a.cfg, service.NewNotifyStore(a.db))
	items, err := fam.Timeline(ctx, a.homeOwnerID())
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	hit := false
	for _, it := range items {
		if it.FileID == docID {
			hit = true
		}
	}
	if !hit {
		t.Errorf("归档文档未出现在一生时间轴（items=%d）", len(items))
	}
}

func TestImFamilyVoiceArchiveDuplicateTitle(t *testing.T) {
	a := newVoiceArchiveAPI(t)
	ctx := context.Background()
	id1, err := a.imFamilyVoiceArchive(ctx, "", "记一条今天的事")
	if err != nil {
		t.Fatalf("archive 1: %v", err)
	}
	id2, err := a.imFamilyVoiceArchive(ctx, "", "记一条今天的事")
	if err != nil {
		t.Fatalf("archive 2（重名应走时间戳后缀兜底）: %v", err)
	}
	if id1 == id2 {
		t.Errorf("重名归档应产生两个文档，实际同 ID %s", id1)
	}
}
