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
)

// validBool 布尔值校验器（归一化为 "true"/"false"）。
func validBool(v string) (string, bool) {
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return "", false
	}
	return strconv.FormatBool(b), true
}

// aiQuotaValid AI 配额数值校验器（非负整数，≤100000）。
func aiQuotaValid(v string) (string, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 || n > 100000 {
		return "", false
	}
	return strconv.Itoa(n), true
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
	"blog.open": {
		desc:  "博客对外开关（true/false）：关闭后公开页/RSS 下线（404），博客目录数据保留",
		valid: validBool,
	},
	"blog.auto_publish_on_upload": {
		desc:  "上传后自动发布（true/false）：开启后博客管理里拖拽/选择的 md 上传即公开；关闭则保持草稿",
		valid: validBool,
	},
	"blog.theme": {
		desc: "博客主题 id：内置主题（default|aiklog|minimal|docs|paper|elevated|parchment|emforum|brutal）" +
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
		desc:  "博客站点名称（公开页标题/RSS/OG 同源；≤60 字符）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 60 {
				return "", false
			}
			return v, true
		},
	},
	"blog.description": {
		desc:  "博客站点简介（公开页描述/SEO 默认；≤200 字符）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"blog.logo": {
		desc:  "博客 Logo 文字/图片 URL（公开页头部；≤200 字符，留空=默认）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"blog.footer": {
		desc:  "博客页脚文案（公开页页脚；≤200 字符，留空=默认）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"blog.seo_default": {
		desc:  "SEO 默认描述（无自定义描述时 head meta 使用；≤300 字符，留空=站点简介）",
		valid: func(v string) (string, bool) {
			if len([]rune(v)) > 300 {
				return "", false
			}
			return v, true
		},
	},
	"plugin_market.index_url": {
		desc:  "应用中心市场索引地址（index.json；留空=默认 aikmap 官方市场）",
		valid: func(v string) (string, bool) {
			if len(v) > 512 {
				return "", false
			}
			return v, true
		},
	},
	"plugin_market.shell_id": {
		desc:  "本壳标识（应用中心多壳模型：插件 target 含本壳才可安装；留空=aiklog）",
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
		desc:  "授权作者白名单（逗号分隔 user id；仅这些成员 + owner/admin 可发布文章；留空=仅管理员）",
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
		desc:  "SMTP 服务器地址（如 smtp.qq.com；留空=停用发信）",
		valid: func(v string) (string, bool) {
			if len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"smtp.port": {
		desc:  "SMTP 端口（如 465/587；1-65535）",
		valid: func(v string) (string, bool) {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 65535 {
				return "", false
			}
			return v, true
		},
	},
	"smtp.user": {
		desc:  "SMTP 登录账号（留空=免认证）",
		valid: func(v string) (string, bool) {
			if len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"smtp.pass": {
		desc:  "SMTP 登录密码/授权码",
		valid: func(v string) (string, bool) {
			if len(v) > 200 {
				return "", false
			}
			return v, true
		},
	},
	"smtp.from": {
		desc:  "发信人地址（留空=同 smtp.user）",
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
}

// updateSettings 修改白名单配置项；写入后立即生效（agent 轮数即时同步）。
func (a *API) updateSettings(w http.ResponseWriter, r *http.Request) {
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
