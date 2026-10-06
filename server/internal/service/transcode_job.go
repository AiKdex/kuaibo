// transcode_job.go 转码任务的异步执行与状态机（B17）。
//
// 为什么必须异步：一次 720p 转码在中档主机上通常是数十秒到数分钟量级，
// 放在 HTTP 请求里同步做必然拖垮连接、也会让站长无从得知「是不是卡住了」。
// 因此这里是一个**进程内的单机任务队列**：
//
//	API 触发 → 入队（queued）→ 调度器按并发闸起 goroutine（running）→ 落派生对象（done）
//
// 状态**双写**：内存（含实时进度）+ file_media 行（跨请求可见、可运维查询）。
// 进度有意**不落库**（每秒数次写盘不划算）；进程重启时把残留的 queued/running
// 一律标为 failed（见 ResetStale）—— 不假装还在跑，也不自动续跑（避免重启风暴时重复烧 CPU）。
//
// 为什么不用外部队列（Redis/ASQ）：本壳是单机自部署产品，引入外部依赖换来的只是
// 「多实例」，而多实例下派生对象的一致性本身还要另做设计 —— 属预留位，不在本批。
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// 转码任务状态（file_media.transcode_status 的取值域）。
const (
	TranscodeQueued   = "queued"
	TranscodeRunning  = "running"
	TranscodeDone     = "done"
	TranscodeFailed   = "failed"
	TranscodeCanceled = "canceled"
)

// 触发侧的拒绝原因（handler 据此映射到语义化错误码）。
var (
	ErrTranscodeNotVideo  = errors.New("transcode: 仅支持视频")
	ErrTranscodeTooLong   = errors.New("transcode: 素材时长超过上限")
	ErrTranscodeNoLocal   = errors.New("transcode: 源文件不在本地存储")
	ErrTranscodeNoManager = errors.New("transcode: 转码模块不可用")
)

// TranscodeJob 一个文件当前的转码视图（内存态与落库态合并后的结果）。
type TranscodeJob struct {
	FileID    string  `json:"file_id"`
	Status    string  `json:"status"`    // queued|running|done|failed|canceled；"" = 从未发起
	Progress  float64 `json:"progress"`  // 0..1；-1 = 时长未知无法估算
	Message   string  `json:"message"`   // 失败/取消原因，成功时为空
	Signature string  `json:"signature"` // 参数指纹（play-<sig>.mp4）
	Height    int     `json:"height"`    // 产物高度（像素；探测不可用时取目标值）
	Bytes     int64   `json:"bytes"`     // 产物字节
	SrcBytes  int64   `json:"src_bytes"` // 源文件字节（用于展示压缩比）
	Ref       string  `json:"-"`         // 派生对象 key：内部引用，不外发
	UpdatedAt int64   `json:"updated_at"`
}

// transcodeRunner 一次转码执行（默认 MakeTranscode；测试注入假实现，避免依赖本机 ffmpeg）。
type transcodeRunner func(ctx context.Context, src, dst string, durationMS int64, opt TranscodeOptions, onProgress TranscodeProgress) (int64, error)

// TranscodeManager 单机转码任务管理器（进程生命周期常驻）。
type TranscodeManager struct {
	files *FileStore
	media *MediaStore
	cfg   cfgGetter
	run   transcodeRunner

	mu      sync.Mutex
	jobs    map[string]*TranscodeJob      // 活跃/近期任务（内存）
	queue   []string                      // 待跑队列（FIFO）
	running int                           // 在跑数量（并发闸）
	cancels map[string]context.CancelFunc // 在跑任务的取消句柄
	wake    chan struct{}                 // 调度信号（容量 1，天然合并重复信号）
}

// NewTranscodeManager 创建管理器并启动调度协程。
// files/media 为 nil 时各方法安全降级（返回空任务/错误），便于缺模块的壳侧装配。
func NewTranscodeManager(files *FileStore, media *MediaStore, cfg cfgGetter) *TranscodeManager {
	m := &TranscodeManager{
		files:   files,
		media:   media,
		cfg:     cfg,
		run:     MakeTranscode,
		jobs:    map[string]*TranscodeJob{},
		cancels: map[string]context.CancelFunc{},
		wake:    make(chan struct{}, 1),
	}
	go m.loop()
	return m
}

// ResetStale 把服务重启后残留的 queued/running 行标为 failed，返回受影响行数。
//
// 为什么不自动续跑：重启多因升级/崩溃，此时立刻重跑一批重任务会把机器再压垮一次。
// 让站长显式重发（幂等触发，点一下就回来）更稳妥。
func (m *TranscodeManager) ResetStale(ctx context.Context) int {
	if m == nil || m.media == nil || m.media.db == nil {
		return 0
	}
	res, err := m.media.db.ExecContext(ctx,
		`UPDATE file_media SET transcode_status=?, transcode_msg=?, transcode_updated_at=?
		  WHERE transcode_status IN (?, ?)`,
		TranscodeFailed, "服务重启中断，可重新发起", time.Now().UnixMilli(),
		TranscodeQueued, TranscodeRunning)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

// Summary 当前调度概况（运维面板展示用）。
func (m *TranscodeManager) Summary() map[string]any {
	if m == nil {
		return map[string]any{"running": 0, "queued": 0, "concurrency": 0}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]any{"running": m.running, "queued": len(m.queue), "concurrency": m.concurrency()}
}

// concurrency 当前并发上限（每次实时读配置 → 改完即时生效）。调用方需持锁或接受竞态。
func (m *TranscodeManager) concurrency() int {
	opt := TranscodeOptionsFrom(m.cfg)
	n := opt.Concurrency
	if n < 1 {
		n = 1
	}
	return n
}

// loop 调度器：收到唤醒信号就把队列尽量填满并发槽。
func (m *TranscodeManager) loop() {
	for range m.wake {
		m.pump()
	}
}

// pump 起任务直到「并发满了」或「队列空了」。
func (m *TranscodeManager) pump() {
	for {
		m.mu.Lock()
		if m.running >= m.concurrency() || len(m.queue) == 0 {
			m.mu.Unlock()
			return
		}
		id := m.queue[0]
		m.queue = m.queue[1:]
		m.running++
		m.mu.Unlock()
		go m.worker(id)
	}
}

// notify 非阻塞地唤醒调度器。
func (m *TranscodeManager) notify() {
	select {
	case m.wake <- struct{}{}:
	default: // 已有待处理信号，无需重复
	}
}

// Trigger 发起转码（幂等）：已在跑/已排队/已完成且参数未变 → 直接返回当前状态。
//
// 幂等是刻意的：前端按钮会被重复点击，改参数后也会再点一次，不该因此重复烧 CPU。
func (m *TranscodeManager) Trigger(ctx context.Context, f *File) (TranscodeJob, error) {
	if m == nil || m.files == nil || m.media == nil || f == nil {
		return TranscodeJob{}, ErrTranscodeNoManager
	}
	if !isVideoMime(f.Mime) {
		return TranscodeJob{}, ErrTranscodeNotVideo
	}
	src, ok := m.files.LocalPath(ctx, f.ID)
	if !ok {
		return TranscodeJob{}, ErrTranscodeNoLocal
	}
	if !FFmpegAvailable() {
		return TranscodeJob{}, ErrTranscodeNoFFmpeg
	}
	opt := TranscodeOptionsFrom(m.cfg)

	// 1) 已在跑 / 已排队 → 幂等返回
	m.mu.Lock()
	if j := m.jobs[f.ID]; j != nil && (j.Status == TranscodeQueued || j.Status == TranscodeRunning) {
		cp := *j
		m.mu.Unlock()
		return cp, nil
	}
	m.mu.Unlock()

	// 2) 素材时长：落库没有就现探一次（转码超时与时长上限都要用）
	info, _ := m.media.Get(ctx, f.ID)
	durationMS := int64(0)
	if info != nil {
		durationMS = info.DurationMS
	}
	if durationMS <= 0 {
		if av, ok := ProbeAV(ctx, src); ok {
			durationMS = av.DurationMS
		}
	}
	if opt.MaxDuration > 0 && durationMS > int64(opt.MaxDuration)*1000 {
		return TranscodeJob{}, ErrTranscodeTooLong
	}

	// 3) 已完成且指纹未变、产物还在 → 幂等返回（不重复转）
	if info != nil && info.TranscodeStatus == TranscodeDone &&
		info.TranscodeSignature == opt.Signature() && info.TranscodedRef != "" {
		if _, exists := m.files.DerivedStat(ctx, info.TranscodedRef); exists {
			return jobOfInfo(info), nil
		}
	}

	// 4) 入队
	job := &TranscodeJob{
		FileID:    f.ID,
		Status:    TranscodeQueued,
		Progress:  -1,
		Signature: opt.Signature(),
		SrcBytes:  f.Size,
		UpdatedAt: time.Now().UnixMilli(),
	}
	m.mu.Lock()
	m.jobs[f.ID] = job
	m.queue = append(m.queue, f.ID)
	cp := *job
	m.mu.Unlock()

	m.markState(ctx, f.ID, TranscodeQueued, "", opt.Signature())
	m.notify()
	return cp, nil
}

// Status 查询某文件的转码状态：优先内存（含实时进度），否则回落落库态。
func (m *TranscodeManager) Status(ctx context.Context, fileID string) TranscodeJob {
	if m == nil || m.media == nil {
		return TranscodeJob{FileID: fileID}
	}
	m.mu.Lock()
	if j := m.jobs[fileID]; j != nil {
		cp := *j
		m.mu.Unlock()
		return cp
	}
	m.mu.Unlock()
	info, ok := m.media.Get(ctx, fileID)
	if !ok || info == nil {
		return TranscodeJob{FileID: fileID, Progress: -1}
	}
	return jobOfInfo(info)
}

// Cancel 取消排队中或运行中的任务；无任务可取消时返回 false。
// 运行中的任务靠取消 context 终止 ffmpeg 子进程（exec.CommandContext 会杀掉它）。
func (m *TranscodeManager) Cancel(ctx context.Context, fileID string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	if c, ok := m.cancels[fileID]; ok {
		c()
		m.mu.Unlock()
		return true
	}
	for i, id := range m.queue {
		if id != fileID {
			continue
		}
		m.queue = append(m.queue[:i], m.queue[i+1:]...)
		if j := m.jobs[fileID]; j != nil {
			j.Status, j.Message, j.UpdatedAt = TranscodeCanceled, "已取消", time.Now().UnixMilli()
			j.Progress = -1
		}
		m.mu.Unlock()
		m.markState(ctx, fileID, TranscodeCanceled, "已取消", "")
		return true
	}
	m.mu.Unlock()
	return false
}

// worker 执行一次转码。任何失败都**不改写**源文件与既有可用产物。
func (m *TranscodeManager) worker(fileID string) {
	defer func() {
		m.mu.Lock()
		m.running--
		delete(m.cancels, fileID)
		m.mu.Unlock()
		m.notify() // 腾出槽位后立刻补下一个
	}()

	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancels[fileID] = cancel
	m.mu.Unlock()
	defer cancel()

	m.setProgress(fileID, -1)
	m.markState(ctx, fileID, TranscodeRunning, "", "")

	fail := func(format string, a ...any) {
		m.markState(ctx, fileID, TranscodeFailed, fmt.Sprintf(format, a...), "")
	}

	f, err := m.files.Get(ctx, fileID)
	if err != nil || f == nil {
		fail("源文件已不存在")
		return
	}
	src, ok := m.files.LocalPath(ctx, fileID)
	if !ok {
		fail("源文件不在本地存储，无法转码")
		return
	}
	prev, _ := m.media.Get(ctx, fileID)
	durationMS := int64(0)
	oldRef := ""
	if prev != nil {
		durationMS, oldRef = prev.DurationMS, prev.TranscodedRef
	}
	opt := TranscodeOptionsFrom(m.cfg)
	// 产物名带参数指纹：参数一改就是新对象，绝不会拿参数不明的旧缓存糊弄人
	key := DerivedKey(f.SpaceID, f.ID, "play-"+opt.Signature()+".mp4")

	tmp, err := os.CreateTemp("", "aiklog-transcode-*.mp4")
	if err != nil {
		fail("无法创建临时文件：%v", err)
		return
	}
	tmpName := tmp.Name()
	// 先关句柄再交给 ffmpeg：Windows 上对同一路径持打开句柄会导致 ffmpeg 写入失败（独占锁）。
	_ = tmp.Close()
	defer os.Remove(tmpName)

	throttle := newThrottle(500 * time.Millisecond)
	size, err := m.run(ctx, src, tmpName, durationMS, opt, func(ratio float64, _ int64) {
		if throttle.allow() {
			m.setProgress(fileID, ratio)
		}
	})
	if err != nil {
		if errors.Is(err, ErrTranscodeNoFFmpeg) {
			fail("本机未安装 ffmpeg")
			return
		}
		if ctx.Err() != nil {
			m.markState(ctx, fileID, TranscodeCanceled, "已取消", "")
			return
		}
		msg := err.Error()
		if oldRef != "" {
			msg += "；播放仍使用上一次的转码产物"
		}
		m.markState(ctx, fileID, TranscodeFailed, msg, "")
		return
	}

	// 产物高度：优先实测（ffprobe），不可用时回退到「目标高度」
	height := opt.MaxHeight
	if av, ok := ProbeAV(ctx, tmpName); ok && av.Height > 0 {
		height = av.Height
	}
	out, err := os.Open(tmpName)
	if err != nil {
		fail("读取产物失败：%v", err)
		return
	}
	defer out.Close()
	if err := m.files.PutDerived(ctx, key, out, size); err != nil {
		fail("写入派生存储失败：%v", err)
		return
	}
	// 旧产物（参数不同的上一版）显式删除，避免 .derived 目录里堆多份播放源
	if oldRef != "" && oldRef != key {
		_ = m.files.DeleteDerivedObject(ctx, oldRef)
	}

	// 落库：重新 Get 一次，只覆盖转码相关字段，不冲掉期间可能被刷新的探测字段
	cur, _ := m.media.Get(ctx, fileID)
	if cur == nil {
		cur = &MediaInfo{FileID: fileID}
	}
	cur.TranscodedRef = key
	cur.TranscodeStatus = TranscodeDone
	cur.TranscodeMsg = ""
	cur.TranscodeSignature = opt.Signature()
	cur.TranscodeBytes = size
	cur.TranscodeHeight = height
	cur.TranscodeUpdatedAt = time.Now().UnixMilli()
	if err := m.media.Upsert(ctx, cur); err != nil {
		fail("状态写库失败：%v", err)
		return
	}
	m.mu.Lock()
	if j := m.jobs[fileID]; j != nil {
		j.Status, j.Message, j.Progress = TranscodeDone, "", 1
		j.Signature, j.Bytes, j.Height = opt.Signature(), size, height
		j.UpdatedAt = cur.TranscodeUpdatedAt
	}
	m.mu.Unlock()
}

// setProgress 更新内存进度（不落库：每秒数次写盘不划算）。
func (m *TranscodeManager) setProgress(fileID string, ratio float64) {
	m.mu.Lock()
	if j := m.jobs[fileID]; j != nil {
		j.Progress = ratio
	}
	m.mu.Unlock()
}

// markState 更新状态：内存 + file_media 行（sig 非空时一并更新参数指纹）。
func (m *TranscodeManager) markState(ctx context.Context, fileID, status, msg, sig string) {
	now := time.Now().UnixMilli()
	m.mu.Lock()
	if j := m.jobs[fileID]; j != nil {
		j.Status, j.Message, j.UpdatedAt = status, msg, now
		if status != TranscodeRunning && status != TranscodeQueued {
			j.Progress = -1
		}
		if sig != "" {
			j.Signature = sig
		}
	}
	m.mu.Unlock()
	if m.media == nil || m.media.db == nil {
		return
	}
	if sig != "" {
		_, _ = m.media.db.ExecContext(ctx,
			`UPDATE file_media SET transcode_status=?, transcode_msg=?, transcode_signature=?, transcode_updated_at=?
			  WHERE file_id=?`, status, msg, sig, now, fileID)
		return
	}
	_, _ = m.media.db.ExecContext(ctx,
		`UPDATE file_media SET transcode_status=?, transcode_msg=?, transcode_updated_at=? WHERE file_id=?`,
		status, msg, now, fileID)
}

// jobOfInfo 由落库元信息拼出任务视图（进程内无内存态时的兜底）。
func jobOfInfo(info *MediaInfo) TranscodeJob {
	if info == nil {
		return TranscodeJob{Progress: -1}
	}
	return TranscodeJob{
		FileID:    info.FileID,
		Status:    info.TranscodeStatus,
		Progress:  -1,
		Message:   info.TranscodeMsg,
		Signature: info.TranscodeSignature,
		Height:    info.TranscodeHeight,
		Bytes:     info.TranscodeBytes,
		Ref:       info.TranscodedRef,
		UpdatedAt: info.TranscodeUpdatedAt,
	}
}

// isVideoMime 判定是否视频（含 mime 参数剥离；"video/quicktime" 这类也算）。
func isVideoMime(mime string) bool {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	return strings.HasPrefix(m, "video/")
}

// throttle 简单节流器（进度回调频率远高于落库频率时需要）。
type throttle struct {
	interval time.Duration
	last     time.Time
}

func newThrottle(d time.Duration) *throttle { return &throttle{interval: d} }

// allow 距上次放行已超过间隔则返回 true（首次调用必放行）。
func (t *throttle) allow() bool {
	now := time.Now()
	if now.Sub(t.last) < t.interval {
		return false
	}
	t.last = now
	return true
}
