// capabilities.go 能力门控判定（共享实现）。
//
// 由 cmd/aikmap/capabilities.go 提取（B14）。提取原因：「启动装配」与「设置页运维面板」
// 必须用同一套判定，否则面板显示的生效态会与真实装配结果漂移。
//
// 判定优先级（高 → 低）：
//  1. 站长显式配置 settings `capability.<name>`（"true"/"false"；空串视为未配置）；
//  2. 应用中心记录 blog_plugins（capability_mode='capacity' 或 kind='capacity'，
//     或 capabilities JSON 数组命中能力名）的 enabled；
//  3. 内置默认：true。
//
// 注意（🔴 历史坑）：第 2 步**不能**用 enabled=1 过滤 —— 那会让「已停用的应用」查不到
// 记录，从而退回第 3 步的内置默认 true，表现为「停用却熄不灭能力」。正确口径是
// 命中即以其 enabled 为准，多条记录取「任一启用即启用」（ORDER BY enabled DESC）。
package service

import (
	"database/sql"
	"strings"
)

// 能力名。
//
// 前六项与 cmd/aikmap/buildCoreModules 的**装配**一一对应（关掉 → 路由不注册 → 404，
// 须重启生效）；CapThumb 是**运行期**开关（B15）：不参与装配，只在处理请求时就地判定，
// 改完立即生效 —— 见 CapabilityApply。
const (
	CapCollector = "collector"
	CapWebDAV    = "webdav"
	CapDigest    = "digest"
	CapInbox     = "inbox"
	CapReview    = "review"
	CapOrg       = "org"
	// CapFamily 家族传承（传家365 同族）：应用中心可安装的垂直能力应用。
	// 与 org 同属「装了才启用」语义（capabilityDefault），默认不装配——
	// 侧栏不出现入口、路由不注册；安装内置应用（com.aiklog.app-family）后重启生效。
	CapFamily = "family"
	// CapThumb 媒体缩略图生成（B15）：关闭后不调 ffmpeg、缩略图端点 404。
	// 单列一项的理由：抽帧是**外部进程 + CPU/磁盘**开销，共享主机上需要一个随时可关的闸门。
	CapThumb = "thumb"
	// CapTranscode 视频转码（B17）：关闭后不接受新任务、公开播放回落到原文件。
	// 与 CapThumb 分开的理由：转码的 CPU/磁盘/耗时长一个量级，站长很可能想留缩略图但关掉转码。
	CapTranscode = "transcode"
	// 客服模块（SPEC-CS-001）：四项独立闸门，默认关（装了才启用）。
	//   inbox   —— 收件箱/会话（挂件与渠道消息的读写都经它）
	//   contacts—— 客户档案与身份图谱（inbox 的前置，单独可关用于只读历史）
	//   channels—— 外部渠道适配与出站外推（关掉=只收不发/不接 webhook）
	//   ai_draft—— AI 起草回复（依赖 AI 配额，单独可关避免模型开销）
	CapCSInbox    = "cs.inbox"
	CapCSContacts = "cs.contacts"
	CapCSChannels = "cs.channels"
	CapCSAIDraft  = "cs.ai_draft"
	// CapStore 商城（B39：WooCommerce 式商品/购物车/收银台/订单/优惠券）；默认开启。
	CapStore = "store"
)

// CapabilityNames 全部能力名（顺序即设置页运维面板的展示顺序）。
var CapabilityNames = []string{
	CapCollector, CapWebDAV, CapDigest, CapInbox, CapReview, CapOrg, CapFamily, CapThumb, CapTranscode,
	CapCSInbox, CapCSContacts, CapCSChannels, CapCSAIDraft, CapStore,
}

// capabilityDefault 内置默认。family/org 是应用中心可安装的「垂直能力应用」：
// 默认不装配（未安装 = 侧栏无入口、路由不注册），安装内置应用条目后由
// blog_plugins capacity 记录点亮；其余核心能力默认开启。
func capabilityDefault(name string) bool {
	switch name {
	case CapFamily, CapOrg, CapCSInbox, CapCSContacts, CapCSChannels, CapCSAIDraft:
		return false
	}
	return true
}

// 生效时机。B14 之前把「能力开关」一律描述为「需重启」，对运行期开关是错的
// （正是「开关语义别混」那个坑）——故此处显式声明，并由 /admin/capabilities 逐项下发。
const (
	// CapApplyRestart 关掉后需重启进程才生效（影响启动装配）。
	CapApplyRestart = "restart"
	// CapApplyLive 改完立即生效（只在请求路径上判定）。
	CapApplyLive = "live"
)

// CapabilityApply 返回某能力的生效时机（未登记的能力按需重启处理，宁可保守）。
func CapabilityApply(name string) string {
	if name == CapThumb || name == CapTranscode {
		return CapApplyLive
	}
	return CapApplyRestart
}

// 判定来源（回答「这个生效态是哪来的」）。
const (
	CapSourceConfig      = "config"            // 站长显式配置
	CapSourcePlugin      = "plugin"            // 应用中心记录（capability_mode / kind 命中）
	CapSourceDeclaration = "plugin-capability" // 应用中心记录的 capabilities 声明命中
	CapSourceDefault     = "default"           // 内置默认
)

// cfgGetter 只声明用到的能力：避免 service 依赖 config 包（也便于测试注入桩）。
type cfgGetter interface{ GetString(string) string }

// CapabilityEnabled 判定某能力当前是否启用，并返回判定来源。
// db 可为 nil（纯配置模式）：此时跳过应用中心查询，直接落内置默认。
func CapabilityEnabled(db *sql.DB, cfg cfgGetter, name string) (bool, string) {
	// 1) 站长显式配置（空串 = 未配置，继续往下走）
	if cfg != nil {
		if v := strings.TrimSpace(cfg.GetString("capability." + name)); v != "" {
			return v == "true" || v == "1", CapSourceConfig
		}
	}
	if db == nil {
		return true, CapSourceDefault
	}
	// 2) 应用中心已装并启用的 capacity 应用。
	// 判据落在 capability_mode='capacity'（能力插件协议 v2），而非 kind —— 本壳 kind 承载的是
	// 应用中心安装类型（plugin|theme），两者取值域冲突，见协作区 REQ-004。
	// 兼容写法：老数据可能直接把 kind 写成 'capacity'（迁移前的手写记录），故一并纳入口径。
	var enabled int
	err := db.QueryRow(
		`SELECT enabled FROM blog_plugins
		  WHERE (capability_mode='capacity' OR kind='capacity') AND (id=? OR name=?)
		  ORDER BY enabled DESC LIMIT 1`,
		name, name).Scan(&enabled)
	if err == nil {
		return enabled == 1, CapSourcePlugin
	}
	// 2b) 能力声明命中：capabilities JSON 数组里含该能力名（第三方包名≠能力名的常规情形）。
	var jsonEnabled int
	err = db.QueryRow(
		`SELECT enabled FROM blog_plugins
		  WHERE capability_mode='capacity'
		    AND EXISTS (SELECT 1 FROM json_each(blog_plugins.capabilities) WHERE json_each.value=?)
		  ORDER BY enabled DESC LIMIT 1`,
		name).Scan(&jsonEnabled)
	if err == nil {
		return jsonEnabled == 1, CapSourceDeclaration
	}
	// 3) 内置默认（family/org 默认关：应用中心装了才启用）
	return capabilityDefault(name), CapSourceDefault
}
