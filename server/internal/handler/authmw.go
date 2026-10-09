// Package handler 的 authmw.go：Bearer token 鉴权中间件。
// 安全加固（2026-09-13）：此前全部 /api/v1 端点裸奔，任何人可删改。
// 规则：
//   - 公开端点白名单（无需登录）：health/version、公开分享（shares token 内容）、
//     对外博客、静态前端资源（页面本身无数据，数据全走 API）。
//   - 其余 /api/v1/* 一律要求 Authorization: Bearer <token>；无效/过期 → 401。
//   - 会话闲置超 14 天踢出；总有效期 30 天。
package handler

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

type ctxKey int

// ctxUserID 注入鉴权后的用户 id。
const ctxUserID ctxKey = 1

// ctxOrigin 注入本次请求的站点 origin（scheme://host）。
//
// 供「脱离 handler 也要拿到绝对 URL」的场景用 —— 目前只有 B44 订单邮件：
// 下载链接必须能被买家在邮件客户端里点开，不能是相对路径；而发信发生在
// service.MarkPaid 内部（那里只有 ctx，没有 *http.Request）。
// 显式注入 context，好过为此新增配置项（配错了会静默发出坏链接）。
//
// 🔴 取值 90，**不可与他人复用**：同包内 ctxUserID=1、ctxSiteID=2 已占用；
// 曾误用 2 导致本键与 ctxSiteID 撞车，邮件链接前缀变成站点 id（"default/…"）。
// context key 一旦撞车不会报错、只会静默读到别人的值 —— 新增键务必取未使用的值。
const ctxOrigin ctxKey = 90

// publicPath 判定是否公开端点（含精确与方法敏感路径）。
func publicPath(r *http.Request) bool {
	p := r.URL.Path
	if !strings.HasPrefix(p, "/api/v1/") {
		return true // 静态前端资源公开（无数据面）
	}
	// 精确公开端点（按方法敏感：写操作一律不公开）
	switch {
	case p == "/api/v1/health" || p == "/api/v1/version":
		return true
	case p == "/api/v1/auth/login" && r.Method == http.MethodPost:
		return true
	case p == "/api/v1/auth/register" && r.Method == http.MethodPost:
		return true // 开放注册（受 site.registration_open 门禁；内部做限流/用户名校验）
	case p == "/api/v1/auth/email-code" && r.Method == http.MethodPost:
		return true // 邮箱验证码发送（注册/绑定流程第一步；内部做邮箱格式+限流校验）
	case p == "/api/v1/auth/config" && r.Method == http.MethodGet:
		return true // 登录页能力探测（多用户/开放注册/邮箱验证开关）公开读
	case p == "/api/v1/public/posts" && r.Method == http.MethodGet:
		return true
	case p == "/api/v1/public/tags" && r.Method == http.MethodGet:
		return true // 公开标签聚合（标签云 / 标签内页数据源）
	case p == "/api/v1/public/site" && r.Method == http.MethodGet:
		return true // 博客站点设置公开读（RSS/OG/主题同源）
	case p == "/api/v1/public/capabilities" && r.Method == http.MethodGet:
		return true // 可安装能力启用布尔公开读（侧栏按安装状态渲染；无敏感信息）
	case p == "/api/v1/public/popular" && r.Method == http.MethodGet:
		return true // A5 公开热门文章榜（只读聚合，草稿/定时未发布已在 handler 内过滤）
	case p == "/api/v1/public/comments" && r.Method == http.MethodGet:
		return true // 评论公开读；写评论（POST /api/v1/comments）走登录
	case p == "/api/v1/public/unlock" && r.Method == http.MethodPost:
		return true // 2.2 文章密码解锁（匿名；校验失败 403，成功发会话级解锁凭证）
	case p == "/api/v1/pay/notify" && r.Method == http.MethodPost:
		return true // B 项通用支付回调（匿名；须过 HMAC 签名校验，幂等签发 grant_token）
	case p == "/api/v1/license/verify" && r.Method == http.MethodPost:
		return true // REQ-007 在线验签公开（无副作用：仅验签不写状态；各壳激活时回源）——7bea3b9 移植
	case p == "/api/v1/public/comments" && r.Method == http.MethodPost:
		return true // 访客评论（受 blog.comments_guest 开关 + IP 限流；默认待审不直接公开）
	case p == "/api/v1/public/blog/ask" && r.Method == http.MethodPost:
		return true // 读者 AI 问答（分层配额：游客/用户/管理员三档 + 全站日预算；仅公开文章上下文）
	case p == "/api/v1/public/blog/ask/quota" && r.Method == http.MethodGet:
		return true // AI 问答余量预览（公开；不计数不调模型，主题可渲染剩余次数）
	case p == "/api/v1/public/blog/pv" && (r.Method == http.MethodPost || r.Method == http.MethodGet):
		return true // 公开 PV 计数
	case p == "/api/v1/public/blog/tts" && r.Method == http.MethodGet:
		return true // 博客文章语音朗读（仅已发布文章，复用 ai.tts；命中磁盘缓存；不读登录身份）
	case p == "/api/v1/public/blog/related" && r.Method == http.MethodGet:
		return true // 相关文章语义推荐（仅已发布文章；无向量时返回空 items，不读登录身份）
	case p == "/api/v1/public/blog/themes" && r.Method == http.MethodGet:
		return true // 已安装主题列表（公开；驱动前端懒加载注册与切换器口径，仅 id/标题/版本）
	case p == "/api/v1/public/cs/start" && r.Method == http.MethodPost:
		return true // 客服挂件初始化（匿名；按 visitor_id 归一联系人，受 cs.enabled 门控）
	case p == "/api/v1/public/cs/send" && r.Method == http.MethodPost:
		return true // 客服挂件发消息（匿名；同 external_id 幂等）
	case p == "/api/v1/public/cs/pull" && r.Method == http.MethodGet:
		return true // 客服挂件拉消息（匿名；断线重连补历史）
	case p == "/api/v1/public/billing/plans" && r.Method == http.MethodGet:
		return true // 套餐目录（匿名只读：安装/选档页用；无敏感信息）
	// ---- B39 商城店铺（匿名可达，WooCommerce 式访客态）----
	// 🔴 白名单纪律：以下端点的 handler 一律「不读登录身份」，故可安全提前 return。
	// 绝不可把 /api/v1/admin/store/* 放进白名单 —— 那些端点调 isAdmin，白名单提前 return
	// 不注入身份，会导致管理员也被判匿名（与 /api/v1/im/status 同类事故，见上方注释）。
	// 登录态是「增强」而非「前提」：handler 内部用 ctxUID(r) 取到就回填 order.user_id，取不到照常下单。
	case p == "/api/v1/store/products" && r.Method == http.MethodGet:
		return true // 商品目录（仅 published；q 搜索）
	case strings.HasPrefix(p, "/api/v1/store/products/") && r.Method == http.MethodGet:
		return true // 商品详情（slug；非 published 返回 404）
	case strings.HasPrefix(p, "/api/v1/store/media/") && r.Method == http.MethodGet:
		return true // 商品封面图：handler 内校验「须被已发布商品引用 + 位图」，非开放文件代理
	case p == "/api/v1/store/cart" && r.Method == http.MethodGet:
		return true // 购物车（匿名 cookie store_cart 承载）
	case (p == "/api/v1/store/cart/add" || p == "/api/v1/store/cart/set" ||
		p == "/api/v1/store/cart/remove" || p == "/api/v1/store/cart/clear") && r.Method == http.MethodPost:
		return true // 购物车增删改（作用域限于本人 cookie）
	case p == "/api/v1/store/gateways" && r.Method == http.MethodGet:
		return true // 可用支付通道 + 币种（只回 configured 布尔，不回密钥）
	case p == "/api/v1/store/checkout" && r.Method == http.MethodPost:
		return true // 收银台下单（访客可下单；库存事务内扣减，幂等由订单号保证）
	case p == "/api/v1/store/orders/lookup" && r.Method == http.MethodGet:
		return true // 订单查询（须订单号 + 下单邮箱双因子匹配，见 handler）
	case p == "/api/v1/store/orders/return" && r.Method == http.MethodPost:
		return true // 买家发起退货（同双因子；handler 内做资格四查 + 同单仅一条待审）
	case strings.HasPrefix(p, "/api/v1/store/order/") && r.Method == http.MethodGet:
		// 订单详情 + 数字商品交付下载（两者同前缀）。
		// 详情：须带 email 校验归属，防 PII 泄露。
		// 下载：handler 内四道闸自校验（归属双因子 / 已支付 / 条目属本单 / kind=digital），
		//       不读登录身份即可放行匿名买家。
		return true
	case strings.HasPrefix(p, "/api/v1/store/mock-pay/") && r.Method == http.MethodPost:
		return true // 演示支付直确认（handler 内强制校验订单 gateway==mock，防真实通道订单被置为已支付）
	case strings.HasPrefix(p, "/api/v1/store/notify/") && r.Method == http.MethodPost:
		return true // 支付异步回调（匿名外部通道；须过各网关签名校验，与 /pay/notify 同模型）
	case strings.HasPrefix(p, "/api/v1/cs/hooks/") && r.Method == http.MethodPost:
		return true // 客服渠道回调（匿名外部平台 webhook；token 不符 404，SPEC-CS-001 §4.1 硬要求）
	case strings.HasPrefix(p, "/api/v1/public/media/") && r.Method == http.MethodGet:
		return true // 博客公开图片（仅 image/*，仅博客子树，handler 内二次校验）
	case p == "/api/v1/blog/feed.xml" && r.Method == http.MethodGet:
		return true
	case p == "/api/v1/blog/sitemap.xml" && r.Method == http.MethodGet:
		return true
	case p == "/api/v1/blog/plugins" && r.Method == http.MethodGet:
		return true // 插件列表公开（前端渲染用）；注册/启停/卸载走登录
	case p == "/api/v1/im/webhook/telegram" && r.Method == http.MethodPost:
		return true // IM webhook 仅 POST 回调公开
	case p == "/api/v1/im/webhook/wecom" && (r.Method == http.MethodPost || r.Method == http.MethodGet):
		return true // 企微回调 POST 消息 + GET URL 验证均公开
		// 🔴 注意：/api/v1/im/status 曾是白名单成员，但它是「管理端点」——
		// M10 修复后 imConfigStatus 用 isAdmin 守卫（只信任 authMiddleware 注入的显式登录身份）。
		// 而白名单路径在 middleware 里是「提前 return，不注入身份」→ 已登录管理员也会被判匿名，
		// 一律 403（B30 线上无头实测抓到：设置页 IM 面板整块报 403）。
		// 正解不是"给它注入身份"，而是**认清它是管理端点**：撤出白名单后
		// 匿名 401、管理员 200，与其余 /api/v1/admin/* 语义一致。
		// 一般规律：凡是白名单内又调用 isAdmin/blogAdminOnly 的端点，必然 403 ——
		// 新增白名单条目时务必确认其 handler 不看登录身份。
	}
	// 公开分享 token 内容（博客外链）——仅 GET；撤销分享（DELETE）等写操作走登录。
	// B19：新增 /thumb 后缀（分享通道专属公开缩略图），同样按 token 鉴权、不强制博客子树。
	if strings.HasPrefix(p, "/api/v1/shares/") && r.Method == http.MethodGet &&
		(p == "/api/v1/shares/"+shareTokenPart(p) || strings.HasSuffix(p, "/content") || strings.HasSuffix(p, "/thumb")) {
		return true
	}
	// 插件数据 KV 公开读（评论等公开数据；写操作仍鉴权）
	if strings.HasPrefix(p, "/api/v1/blog/plugins/") && strings.HasSuffix(p, "/kv") && r.Method == http.MethodGet {
		return true
	}
	// 插件设置 schema 公开读（无敏感信息；前端动态渲染设置表单）
	if strings.HasPrefix(p, "/api/v1/blog/plugins/") && strings.HasSuffix(p, "/settings-schema") && r.Method == http.MethodGet {
		return true
	}
	return false
}

// shareTokenPart 从 /api/v1/shares/{token}[/content] 提取 token 段（不做形态校验——
// 分享 token 由 newShareToken 生成（24 hex），内置博客 token 为 "blog" 字母串，
// 形态由生成器保证；此处仅负责路径切分，供公开分享路径匹配与内容路由使用）。
func shareTokenPart(p string) string {
	rest := strings.TrimPrefix(p, "/api/v1/shares/")
	if rest == "" {
		return ""
	}
	seg := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		seg = rest[:i]
	}
	return seg
}

// serviceTokenPath 平台令牌放行清单（外部服务/连接器/第三方采集引擎全生命周期：
// 读列表 / 建文档 / 建目录 / 改正文 / 发布 / 读正文 / 清理产物 / 建分享 / 查许可证；
// 其余端点一律不走平台令牌，防止令牌泄露后横向扩散）。
// 返回角色：read=只读（列表+正文 GET）；ingest=采集写（doc/mkdir/DELETE/PUT content/PUT status/POST shares）。
// 契约来源：上游《AiKlog需求清单v3.0回执》§二 R4（config `security.service_token`，主系统 authmw.go:139/175）。
func serviceTokenPath(r *http.Request) (string, bool) {
	switch p := r.URL.Path; {
	case p == "/api/v1/files" && r.Method == http.MethodGet:
		return "read", true
	case strings.HasPrefix(p, "/api/v1/files/") && r.Method == http.MethodGet && strings.HasSuffix(p, "/content"):
		// 读取文档正文（采集管道验证/比对/解读用）。注意：与"共享免登录"是两回事——
		// 对外共享请走 shares 分享链接机制（publicPath 已放行），令牌 GET 只服务持有令牌的进程。
		return "read", true
	case p == "/api/v1/license" && r.Method == http.MethodGet:
		// 许可证状态查询（只读令牌可感知付费状态；激活/吊销仍走登录会话）
		return "read", true
	case p == "/api/v1/files/doc" && r.Method == http.MethodPost:
		return "ingest", true
	case p == "/api/v1/files/mkdir" && r.Method == http.MethodPost:
		return "ingest", true
	case strings.HasPrefix(p, "/api/v1/files/") && r.Method == http.MethodDelete:
		// 删除单个文件（桥接器剔除错误采集产物用）
		return "ingest", true
	case strings.HasPrefix(p, "/api/v1/files/") && r.Method == http.MethodPut && strings.HasSuffix(p, "/content"):
		// 修订已存在文档正文（共享采集源清单等持续更新）
		return "ingest", true
	case strings.HasPrefix(p, "/api/v1/files/") && r.Method == http.MethodPut && strings.HasSuffix(p, "/status"):
		// 发布/下架（draft<->published）：桥接器链路"写文档→发布→创建分享"的前置
		return "ingest", true
	case p == "/api/v1/shares" && r.Method == http.MethodPost:
		// 创建公开分享（桥接器自动化：写完清单→POST /shares 拿 /p/{token} 输出链接）。
		// 只给 ingest 令牌；只读令牌无此权限（公开面只能读）。
		return "ingest", true
	}
	return "", false
}

// serviceTokenOK 按角色匹配平台令牌：全量令牌放行 read+ingest；专用令牌只放行自身角色。
// M1 修复：令牌比较改为恒定时间（subtle.ConstantTimeCompare），防时序侧信道猜解。
func (a *API) serviceTokenOK(tok, role string) bool {
	eq := func(a, b string) bool {
		if len(a) != len(b) {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
	}
	if svc := a.cfg.GetString("security.service_token"); svc != "" && eq(tok, svc) {
		return true
	}
	switch role {
	case "read":
		if rt := a.cfg.GetString("security.readonly_token"); rt != "" && eq(tok, rt) {
			return true
		}
	case "ingest":
		if it := a.cfg.GetString("security.ingest_token"); it != "" && eq(tok, it) {
			return true
		}
	}
	return false
}

// authMiddleware 校验 Bearer token（公开端点放行）。
// requestOrigin 推断本次请求的站点 origin（scheme://host）。
//
// 生产在 nginx 反代之后，r.TLS 恒为 nil 且 r.Host 可能是反代 Host —— 故优先采信
// X-Forwarded-Proto / X-Forwarded-Host。只取**第一个**代理层（与 clientIP 同口径），
// 不取最后一个，避免被客户端伪造的头带偏。
func requestOrigin(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	if v := firstCSV(r.Header.Get("X-Forwarded-Proto")); v != "" {
		scheme = v
	}
	host := r.Host
	if v := firstCSV(r.Header.Get("X-Forwarded-Host")); v != "" {
		host = v
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func firstCSV(s string) string {
	if i := strings.IndexByte(s, ','); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func (a *API) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 站点 origin 注入（B44 订单邮件要拼绝对下载链接）。
		// 放在最前面：publicPath 会提前 return，而支付回调/mock 确认**都是公开端点**，
		// 放在后面这些路径就拿不到 origin，邮件里的链接会退化成不可点的相对路径。
		// 优先用反代传入的 X-Forwarded-Proto/Host（生产在 nginx 之后，r.TLS 恒为 nil）。
		r = r.WithContext(context.WithValue(r.Context(), ctxOrigin, requestOrigin(r)))
		if publicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		tok := bearerToken(r)
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "需要登录")
			return
		}
		// 平台令牌（security.service_token / readonly_token / ingest_token）：
		// 按角色白名单放行（read=只读 GET；ingest=采集写；全量令牌两者皆可），
		// 其余端点一律不认（防令牌泄露后横向扩散）。身份记为该实例的 owner。
		if role, ok := serviceTokenPath(r); ok {
			if a.serviceTokenOK(tok, role) {
				ctx := context.WithValue(r.Context(), ctxUserID, service.SystemOwnerID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		var (
			uid       string
			expiresAt int64
			lastSeen  int64
		)
		err := a.db.QueryRowContext(r.Context(),
			`SELECT user_id, expires_at, last_seen_at FROM sessions WHERE token=?`, tok).
			Scan(&uid, &expiresAt, &lastSeen)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "会话无效或已过期，请重新登录")
			return
		}
		now := time.Now().Unix()
		if now > expiresAt || now-lastSeen > int64(sessionIdleTTL.Seconds()) {
			_, _ = a.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token=?`, tok)
			writeErr(w, http.StatusUnauthorized, "AUTH_REQUIRED", "会话已过期，请重新登录")
			return
		}
		// 滑动续期：距上次活跃超 1 小时才写库（降写放大）
		if now-lastSeen > 3600 {
			_, _ = a.db.ExecContext(r.Context(),
				`UPDATE sessions SET last_seen_at=? WHERE token=?`, now, tok)
		}
		ctx := context.WithValue(r.Context(), ctxUserID, uid)
		ctx = ai.WithSubject(ctx, "user:"+uid) // AI 用量管控主体（meter.go）；匿名端点默认 "public"
		r = r.WithContext(ctx)
		// C1/H2/H3/M9/M10 修复：管理面统一守卫（路由级，杜绝逐 handler 手补漏加）。
		// 凡 /api/v1/admin/ 前缀一律要求 owner/admin；此前 updateSettings、settings GET、
		// auditList、ai_providers、capabilityStates 等三十余个管理端点均无守卫，
		// 任意登录用户可写 security.service_token 提权接管站点。此处收口后，
		// 新增管理端点默认受保护，无需记得手加守卫。
		if strings.HasPrefix(r.URL.Path, "/api/v1/admin/") && !a.isAdmin(r) {
			writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
			return
		}
		next.ServeHTTP(w, r)
	})
}
