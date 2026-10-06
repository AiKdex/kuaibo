// transcode.go 视频转码（B17）：把上传的视频转成 Web 友好的 H.264/AAC MP4 播放源。
//
// 为什么需要：手机/相机直出常见 HEVC(H.265)/ProRes/4K 高码率，浏览器支持面窄、移动端流量贵。
// 本批**不做**自适应码率（HLS/ABR，见 media.hls_enabled 预留位），只做一次「按需转码」，
// 产物落 B15 的派生对象目录（spaces/{space}/.derived/{fid}/）—— 因此天然零污染
// 公开列表/配额/索引，并随源文件 Purge 按前缀一起清掉。
//
// 与缩略图（B15/B16）刻意保持同构：
//   - 参数全部可配（media.transcode_*，运行期读取，改完**即时生效**）；
//   - ffmpeg 缺失 → 安静降级（ErrTranscodeNoFFmpeg），不落坏状态，装好后自动可用；
//   - 参数以 argv 数组传递、路径不参与 shell 拼接 → 无命令注入面；-nostdin 必加；
//   - 有超时保护，且超时**与素材时长成正比**（转码是长任务，不能用缩略图那种固定秒数）。
//
// 与缩略图的关键差异：转码耗时可达分钟级，**绝不能放在 HTTP 请求里同步做** ——
// 故由 TranscodeManager 异步执行（见 transcode_job.go），本文件只负责「一次 ffmpeg 调用」。
package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrTranscodeNoFFmpeg 环境不具备 ffmpeg。调用方据此安静降级（不落坏状态）。
var ErrTranscodeNoFFmpeg = errors.New("transcode: ffmpeg not available")

// 配置键（settings）：由站长在后台「运维 → 媒体派生资产」调整，运行期读取 → 改完即时生效。
// 为什么不做成常量：主机 CPU 余量、目标受众网络、素材分辨率差异都很大，硬编码等于拿走站长的调优权。
const (
	KeyTranscodeMaxHeight   = "media.transcode_max_height"
	KeyTranscodeCRF         = "media.transcode_crf"
	KeyTranscodePreset      = "media.transcode_preset"
	KeyTranscodeAudioKbps   = "media.transcode_audio_kbps"
	KeyTranscodeConcurrency = "media.transcode_concurrency"
	KeyTranscodeMaxDuration = "media.transcode_max_duration_s"
)

const (
	// DefaultTranscodeMaxHeight 输出最大高度（像素）。720p 在清晰度与体积/CPU 之间最平衡；
	// 0 = 不缩放（保留原分辨率，只做重新编码）。
	DefaultTranscodeMaxHeight = 720
	// DefaultTranscodeCRF x264 恒定质量因子：越小越清晰、越大越小。23 是上游公认的默认档。
	DefaultTranscodeCRF = 23
	// DefaultTranscodePreset x264 速度档。medium 是「编码效率 vs 耗时」的平衡点。
	DefaultTranscodePreset = "medium"
	// DefaultTranscodeAudioKbps AAC 音频码率；0 = 去掉音轨（纯画面素材）。
	DefaultTranscodeAudioKbps = 128
	// DefaultTranscodeConcurrency 同时进行的转码任务数。共享生产机上默认 1，避免打满 CPU。
	DefaultTranscodeConcurrency = 1
	// DefaultTranscodeMaxDuration 允许转码的最长素材时长（秒）；0 = 不限。
	// 存在意义：一条 2 小时的素材在单核档位上可能占住 worker 数十分钟，需要一道闸。
	DefaultTranscodeMaxDuration = 1800
)

// 参数合法区间。与 handler/settings.go 白名单同源（那边拒绝越界，这边夹取兜底）：
// 配置也可能经导入快照/直接改库写入，不能假定一定过白名单。
const (
	TranscodeMaxHeightMin, TranscodeMaxHeightMax     = 0, 2160
	TranscodeCRFMin, TranscodeCRFMax                 = 0, 51
	TranscodeAudioKbpsMin, TranscodeAudioKbpsMax     = 0, 320
	TranscodeConcurrencyMin, TranscodeConcurrencyMax = 1, 4
	TranscodeMaxDurationMin, TranscodeMaxDurationMax = 0, 86400
)

// TranscodePresets x264 预设白名单（顺序即设置页下拉框顺序）。
var TranscodePresets = []string{
	"ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow",
}

// TranscodePresetValid 判定预设名是否合法（大小写不敏感，先归一为小写再比）。
func TranscodePresetValid(p string) bool {
	p = strings.ToLower(strings.TrimSpace(p))
	for _, v := range TranscodePresets {
		if v == p {
			return true
		}
	}
	return false
}

// TranscodeOptions 转码参数（B17；消灭散落的 crf/preset/分辨率硬编码）。
type TranscodeOptions struct {
	MaxHeight   int    `json:"max_height"`     // 输出最大高度（像素），只缩不放；0=不缩放
	CRF         int    `json:"crf"`            // x264 质量因子 0..51（18-28 为常用档）
	Preset      string `json:"preset"`         // x264 速度档
	AudioKbps   int    `json:"audio_kbps"`     // AAC 音频码率；0=去音轨
	Concurrency int    `json:"concurrency"`    // 同时进行的任务数（只影响调度，不影响产物）
	MaxDuration int    `json:"max_duration_s"` // 允许转码的最长时长（秒）；0=不限
}

// DefaultTranscodeOptions 内置默认参数（配置缺失、非法或越界时的回退值）。
func DefaultTranscodeOptions() TranscodeOptions {
	return TranscodeOptions{
		MaxHeight:   DefaultTranscodeMaxHeight,
		CRF:         DefaultTranscodeCRF,
		Preset:      DefaultTranscodePreset,
		AudioKbps:   DefaultTranscodeAudioKbps,
		Concurrency: DefaultTranscodeConcurrency,
		MaxDuration: DefaultTranscodeMaxDuration,
	}
}

// Clamp 把越界参数夹到合法区间（防御手工构造的 TranscodeOptions）。
//
// 注意 0 在这里是**合法值**（MaxHeight=0 不缩放 / AudioKbps=0 去音轨 / MaxDuration=0 不限），
// 故这三个区间下界为 0 —— clampIntRange 对「下界为 0」不会把 0 当未配置处理。
func (o TranscodeOptions) Clamp() TranscodeOptions {
	o.MaxHeight = clampIntRange(o.MaxHeight, TranscodeMaxHeightMin, TranscodeMaxHeightMax, DefaultTranscodeMaxHeight)
	o.CRF = clampIntRange(o.CRF, TranscodeCRFMin, TranscodeCRFMax, DefaultTranscodeCRF)
	o.AudioKbps = clampIntRange(o.AudioKbps, TranscodeAudioKbpsMin, TranscodeAudioKbpsMax, DefaultTranscodeAudioKbps)
	o.Concurrency = clampIntRange(o.Concurrency, TranscodeConcurrencyMin, TranscodeConcurrencyMax, DefaultTranscodeConcurrency)
	o.MaxDuration = clampIntRange(o.MaxDuration, TranscodeMaxDurationMin, TranscodeMaxDurationMax, DefaultTranscodeMaxDuration)
	if !TranscodePresetValid(o.Preset) {
		o.Preset = DefaultTranscodePreset
	}
	o.Preset = strings.ToLower(strings.TrimSpace(o.Preset))
	return o
}

// Signature 参数指纹：只含**影响产物**的字段（并发与时长上限只影响调度，故意不入指纹）。
//
// 用途有二：
//  1. 产物命名（play-<sig>.mp4）—— 参数一改就写新对象、旧对象显式删除，不会留下参数不明的缓存；
//  2. 幂等判定 —— 已完成且指纹未变时重复触发直接返回，不白烧一次 CPU。
func (o TranscodeOptions) Signature() string {
	o = o.Clamp()
	return fmt.Sprintf("h%d-crf%d-%s-a%d", o.MaxHeight, o.CRF, o.Preset, o.AudioKbps)
}

// TranscodeOptionsFrom 从配置中心读取转码参数（cfg 可为 nil → 全默认）。
func TranscodeOptionsFrom(cfg cfgGetter) TranscodeOptions {
	if cfg == nil {
		return DefaultTranscodeOptions()
	}
	opt := TranscodeOptions{
		MaxHeight: intCfg(cfg, KeyTranscodeMaxHeight, DefaultTranscodeMaxHeight, TranscodeMaxHeightMin, TranscodeMaxHeightMax),
		CRF:       intCfg(cfg, KeyTranscodeCRF, DefaultTranscodeCRF, TranscodeCRFMin, TranscodeCRFMax),
		Preset:    strCfg(cfg, KeyTranscodePreset, DefaultTranscodePreset, TranscodePresetValid),
		AudioKbps: intCfg(cfg, KeyTranscodeAudioKbps, DefaultTranscodeAudioKbps, TranscodeAudioKbpsMin, TranscodeAudioKbpsMax),
		Concurrency: intCfg(cfg, KeyTranscodeConcurrency, DefaultTranscodeConcurrency,
			TranscodeConcurrencyMin, TranscodeConcurrencyMax),
		MaxDuration: intCfg(cfg, KeyTranscodeMaxDuration, DefaultTranscodeMaxDuration,
			TranscodeMaxDurationMin, TranscodeMaxDurationMax),
	}
	return opt
}

// strCfg 读一个字符串型配置：**空串 = 未配置** → 回退默认；非法值也回退默认
// （宁可按默认真实执行，也不要因为一个错别字让转码整条链路报错）。
func strCfg(cfg cfgGetter, key, def string, valid func(string) bool) string {
	v := strings.ToLower(strings.TrimSpace(cfg.GetString(key)))
	if v == "" {
		return def
	}
	if valid != nil && !valid(v) {
		return def
	}
	return v
}

// 转码超时的上下限与未知时长兜底。
const (
	transcodeTimeoutFloor   = 180 * time.Second
	transcodeTimeoutCap     = 3600 * time.Second
	transcodeTimeoutUnknown = 600 * time.Second
)

// TranscodeTimeout 单次转码超时：2×时长 + 60s，夹在 [180s, 3600s]。
//
// 为什么与时长成正比：x264 medium 在共享主机上大致是 0.3–1× 实时，
// 固定秒数要么对长素材过短（必然误杀），要么对短片过长（占住 worker 不放手）。
// 时长未知时给 600s 兜底 —— 足够覆盖常见短片，又不至于让坏文件长期占位。
func TranscodeTimeout(durationMS int64) time.Duration {
	if durationMS <= 0 {
		return transcodeTimeoutUnknown
	}
	d := 2*time.Duration(durationMS)*time.Millisecond + 60*time.Second
	if d < transcodeTimeoutFloor {
		return transcodeTimeoutFloor
	}
	if d > transcodeTimeoutCap {
		return transcodeTimeoutCap
	}
	return d
}

// transcodeScaleFilter 生成缩放滤镜：宽度按比例自动算并对齐偶数，高度取「上限与原高的较小者」。
// 只缩不放 —— min() 让原本就低于上限的素材保持原分辨率（放大只会更糊且更大）。
// 单引号是 ffmpeg filter 语法的一部分（保护表达式内的逗号），这里是传给 argv 的字面量，
// 不经过 shell，故与注入无关。
func transcodeScaleFilter(maxHeight int) string {
	return fmt.Sprintf("scale=-2:'min(%d,ih)'", maxHeight)
}

// TranscodeProgress 进度回调：ratio ∈ [0,1]，<0 表示时长未知无法估算。
// 回调按 ffmpeg 的输出节拍触发（约每秒数次），实现方若需落库请自行节流。
type TranscodeProgress func(ratio float64, outTimeMS int64)

// parseProgressLine 解析 ffmpeg `-progress pipe:1` 的一行 "key=value"。
// 非 "k=v" 形态（空行、警告）返回 ok=false，调用方忽略即可。
func parseProgressLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	i := strings.IndexByte(line, '=')
	if i <= 0 {
		return "", "", false
	}
	return line[:i], line[i+1:], true
}

// progressRatio 由 ffmpeg 的 out_time 与素材总时长算进度。
//
// 🔴 ffmpeg 的 `-progress` 输出里 `out_time_ms` **实际是微秒**（历史遗留的字段名错误），
// 与 `out_time_us` 同值。故两个键都按微秒解释；时长未知（<=0）返回 -1。
func progressRatio(outTimeUS, durationMS int64) float64 {
	if durationMS <= 0 || outTimeUS <= 0 {
		return -1
	}
	r := float64(outTimeUS) / 1000.0 / float64(durationMS)
	if r > 1 {
		r = 1
	}
	return r
}

// MakeTranscode 把源视频转成 H.264/AAC MP4（faststart），返回产物字节数。
//
//	-durationMS 仅用于超时与进度估算，<=0 时按未知处理（不阻塞执行）；
//	onProgress 可为 nil。
//
// 返回 error 只表示**本次执行失败**；ffmpeg 缺失时返回 ErrTranscodeNoFFmpeg（errors.Is 可辨）。
// 调用方约定：失败不改写既有可用产物（见 TranscodeManager.worker）。
func MakeTranscode(ctx context.Context, srcPath, dstPath string, durationMS int64, opt TranscodeOptions, onProgress TranscodeProgress) (int64, error) {
	if strings.TrimSpace(srcPath) == "" || strings.TrimSpace(dstPath) == "" {
		return 0, errors.New("transcode: empty src or dst path")
	}
	bin, err := exec.LookPath(ffmpegBin)
	if err != nil {
		return 0, ErrTranscodeNoFFmpeg
	}
	opt = opt.Clamp() // 防御直接构造的越界值

	// -nostdin：服务进程下必须加，否则 ffmpeg 可能尝试读 stdin 而挂住
	// -progress pipe:1：把机器可读的进度写到 stdout（与 -v error 的 stderr 分离）
	args := []string{"-v", "error", "-nostdin", "-progress", "pipe:1", "-i", srcPath,
		"-c:v", "libx264", "-preset", opt.Preset, "-crf", strconv.Itoa(opt.CRF),
		// yuv420p：H.264 的兼容基线，缺了它部分浏览器/Safari 直接黑屏
		"-pix_fmt", "yuv420p"}
	if opt.MaxHeight > 0 {
		args = append(args, "-vf", transcodeScaleFilter(opt.MaxHeight))
	}
	if opt.AudioKbps > 0 {
		args = append(args, "-c:a", "aac", "-b:a", strconv.Itoa(opt.AudioKbps)+"k")
	} else {
		args = append(args, "-an")
	}
	// +faststart：把 moov 原子前移，浏览器才能边下边播（否则要下完整个文件）
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", dstPath)

	cctx, cancel := context.WithTimeout(ctx, TranscodeTimeout(durationMS))
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("transcode: stdout pipe: %w", err)
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("transcode: start: %w", err)
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		k, v, ok := parseProgressLine(sc.Text())
		if !ok {
			continue
		}
		switch k {
		case "out_time_us", "out_time_ms": // 两者同值，单位都是微秒（见 progressRatio 注释）
			if us, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && onProgress != nil {
				onProgress(progressRatio(us, durationMS), us/1000)
			}
		}
	}
	werr := cmd.Wait()
	if cctx.Err() != nil {
		return 0, fmt.Errorf("transcode: 超时（上限 %s）: %w", TranscodeTimeout(durationMS), cctx.Err())
	}
	if werr != nil {
		return 0, fmt.Errorf("transcode: ffmpeg: %w (%s)", werr, strings.TrimSpace(errBuf.String()))
	}
	st, err := os.Stat(dstPath)
	if err != nil || st.Size() <= 0 {
		return 0, errors.New("transcode: 产物为空")
	}
	if onProgress != nil {
		onProgress(1, durationMS)
	}
	return st.Size(), nil
}
