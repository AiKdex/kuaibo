// transcode_test.go 视频转码（B17）单元测试。
//
// 覆盖点：
//  1. 纯函数（确定性、无环境依赖）：Signature 指纹稳定性、TranscodeTimeout 边界、isVideoMime、
//     progressRatio、parseProgressLine；
//  2. 管理器降级（nil stores → ErrTranscodeNoManager，Status/Cancel 不 panic）；
//  3. 拒绝非视频（ErrTranscodeNotVideo）、拒绝超长（ErrTranscodeTooLong）；
//  4. 真实内存 DB + 本地后端：触发 → 异步 done → 幂等（再次触发直接回 done）→ 落库态一致；
//  5. ResetStale 把重启残留的 queued 标 failed、不动 done 行。
//
// 集成链路（真 ffmpeg 转码 + 公开播放回落）由部署后 e2e 覆盖，本文件不依赖宿主机 ffmpeg。
package service

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// fakeCfgGetter 测试用配置桩：仅实现 cfgGetter 的 GetString（默认全空 → 走内置默认）。
type fakeCfgGetter struct{ m map[string]string }

func (c fakeCfgGetter) GetString(k string) string {
	if c.m == nil {
		return ""
	}
	return c.m[k]
}

// openTranscodeTestDB 打开内存 DB 并预置 files 表外键所需的 space/user 最小父行。
func openTranscodeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open :memory: db: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users(id, username, pass_hash, role, status, created_at, updated_at) VALUES('u1','u','x','owner','active',0,0)`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO spaces(id, owner_id, name, kind, created_at, updated_at) VALUES('sp1','u1','sp','home',0,0)`); err != nil {
		t.Fatalf("seed space: %v", err)
	}
	return db
}

// setupTranscodeManager 构造带真实内存 DB + 本地后端的转码管理器。
// ffmpegBin 指向测试可执行文件自身（仅让 FFmpegAvailable 在测试环境返回 true；实际转码由注入的假 runner 完成）。
func setupTranscodeManager(t *testing.T) (*TranscodeManager, *sql.DB, storage.Backend) {
	t.Helper()
	db := openTranscodeTestDB(t)
	st, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("new local backend: %v", err)
	}
	files := NewFileStore(db, st, nil, nil)
	media := NewMediaStore(db)
	m := NewTranscodeManager(files, media, fakeCfgGetter{})
	// 注入假 runner：写一个最小「MP4」到 dst，返回其大小（完全不走真 ffmpeg）。
	m.run = func(ctx context.Context, src, dst string, durationMS int64, opt TranscodeOptions, onProgress TranscodeProgress) (int64, error) {
		if onProgress != nil {
			onProgress(1, durationMS)
		}
		err := os.WriteFile(dst, []byte("fake-mp4-bytes"), 0o644)
		return int64(len("fake-mp4-bytes")), err
	}
	old := ffmpegBin
	ffmpegBin = os.Args[0] // 本测试可执行文件必然存在 → FFmpegAvailable 返回 true
	t.Cleanup(func() { ffmpegBin = old })
	return m, db, st
}

// seedVideoFile 写入一个视频文件（行 + 本地内容 + 可选媒体时长）。
func seedVideoFile(t *testing.T, db *sql.DB, st storage.Backend, id string, durationMS int64) {
	t.Helper()
	spaceID, ref := "sp1", "ref-"+id
	content := []byte("source-video-content")
	if err := st.Put(context.Background(), storagePath(spaceID, ref), bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("put source: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO files(id, space_id, owner_id, name, kind, mime, size, storage_backend, storage_ref, version, content_state, created_at, updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, spaceID, "u1", "v"+id+".mp4", "file", "video/mp4", int64(len(content)), "local", ref, 1,
		`{"visibility":"private","status":"published"}`, 0, 0); err != nil {
		t.Fatalf("insert file: %v", err)
	}
	if durationMS > 0 {
		if err := NewMediaStore(db).Upsert(context.Background(), &MediaInfo{FileID: id, DurationMS: durationMS, ProbeStatus: MediaOK}); err != nil {
			t.Fatalf("upsert media: %v", err)
		}
	}
}

// pollUntilDone 轮询转码状态直到终态（done/failed/canceled）或超时。
func pollUntilDone(t *testing.T, m *TranscodeManager, id string, timeout time.Duration) TranscodeJob {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		j := m.Status(context.Background(), id)
		if j.Status == TranscodeDone || j.Status == TranscodeFailed || j.Status == TranscodeCanceled {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	return m.Status(context.Background(), id)
}

func TestTranscodePure_SignatureStable(t *testing.T) {
	a := TranscodeOptions{MaxHeight: 720, CRF: 23, Preset: "medium", AudioKbps: 128, Concurrency: 4, MaxDuration: 9999}.Signature()
	b := TranscodeOptions{MaxHeight: 720, CRF: 23, Preset: "medium", AudioKbps: 128, Concurrency: 1, MaxDuration: 0}.Signature()
	if a != b {
		t.Fatalf("并发/时长不应进指纹: %q vs %q", a, b)
	}
	c := TranscodeOptions{MaxHeight: 1080, CRF: 23, Preset: "medium", AudioKbps: 128}.Signature()
	if c == a {
		t.Fatalf("改高度应改变指纹: %q vs %q", c, a)
	}
	d := TranscodeOptions{MaxHeight: 99999, CRF: -1, Preset: "MEDIUM", AudioKbps: 0}.Clamp()
	if d.MaxHeight != TranscodeMaxHeightMax || d.CRF != TranscodeCRFMin || d.AudioKbps != TranscodeAudioKbpsMin || d.Preset != "medium" {
		t.Fatalf("clamp should bound values, got %+v", d)
	}
}

func TestTranscodePure_Timeout(t *testing.T) {
	if got := TranscodeTimeout(0); got.Seconds() != 600 {
		t.Fatalf("未知时长应兜底 600s, got %v", got)
	}
	if got := TranscodeTimeout(10000); got.Seconds() != 180 {
		t.Fatalf("10s 算出 80s < 下限 180s 应夹到 180s, got %v", got)
	}
	if got := TranscodeTimeout(90000); got.Seconds() != 240 {
		t.Fatalf("90s → 2*90+60=240s, got %v", got)
	}
	if got := TranscodeTimeout(3600000); got.Seconds() != 3600 {
		t.Fatalf("1h 算出 7260s > 上限 3600s 应夹到 3600s, got %v", got)
	}
}

func TestTranscodePure_IsVideoMime(t *testing.T) {
	if !isVideoMime("video/mp4") {
		t.Fatal("video/mp4 应为视频")
	}
	if !isVideoMime("video/quicktime; charset=utf-8") {
		t.Fatal("应剥离 mime 参数")
	}
	if isVideoMime("image/png") || isVideoMime("audio/mp3") || isVideoMime("") {
		t.Fatal("非 video 应为 false")
	}
}

func TestTranscodePure_ProgressRatio(t *testing.T) {
	if got := progressRatio(500000, 1000); got != 0.5 {
		t.Fatalf("0.5 got %v", got)
	}
	if got := progressRatio(0, 1000); got != -1 {
		t.Fatalf("无 out_time 应为 -1, got %v", got)
	}
	if got := progressRatio(500000, 0); got != -1 {
		t.Fatalf("无时长应为 -1, got %v", got)
	}
	if got := progressRatio(2000000, 1000); got != 1 {
		t.Fatalf("超 1 应夹 1, got %v", got)
	}
}

func TestTranscodePure_ParseProgressLine(t *testing.T) {
	k, v, ok := parseProgressLine("out_time_us=150000")
	if !ok || k != "out_time_us" || v != "150000" {
		t.Fatalf("解析失败: %q %q %v", k, v, ok)
	}
	if _, _, ok := parseProgressLine("garbage"); ok {
		t.Fatal("非 k=v 应 false")
	}
	if _, _, ok := parseProgressLine("=noval"); ok {
		t.Fatal("空 key 应 false")
	}
}

func TestTranscodeManager_DegradesWithoutStores(t *testing.T) {
	m := NewTranscodeManager(nil, nil, nil)
	if m == nil {
		t.Fatal("构造不应返回 nil")
	}
	if _, err := m.Trigger(context.Background(), &File{ID: "x", Mime: "video/mp4"}); err != ErrTranscodeNoManager {
		t.Fatalf("nil stores → ErrTranscodeNoManager, got %v", err)
	}
	if j := m.Status(context.Background(), "x"); j.FileID != "x" {
		t.Fatal("Status 应回空任务而非 panic")
	}
	if m.Cancel(context.Background(), "x") {
		t.Fatal("Cancel 应返回 false")
	}
	if _, ok := m.Summary()["concurrency"]; !ok {
		t.Fatal("Summary 应可返回")
	}
}

func TestTranscodeManager_RejectsNonVideo(t *testing.T) {
	m, _, _ := setupTranscodeManager(t)
	if _, err := m.Trigger(context.Background(), &File{ID: "img1", Mime: "image/png"}); err != ErrTranscodeNotVideo {
		t.Fatalf("非视频应返回 ErrTranscodeNotVideo, got %v", err)
	}
}

func TestTranscodeManager_RejectsTooLong(t *testing.T) {
	m, db, st := setupTranscodeManager(t)
	seedVideoFile(t, db, st, "long1", 2000000) // 2000s > 默认上限 1800s
	if _, err := m.Trigger(context.Background(), &File{ID: "long1", SpaceID: "sp1", Mime: "video/mp4"}); err != ErrTranscodeTooLong {
		t.Fatalf("超长应返回 ErrTranscodeTooLong, got %v", err)
	}
}

func TestTranscodeManager_TriggerDoneAndIdempotent(t *testing.T) {
	m, db, st := setupTranscodeManager(t)
	seedVideoFile(t, db, st, "vid1", 30000)
	job, err := m.Trigger(context.Background(), &File{ID: "vid1", SpaceID: "sp1", Mime: "video/mp4"})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if job.Status != TranscodeQueued && job.Status != TranscodeRunning {
		t.Fatalf("应入队/在跑, got %v", job.Status)
	}
	final := pollUntilDone(t, m, "vid1", 3*time.Second)
	if final.Status != TranscodeDone {
		t.Fatalf("应 done, got %v msg=%q", final.Status, final.Message)
	}
	if final.Signature != DefaultTranscodeOptions().Signature() {
		t.Fatalf("指纹应与默认一致: %q", final.Signature)
	}
	// 幂等：再次触发应直接返回 done（不重新入队）
	job2, err := m.Trigger(context.Background(), &File{ID: "vid1", SpaceID: "sp1", Mime: "video/mp4"})
	if err != nil {
		t.Fatalf("trigger2: %v", err)
	}
	if job2.Status != TranscodeDone {
		t.Fatalf("幂等应回 done, got %v", job2.Status)
	}
	// 落库态也应 done 且 ref 非空
	if info, ok := m.media.Get(context.Background(), "vid1"); !ok || info.TranscodeStatus != TranscodeDone || info.TranscodedRef == "" {
		t.Fatalf("落库应 done 且 ref 非空: ok=%v info=%+v", ok, info)
	}
}

func TestTranscodeManager_ResetStale(t *testing.T) {
	m, db, _ := setupTranscodeManager(t)
	if _, err := db.Exec(`INSERT INTO file_media(file_id, probe_status, transcode_status, updated_at, transcode_updated_at) VALUES('stale1','ok','queued',0,0)`); err != nil {
		t.Fatalf("insert stale: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO file_media(file_id, probe_status, transcode_status, updated_at, transcode_updated_at) VALUES('done1','ok','done',0,0)`); err != nil {
		t.Fatalf("insert done: %v", err)
	}
	n := m.ResetStale(context.Background())
	if n != 1 {
		t.Fatalf("ResetStale 应标 1 条为 failed, got %d", n)
	}
	var s string
	if err := db.QueryRow(`SELECT transcode_status FROM file_media WHERE file_id='stale1'`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	if s != TranscodeFailed {
		t.Fatalf("stale 应置 failed, got %q", s)
	}
	if err := db.QueryRow(`SELECT transcode_status FROM file_media WHERE file_id='done1'`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	if s != TranscodeDone {
		t.Fatalf("done 行不应被改, got %q", s)
	}
}
