// media.go 媒体元信息（B8 媒体底座）。
//
// 落地范围（有意收窄）：只做**不依赖外部二进制**的部分 —— 图片宽高走 Go 标准库
// image.DecodeConfig 只解 header，不解全图（省内存、微秒级）。
//
// 为什么不做转码/缩略图：服务器上没有 ffmpeg/ffprobe（已实测），且上游 media.go /
// media_worker.go / transcriber.go **在本壳可获取的上游源码里并不存在** ——
// 方案文档里的 B8 清单同样是"假设"。所以本批把**表结构与只读链路**先落定，
// 音视频统一记 probe_status='unsupported'，等 ffmpeg 就位后由 worker 补齐字段即可，
// **不需要二次迁移表结构**。
//
// 探测策略 = 懒探测：首次读取该文件媒体信息时探测并写缓存，零侵入上传链路。
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"image"
	_ "image/gif"  // 注册 GIF 解码器（DecodeConfig 只读 header）
	_ "image/jpeg" // 注册 JPEG
	_ "image/png"  // 注册 PNG
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// 探测状态取值。
const (
	MediaPending     = "pending"     // 尚未探测
	MediaOK          = "ok"          // 探测成功（图片宽高 / 音视频时长·分辨率·编码）
	MediaUnsupported = "unsupported" // 当前环境无法探测（未装 ffprobe，或存储后端非本地盘）
	MediaFailed      = "failed"      // 探测报错
)

// MediaInfo 媒体元信息（与上游 file_media 预留位字段一一对应）。
type MediaInfo struct {
	FileID        string `json:"file_id"`
	DurationMS    int64  `json:"duration_ms"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Codec         string `json:"codec"`
	Bitrate       int64  `json:"bitrate"`
	ThumbnailRef  string `json:"thumbnail_ref"`
	TranscodedRef string `json:"transcoded_ref"`
	// B17 转码状态（与 transcoded_ref 配套）：status 取值域见 transcode_job.go；
	// 空串表示「从未发起过转码」，与 failed 区分开（后者代表跑过但失败）。
	TranscodeStatus    string `json:"transcode_status"`
	TranscodeMsg       string `json:"transcode_msg"`
	TranscodeSignature string `json:"transcode_signature"`
	TranscodeBytes     int64  `json:"transcode_bytes"`
	TranscodeHeight    int    `json:"transcode_height"`
	TranscodeUpdatedAt int64  `json:"transcode_updated_at"`
	ProbeStatus        string `json:"probe_status"`
	UpdatedAt          int64  `json:"updated_at"`
}

// CarryDerived 把派生资产相关字段从上一条记录带过来。
//
// 🔴 为什么必须显式带：Upsert 是**全列覆盖**，而 probe_status='unsupported' / 'pending'
// 都不是终态（会被反复重探）—— 重探时若不带派生字段，会把已生成的缩略图引用与
// 转码状态一并清空，表现为「缩略图生成过一次后又没了」「转码完成了又显示未发起」。
func (m *MediaInfo) CarryDerived(prev *MediaInfo) {
	if m == nil || prev == nil {
		return
	}
	m.ThumbnailRef = prev.ThumbnailRef
	m.TranscodedRef = prev.TranscodedRef
	m.TranscodeStatus = prev.TranscodeStatus
	m.TranscodeMsg = prev.TranscodeMsg
	m.TranscodeSignature = prev.TranscodeSignature
	m.TranscodeBytes = prev.TranscodeBytes
	m.TranscodeHeight = prev.TranscodeHeight
	m.TranscodeUpdatedAt = prev.TranscodeUpdatedAt
}

// MediaStore 媒体元信息数据访问。
type MediaStore struct{ db *sql.DB }

// NewMediaStore 创建媒体仓储。
func NewMediaStore(db *sql.DB) *MediaStore { return &MediaStore{db: db} }

// Get 读取已缓存的媒体元信息。
func (s *MediaStore) Get(ctx context.Context, fileID string) (*MediaInfo, bool) {
	var m MediaInfo
	err := s.db.QueryRowContext(ctx,
		`SELECT file_id, duration_ms, width, height, codec, bitrate, thumbnail_ref, transcoded_ref,
		        transcode_status, transcode_msg, transcode_signature, transcode_bytes, transcode_height,
		        transcode_updated_at, probe_status, updated_at
		 FROM file_media WHERE file_id=?`, fileID).
		Scan(&m.FileID, &m.DurationMS, &m.Width, &m.Height, &m.Codec, &m.Bitrate,
			&m.ThumbnailRef, &m.TranscodedRef,
			&m.TranscodeStatus, &m.TranscodeMsg, &m.TranscodeSignature, &m.TranscodeBytes,
			&m.TranscodeHeight, &m.TranscodeUpdatedAt, &m.ProbeStatus, &m.UpdatedAt)
	if err != nil {
		return nil, false
	}
	return &m, true
}

// Upsert 写入/更新媒体元信息。
func (s *MediaStore) Upsert(ctx context.Context, m *MediaInfo) error {
	if m.ProbeStatus == "" {
		m.ProbeStatus = MediaPending
	}
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO file_media
		   (file_id, duration_ms, width, height, codec, bitrate, thumbnail_ref, transcoded_ref,
		    transcode_status, transcode_msg, transcode_signature, transcode_bytes, transcode_height,
		    transcode_updated_at, probe_status, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(file_id) DO UPDATE SET
		   duration_ms=excluded.duration_ms, width=excluded.width, height=excluded.height,
		   codec=excluded.codec, bitrate=excluded.bitrate,
		   thumbnail_ref=excluded.thumbnail_ref, transcoded_ref=excluded.transcoded_ref,
		   transcode_status=excluded.transcode_status, transcode_msg=excluded.transcode_msg,
		   transcode_signature=excluded.transcode_signature, transcode_bytes=excluded.transcode_bytes,
		   transcode_height=excluded.transcode_height, transcode_updated_at=excluded.transcode_updated_at,
		   probe_status=excluded.probe_status, updated_at=excluded.updated_at`,
		m.FileID, m.DurationMS, m.Width, m.Height, m.Codec, m.Bitrate,
		m.ThumbnailRef, m.TranscodedRef,
		m.TranscodeStatus, m.TranscodeMsg, m.TranscodeSignature, m.TranscodeBytes,
		m.TranscodeHeight, m.TranscodeUpdatedAt, m.ProbeStatus, now)
	if err != nil {
		return err
	}
	m.UpdatedAt = now
	return nil
}

// PurgeFile 删除某文件的媒体记录（文件彻底删除时调用）。
func (s *MediaStore) PurgeFile(ctx context.Context, fileID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM file_media WHERE file_id=?`, fileID)
	return err
}

// ProbeImage 只解图片 header 取宽高（不解全图）。支持 jpeg/png/gif；失败返回 ok=false。
func ProbeImage(r io.Reader) (w, h int, format string, ok bool) {
	cfg, format, err := image.DecodeConfig(r)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, "", false
	}
	return cfg.Width, cfg.Height, format, true
}

// ffprobeBin 探测所需的二进制名。声明为变量以便测试注入「不存在的路径」，
// 验证二进制缺失时必须**安全降级**而不是报错。
var ffprobeBin = "ffprobe"

// avProbeTimeout 单次探测上限（本地文件正常在 10ms 量级；坏文件不应挂住 HTTP 请求）。
const avProbeTimeout = 5 * time.Second

// AVInfo 音视频探测结果（ProbeAV 的返回值，字段与 file_media 预留列一一对应）。
type AVInfo struct {
	DurationMS int64  // 时长（毫秒）
	Width      int    // 视频宽（纯音频为 0）
	Height     int    // 视频高（纯音频为 0）
	Codec      string // 视频优先取视频流编码；纯音频取音频流编码
	Bitrate    int64  // 整体码率（bps）
}

// ffprobeOutput ffprobe -print_format json 的输出子集（只取要落库的字段）。
//
// 注意：duration 与 bit_rate 在 ffprobe 的 JSON 里是**字符串**（可能为 "N/A"），
// 声明成数字会让整个 Unmarshal 失败 —— 这是最容易踩的一个坑。
type ffprobeOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

// ProbeAV 用 ffprobe 探测音视频元信息（时长 / 分辨率 / 编码 / 码率）。
//
// 为什么必须走外部二进制：容器格式（mp4/mkv/mov）没有「解 header 就够」的通用做法，
// moov 原子可能位于文件尾，纯流式读取不可靠；ffprobe 是事实标准。
//
// 边界与降级（有意设计，不是缺陷）：
//   - 需要**宿主机路径**（随机访问），故只对本地磁盘后端有意义；非本地后端由调用方提前拦下；
//   - ffprobe 不在 PATH 时返回 ok=false，调用方记 probe_status='unsupported'。
//     装好 ffmpeg 后**无需清缓存**即可自动生效 —— unsupported 不是终态，见 handler 侧缓存条件；
//   - 参数以 args 数组传递、路径不参与 shell 拼接 → 无命令注入面；
//   - 有超时保护，损坏/超大文件不会挂死请求。
func ProbeAV(ctx context.Context, path string) (*AVInfo, bool) {
	if path == "" {
		return nil, false
	}
	bin, err := exec.LookPath(ffprobeBin)
	if err != nil {
		return nil, false // 环境不具备：安静降级，不返回 error
	}
	cctx, cancel := context.WithTimeout(ctx, avProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(cctx, bin,
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams",
		"-i", path,
	).Output()
	if err != nil || len(out) == 0 {
		return nil, false
	}
	var raw ffprobeOutput
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, false
	}
	info := &AVInfo{}
	hasAV := false
	for _, st := range raw.Streams {
		switch st.CodecType {
		case "video":
			hasAV = true
			// 只认第一条视频流（多角度/多轨场景取主轨，与播放器默认行为一致）
			if info.Width == 0 && st.Width > 0 {
				info.Width, info.Height = st.Width, st.Height
				if info.Codec == "" {
					info.Codec = st.CodecName
				}
			}
		case "audio":
			hasAV = true
			if info.Codec == "" { // 纯音频：没有视频流时用音频编码补位
				info.Codec = st.CodecName
			}
		}
	}
	if !hasAV {
		return nil, false // 既无视频也无音频流（如纯字幕/数据流）→ 无可展示元信息
	}
	if d, err := strconv.ParseFloat(strings.TrimSpace(raw.Format.Duration), 64); err == nil && d > 0 {
		info.DurationMS = int64(d*1000 + 0.5)
	}
	if b, err := strconv.ParseInt(strings.TrimSpace(raw.Format.BitRate), 10, 64); err == nil && b > 0 {
		info.Bitrate = b
	}
	return info, true
}

// ListByIDs 批量读取已缓存的媒体元信息（返回 map[fileID]MediaInfo）。
//
// 有意**不触发探测/生成**：批量端点若逐个 ffprobe，一次列表请求可能拉起上百个子进程
// （共享生产机上不可接受）。调用方应把本方法返回视为「已知的全部」，
// 未命中者由前端按需（逐个、且按可见性）再取。
func (s *MediaStore) ListByIDs(ctx context.Context, ids []string) (map[string]*MediaInfo, error) {
	out := make(map[string]*MediaInfo, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT file_id, duration_ms, width, height, codec, bitrate, thumbnail_ref, transcoded_ref,
		        transcode_status, transcode_msg, transcode_signature, transcode_bytes, transcode_height,
		        transcode_updated_at, probe_status, updated_at
		 FROM file_media WHERE file_id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m MediaInfo
		if err := rows.Scan(&m.FileID, &m.DurationMS, &m.Width, &m.Height, &m.Codec, &m.Bitrate,
			&m.ThumbnailRef, &m.TranscodedRef,
			&m.TranscodeStatus, &m.TranscodeMsg, &m.TranscodeSignature, &m.TranscodeBytes,
			&m.TranscodeHeight, &m.TranscodeUpdatedAt, &m.ProbeStatus, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out[m.FileID] = &m
	}
	return out, rows.Err()
}

// OrphanRef 是一条已失去源文件的媒体记录（对账清理的目标）。
//
// B17：把 transcoded_ref 也带上 —— 否则「只有转码产物、没有缩略图」的文件
// （纯视频且从未取过缩略图）在对账清理时拿不到引用，派生目录会永久残留。
// 两个引用同处 spaces/{sp}/.derived/{fid}/ 下，调用方按前缀去重后清一次即可。
type OrphanRef struct {
	FileID        string
	ThumbnailRef  string
	TranscodedRef string
}

// DeleteOrphans 清理「源文件已不在 files 表」的媒体记录，返回被清理的引用。
//
// 定位：**兜底对账**。Purge 的子表清扫（service/file.go）自 B8 起就包含 file_media，
// 所以正常删除路径不会留下孤儿行；本函数覆盖的是 files 行以其它途径消失（组织/空间
// 级联删除、导入器回滚、手工改库）而 file_media 行残留的情形。
//
// 注意口径：files 表里**软删**（deleted_at）的行仍在表中，故软删文件（可恢复）的
// 媒体记录**不会**被误清 —— 只有真正消失的才会成为孤儿。
//
// 返回的 ThumbnailRef 供调用方按前缀清派生对象，但要清楚它**只对仍存在的行有效** ——
// 行若已被 Purge 的子表清扫删掉，这里就再也拿不到引用（B15 实测）。派生对象的清理因此
// 必须放在 Purge 内部、删行之前完成，本函数只做兜底。
func (s *MediaStore) DeleteOrphans(ctx context.Context) ([]OrphanRef, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT file_id, thumbnail_ref, transcoded_ref FROM file_media
		  WHERE file_id NOT IN (SELECT id FROM files)`)
	if err != nil {
		return nil, err
	}
	var out []OrphanRef
	for rows.Next() {
		var o OrphanRef
		if err := rows.Scan(&o.FileID, &o.ThumbnailRef, &o.TranscodedRef); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM file_media WHERE file_id NOT IN (SELECT id FROM files)`); err != nil {
		return nil, err
	}
	return out, nil
}
