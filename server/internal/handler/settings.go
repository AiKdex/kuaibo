// settings.go 实现设置写接口（白名单化）：PUT /api/v1/admin/settings。
// 安全原则：不是所有配置都可在线修改，仅允许显式声明的运行时可变项；
// 涉及 Agent 行为的配置（如工具轮数）写入后即时生效。
package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// validBool 布尔值校验器（归一化为 "true"/"false"）。
func validBool(v string) (string, bool) {
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return "", false
	}
	return strconv.FormatBool(b), true
}

// tripleBool 三态布尔校验器（"true" / "false" / ""）：空串 = 清除显式配置，
// 回到「跟随应用中心记录 → 内置默认」的判定链（见 service.CapabilityEnabled）。
func tripleBool(v string) (string, bool) {
	if strings.TrimSpace(v) == "" {
		return "", true
	}
	return validBool(v)
}

// aiQuotaValid AI 配额数值校验器（非负整数，≤100000）。
func aiQuotaValid(v string) (string, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 || n > 100000 {
		return "", false
	}
	return strconv.Itoa(n), true
}

// thumbIntValid 整型区间校验器工厂（B16）：把区间写在一处，供 K 个数值型配置复用。
// 空串放行 → 语义为「清除显式配置，回退内置默认」，与读取层 service.thumbIntCfg 同一口径。
func thumbIntValid(lo, hi int) func(string) (string, bool) {
	return func(v string) (string, bool) {
		v = strings.TrimSpace(v)
		if v == "" {
			return "", true
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < lo || n > hi {
			return "", false
		}
		return strconv.Itoa(n), true
	}
}

// settableKeys 允许在线修改的配置项（白名单）。
var settableKeys = map[string]struct {
	desc  string
	valid func(string) (string, bool)
}{
	"ai.agent.max_rounds": {
		desc: "单次对话最大工具调用轮数（免费/付费分级参数，1-32）",
		valid: func(v string) (string, bool) {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 32 {
				return "", false
			}
			return v, true
		},
	},
	// 语音合成默认参数（family.8.1）：调用方未显式传参时的回退值（voice.go 空参回退链）。
	// 音色/风格语义由当前 tts provider 自定（如 MiMo：冰糖/茉莉/苏打/白桦/Mia/Chloe/Milo/Dean），
	// 内核不做枚举硬编码；候选列表来自 providers.json voice_cands，仅作设置页提示。
	"ai.tts.voice": {
		desc: "语音合成默认音色（留空=provider 默认音色；MiMo 可选 冰糖/茉莉/苏打/白桦/Mia/Chloe/Milo/Dean）",
		valid: func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if len(v) > 64 {
				return "", false
			}
			return v, true
		},
	},
	"ai.tts.format": {
		desc:  "语音合成默认音频格式（mp3|wav|pcm16；留空=mp3）",
		valid: func(v string) (string, bool) {
			switch strings.TrimSpace(v) {
			case "", "mp3", "wav", "pcm16":
				return strings.TrimSpace(v), true
			}
			return "", false
		},
	},
	"ai.tts.style": {
		desc:  "语音合成默认风格指令（如 温柔自然、播报感；留空=不加风格）",
		valid: func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"ai.summarize.exts": {
		desc: "AI 入库解读类型白名单（逗号分隔扩展名，如 .md,.docx,.pdf；留空=全部文本类型）",
		valid: func(v string) (string, bool) {
			if len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"storage.recycle_days": {
		desc: "回收站保留天数（1-365）",
		valid: func(v string) (string, bool) {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 365 {
				return "", false
			}
			return v, true
		},
	},
	"ocr.auto_clean": {
		desc:  "OCR 结果自动后处理开关（true/false）：结构结果（HTML 标签占比高）自动整理为干净 Markdown 入库",
		valid: validBool,
	},
	"ocr.clean_on_ai": {
		desc:  "OCR 结构结果是否调用 AI 整理（true/false）：关闭则省 token、原样入库",
		valid: validBool,
	},
	// B 项内容付费：支付回调签名密钥（站长自助配置；支付对接插件转发 notify 时验签）
	// 键名对齐上游契约（paid_access.go / settings.go:138 的 pay.notify_secret）；
	// 代码同时兼容旧键名 pay.webhook_secret（见 paidSecret()），避免已配置站点失效。
	"pay.notify_secret": {
		desc: "支付回调签名密钥（HMAC-SHA256）：支付对接插件/工具完成收款后转发 " +
			"POST /api/v1/pay/notify 时用于验签；留空=关闭支付回调（回调返回 503）",
		valid: func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if v == "" {
				return "", true // 留空=关闭支付回调
			}
			if len(v) < 8 || len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	// R4 平台令牌（service token）：外部服务（连接器/第三方采集引擎）调用内核 API 的凭据。
	// 契约见上游《AiKlog需求清单v3.0回执》§二 R4：全量令牌放行 read+ingest，专用令牌只放行自身角色。
	"security.service_token": {
		desc: "平台令牌（全量）：外部服务/连接器/第三方采集引擎以 Bearer 调用内核 API 时使用，" +
			"放行只读与采集写白名单端点；留空=停用平台令牌；轮换=改此值并重启",
		valid: func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if v == "" {
				return "", true // 留空=停用
			}
			if len(v) < 16 || len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"security.readonly_token": {
		desc: "平台令牌（只读）：仅放行读列表 / 读正文 / 查许可证状态；留空=停用",
		valid: func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if v == "" {
				return "", true
			}
			if len(v) < 16 || len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"security.ingest_token": {
		desc: "平台令牌（采集写）：仅放行建文档/建目录/改正文/发布/删除/建分享；留空=停用",
		valid: func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if v == "" {
				return "", true
			}
			if len(v) < 16 || len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	// R2 能力开关（capability gate）：内核模块的启用判定。
	// 优先级：本站开关 > 应用中心 capacity 应用记录（blog_plugins kind='capacity'）> 内置默认开启。
	// 关闭后对应路由不注册（404 = 能力未启用），数据保留。详见 cmd/aikmap/capabilities.go。
	"capability.collector": {
		desc:  "能力开关 · 采集引擎（true/false）：关闭后采集源/采集运行端点下线（404），数据保留",
		valid: tripleBool,
	},
	"capability.webdav": {
		desc:  "能力开关 · 外部 WebDAV 挂载（true/false）：关闭后 WebDAV 挂载端点下线（404）",
		valid: tripleBool,
	},
	"capability.digest": {
		desc:  "能力开关 · 每日知识日报（true/false）：关闭后日报端点下线（404）",
		valid: tripleBool,
	},
	"capability.inbox": {
		desc:  "能力开关 · 收件箱（true/false）：关闭后收件箱端点下线（404）",
		valid: tripleBool,
	},
	"capability.review": {
		desc:  "能力开关 · 知识复习队列（true/false）：关闭后复习端点下线（404）",
		valid: tripleBool,
	},
	"capability.org": {
		desc:  "能力开关 · 组织模块（true/false）：关闭后组织树/部门/移交端点下线（404）",
		valid: tripleBool,
	},
	"capability.family": {
		desc:  "能力开关 · 家族传承（true/false）：关闭后家族记录/AI 参谋端点下线",
		valid: tripleBool,
	},
	"capability.cs.inbox": {
		desc:  "能力开关 · 客服收件箱（true/false）：关闭后会话读写端点下线（挂件与工作台均不可用）",
		valid: tripleBool,
	},
	"capability.cs.contacts": {
		desc:  "能力开关 · 客服客户档案（true/false）：关闭后停止新建/归一客户档案（历史只读）",
		valid: tripleBool,
	},
	"capability.cs.channels": {
		desc:  "能力开关 · 客服渠道适配（true/false）：关闭后不接外部 webhook、不做出站外推",
		valid: tripleBool,
	},
	"capability.cs.ai_draft": {
		desc:  "能力开关 · 客服 AI 起草（true/false）：关闭后不调用模型起草回复（省额度）",
		valid: tripleBool,
	},
	"cs.enabled": {
		desc:  "客服模块总闸（true/false）：关闭后挂件与全部客服端点返回 403（数据保留）",
		valid: tripleBool,
	},
	"capability.thumb": {
		desc:  "能力开关 · 媒体缩略图（true/false）：关闭后不生成缩略图、缩略图端点返回 404（原图不受影响）；即时生效，无需重启",
		valid: tripleBool,
	},
	// 媒体派生资产（缩略图）参数（B16）：运行期读取，改完**即时生效**（无需重启）。
	// 与 capability.thumb 的分工：那个管「开不开」，这里管「怎么生成」。
	// 空串 = 清除显式配置，回到内置默认（与 capability.* 同口径）。
	// 区间上限/下限与 service 层同一组常量，改一处即两处同步（无第二份魔法数字）。
	"media.thumb_max_edge": {
		desc:  "缩略图长边上限（像素，64-4096）：只缩不放（小图不会被放大）；留空=默认 480",
		valid: thumbIntValid(service.ThumbMaxEdgeMin, service.ThumbMaxEdgeMax),
	},
	"media.thumb_seek_ms": {
		desc: "视频抽帧位置上限（毫秒，0-60000）：取时长的 1/10 并夹在此上限内，" +
			"用于跳过片头黑场/台标；0=恒抽第 0 帧；留空=默认 1000",
		valid: thumbIntValid(service.ThumbSeekMSMin, service.ThumbSeekMSMax),
	},
	"media.thumb_quality": {
		desc:  "缩略图 JPEG 质量（2-31，数值越小越清晰、体积越大；默认 5）",
		valid: thumbIntValid(service.ThumbQualityMin, service.ThumbQualityMax),
	},
	// 组织模块运行期子开关（B4 落能力、B14 补开关）。总开关关闭时三个子能力一律视同关闭
	// （判定见 handler/org.go 与 service/org.go 的 Enabled/TreeEnabled/DepartmentEnabled/TransferEnabled）。
	// 空串 = 未配置（等同关闭），与 capability.* 的三态口径一致。
	"org.enabled": {
		desc:  "组织模块总开关（true/false）：关闭后组织端点一律 403（含组织树建/改/删；数据保留）；需 capability.org 已装配",
		valid: tripleBool,
	},
	"org.tree": {
		desc:  "组织树子开关（true/false）：需 org.enabled 同时开启；关闭后建/改/删节点与自动打标返回 403/不生效（读取仍可）",
		valid: tripleBool,
	},
	"org.department": {
		desc:  "部门空间子开关（true/false）：需 org.enabled 同时开启；关闭后部门端点返回 403 ORG_DEPT_DISABLED",
		valid: tripleBool,
	},
	"org.transfer": {
		desc:  "岗位移交流子开关（true/false）：需 org.enabled 同时开启；关闭后移交流端点返回 403",
		valid: tripleBool,
	},
	"family.enabled": {
		desc:  "家族传承记录模块总开关（[family]；true/false）：开启后一生时间轴/家族树/纪念日提醒可用（模块组件，不装不用）",
		valid: tripleBool,
	},
	"blog.open": {
		desc:  "博客对外开关（true/false）：关闭后公开页/RSS 下线（404），博客目录数据保留",
		valid: validBool,
	},
	"blog.auto_publish_on_upload": {
		desc:  "上传后自动发布（true/false）：开启后博客管理里拖拽/选择的 md 上传即公开；关闭则保持草稿",
		valid: validBool,
	},
	"blog.list_attachments": {
		desc: "公开列表是否收录附件（true/false）：false（默认）=公开列表/热门榜/RSS/sitemap 只列文章，" +
			"图片音视频等附件不进这些出口（管理轨与分享页照常可见）；true=附件也作为内容列出",
		valid: validBool,
	},
	"blog.theme": {
		desc: "博客主题 id：内置主题（default|aiklog|minimal|docs|paper|elevated|parchment|emforum|brutal|aiknav|chenxi|jaded|zhicang|zircon）" +
			"或 data/themes 下已安装的外置主题（应用中心装 theme 包即出现，无需重新编译；" +
			"外置主题作用于公网静态页 /blog，交互版 SPA 未注册时回退默认）",
		valid: func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if v == "" {
				return "aiklog", true
			}
			if !validThemeID(v) {
				return "", false
			}
			for _, t := range builtinThemes {
				if t.ID == v {
					return v, true
				}
			}
			// 外置主题：data/themes/<id>/ 目录存在即接受（装完即可切换，无需改此白名单）
			if st, err := os.Stat(filepath.Join(themesRoot(), v)); err == nil && st.IsDir() {
				return v, true
			}
			return "", false
		},
	},
	"blog.custom_css": {
		desc: "站点自定义 CSS（前台公开页全主题生效，注入 <style>；≤20000 字符）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 20000 {
				return "", false
			}
			return v, true
		},
	},
	"blog.custom_js": {
		desc: "站点自定义 JS（前台公开页全主题生效，注入 <script>；≤20000 字符）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 20000 {
				return "", false
			}
			return v, true
		},
	},
	"blog.base_url": {
		desc: "站点基础域名（RSS/canonical/OG 的 origin；如 https://blog.example.com；留空=跟随请求域）",
		valid: func(v string) (string, bool) {
			if v != "" && !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
				return "", false
			}
			return v, true
		},
	},
	"blog.locale": {
		desc: "站点语言（如 zh-cn / en；RSS language 与前端 locale 注入点同源）",
		valid: func(v string) (string, bool) {
			if len(v) > 20 {
				return "", false
			}
			return v, true
		},
	},
	"blog.comments_guest": {
		desc: "访客评论开关（true=允许未登录读者评论，默认待审核；false=仅登录用户可评论）",
		valid: func(v string) (string, bool) {
			if v != "true" && v != "false" {
				return "", false
			}
			return v, true
		},
	},
	"blog.ai_ask_open": {
		desc: "博客 AI 问答开关（true/false，默认 true）：关闭后公开端点 /public/blog/ask 返回 404，主题应隐藏 AI 对话窗入口",
		valid: func(v string) (string, bool) {
			if v != "true" && v != "false" {
				return "", false
			}
			return v, true
		},
	},
	"blog.ai_ask_guest_quota": {
		desc:  "博客 AI 问答·游客每日次数（整数，默认 3；0=关闭游客使用）",
		valid: aiQuotaValid,
	},
	"blog.ai_ask_user_quota": {
		desc:  "博客 AI 问答·登录用户每日次数（整数，默认 10）",
		valid: aiQuotaValid,
	},
	"blog.ai_ask_admin_quota": {
		desc:  "博客 AI 问答·管理员每日次数（整数，默认 0=不限）",
		valid: aiQuotaValid,
	},
	"blog.ai_ask_daily_budget": {
		desc:  "博客 AI 问答·全站每日总用量上限（整数，默认 200；0=不限；超限当日全站降级）",
		valid: aiQuotaValid,
	},
	"blog.title": {
		desc: "博客站点名称（公开页标题/RSS/OG 同源；≤60 字符）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 60 {
				return "", false
			}
			return v, true
		},
	},
	"blog.description": {
		desc: "博客站点简介（公开页描述/SEO 默认；≤200 字符）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"blog.logo": {
		desc: "博客 Logo 文字/图片 URL（公开页头部；≤200 字符，留空=默认）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"blog.footer": {
		desc: "博客页脚文案（公开页页脚；≤200 字符，留空=默认）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"blog.seo_default": {
		desc: "SEO 默认描述（无自定义描述时 head meta 使用；≤300 字符，留空=站点简介）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 300 {
				return "", false
			}
			return v, true
		},
	},
	"plugin_market.index_url": {
		desc: "应用中心远程市场索引地址（index.json；留空=优先本地市场目录 data/market/index.json，无本地目录时用上游官方市场）。注意：免费版若本地无市场目录，拉取此地址会带上匿名 install_id 与版本号，供官方统计去重装机数（不含任何个人信息）",
		valid: func(v string) (string, bool) {
			if len(v) > 512 {
				return "", false
			}
			return v, true
		},
	},
	"plugin_market.report_identity": {
		desc:  "回源上报匿名装机标识开关（true/false，默认 true）：关闭后拉取应用中心索引不再附带 install_id/版本号，功能不受影响，仅官方侧看不到你的装机记录",
		valid: validBool,
	},
	"plugin_market.shell_id": {
		desc: "本壳标识（应用中心多壳模型：插件 target 含本壳才可安装；留空=aiklog）",
		valid: func(v string) (string, bool) {
			if len(v) > 64 {
				return "", false
			}
			return v, true
		},
	},
	// ---- 多用户（Option B：AiKlog 自带同构多用户，注册策略由站长后台配置） ----
	"site.registration_open": {
		desc:  "开放注册开关（true/false）：关闭后新用户无法自助注册（私有博客/邀请制）；开启后用户可自助注册",
		valid: validBool,
	},
	"site.email_required": {
		desc:  "注册邮箱验证开关（true/false）：开启后注册必须填邮箱并通过验证码（需先配置发信邮箱 smtp.*）",
		valid: validBool,
	},
	"blog.authors": {
		desc: "授权作者白名单（逗号分隔 user id；仅这些成员 + owner/admin 可发布文章；留空=仅管理员）",
		valid: func(v string) (string, bool) {
			if len(v) > 4096 {
				return "", false
			}
			return v, true
		},
	},
	// ---- 发信邮箱（邮箱验证码/通知用；未配置则邮箱验证类功能降级为不可用） ----
	"smtp.enable": {
		desc:  "启用 SMTP 发信（true/false）",
		valid: validBool,
	},
	"smtp.host": {
		desc: "SMTP 服务器地址（如 smtp.qq.com；留空=停用发信）",
		valid: func(v string) (string, bool) {
			if len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"smtp.port": {
		desc: "SMTP 端口（如 465/587；1-65535）",
		valid: func(v string) (string, bool) {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 65535 {
				return "", false
			}
			return v, true
		},
	},
	"smtp.user": {
		desc: "SMTP 登录账号（留空=免认证）",
		valid: func(v string) (string, bool) {
			if len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"smtp.pass": {
		desc: "SMTP 登录密码/授权码",
		valid: func(v string) (string, bool) {
			if len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"smtp.from": {
		desc: "发信人地址（留空=同 smtp.user）",
		valid: func(v string) (string, bool) {
			if len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"smtp.debug_code": {
		desc:  "验证码调试模式（true/false）：开发用，验证码写日志+随响应返回，不真实发信",
		valid: validBool,
	},
	// 商城（B39）：结算币种 + 三类真实支付通道密钥（缺密钥=通道不可选，收银台自动隐藏）。
	"store.currency": {
		desc: "商城结算币种（如 CNY / USD；留空=CNY）",
		valid: func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if len(v) > 8 {
				return "", false
			}
			return v, true
		},
	},
	"store.return_window_days": {
		desc: "买家可申请退货的天数（从支付时间算；0=不限制）",
		valid: func(v string) (string, bool) {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 || n > 365 {
				return "", false
			}
			return v, true
		},
	},
	"store.pay.wechat_appid":   {desc: "微信支付 AppID", valid: strLenLE(64)},
	"store.pay.wechat_mch_id": {desc: "微信支付 商户号 MCH_ID", valid: strLenLE(64)},
	"store.pay.wechat_apikey": {desc: "微信支付 API 密钥（v2 Key）", valid: strLenLE(64)},
	"store.pay.alipay_appid":  {desc: "支付宝 应用 AppID", valid: strLenLE(64)},
	"store.pay.alipay_private_key": {desc: "支付宝 应用私钥（PKCS1/PKCS8 PEM）", valid: strLenLE(4096)},
	"store.pay.alipay_public_key": {desc: "支付宝 支付宝公钥（PKIX PEM，验签用）", valid: strLenLE(4096)},
	"store.pay.stripe_secret":       {desc: "Stripe 密钥（sk_...）", valid: strLenLE(256)},
	"store.pay.stripe_publishable":  {desc: "Stripe 可发布密钥（pk_...）", valid: strLenLE(256)},
	"store.pay.stripe_webhook_secret": {desc: "Stripe Webhook 签名密钥（whsec_...）", valid: strLenLE(256)},
}

// strLenLE 长度上限校验器（密钥类配置防越界）。
func strLenLE(max int) func(string) (string, bool) {
	return func(v string) (string, bool) {
		if len(v) > max {
			return "", false
		}
		return v, true
	}
}

// updateSettings 修改白名单配置项；写入后立即生效（agent 轮数即时同步）。
func (a *API) updateSettings(w http.ResponseWriter, r *http.Request) {
	// C1 修复：写配置是提权链的关键一环（security.service_token → 以 owner 身份读写全站文件；
	// blog.authors → 进作者白名单）。必须管理员；纵深防御在路由级 admin 前缀守卫之上。
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "SETTING_BAD_REQ", err.Error())
		return
	}
	spec, ok := settableKeys[req.Key]
	if !ok {
		writeErr(w, http.StatusForbidden, "SETTING_READONLY", "该配置项不允许在线修改")
		return
	}
	val, ok := spec.valid(req.Value)
	if !ok {
		writeErr(w, http.StatusBadRequest, "SETTING_INVALID", spec.desc)
		return
	}
	if _, err := a.cfg.Set(r.Context(), req.Key, val, "string", "设置页修改", "admin"); err != nil {
		writeErr(w, http.StatusInternalServerError, "SETTING_FAILED", err.Error())
		return
	}
	// Agent 轮数即时生效（无需重启）
	if req.Key == "ai.agent.max_rounds" && a.agent != nil {
		if n, err := strconv.Atoi(val); err == nil {
			a.agent.SetMaxRounds(n)
		}
	}
	// 解读类型白名单即时生效（无需重启；Summarizer 持有当前快照）
	if req.Key == "ai.summarize.exts" && a.summarizer != nil {
		a.summarizer.SetExts(val)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": req.Key, "value": val})
}

// capabilityMeta 能力开关的中文名与说明（设置页运维面板展示用）。
// Apply 声明生效时机（service.CapabilityApply）：restart=改了要重启，live=立即生效。
// 面板必须逐项展示，否则用户会以为「关掉采集要重启」也适用于「关掉缩略图」。
var capabilityMeta = []struct{ Name, Label, Note string }{
	{service.CapCollector, "采集引擎", "采集源/采集运行端点"},
	{service.CapWebDAV, "外部 WebDAV 挂载", "WebDAV 挂载导入端点"},
	{service.CapDigest, "每日知识日报", "日报生成与订阅端点"},
	{service.CapInbox, "收件箱", "采集聚合/未读/归档端点"},
	{service.CapReview, "知识复习队列", "SM-2 复习队列端点"},
	{service.CapOrg, "组织架构", "组织树/任职/部门空间/移交流端点"},
	{service.CapFamily, "家族传承", "家族传承记录/AI 人生参谋端点；应用中心可安装，装了才亮"},
	{service.CapThumb, "媒体缩略图", "图片/视频缩略图生成；关闭后不再调 ffmpeg，两个缩略图端点一律 404（含已缓存的）"},
	{service.CapTranscode, "视频转码", "视频按需转成 H.264/AAC MP4 播放源；关闭后不接新任务、公开播放回落原文件（已生成的仍保留）"},
	{service.CapCSInbox, "客服收件箱", "会话与消息读写端点；关闭后挂件与收件箱均不可用"},
	{service.CapCSContacts, "客服客户档案", "客户档案与渠道身份图谱；关闭后只读历史、停止新建联系人"},
	{service.CapCSChannels, "客服渠道适配", "外部渠道 webhook 入站与出站外推；关闭后不接回调、不外发"},
	{service.CapCSAIDraft, "客服 AI 起草", "AI 依据知识库起草回复（消耗模型额度，受 AI 配额三闸门约束）"},
	{service.CapStore, "商城", "WooCommerce 式商品/购物车/收银台/订单/优惠券；关闭后商城路由不注册"},
}

// capabilityStates GET /api/v1/admin/capabilities —— 能力模块生效态与判定来源（只读）。
// 生效态由 service.CapabilityEnabled 判定：与启动装配同一套逻辑，故面板不会与真实状态漂移。
// source ∈ config | plugin | plugin-capability | default；configured=是否被站长显式配置。
func (a *API) capabilityStates(w http.ResponseWriter, _ *http.Request) {
	items := make([]map[string]any, 0, len(capabilityMeta))
	for _, m := range capabilityMeta {
		on, src := service.CapabilityEnabled(a.db, a.cfg, m.Name)
		configured := false
		if a.cfg != nil {
			configured = strings.TrimSpace(a.cfg.GetString("capability."+m.Name)) != ""
		}
		items = append(items, map[string]any{
			"name": m.Name, "label": m.Label, "note": m.Note,
			"enabled": on, "source": src, "configured": configured,
			"apply": service.CapabilityApply(m.Name),
		})
	}
	// 顶层 restart_required 保留「存在需要重启才能生效的项」这一含义（装配类能力必然存在），
	// 逐项的 apply 字段才是精确口径：live 的项改完立刻生效（B15 媒体缩略图即此类）。
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "restart_required": true})
}

// publicCapabilities GET /api/v1/public/capabilities —— 公开能力开关（只读、无敏感信息）。
// 供前端侧栏按「应用中心安装状态」渲染导航：family/org 装了才亮，未装不出现入口。
// 只暴露可安装能力的启用布尔，不回配置细节；管理端细节数据仍走 /admin/capabilities。
func (a *API) publicCapabilities(w http.ResponseWriter, _ *http.Request) {
	out := map[string]bool{}
	for _, n := range []string{service.CapFamily, service.CapOrg, service.CapCSInbox, service.CapStore} {
		on, _ := service.CapabilityEnabled(a.db, a.cfg, n)
		out[n] = on
	}
	writeJSON(w, http.StatusOK, out)
}
