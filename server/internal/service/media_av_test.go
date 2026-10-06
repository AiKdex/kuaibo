// media_av_test.go 音视频探测（B13）回归。
//
// 覆盖点：
//  1. 二进制缺失时**安全降级** —— 只返回 ok=false，不 panic、不返回 error
//     （这是"能力可降级"的底线：没装 ffprobe 的机器上整个链路必须照常工作）；
//  2. 非法输入（空路径 / 不存在文件）不 panic；
//  3. 环境齐备（ffprobe + ffmpeg）时真机探测：时长/宽高/编码与生成参数一致。
//     本机通常无 ffmpeg → 自动 skip；部署机装了 → 真跑。
package service

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestProbeAVMissingBinaryDegradesQuietly 二进制缺失必须安静降级。
func TestProbeAVMissingBinaryDegradesQuietly(t *testing.T) {
	old := ffprobeBin
	ffprobeBin = filepath.Join(t.TempDir(), "definitely-not-a-ffprobe")
	t.Cleanup(func() { ffprobeBin = old })

	if _, ok := ProbeAV(context.Background(), "/etc/hostname"); ok {
		t.Fatal("ffprobe 缺失时应返回 ok=false（安静降级），不能误判成功")
	}
}

// TestProbeAVRejectsBadInput 非法输入不得 panic。
func TestProbeAVRejectsBadInput(t *testing.T) {
	if _, ok := ProbeAV(context.Background(), ""); ok {
		t.Fatal("空路径应返回 ok=false")
	}
	if _, err := exec.LookPath(ffprobeBin); err != nil {
		t.Skip("环境无 ffprobe，跳过「不存在文件」分支")
	}
	if _, ok := ProbeAV(context.Background(), filepath.Join(t.TempDir(), "nope.mp4")); ok {
		t.Fatal("不存在的文件应返回 ok=false")
	}
}

// TestProbeAVRealMedia 真机探测（需要 ffprobe + ffmpeg，否则 skip）。
func TestProbeAVRealMedia(t *testing.T) {
	if _, err := exec.LookPath(ffprobeBin); err != nil {
		t.Skip("环境无 ffprobe，跳过真机探测")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("环境无 ffmpeg，无法生成测试样本")
	}
	dir := t.TempDir()

	// 1) 视频：320x240 / 1s / yuv420p（ffmpeg 默认 h264）
	mp4 := filepath.Join(dir, "sample.mp4")
	if out, err := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=10",
		"-pix_fmt", "yuv420p", mp4).CombinedOutput(); err != nil {
		t.Fatalf("生成视频样本失败: %v %s", err, out)
	}
	info, ok := ProbeAV(context.Background(), mp4)
	if !ok {
		t.Fatal("对合法 mp4 探测失败")
	}
	if info.Width != 320 || info.Height != 240 {
		t.Fatalf("宽高 = %dx%d, want 320x240", info.Width, info.Height)
	}
	if info.DurationMS < 900 || info.DurationMS > 1200 {
		t.Fatalf("时长 = %dms, want ~1000ms", info.DurationMS)
	}
	if !strings.Contains(info.Codec, "264") {
		t.Fatalf("编码 = %q, want 含 264", info.Codec)
	}
	if info.Bitrate <= 0 {
		t.Fatalf("码率 = %d, want > 0", info.Bitrate)
	}

	// 2) 纯音频：视频字段必须为 0（否则主题会把音频当视频渲染），编码取音频流
	wav := filepath.Join(dir, "sample.wav")
	if out, err := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1", wav).CombinedOutput(); err != nil {
		t.Fatalf("生成音频样本失败: %v %s", err, out)
	}
	ainfo, ok := ProbeAV(context.Background(), wav)
	if !ok {
		t.Fatal("对合法 wav 探测失败")
	}
	if ainfo.Width != 0 || ainfo.Height != 0 {
		t.Fatalf("纯音频宽高应为 0，得 %dx%d", ainfo.Width, ainfo.Height)
	}
	if ainfo.Codec == "" {
		t.Fatal("纯音频也应给出编码名")
	}
	if ainfo.DurationMS < 900 || ainfo.DurationMS > 1200 {
		t.Fatalf("音频时长 = %dms, want ~1000ms", ainfo.DurationMS)
	}
}
