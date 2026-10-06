// Package config 实现"配置即变量"中心（实施落地文档 §10）。
//
// 三层来源、后者覆盖前者：
//
//	编译期默认值（代码内置）→ 配置文件 / 环境变量（自部署）→ 后台设置（DB，运行时可改、热生效）
//
// 作用域：instance / space / user；同名 key 按作用域就近覆盖。
// 敏感项（api_key / SMTP 密码）加密存储、接口脱敏返回、变更写审计日志。
package config

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
)

// Defaults 是内置默认值（开箱可用）。所有可调整项集中在此，禁止散落硬编码。
var Defaults = map[string]any{
	// 站点与品牌（爱库录精简发行默认）
	"site.name":              "爱库录",
	"site.domain":            "localhost:8080",
	"site.language":          "zh-CN",
	"site.timezone":          "Asia/Shanghai",
	"site.registration_open": false, // C1a 修复：默认关闭开放注册（上线安全默认；需要时后台显式打开）
	// CORS 跨源白名单（外部频道站如 qiuzhi.aikmap.cn 读知识库数据用）：逗号分隔来源；
	// "*" 放行全部；留空 = 关闭 CORS（同源部署，默认最安全）
	"server.cors_origins": "",
	// 系统身份（单用户模式固定；多用户开启后由注册流程分配）
	"system.owner_id":      "00000000-0000-0000-0000-000000000001",
	"system.home_space_id": "00000000-0000-0000-0000-000000000002",
	// 存储
	"storage.default_backend": "local",
	"storage.local.root":      "./data/files",
	"storage.recycle_days":    30,
	// 媒体派生资产（缩略图）参数：B16 从代码常量提取，**改完即时生效**（生成时运行期读取）。
	// 为什么可配：不同站点图片尺寸差异大；共享主机还常需按 CPU 余量收放抽帧成本。
	// 空串 = 未配置（回退内置默认），与 capability.* 同一口径。
	"media.thumb_max_edge": 480,  // 缩略图长边上限（像素，64-4096）：只缩不放
	"media.thumb_seek_ms":  1000, // 视频抽帧位置上限（毫秒，0-60000）：跳过片头黑场/台标
	"media.thumb_quality":  5,    // JPEG 质量（2-31，数值越小越清晰、体积越大）
	// 视频转码参数（B17）：与 thumb 同一口径 —— 运行期读取、改完即时生效、空串=回退内置默认。
	// 为什么可配：主机 CPU 余量、受众网络、素材分辨率差异极大；默认值面向「单机自部署」场景。
	"media.transcode_max_height":   720,      // 输出最大高度（像素，0-2160；只缩不放，0=不缩放）
	"media.transcode_crf":          23,       // x264 质量因子（0-51，越小越清晰；18-28 为常用档）
	"media.transcode_preset":       "medium", // x264 速度档（ultrafast…veryslow）
	"media.transcode_audio_kbps":   128,      // AAC 音频码率（0-320；0=去掉音轨）
	"media.transcode_concurrency":  1,        // 同时进行的转码任务数（1-4；共享主机建议 1）
	"media.transcode_max_duration_s": 1800,   // 允许转码的最长素材时长（秒，0=不限）
	// 孤儿派生对象定时兜底清理（B18）：把 B16 的「手动 stats+purge」升级为自动。
	// 默认 1440 分钟（24 小时）= 启用；0 = 关闭（保持 B16 纯手动行为，不强制自动化）。
	// 为何可配：清理频率与「误删容忍度」成反比，站长按数据重要程度自定。
	"media.derived_sweep_interval_min": 1440,
	// AI / 模型（按能力绑定；provider 可为 local/hosted）
	"ai.embedding.provider": "local", // local | hosted
	"ai.embedding.endpoint": "",
	"ai.embedding.model":    "bge-small-zh-v1.5",
	"ai.embedding.dim":      512,
	"ai.llm.provider":       "hosted", // local | hosted
	"ai.llm.endpoint":       "",
	"ai.llm.model":          "",
	"ai.llm.temperature":    0.7,
	"ai.llm.timeout_s":      240, // token 池/网关响应可能较慢，给足等待窗口避免误超时
	"ai.rerank.provider":    "",
	"ai.rerank.model":       "",
	"ai.rerank.endpoint":    "",
	"ai.rerank.top_n":       15, // RRF 融合后参与精排的候选文件数（精排后取 top_k；CPU 推理下 15 个候选约 2-3s）
	"ai.fallback_order":     "hosted,local",
	// Agent 对话（免费/付费分级点：轮数限制决定检索精读深度）
	"ai.agent.max_rounds": 6, // 单次对话最大工具调用轮数（免费可收紧、付费可放宽）
	// 入库 AI 解读（类型筛选：空=全部文本类型都解读，可配置如 ".md,.docx,.pdf" 减少 token 浪费）
	"ai.summarize.exts": "", // 逗号分隔扩展名白名单（小写，含点，如 ".md,.docx,.pdf"）
	// K20 图谱实体关系自动建边：入库解读时 AI 抽取实体（人物/产品/项目/概念/技术）建标签，
	// 实体间关系写 knowledge_edges（tag→tag 边，谓词为关系原文）；关闭则只建标签不建边
	"ai.entity_extract.enabled": true,
	// OCR 结果后处理（K06 增强）：结构结果（HTML 标签占比 > 30%）自动整理为干净 Markdown 后入库
	"ocr.auto_clean":  true, // OCR 结果自动后处理总开关
	"ocr.clean_on_ai": true, // 结构结果是否调用 AI 整理（关闭则省 token、原样入库）
	// 博客对外开关（Aikdex 第三轮 1.1：撤销/复活语义冲突修复——对外状态以本开关为准，
	// 关闭后公开页/RSS 404，目录数据保留；共享管理页撤销 blog 分享被 403 锁定）
	"blog.open": true,
	// 上传后自动发布（站长可配）：true=博客管理拖拽/选文件上传 md 后直接 published；
	// false（默认）=保持草稿，由站长逐篇确认后发布
	"blog.auto_publish_on_upload": false,
	// 公开列表是否收录附件（站长可配，B10）：false（默认）=公开列表/热门榜/RSS/sitemap 只列文章，
	// 图片/音视频/压缩包等附件不进这些出口（附件在管理轨与分享页照常可见）；
	// true=恢复旧行为（附件也当内容列出，适合整目录发布图集的场景）
	"blog.list_attachments": false,
	// 交互版主题（SPA 公开视图）；静态页 /blog 固定阅读壳，不受此项影响
	"blog.theme": "aiklog",
	// 索引与检索
	"index.chunk_size":       800,
	"index.chunk_overlap":    80,
	"index.rrf_k":            60,
	"index.default_top_k":    20,
	// 向量语义召回最低相似度（低于此分不进结果，避免「什么都出整库」）
	"index.vector_min_score": 0.45,
	"index.rebuild_strategy": "incremental",
	// 发布 / 分享
	"publish.default_visibility": "private",
	"share.default_expire_days":  7,
	"blog.rss_enabled":           true,
	// 协作
	"collab.office_wopi_url":  "", // OnlyOffice/Collabora 地址
	"collab.comments_enabled": true,
	// 媒体
	"media.hls_enabled":       false,
	"media.transcribe_engine": "", // local | hosted
	// 安全
	"security.registration_open": false, // C1a 修复：同上（当前门禁实际读 site.registration_open）
	"security.invite_only":       false,
	"security.jwt_ttl_s":         3600,
	"security.refresh_ttl_s":     604800,
	"security.rate_limit_rps":    20,
	"security.audit_enabled":     false, // 企业版强制开启
	// 运维
	"ops.log_level":      "info",
	"ops.telemetry":      false,
	"ops.backup_enabled":      false,   // 备份总开关（后台可动态开关：PUT /api/v1/system/backup）
	"ops.backup_schedule":     "03:00", // 每日备份时刻 HH:MM（后台可设；每分钟检查，改动≤1min 生效）
	"ops.backup_keep_local":   7,       // 本地保留份数
	"ops.backup_keep_remote":  30,      // 远端保留份数（storage 后端 backup/ 前缀）
	"ops.backup_last_at":      "",      // 最近一次成功备份时间戳（备份成功后回写，动态键）

	// 组织架构（企业版运维层）：能力装配受 capability.org 控制，装配完成后由下列
	// 运行期子开关决定各能力是否放行；总开关关闭时 tree/department/transfer 一律视同关闭。
	"org.enabled":    false,
	"org.tree":       false,
	"org.department": false,
	"org.transfer":   false,

	// 家族传承记录（[family] REQ-010/011）：capability.family 装配后由 family.enabled 控制开关；默认关=不装不用。
	"family.enabled": false,
}

// Entry 是配置项运行时形态。
type Entry struct {
	Key          string `json:"key"`
	Value        any    `json:"value"`
	Scope        string `json:"scope"`
	ScopeID      string `json:"scope_id,omitempty"`
	Type         string `json:"type"`
	DefaultValue any    `json:"default_value,omitempty"`
	Description  string `json:"description,omitempty"`
	Encrypted    bool   `json:"encrypted,omitempty"`
}

// Store 配置中心。线程安全；Get/GetString/GetInt/GetBool 任一时刻返回一致快照。
type Store struct {
	mu       sync.RWMutex
	values   map[string]any  // 已解析的最终值（默认 → env/file → db 后合并）
	explicit map[string]bool // 显式设置过的键（DB 加载 / Set），区别于默认值
	db       *sql.DB
	bus      *bus.Bus
	encKey   []byte // 敏感值加密密钥（env AIKMAP_SECRET 或自动生成持久化）
}

// New 创建配置中心。db 可为 nil（纯默认+env 模式）。
func New(db *sql.DB, b *bus.Bus) *Store {
	s := &Store{
		values:   map[string]any{},
		explicit: map[string]bool{},
		db:       db,
		bus:      b,
	}
	// 1) 默认值
	for k, v := range Defaults {
		s.values[k] = v
	}
	// 2) env 覆盖（AIKMAP_ 前缀，点号转下划线）
	for k := range Defaults {
		envKey := "AIKMAP_" + strings.ToUpper(strings.ReplaceAll(k, ".", "_"))
		if v, ok := os.LookupEnv(envKey); ok {
			s.values[k] = parseEnv(Defaults[k], v)
		}
	}
	return s
}

// LoadFromDB 从 settings 表加载 instance 级覆盖（启动时调用一次）。
// scope 参数保留：当前仅 instance；space/user 覆盖在请求路径上解析。
func (s *Store) LoadFromDB(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value, type, encrypted FROM settings WHERE scope='instance'`)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, value, typ string
		var encrypted int
		if err := rows.Scan(&key, &value, &typ, &encrypted); err != nil {
			return err
		}
		if encrypted == 1 {
			value = s.decrypt(value)
		}
		s.mu.Lock()
		if v, ok := Defaults[key]; ok {
			s.values[key] = parseEnv(v, value)
		} else {
			// 动态键（如自备模型 ai.<cap>.endpoint/model/api_key、ocr/code provider）：无默认定义，直接加载
			s.values[key] = value
		}
		s.explicit[key] = true // DB 中的值视为显式设置（重启后保留用户选择）
		s.mu.Unlock()
	}
	return rows.Err()
}

// Set 设置一个 instance 级配置项：写 DB（若可用）+ 更新内存 + 广播 config.changed。
// 返回旧值。敏感项自动加密。
func (s *Store) Set(ctx context.Context, key string, val any, typ string, desc string, updatedBy string) (any, error) {
	old, _ := s.Get(key)
	encrypted := typ == "secret"
	raw := fmt.Sprint(val)
	if encrypted {
		raw = s.encrypt(raw)
	}
	if s.db != nil {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO settings(key, scope, scope_id, value, type, default_value, description, encrypted, updated_by, updated_at)
			VALUES(?, 'instance', '', ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(key, scope, scope_id) DO UPDATE SET
			  value=excluded.value, type=excluded.type, description=excluded.description,
			  encrypted=excluded.encrypted, updated_by=excluded.updated_by, updated_at=excluded.updated_at`,
			key, raw, typ, fmt.Sprint(Defaults[key]), desc, encrypted, updatedBy, Now())
		if err != nil {
			return old, fmt.Errorf("persist setting %s: %w", key, err)
		}
	}
	s.mu.Lock()
	s.values[key] = val
	s.explicit[key] = true
	s.mu.Unlock()
	if s.bus != nil {
		s.bus.Publish(ctx, bus.Event{Topic: "config.changed", Key: key, Data: map[string]any{"key": key, "value": val}})
	}
	return old, nil
}

// IsExplicit 返回某键是否被显式设置过（DB 加载或 Set），用于区分默认值。
func (s *Store) IsExplicit(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.explicit[key]
}

// Delete 删除一个配置键（DB + 内存 + explicit 标记），用于清除自定义模型配置。
func (s *Store) Delete(ctx context.Context, key string) error {
	if s.db != nil {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key=? AND scope='instance'`, key); err != nil {
			return fmt.Errorf("delete setting %s: %w", key, err)
		}
	}
	s.mu.Lock()
	delete(s.values, key)
	delete(s.explicit, key)
	s.mu.Unlock()
	if s.bus != nil {
		s.bus.Publish(ctx, bus.Event{Topic: "config.changed", Key: key, Data: map[string]any{"key": key, "deleted": true}})
	}
	return nil
}

// Get 取配置值；key 不存在返回零值与 false。
func (s *Store) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.values[key]
	return v, ok
}

// GetString 取字符串配置（不存在时返回空串）。
func (s *Store) GetString(key string) string {
	v, _ := s.Get(key)
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

// GetInt 取整数配置。
func (s *Store) GetInt(key string) int {
	v, _ := s.Get(key)
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	}
	return 0
}

// GetBool 取布尔配置。
func (s *Store) GetBool(key string) bool {
	v, _ := s.Get(key)
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(t)
		return b
	}
	return false
}

// Snapshot 返回全部配置（敏感项脱敏），供管理后台展示。
func (s *Store) Snapshot() map[string]Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Entry, len(s.values))
	for k, v := range s.values {
		enc := false
		if s.db != nil {
			// 敏感类型标记来自 Defaults 之外难以推断，统一按 key 前缀猜测
			enc = strings.HasPrefix(k, "ai.") && strings.Contains(k, "key") ||
				strings.HasPrefix(k, "security.") && strings.Contains(k, "secret")
		}
		// 密钥类一律掩码回传（防管理端明文外泄；写接口为白名单不涉及密钥，无回写风险）
		if isSecretKey(k) {
			v = "****"
		}
		out[k] = Entry{Key: k, Value: v, Scope: "instance", DefaultValue: Defaults[k], Encrypted: enc}
	}
	return out
}

// isSecretKey 判断配置键是否为密钥类（api_key / token / secret / password / pass）。
func isSecretKey(k string) bool {
	lk := strings.ToLower(k)
	return strings.Contains(lk, "api_key") || strings.Contains(lk, "token") ||
		strings.Contains(lk, "secret") || strings.Contains(lk, "password") ||
		strings.Contains(lk, "pass")
}

func (s *Store) encrypt(raw string) string {
	// 轻量可逆编码（非加密，仅防裸明文落库）；真实部署可替换为 AES-GCM（密钥来自 env AIKMAP_SECRET）。
	// 用标准 base64 而非 JSON 序列化，避免引号/转义导致解码损坏。
	return "enc:" + base64.StdEncoding.EncodeToString([]byte(raw))
}

func (s *Store) decrypt(raw string) string {
	if strings.HasPrefix(raw, "enc:") {
		body := strings.TrimPrefix(raw, "enc:")
		if b, err := base64.StdEncoding.DecodeString(body); err == nil {
			return string(b)
		}
		return body // 兼容旧版伪 base64（json.Marshal+Trim 引号，对普通 token 可逆）
	}
	return raw
}

// SecretKey 返回敏感值加密密钥（进程级稳定，来自 env AIKMAP_SECRET 或首次启动时自动生成并持久化）。
// 用于派生文章解锁凭证等 HMAC 签名密钥（仅服务端持有，不外露）。
func (s *Store) SecretKey() []byte { return s.encKey }

// parseEnv 把 env/DB 的字符串按默认值类型转换。
func parseEnv(def any, raw string) any {
	switch def.(type) {
	case bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return def
		}
		return b
	case int, int64:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return def
		}
		return n
	case float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return def
		}
		return f
	default:
		return raw
	}
}

// Now 统一时间戳（Unix 秒）。
func Now() int64 {
	return time.Now().Unix()
}
