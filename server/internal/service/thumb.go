// thumb.go 派生资产：缩略图生成（B15）。
//
// 为什么走 ffmpeg 而不是纯 Go：
//   - 图片缩放本可用 golang.org/x/image/draw，但**视频抽帧没有纯 Go 通用解**；
//   - 两者共用同一条 ffmpeg 路径，本批**零新增依赖**（不动 go.mod/go.sum），
//     且与 B13 的 ffprobe 探测共享同一种契约：环境不具备就**安静降级**，不报错、不落坏状态。
//
// 与 ProbeAV 一致的边界：
//   - 需要**宿主机路径**（随机访问）；调用方必须先用 FileStore.LocalPath 判本地后端；
//   - ffmpeg 不在 PATH 时返回 ErrThumbNoFFmpeg，调用方**不写任何状态**，
//     装好 ffmpeg 后下一次请求自动生效（生成失败不是终态）；
//   - 参数以 args 数组传递、路径不参与 shell 拼接 → 无命令注入面；
//   - -nostdin 必加：服务进程下 ffmpeg 若读到 stdin 可能挂住；
//   - 有超时保护，损坏/超大文件不会挂死 HTTP 请求。
package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrThumbNoFFmpeg 环境不具备 ffmpeg。调用方据此安静降级（不记 failed、不落库）。
var ErrThumbNoFFmpeg = errors.New("thumb: ffmpeg not available")

// ffmpegBin 生成所需的二进制名。声明为变量以便测试注入「不存在的路径」，
// 验证二进制缺失时**安全降级**而不是报错（与 ffprobeBin 同一套路）。
var ffmpegBin = "ffmpeg"

// 配置键（settings）：B16 起缩略图参数由站长在后台「运维 → 媒体派生资产」调整。
// 为什么不做成常量：图片尺寸与抽帧成本随站点/主机差异很大，硬编码等于把站长的调优权拿走。
const (
	KeyThumbMaxEdge = "media.thumb_max_edge"
	KeyThumbSeekMS  = "media.thumb_seek_ms"
	KeyThumbQuality = "media.thumb_quality"
)

const (
	// DefaultThumbMaxEdge 缩略图长边上限默认值（像素）。列表/卡片展示足够；JPEG 单张通常几十 KB。
	DefaultThumbMaxEdge = 480
	// DefaultThumbSeekMS 视频抽帧位置上限默认值（毫秒）：跳过片头黑场/台标，避免缩略图全黑。
	DefaultThumbSeekMS = 1000
	// DefaultThumbQuality JPEG 质量默认值（2=最好 31=最差）。
	DefaultThumbQuality = 5
	// thumbTimeout 单次生成上限。抽帧偶发抖动，但绝不能挂住 HTTP 请求。
	thumbTimeout = 20 * time.Second
)

// 参数合法区间。与 handler/settings.go 的白名单校验同源（那边拒绝越界，这边夹取兜底）——
// 双重保护的理由：配置也可能经导入快照/直接改库写入，不能假定一定过白名单。
const (
	ThumbMaxEdgeMin, ThumbMaxEdgeMax = 64, 4096
	ThumbSeekMSMin, ThumbSeekMSMax   = 0, 60000
	ThumbQualityMin, ThumbQualityMax = 2, 31
)

// ThumbOptions 缩略图生成参数（B16 参数化，消灭原先散落的 480 / 1000 / -q:v 5 硬编码）。
type ThumbOptions struct {
	MaxEdge int   `json:"max_edge"` // 长边上限（像素），只缩不放
	SeekMS  int64 `json:"seek_ms"`  // 视频抽帧位置上限（毫秒）
	Quality int   `json:"quality"`  // JPEG 质量 2..31
}

// DefaultThumbOptions 内置默认参数（配置缺失、非法或越界时的回退值）。
func DefaultThumbOptions() ThumbOptions {
	return ThumbOptions{MaxEdge: DefaultThumbMaxEdge, SeekMS: DefaultThumbSeekMS, Quality: DefaultThumbQuality}
}

// Clamp 把越界参数夹到合法区间（防御手工构造的 ThumbOptions）。
func (o ThumbOptions) Clamp() ThumbOptions {
	o.MaxEdge = clampIntRange(o.MaxEdge, ThumbMaxEdgeMin, ThumbMaxEdgeMax, DefaultThumbMaxEdge)
	o.SeekMS = int64(clampIntRange(int(o.SeekMS), ThumbSeekMSMin, ThumbSeekMSMax, DefaultThumbSeekMS))
	o.Quality = clampIntRange(o.Quality, ThumbQualityMin, ThumbQualityMax, DefaultThumbQuality)
	return o
}

// clampIntRange 区间夹取。三种取值刻意区分开：
//
//	v <= 0 且下界 > 0 → 回退默认（0/负数对 MaxEdge/Quality 无意义，视为未配置）；
//	v < lo            → 夹到下界（站长配 1 的意图是「要小图」，回退默认等于吃掉他的输入）；
//	v > hi            → 夹到上界（同理，配 99999 的意图是「要清晰」）。
func clampIntRange(v, lo, hi, def int) int {
	if v <= 0 && lo > 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ThumbOptionsFrom 从配置中心读取缩略图参数（cfg 可为 nil → 全默认）。
func ThumbOptionsFrom(cfg cfgGetter) ThumbOptions {
	if cfg == nil {
		return DefaultThumbOptions()
	}
	return ThumbOptions{
		MaxEdge: intCfg(cfg, KeyThumbMaxEdge, DefaultThumbMaxEdge, ThumbMaxEdgeMin, ThumbMaxEdgeMax),
		SeekMS:  int64(intCfg(cfg, KeyThumbSeekMS, DefaultThumbSeekMS, ThumbSeekMSMin, ThumbSeekMSMax)),
		Quality: intCfg(cfg, KeyThumbQuality, DefaultThumbQuality, ThumbQualityMin, ThumbQualityMax),
	}
}

// intCfg 读一个整型配置：**空串 = 未配置** → 回退默认，与 capability.* 同一口径。
//
// 为什么用 GetString 而不是 GetInt：GetInt 对「键不存在」与「显式配置 0」返回同一个 0，
// 无法区分；而 media.thumb_seek_ms 的 0 是合法值（从第 0 帧抽帧），存在性必须靠字符串形态判。
func intCfg(cfg cfgGetter, key string, def, lo, hi int) int {
	v := strings.TrimSpace(cfg.GetString(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return clampIntRange(n, lo, hi, def)
}

// FFmpegAvailable 报告本机是否具备 ffmpeg（生成缩略图的前提）。
func FFmpegAvailable() bool {
	_, err := exec.LookPath(ffmpegBin)
	return err == nil
}

// ThumbSeekMS 计算视频抽帧位置：取 1/10 时长，夹在 [0, capMS]。
// capMS 由调用方传入（B16 起来自 media.thumb_seek_ms），非法值按 0 处理。
//
// 为什么不是固定第 1 秒：短片（<2s）会抽到空白尾帧；长片第 1 秒常是黑场/台标。
// 取 1/10 是「内容通常已经展开」的经验值，夹上限保证长片不会抽到中段而失去辨识度。
func ThumbSeekMS(durationMS, capMS int64) int64 {
	if durationMS <= 0 {
		return 0
	}
	if capMS < 0 {
		capMS = 0
	}
	s := durationMS / 10
	if s > capMS {
		s = capMS
	}
	return s
}

// DerivedPrefixOfKey 由派生对象 key 反推其所属前缀。
//
// 为什么需要：thumbnail_ref 里存的是**完整 key**（spaces/{sid}/.derived/{fid}/thumb.jpg），
// 而孤儿对账时源文件行已被删除、拿不到 space_id —— 若无此函数就只能删单个文件，
// 无法保证「同一文件将来新增的其它派生对象」也被一并清掉。
func DerivedPrefixOfKey(key string) string {
	if i := strings.LastIndex(key, "/"); i > 0 {
		return key[:i]
	}
	return key
}

// thumbScaleFilter 生成缩放滤镜表达式（maxEdge = 长边上限，B16 起来自配置）。
//
// 只缩不放：min(N, iw) 让宽度取「原宽与上限的较小者」，小图不会被放大
// （放大只会更糊且更大），高度 -2 表示按比例自动计算并对齐偶数（JPEG 要求）。
// 单引号是 ffmpeg filter 语法的一部分（保护表达式内的逗号），此处是**传给 argv 的字面量**，
// 不经过 shell，故不是引号转义问题 —— 实测 `-vf "scale='min(480,iw)':-2"` 有效。
func thumbScaleFilter(maxEdge int) string {
	return fmt.Sprintf("scale='min(%d,iw)':-2", maxEdge)
}

// MakeThumb 由源文件生成 JPEG 缩略图。
//
//	video=true  → 抽单帧（seekMS 为抽帧位置，调用方用 ThumbSeekMS 计算）
//	video=false → 按图片处理（image2 解码器覆盖 jpeg/png/webp/bmp，gif 取首帧）
//
// 返回 error 只表示**启动/执行失败**，且调用方约定「不落状态、下次重试」；
// ffmpeg 缺失时返回 ErrThumbNoFFmpeg（可用 errors.Is 区分）。
func MakeThumb(ctx context.Context, srcPath, dstPath string, video bool, seekMS int64, opt ThumbOptions) error {
	if strings.TrimSpace(srcPath) == "" || strings.TrimSpace(dstPath) == "" {
		return errors.New("thumb: empty src or dst path")
	}
	bin, err := exec.LookPath(ffmpegBin)
	if err != nil {
		return ErrThumbNoFFmpeg
	}
	opt = opt.Clamp() // 防御直接构造的越界值
	if seekMS < 0 {
		seekMS = 0
	}
	if seekMS > opt.SeekMS {
		seekMS = opt.SeekMS
	}
	// -nostdin：服务进程下必须加，否则 ffmpeg 可能尝试读 stdin 而挂住
	args := []string{"-v", "error", "-nostdin"}
	if video {
		// -ss 置于 -i 之前 = 关键帧级快速定位（不解码前半段），落点可能略偏前，对缩略图无妨
		args = append(args, "-ss", fmt.Sprintf("%.3f", float64(seekMS)/1000))
	}
	args = append(args, "-i", srcPath)
	if video {
		args = append(args, "-frames:v", "1")
	}
	// -q:v 5 ≈ JPEG 高质量（2=最好 31=最差）；-f image2 -y 显式覆盖
	args = append(args, "-vf", thumbScaleFilter(opt.MaxEdge), "-q:v", strconv.Itoa(opt.Quality), "-f", "image2", "-y", dstPath)

	cctx, cancel := context.WithTimeout(ctx, thumbTimeout)
	defer cancel()
	if out, err := exec.CommandContext(cctx, bin, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("thumb: ffmpeg: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
