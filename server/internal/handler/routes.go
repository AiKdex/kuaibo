// Package handler 提供 HTTP/WS 入口：参数校验、鉴权、路由（实施文档 §8）。
// 阶段 0 最小面：健康检查、版本、配置快照、审计查询；业务端点随阶段逐步开启。
package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/config"
	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// API 汇聚依赖。
type API struct {
	db         *sql.DB
	cfg        *config.Store
	b          *bus.Bus
	aud        *service.AuditStore
	ai         *ai.Gateway
	files      *service.FileStore
	prompts    *service.PromptStore       // B5 写作增强：AI 写作提示词模板（提示词参数化，非硬编码）
	subs       *service.SubscriptionStore // B6 订阅与通知：内容订阅（订阅文件/目录，更新即通知）
	mentions   *service.MentionStore      // B6 订阅与通知：@提及（解析/落库/通知）
	imBinds    *service.IMBindingStore    // B7 IM 绑定：站内用户 ↔ IM 平台身份映射（绑定码/改绑/解绑）
	media      *service.MediaStore        // B8 媒体底座：媒体元信息（图片宽高；音视频待 ffmpeg）
	// B17 视频转码：进程内单机异步任务管理器（与 media 同源装配；nil=模块缺省降级）。
	transcode  *service.TranscodeManager
	skills     *service.SkillStore        // B9 技能包商业化：上架 / 授权 / 分发（与应用中心同源不同表）
	tags       *service.TagStore
	kb         *service.KBStore
	agent      *ai.Agent
	summarizer *ai.Summarizer
	conceptSum *ai.ConceptSummarizer // A09+A5：概念页 AI 摘要（nil=能力未启用）
	impex      *service.ImpexStore
	notify     *service.NotifyStore
	hub        *webhookHub // 事件投递器（webhook 出站）
	// CoreModules 可选内核模块（DI 注入；nil 即路由不注册，404 = 能力未启用）。
	// 装配由 main 按"应用中心已启用的 capacity 应用"决定（见 main.go 能力门控）。
	sources   *service.SourceStore
	collector Collector
	webdav    *service.WebDAVStore
	digest    *service.DigestStore
	inbox     *service.InboxStore
	review    *service.ReviewStore
	org       *service.OrgStore
	family    *service.FamilyStore // 家族传承记录模块（传家365 同族：一生时间轴/家族树/纪念日；nil=模块未启用）
	store     *service.StoreStore  // 商城（B39：WooCommerce 式；nil=模块未启用）
	// sites 多站点数据访问（SPEC-MS-001 M1）：路由解析 + 配置/主题按站点。
	sites *service.SiteStore
	// reg 内核工具注册表（B2 workflow 的 Type=tool 步骤通过它执行通用工具）。
	// main 装配完成后由 SetRegistry 注入；未注入时 Type=tool 步骤会返回明确错误而非 panic。
	reg *ai.Registry
}

// CoreModules 可选内核模块集合（DI 注入；字段可为 nil——壳侧缺模块时传 CoreModules{} 即可。
// 新增内核模块只扩展本 struct 字段，handler.New 签名不变 → 跨壳同步零签名改动；
// 对应路由在模块为 nil 时不注册（404=能力未启用），不会因缺模块 panic。
type CoreModules struct {
	Sources   *service.SourceStore
	Collector Collector
	WebDAV    *service.WebDAVStore
	Digest    *service.DigestStore
	Inbox     *service.InboxStore
	Review    *service.ReviewStore
	Org       *service.OrgStore
	Family    *service.FamilyStore // 家族传承记录模块（传家365 同族；nil=模块未启用）
	Store     *service.StoreStore   // 商城（B39：WooCommerce 式；nil=模块未启用）
	// Channel 出站通道（上游第 8 字段，P1 预留占位）：恒 nil=未启用（路由不注册）。
	// 上游契约警告：壳侧字段清单不齐不会报错，新能力会静默不可用——故先补齐占位。
	Channel any
	// Skills 能力包商业化（上游第 9 字段）：本壳技能包模块尚未落地，
	// 先以 any 占位保持装配层前向兼容；待 skill_packages 移植后收紧为 *service.SkillStore。
	Skills any
}

// Collector 采集能力接口（seam）：默认实现 *service.Collector，由 main 装配注入。
// 只暴露路由层所需的最小方法集（上游 seam 同义）；为 nil 时采集端点不注册。
type Collector interface {
	StartRunAsync(ctx context.Context, city string, sourceIDs []string, limit int, force bool, trigger string) (int64, error)
}

// New 创建 API。
func New(db *sql.DB, cfg *config.Store, b *bus.Bus, aud *service.AuditStore, gate *ai.Gateway, files *service.FileStore, tags *service.TagStore, kb *service.KBStore, agent *ai.Agent, summarizer *ai.Summarizer, impex *service.ImpexStore, notify *service.NotifyStore, mods CoreModules, sites *service.SiteStore) *API {
	a := &API{db: db, cfg: cfg, b: b, aud: aud, ai: gate, files: files, tags: tags, kb: kb, agent: agent, summarizer: summarizer, impex: impex, notify: notify,
		sources: mods.Sources, collector: mods.Collector, webdav: mods.WebDAV, digest: mods.Digest, inbox: mods.Inbox, review: mods.Review, org: mods.Org, family: mods.Family, sites: sites, store: mods.Store}
	// B39 商城：支付网关全局注册（幂等；缺密钥的通道 Configured()=false，收银台自动隐藏）
	service.RegisterStoreGateways(a.cfg.GetString)
	// 商城服务（由 CoreModules.Store 注入；nil=能力未启用，路由层不注册）
	a.store = mods.Store
	// B44：支付成功 → 订单邮件（含数字商品下载链接）。
	// 钩子挂在 service.MarkPaid 内部，mock/微信/支付宝/Stripe 四条支付路径自动覆盖；
	// 未配 SMTP 时通知器直接静默返回（见 store_notify.go）。
	if a.store != nil {
		a.store.SetPaidNotifier(a.storePaidNotifier)
	}
	// B5：提示词模板仓储（只依赖 db，故在此构造，不动 handler.New 签名）
	a.prompts = service.NewPromptStore(db)
	// B6：订阅 / @提及 仓储（同上，只依赖 db）
	a.subs = service.NewSubscriptionStore(db)
	a.mentions = service.NewMentionStore(db)
	// B7：IM 绑定仓储（同上，只依赖 db）
	a.imBinds = service.NewIMBindingStore(db)
	// B8/B9：媒体元信息与技能包仓储（同上，只依赖 db）
	a.media = service.NewMediaStore(db)
	// B17 视频转码管理器（依赖 files+media+cfg；与 media 同构装配）。
	// ResetStale 把重启残留的 queued/running 标为 failed，避免重启风暴时重复烧 CPU。
	a.transcode = service.NewTranscodeManager(a.files, a.media, a.cfg)
	a.transcode.ResetStale(context.Background())
	a.skills = service.NewSkillStore(db)
	// 事件投递器：订阅全部内核 topic，按站长注册的 webhook 出站投递（进程生命周期常驻）
	a.hub = newWebhookHub(db)
	a.hub.Start(b)
	return a
}

// Routes 组装路由（Go 1.22+ 方法路由）。
func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/ai/ocr", a.aiOCR)
	mux.HandleFunc("POST /api/v1/ai/clean", a.aiClean)
	mux.HandleFunc("POST /api/v1/ai/organize/suggest", a.organizeSuggestBatch)
	mux.HandleFunc("POST /api/v1/ai/organize/apply", a.organizeApply)
	mux.HandleFunc("GET /api/v1/health", a.health)
	mux.HandleFunc("GET /api/v1/version", a.version)
	// 鉴权（安全加固：登录会话 + Bearer token）
	mux.HandleFunc("POST /api/v1/auth/login", a.authLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", a.authLogout)
	mux.HandleFunc("POST /api/v1/auth/password", a.authPassword)
	mux.HandleFunc("GET /api/v1/auth/me", a.authMe)
	// 多用户：注册 / 邮箱验证码 / 绑定 / 资料 / 能力探测（注册与验证码、config 公开）
	mux.HandleFunc("POST /api/v1/auth/register", a.authRegister)
	mux.HandleFunc("POST /api/v1/auth/email-code", a.emailCodeSend)
	mux.HandleFunc("POST /api/v1/auth/email-bind", a.emailBind)
	mux.HandleFunc("PUT /api/v1/auth/profile", a.profileUpdate)
	mux.HandleFunc("GET /api/v1/auth/config", a.authConfig)
	// 多用户：用户管理（owner/admin）
	mux.HandleFunc("GET /api/v1/admin/users", a.adminUsersList)
	mux.HandleFunc("PUT /api/v1/admin/users/{id}", a.adminUsersUpdate)
	// IM 网关（Telegram / 企业微信 → 发博客）
	mux.HandleFunc("POST /api/v1/im/webhook/telegram", a.imWebhook)
	mux.HandleFunc("GET /api/v1/im/webhook/wecom", a.imWebhookWeCom)
	mux.HandleFunc("POST /api/v1/im/webhook/wecom", a.imWebhookWeCom)
	mux.HandleFunc("GET /api/v1/im/status", a.imConfigStatus)
	mux.HandleFunc("PUT /api/v1/im/telegram/config", a.imTelegramConfig)
	mux.HandleFunc("PUT /api/v1/im/wecom/config", a.imWeComConfig)
	// IM 绑定（B7）：网页端生成绑定码 → IM 端 /bind <码> 消费；列表与解绑按当前用户隔离
	mux.HandleFunc("GET /api/v1/im/bindings", a.imBindingsList)
	mux.HandleFunc("POST /api/v1/im/bindings/code", a.imBindingsCreateCode)
	mux.HandleFunc("DELETE /api/v1/im/bindings/{id}", a.imBindingsDelete)
	mux.HandleFunc("GET /api/v1/admin/settings", a.settings)
	mux.HandleFunc("PUT /api/v1/admin/settings", a.updateSettings)
	mux.HandleFunc("GET /api/v1/system/backup", a.systemBackupGet)
	mux.HandleFunc("PUT /api/v1/system/backup", a.systemBackupPut)
	// 能力模块生效态（设置页运维面板；只读，含判定来源）
	mux.HandleFunc("GET /api/v1/admin/capabilities", a.capabilityStates)
	// 公开能力开关（只读布尔）：侧栏按应用中心安装状态渲染 family/org 导航入口
	mux.HandleFunc("GET /api/v1/public/capabilities", a.publicCapabilities)
	// 派生资产运维（B16）：占用/孤儿统计 + 一键清理（清孤儿、或改参数后强制按新参数重生成）
	mux.HandleFunc("GET /api/v1/admin/derived/stats", a.derivedStats)
	mux.HandleFunc("POST /api/v1/admin/derived/purge", a.derivedPurge)
	mux.HandleFunc("POST /api/v1/admin/derived/sweep", a.derivedSweep)
	mux.HandleFunc("GET /api/v1/admin/ai/providers", a.aiProvidersList)
	mux.HandleFunc("GET /api/v1/admin/ai/providers/all", a.aiProvidersManage) // 全量注册表（管理面板）
	mux.HandleFunc("PUT /api/v1/admin/ai/providers", a.aiProviderUpsert)       // 新增/更新 provider（热更）
	mux.HandleFunc("DELETE /api/v1/admin/ai/providers", a.aiProviderDelete)    // 删除 provider（禁删内置）
	mux.HandleFunc("POST /api/v1/admin/ai/providers/test", a.aiProviderTest)   // 连通性校验
	mux.HandleFunc("GET /api/v1/admin/ai/usage", a.adminAIUsage) // B9 ai_ask_usage 聚合（生产 UI 消费）
	// 平台模型用量管控（ai_billing.go：全能力 token 级配额 + 增值计费，自备模型不计量）
	mux.HandleFunc("GET /api/v1/admin/ai/quota-policy", a.aiQuotaPolicyGet)
	mux.HandleFunc("PUT /api/v1/admin/ai/quota-policy", a.aiQuotaPolicySave)
	mux.HandleFunc("GET /api/v1/admin/ai/balance", a.aiBalanceGet)
	mux.HandleFunc("POST /api/v1/admin/ai/balance", a.aiBalanceGrant)
	mux.HandleFunc("GET /api/v1/admin/ai/llm-usage", a.aiUsageList) // B27 A1 llm_usage 聚合（上游同路径已占用，改挂 llm-usage）
	mux.HandleFunc("POST /api/v1/admin/ai/providers/switch", a.aiProvidersSwitch)
	mux.HandleFunc("POST /api/v1/admin/ai/providers/custom", a.aiCustomProviderSave)
	mux.HandleFunc("DELETE /api/v1/admin/ai/providers/custom", a.aiCustomProviderClear)
	mux.HandleFunc("POST /api/v1/admin/ai/tts", a.aiTTS) // 语音合成自测/试听（MiMo chat-audio 契约）
	mux.HandleFunc("POST /api/v1/admin/ai/asr", a.aiASR) // 语音识别自测（预留产品级消费入口）
	mux.HandleFunc("POST /api/v1/admin/tags/apply-rules", a.ruleTagsApply)
	mux.HandleFunc("GET /api/v1/admin/audit", a.auditList)
	mux.HandleFunc("GET /api/v1/admin/audit/verify", a.auditVerify)
	mux.HandleFunc("GET /api/v1/admin/analytics/overview", a.adminAnalyticsOverview) // A5 站级阅读看板
	// 技能包（B9）：目录只读 / 我的授权 / 免费包自助领取；上架与发放限 owner/admin
	mux.HandleFunc("GET /api/v1/skills", a.skillsList)
	mux.HandleFunc("GET /api/v1/skills/grants", a.skillsGrants)
	mux.HandleFunc("POST /api/v1/skills/{id}/claim", a.skillsClaim)
	mux.HandleFunc("POST /api/v1/admin/skills", a.adminSkillUpsert)
	mux.HandleFunc("POST /api/v1/admin/skills/{id}/grant", a.adminSkillGrant)
	// 文件 API（阶段 1）
	mux.HandleFunc("GET /api/v1/files", a.filesList)
	mux.HandleFunc("GET /api/v1/files/resolve", a.filesResolveWiki)
	mux.HandleFunc("GET /api/v1/search", a.search)
	mux.HandleFunc("GET /api/v1/files/tree", a.filesTree)
	mux.HandleFunc("GET /api/v1/files/{id}/summary", a.filesSummary)
	mux.HandleFunc("GET /api/v1/files/{id}/links", a.fileLinks)
	mux.HandleFunc("GET /api/v1/files/{id}/locate", a.locateChunk)
	mux.HandleFunc("GET /api/v1/files/{id}/heatmap", a.fileHeatmap) // A5 单篇阅读热力图（站长后台）
	// B5 版本历史：列表 / 下载 / 恢复（挂在 files 组内，路径与上游契约一致）
	mux.HandleFunc("GET /api/v1/files/{id}/versions", a.fileVersions)
	mux.HandleFunc("GET /api/v1/files/{id}/versions/{v}/content", a.fileVersionContent)
	mux.HandleFunc("POST /api/v1/files/{id}/versions/{v}/restore", a.fileVersionRestore)
	mux.HandleFunc("GET /api/v1/chunks/{id}", a.chunkGet)
	mux.HandleFunc("POST /api/v1/files/upload", a.filesUpload)
	mux.HandleFunc("POST /api/v1/files/mkdir", a.filesMkdir)
	mux.HandleFunc("POST /api/v1/files/doc", a.filesCreateDoc)
	mux.HandleFunc("PUT /api/v1/files/{id}/content", a.filesUpdateContent)
	mux.HandleFunc("PUT /api/v1/files/{id}/status", a.filesSetStatus)
	mux.HandleFunc("GET /api/v1/files/{id}", a.filesGet)
	mux.HandleFunc("POST /api/v1/files/{id}/move", a.filesMove)
	mux.HandleFunc("POST /api/v1/files/{id}/copy", a.filesCopy)
	mux.HandleFunc("DELETE /api/v1/files/{id}", a.filesDelete)
	// 回收站
	mux.HandleFunc("POST /api/v1/files/{id}/restore", a.filesRestore)
	mux.HandleFunc("DELETE /api/v1/files/{id}/purge", a.filesPurge)
	mux.HandleFunc("POST /api/v1/files/trash/empty", a.filesEmptyTrash)
	mux.HandleFunc("GET /api/v1/files/{id}/content", a.filesContent)

	// B5 写作增强：AI 写作提示词模板（列表/增改删/渲染）
	mux.HandleFunc("GET /api/v1/prompts", a.promptsList)
	mux.HandleFunc("POST /api/v1/prompts", a.promptsCreate)
	mux.HandleFunc("GET /api/v1/prompts/{id}", a.promptsGet)
	mux.HandleFunc("PUT /api/v1/prompts/{id}", a.promptsUpdate)
	mux.HandleFunc("DELETE /api/v1/prompts/{id}", a.promptsDelete)
	mux.HandleFunc("POST /api/v1/prompts/{id}/render", a.promptsRender)
	mux.HandleFunc("GET /api/v1/files/{id}/tags", a.fileTagsGet)
	mux.HandleFunc("PUT /api/v1/files/{id}/tags", a.fileTagsSet)
	// 标签 / 分类（树状，网盘打标 → 知识库/博客/检索共用）
	mux.HandleFunc("GET /api/v1/tags", a.tagsList)
	mux.HandleFunc("POST /api/v1/tags", a.tagsCreate)
	mux.HandleFunc("PUT /api/v1/tags/{id}", a.tagsRename)
	mux.HandleFunc("DELETE /api/v1/tags/{id}", a.tagsDelete)
	// 知识库聚合层（阶段 2：总览/标签聚合/概念页/图谱/集合）
	mux.HandleFunc("GET /api/v1/kb/overview", a.kbOverview)
	mux.HandleFunc("POST /api/v1/kb/summaries/retry", a.kbSummariesRetry)
	mux.HandleFunc("POST /api/v1/kb/summaries/requeue", a.kbSummariesRequeue)
	mux.HandleFunc("GET /api/v1/kb/tags", a.kbTags)
	// A2 FAQ 知识库类型（WeKnora 借鉴：标准问/相似问/反例问/答案，问题即检索单元；B27 移植）
	mux.HandleFunc("GET /api/v1/kb/faqs", a.kbFAQList)
	mux.HandleFunc("POST /api/v1/kb/faqs", a.kbFAQCreate)
	mux.HandleFunc("PUT /api/v1/kb/faqs/{id}", a.kbFAQUpdate)
	mux.HandleFunc("DELETE /api/v1/kb/faqs/{id}", a.kbFAQDelete)
	mux.HandleFunc("GET /api/v1/kb/faqs/search", a.kbFAQSearch)
	// A3 空间级长期记忆（WeKnora 借鉴 + 云雇工叙事：Agent/任务读写空间上下文；B27 移植）
	mux.HandleFunc("GET /api/v1/kb/memory", a.kbMemoryList)
	mux.HandleFunc("POST /api/v1/kb/memory", a.kbMemorySet)
	mux.HandleFunc("DELETE /api/v1/kb/memory", a.kbMemoryDelete)
	mux.HandleFunc("GET /api/v1/kb/memory/context", a.kbMemoryContext)
	mux.HandleFunc("GET /api/v1/kb/concepts/{id}", a.kbConcept)
	mux.HandleFunc("POST /api/v1/kb/concepts/{id}/summarize", a.kbConceptSummarize) // A09：概念页 AI 独立摘要（生成并缓存）
	mux.HandleFunc("GET /api/v1/kb/graph", a.kbGraph)
	mux.HandleFunc("GET /api/v1/kb/lint", a.kbLint)
	mux.HandleFunc("POST /api/v1/kb/dedup/resolve", a.kbDedupResolve)
	mux.HandleFunc("GET /api/v1/collections", a.collectionsList)
	mux.HandleFunc("POST /api/v1/collections", a.collectionsCreate)
	mux.HandleFunc("DELETE /api/v1/collections/{id}", a.collectionsDelete)
	mux.HandleFunc("GET /api/v1/collections/{id}/files", a.collectionsFiles)
	mux.HandleFunc("POST /api/v1/collections/{id}/files", a.collectionsAddFile)
	mux.HandleFunc("DELETE /api/v1/collections/{id}/files/{fileId}", a.collectionsRemoveFile)
	// AI 对话（阶段 2：Agent + 工具链 + 多会话）
	mux.HandleFunc("GET /api/v1/ai/conversations", a.convList)
	mux.HandleFunc("POST /api/v1/ai/conversations", a.convCreate)
	mux.HandleFunc("DELETE /api/v1/ai/conversations/{id}", a.convDelete)
	mux.HandleFunc("GET /api/v1/ai/conversations/{id}/messages", a.convMessages)
	mux.HandleFunc("POST /api/v1/ai/chat", a.aiChat)
	// 分享/发布（博客公开页底层；token 访问不依赖内部文件 ID）
	mux.HandleFunc("GET /api/v1/shares", a.sharesList)
	mux.HandleFunc("POST /api/v1/shares", a.sharesCreate)
	mux.HandleFunc("GET /api/v1/public/posts", a.publicPosts)
	mux.HandleFunc("GET /api/v1/public/tags", a.publicTags)
	mux.HandleFunc("GET /api/v1/public/site", a.blogSiteGet)
	mux.HandleFunc("GET /api/v1/public/popular", a.popularPosts)
	mux.HandleFunc("POST /api/v1/public/blog/ask", a.publicBlogAsk)
	mux.HandleFunc("GET /api/v1/public/blog/ask/quota", a.publicBlogAskQuota)
	// 博客文章语音朗读（公开；仅已发布文章，复用 ai.tts 能力级路由 + 磁盘缓存）
	mux.HandleFunc("GET /api/v1/public/blog/tts", a.publicBlogTTS)
	// 相关文章语义推荐（公开；仅已发布文章，向量质心余弦；无向量时返回空由前端降级）
	mux.HandleFunc("GET /api/v1/public/blog/related", a.publicBlogRelated)
	// 客服模块（SPEC-CS-001 M0）：公开挂件轨 + 后台工作台 + 渠道回调
	mux.HandleFunc("POST /api/v1/public/cs/start", a.publicCSStart)
	mux.HandleFunc("POST /api/v1/public/cs/send", a.publicCSSend)
	mux.HandleFunc("GET /api/v1/public/cs/pull", a.publicCSPull)
	mux.HandleFunc("GET /api/v1/cs/inbox", a.csInbox)
	mux.HandleFunc("GET /api/v1/cs/contacts", a.csContacts)
	mux.HandleFunc("GET /api/v1/cs/conversations/{id}/messages", a.csConversationMessages)
	mux.HandleFunc("POST /api/v1/cs/conversations/{id}/reply", a.csConversationReply)
	mux.HandleFunc("POST /api/v1/cs/conversations/{id}/draft", a.csConversationDraft)
	mux.HandleFunc("POST /api/v1/cs/conversations/{id}/status", a.csSetStatus)
	mux.HandleFunc("POST /api/v1/cs/hooks/{channel}/{token}", a.csHook)
	// 多租户 SaaS 计费（SPEC-BILLING）
	mux.HandleFunc("GET /api/v1/public/billing/plans", a.publicBillingPlans)
	mux.HandleFunc("GET /api/v1/admin/billing/plans", a.billingPlans)
	mux.HandleFunc("POST /api/v1/admin/billing/plans", a.billingPlanUpdate)
	mux.HandleFunc("GET /api/v1/admin/billing/sites/{site}/subscription", a.billingSiteSub)
	mux.HandleFunc("POST /api/v1/admin/billing/sites/{site}/subscription", a.billingSiteSubscribe)
	mux.HandleFunc("POST /api/v1/admin/billing/sites/{site}/subscription/cancel", a.billingSiteCancel)
	mux.HandleFunc("GET /api/v1/admin/billing/sites/{site}/usage", a.billingUsage)
	mux.HandleFunc("GET /api/v1/public/media/{id}", a.publicMediaGet)
	// 公开媒体元信息（B13）：匿名可读，仅博客子树内的 image|video|audio
	mux.HandleFunc("GET /api/v1/public/media/{id}/info", a.publicMediaInfoGet)
	// 公开缩略图（B15）：列表/卡片用 480px JPEG，避免拉原图（常为 MB 级）
	mux.HandleFunc("GET /api/v1/public/media/{id}/thumb", a.publicMediaThumb)
	// 公开转码播放源（B17）：门控关闭或暂无产物时回落原文件（边界与 publicMediaThumb 完全一致）。
	mux.HandleFunc("GET /api/v1/public/media/{id}/play", a.publicMediaPlay)
	// 媒体元信息（B8/B13）：图片宽高（header 级探测）+ 音视频时长/分辨率/编码（ffprobe）
	mux.HandleFunc("GET /api/v1/files/{id}/media", a.fileMediaGet)
	// 缩略图（B15，登录态）：文件管理器网格/列表用；生成是懒的（首次请求时调 ffmpeg）
	mux.HandleFunc("GET /api/v1/files/{id}/thumb", a.fileMediaThumb)
	// 批量媒体信息（B15）：一次取回整个列表已缓存的元信息（只读缓存，不触发探测）
	mux.HandleFunc("POST /api/v1/files/media/batch", a.filesMediaBatch)
	// 视频转码（B17）：触发 / 状态 / 取消（登录态；门控关闭时端点 404）。
	mux.HandleFunc("POST /api/v1/files/{id}/transcode", a.fileTranscodeTrigger)
	mux.HandleFunc("GET /api/v1/files/{id}/transcode", a.fileTranscodeStatus)
	mux.HandleFunc("POST /api/v1/files/{id}/transcode/cancel", a.fileTranscodeCancel)
	mux.HandleFunc("POST /api/v1/public/blog/pv", a.publicBlogPV)
	mux.HandleFunc("GET /api/v1/public/blog/pv", a.publicBlogPVGet)
	mux.HandleFunc("GET /api/v1/public/comments", a.commentsList)
	mux.HandleFunc("POST /api/v1/comments", a.commentsCreate)
	mux.HandleFunc("POST /api/v1/public/comments", a.publicGuestComment)
	// 评论管理（站长）：列表/审核/删除
	mux.HandleFunc("GET /api/v1/blog/comments", a.blogCommentsAdmin)
	mux.HandleFunc("POST /api/v1/blog/comments/{id}/approve", a.blogCommentApprove)
	mux.HandleFunc("DELETE /api/v1/blog/comments/{id}", a.blogCommentDelete)
	// Webhook 投递端点管理（事件契约 v1 投递器）
	mux.HandleFunc("GET /api/v1/blog/webhooks", a.blogWebhookList)
	mux.HandleFunc("POST /api/v1/blog/webhooks", a.blogWebhookCreate)
	mux.HandleFunc("DELETE /api/v1/blog/webhooks/{id}", a.blogWebhookDelete)
	mux.HandleFunc("POST /api/v1/blog/webhooks/{id}/test", a.blogWebhookTest)
	// AI 写作工作台（登录）：素材检索 / 大纲 / 续写 / 润色 / 标签建议
	mux.HandleFunc("POST /api/v1/blog/ai/search", a.blogAISearch)
	mux.HandleFunc("POST /api/v1/blog/ai/outline", a.blogAIOutline)
	mux.HandleFunc("POST /api/v1/blog/ai/continue", a.blogAIContinue)
	mux.HandleFunc("POST /api/v1/blog/ai/polish", a.blogAIPolish)
	mux.HandleFunc("POST /api/v1/blog/ai/tags", a.blogAITags)
	mux.HandleFunc("GET /api/v1/blog/manage", a.blogManage)
	mux.HandleFunc("GET /api/v1/blog/stats", a.blogStats)
	mux.HandleFunc("POST /api/v1/blog/categories/sort", a.blogCategoriesSort)
	mux.HandleFunc("POST /api/v1/blog/posts/pin", a.blogPostsPin)
	mux.HandleFunc("POST /api/v1/blog/posts/meta", a.blogPostMeta)
	mux.HandleFunc("POST /api/v1/blog/posts/access", a.blogPostsAccess)
	mux.HandleFunc("POST /api/v1/blog/posts/schedule", a.blogPostsSchedule)
	// B 项内容付费（自上游同步移植）：访问控制统一设置（none/password/paid）+ 通用支付回调
	mux.HandleFunc("POST /api/v1/blog/posts/paid", a.blogPostsPaid)
	// P2 链接体检：公开只读端点（仅 http/https，拒绝私网/保留地址；SSRF 防护见 link_check.go）
	mux.HandleFunc("GET /api/v1/public/link-check", a.publicLinkCheck)
	// C 项批量编辑：多选后批量改 node_type/fields/封面/摘要/SEO（逐篇更新，单条失败不阻断）
	mux.HandleFunc("POST /api/v1/blog/posts/batch", a.blogPostsBatch)
	// 支付回调匿名可达（HMAC 签名校验 + 幂等签发 grant_token；见 authmw 白名单）
	mux.HandleFunc("POST /api/v1/pay/notify", a.payNotify)
	mux.HandleFunc("POST /api/v1/public/unlock", a.publicUnlock)
	// 博客功能插件协议 + RSS
	mux.HandleFunc("GET /api/v1/blog/plugins", a.blogPluginList)
	mux.HandleFunc("POST /api/v1/blog/plugins", a.blogPluginUpsert)
	mux.HandleFunc("POST /api/v1/admin/apps/install-zip", a.appInstallZip)
	// 采集源包（SPEC-SP-001）：与插件/主题同走应用中心安装链路，落点不同（写 sources 表）
	mux.HandleFunc("POST /api/v1/admin/apps/install-source-pack", a.sourcePackInstall)
	mux.HandleFunc("GET /api/v1/admin/apps/source-packs", a.sourcePackList)
	mux.HandleFunc("DELETE /api/v1/admin/apps/source-packs/{id}", a.sourcePackUninstall)
	// 应用中心（在线目录）：浏览远程市场 + 一键安装
	mux.HandleFunc("GET /api/v1/admin/apps/market", a.marketList)
	mux.HandleFunc("POST /api/v1/admin/apps/market/install", a.marketInstall)
	// 站长博客侧市场（上游 blog_market.go 三端点移植，2026-09-18 授权；与 admin 侧同构同管线）
	mux.HandleFunc("GET /api/v1/blog/market", a.marketList)
	mux.HandleFunc("POST /api/v1/blog/market/install", a.marketInstall)
	// 公开 /market 门户（无需登录浏览；index.json 为本实例合并视图）
	mux.HandleFunc("GET /market", a.marketPortal)
	mux.HandleFunc("GET /market/", a.marketPortal)
	mux.HandleFunc("GET /market/index.json", a.marketIndexFile)
	mux.HandleFunc("GET /market/official/{file}", a.officialMarketFile)
	// B47 回源统计看板（登录态；官方侧聚合，不含用户数据）
	mux.HandleFunc("GET /api/v1/admin/market/beacon", a.marketBeaconStats)
	// 实例许可证（0.5.x 市场多壳模型配套；付费安装门禁用）
	mux.HandleFunc("GET /api/v1/license", a.licenseStatus)
	mux.HandleFunc("POST /api/v1/license", a.licenseActivate)
	mux.HandleFunc("DELETE /api/v1/license", a.licenseDeactivate)
	mux.HandleFunc("POST /api/v1/license/verify", a.licenseVerify) // REQ-007：在线验签（各壳回源，公开）

	// 内容工作流（B2）：步骤链引擎（triggers/steps/on_fail + $msg.$prev.$ctx 模板）。
	// 端点与上游同构，便于 workflow 定义跨壳搬运；鉴权=站点管理员（owner/admin）。
	mux.HandleFunc("GET /api/v1/workflows", a.wfList)
	mux.HandleFunc("POST /api/v1/workflows", a.wfSave)
	mux.HandleFunc("POST /api/v1/workflows/{id}/run", a.wfRunAPI)
	mux.HandleFunc("DELETE /api/v1/workflows/{id}", a.wfDelete)

	// B3 评论收录（内容引用与归因）：引用式直写 / 融合式两段式（draft→accept）/ 区块级撤销 + 回查。
	mux.HandleFunc("POST /api/v1/ingests/quote", a.ingestQuote)
	mux.HandleFunc("POST /api/v1/ingests/fuse-draft", a.ingestFuseDraft)
	mux.HandleFunc("GET /api/v1/ingests", a.ingestList)
	mux.HandleFunc("GET /api/v1/ingests/pending", a.ingestPendings)
	mux.HandleFunc("GET /api/v1/ingests/my-count", a.ingestMyCount)
	mux.HandleFunc("POST /api/v1/ingests/{id}/accept", a.ingestAccept)
	mux.HandleFunc("DELETE /api/v1/ingests/{id}", a.ingestRevert)
	// 多站点管理后台（SPEC-MS-001 M3）：站点 CRUD + 设置 + 站点数授权（owner/admin）。
	mux.HandleFunc("GET /api/v1/admin/sites", a.adminSitesList)
	mux.HandleFunc("POST /api/v1/admin/sites", a.adminSiteCreate)
	mux.HandleFunc("GET /api/v1/admin/sites/{id}", a.adminSiteGet)
	mux.HandleFunc("PUT /api/v1/admin/sites/{id}", a.adminSiteUpdate)
	mux.HandleFunc("DELETE /api/v1/admin/sites/{id}", a.adminSiteDelete)
	mux.HandleFunc("GET /api/v1/admin/sites/{id}/settings", a.adminSiteSettingsGet)
	mux.HandleFunc("PUT /api/v1/admin/sites/{id}/settings", a.adminSiteSettingsUpdate)
	mux.HandleFunc("GET /api/v1/admin/sites/licenses", a.adminSiteLicenses)
	mux.HandleFunc("POST /api/v1/admin/sites/licenses", a.adminSiteLicenseGrant)
	mux.HandleFunc("POST /api/v1/blog/plugins/{id}/toggle", a.blogPluginToggle)
	mux.HandleFunc("DELETE /api/v1/blog/plugins/{id}", a.blogPluginDelete)
	mux.HandleFunc("GET /api/v1/blog/feed.xml", a.blogRSS)
	mux.HandleFunc("GET /api/v1/blog/sitemap.xml", a.blogSitemap)
	// 博客文章稳定入库（Aikdex 采集投递）+ 插件数据 KV
	mux.HandleFunc("POST /api/v1/blog/posts", a.blogPostCreate)
	mux.HandleFunc("GET /api/v1/blog/plugins/{id}/settings-schema", a.pluginSettingsSchema)
	mux.HandleFunc("GET /api/v1/blog/plugins/{id}/kv", a.blogPluginKVList)
	mux.HandleFunc("POST /api/v1/blog/plugins/{id}/kv", a.blogPluginKVSet)
	mux.HandleFunc("DELETE /api/v1/blog/plugins/{id}/kv", a.blogPluginKVDelete)
	mux.HandleFunc("GET /api/v1/shares/{token}", a.sharesGet)
	mux.HandleFunc("GET /api/v1/shares/{token}/content", a.sharesContent)
	mux.HandleFunc("GET /api/v1/shares/{token}/thumb", a.sharesThumb)
	mux.HandleFunc("DELETE /api/v1/shares/{token}", a.sharesRevoke)
	// 导入导出中心（R11/R12：异步任务 + 进度 + 失败清单可重试）
	mux.HandleFunc("POST /api/v1/imports", a.impexCreateImport)
	mux.HandleFunc("GET /api/v1/imports", a.impexListImports)
	mux.HandleFunc("GET /api/v1/imports/{id}", a.impexGet)
	mux.HandleFunc("POST /api/v1/imports/{id}/retry", a.impexRetry)
	mux.HandleFunc("POST /api/v1/exports", a.impexCreateExport)
	mux.HandleFunc("GET /api/v1/exports", a.impexListExports)
	mux.HandleFunc("GET /api/v1/exports/{id}", a.impexGet)
	mux.HandleFunc("GET /api/v1/exports/{id}/download", a.impexDownload)
	mux.HandleFunc("POST /api/v1/exports/{id}/retry", a.impexRetry)
	// 消息通知中心（R-notify：导入导出/索引终态提醒 + 未读红点轮询）
	mux.HandleFunc("GET /api/v1/notifications", a.notifications)
	mux.HandleFunc("POST /api/v1/notifications/read", a.notificationsRead)
	mux.HandleFunc("GET /api/v1/notifications/unread-count", a.notificationsUnread)
	// 内容订阅（B6 订阅与通知）：订阅文件/目录，内容更新时向其投递通知
	mux.HandleFunc("GET /api/v1/subscriptions", a.subscriptionsList)
	mux.HandleFunc("GET /api/v1/subscriptions/status", a.subscriptionsStatus)
	mux.HandleFunc("POST /api/v1/subscriptions", a.subscriptionsCreate)
	mux.HandleFunc("DELETE /api/v1/subscriptions/{target_type}/{target_id}", a.subscriptionsDelete)
	// @提及（B6）：我被提及的记录 / 未读数 / 全部已读
	mux.HandleFunc("GET /api/v1/mentions", a.mentionsList)
	mux.HandleFunc("GET /api/v1/mentions/unread-count", a.mentionsUnread)
	mux.HandleFunc("POST /api/v1/mentions/read", a.mentionsRead)
	// 公开博客服务端 HTML（SEO：无哈希路径，爬虫可读）
	// 中文根路径 /{slug} 为主链；/blog/{slug} 兼容保留
	mux.HandleFunc("GET /blog", a.blogHTMLIndex)
	mux.HandleFunc("GET /blog/{slug}", a.blogHTMLPost)
	mux.HandleFunc("GET /{slug}", a.blogHTMLPost)
	// ---- CoreModules：可选内核模块路由（nil 即不注册，404 = 能力未启用）----
	// 采集源配置与采集运行（R5 契约：POST /collect/run 立即返回 run_id，异步入库；
	// GET /collect/runs 查状态。第三方采集引擎持 service token 调用即此组端点。）
	if a.sources != nil {
		mux.HandleFunc("GET /api/v1/sources", a.sourcesList)
		mux.HandleFunc("POST /api/v1/sources", a.sourcesCreate)
		mux.HandleFunc("PUT /api/v1/sources/{id}", a.sourcesUpdate)
		mux.HandleFunc("DELETE /api/v1/sources/{id}", a.sourcesDelete)
		mux.HandleFunc("POST /api/v1/collect/run", a.collectRun)
		mux.HandleFunc("GET /api/v1/collect/runs", a.collectRuns)
	}
	// 收件箱（采集聚合/未读/归档）
	if a.inbox != nil {
		mux.HandleFunc("GET /api/v1/inbox", a.inboxList)
		mux.HandleFunc("GET /api/v1/inbox/unread-count", a.inboxUnread)
		mux.HandleFunc("POST /api/v1/inbox/{id}/archive", a.inboxArchive)
		mux.HandleFunc("POST /api/v1/inbox/archive-all", a.inboxArchiveAll)
	}
	// 每日知识日报
	if a.digest != nil {
		mux.HandleFunc("POST /api/v1/digest/run", a.digestRun)
		mux.HandleFunc("GET /api/v1/digest/preview", a.digestPreview)
	}
	// 知识质量评审
	if a.review != nil {
		mux.HandleFunc("GET /api/v1/review/queue", a.reviewQueue)
		mux.HandleFunc("POST /api/v1/review/rate", a.reviewRate)
		mux.HandleFunc("GET /api/v1/review/stats", a.reviewStats)
		mux.HandleFunc("POST /api/v1/review/enroll", a.reviewEnroll)
		mux.HandleFunc("POST /api/v1/review/status", a.reviewStatus)
	}
	// WebDAV：对外服务端（/api/v1/dav/ 前缀，Basic 认证）+ 外部挂载管理（导入远端文件）
	// 服务端需额外开关 dav.enabled（默认开）；挂载管理随 webdav 模块装配。
	if a.webdav != nil {
		if a.davEnabled() {
			mux.Handle("/api/v1/dav/", a.davHandler())
		}
		mux.HandleFunc("GET /api/v1/webdav/mounts", a.webdavMountsList)
		mux.HandleFunc("POST /api/v1/webdav/mounts", a.webdavMountsCreate)
		mux.HandleFunc("PUT /api/v1/webdav/mounts/{id}", a.webdavMountsUpdate)
		mux.HandleFunc("DELETE /api/v1/webdav/mounts/{id}", a.webdavMountsDelete)
		mux.HandleFunc("POST /api/v1/webdav/mounts/{id}/test", a.webdavMountTest)
		mux.HandleFunc("GET /api/v1/webdav/mounts/{id}/list", a.webdavMountList)
		mux.HandleFunc("GET /api/v1/webdav/mounts/{id}/content", a.webdavMountContent)
		mux.HandleFunc("POST /api/v1/webdav/mounts/{id}/import", a.webdavMountImport)
	}
	// 组织架构（Org）：组织树/任职/部门空间/移交流
	if a.org != nil {
		mux.HandleFunc("GET /api/v1/org/settings", a.orgSettings)
		mux.HandleFunc("POST /api/v1/org/departments", a.orgDepartmentsCreate)
		mux.HandleFunc("GET /api/v1/org/departments", a.orgDepartmentsList)
		mux.HandleFunc("POST /api/v1/org/departments/{id}/members", a.orgDepartmentsMembers)
		mux.HandleFunc("GET /api/v1/org/departments/{id}/files", a.orgDepartmentsFiles)
		mux.HandleFunc("GET /api/v1/org/tree", a.orgTreeList)
		mux.HandleFunc("POST /api/v1/org/tree", a.orgTreeCreate)
		mux.HandleFunc("PUT /api/v1/org/tree/{id}", a.orgTreeUpdate)
		mux.HandleFunc("DELETE /api/v1/org/tree/{id}", a.orgTreeDelete)
		mux.HandleFunc("GET /api/v1/org/tree/paths", a.orgTreePaths)
		mux.HandleFunc("POST /api/v1/org/memberships", a.orgMembershipsSet)
		mux.HandleFunc("GET /api/v1/org/members", a.orgNodeMembers)
		mux.HandleFunc("GET /api/v1/org/memberships", a.orgMembershipsHistory)
		mux.HandleFunc("POST /api/v1/org/transfers", a.orgTransfersCreate)
		mux.HandleFunc("GET /api/v1/org/transfers", a.orgTransfersList)
		mux.HandleFunc("GET /api/v1/org/transfers/{id}/items", a.orgTransferItems)
		mux.HandleFunc("POST /api/v1/org/transfers/{id}/preview", a.orgTransferPreview)
		mux.HandleFunc("POST /api/v1/org/transfers/{id}/items/{itemId}", a.orgTransferItemSkip)
		mux.HandleFunc("POST /api/v1/org/transfers/{id}/execute", a.orgTransferExecute)
		mux.HandleFunc("PUT /api/v1/org/transfers/{id}/accept", a.orgTransferAccept)
		mux.HandleFunc("POST /api/v1/org/transfers/{id}/cancel", a.orgTransferCancel)
		mux.HandleFunc("GET /api/v1/org/custodian-count", a.orgCustodianCount)
	}
	// 家族传承记录模块（[family] REQ-010/011 移植：一生时间轴/家族树/纪念日；开关 settings family.enabled，模块 nil=不注册）。
	// 付费门禁：除 settings（开关状态，供前端入口判定）外全部端点经 familyPaidGate（scope=feature:family，复用 orgPaidGate 模型）。
	if a.family != nil {
		fam := func(h http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				if !a.familyPaidGate(w, r) {
					return
				}
				h(w, r)
			}
		}
		mux.HandleFunc("GET /api/v1/family/settings", a.familySettings)
		mux.HandleFunc("GET /api/v1/family/members", fam(a.familyMembersList))
		mux.HandleFunc("POST /api/v1/family/members", fam(a.familyMembersCreate))
		mux.HandleFunc("PUT /api/v1/family/members/{id}", fam(a.familyMembersUpdate))
		mux.HandleFunc("DELETE /api/v1/family/members/{id}", fam(a.familyMembersDelete))
		mux.HandleFunc("GET /api/v1/family/relations", fam(a.familyRelationsList))
		mux.HandleFunc("POST /api/v1/family/relations", fam(a.familyRelationsCreate))
		mux.HandleFunc("DELETE /api/v1/family/relations/{id}", fam(a.familyRelationsDelete))
		mux.HandleFunc("GET /api/v1/family/tree", fam(a.familyTree))
		mux.HandleFunc("GET /api/v1/family/timeline", fam(a.familyTimeline))
		mux.HandleFunc("POST /api/v1/family/advise", fam(a.familyAdvise)) // AI 人生参谋（二期①）
		mux.HandleFunc("GET /api/v1/family/anniversaries", fam(a.familyAnniversariesList))
		mux.HandleFunc("POST /api/v1/family/anniversaries", fam(a.familyAnniversariesCreate))
		mux.HandleFunc("PUT /api/v1/family/anniversaries/{id}", fam(a.familyAnniversariesUpdate))
		mux.HandleFunc("DELETE /api/v1/family/anniversaries/{id}", fam(a.familyAnniversariesDelete))
	}
	// 商城（B39：WooCommerce 式）—— 商品/购物车/收银台/订单/优惠券/支付回调。
	// 模块 nil=不注册（能力门控）；公开端点匿名可达，管理端点经 isAdmin，回调端点经网关签名校验。
	if a.store != nil {
		// 公开：商品目录 / 详情
		mux.HandleFunc("GET /api/v1/store/products", a.storeProducts)
		mux.HandleFunc("GET /api/v1/store/products/{slug}", a.storeProductDetail)
		// 公开：商品封面图（受限图床，仅已发布商品引用的位图）
		mux.HandleFunc("GET /api/v1/store/media/{id}", a.storeMediaGet)
		// 公开：购物车（匿名 cookie）
		mux.HandleFunc("GET /api/v1/store/cart", a.storeCartGet)
		mux.HandleFunc("POST /api/v1/store/cart/add", a.storeCartAdd)
		mux.HandleFunc("POST /api/v1/store/cart/set", a.storeCartSet)
		mux.HandleFunc("POST /api/v1/store/cart/remove", a.storeCartRemove)
		mux.HandleFunc("POST /api/v1/store/cart/clear", a.storeCartClear)
		// 公开：可用支付通道 + 收银台下单 + 订单查询 + mock 直 confirm
		mux.HandleFunc("GET /api/v1/store/gateways", a.storeGateways)
		mux.HandleFunc("POST /api/v1/store/checkout", a.storeCheckout)
		mux.HandleFunc("GET /api/v1/store/orders/lookup", a.storeOrderLookup)
		mux.HandleFunc("GET /api/v1/store/order/{no}", a.storeOrderDetail)
		// 公开：买家发起退货申请（订单号 + 下单邮箱双因子）
		mux.HandleFunc("POST /api/v1/store/orders/return", a.storeReturnCreate)
		mux.HandleFunc("GET /api/v1/store/order/{no}/download/{itemId}", a.storeOrderDownload)
		mux.HandleFunc("POST /api/v1/store/mock-pay/{no}", a.storeMockPay)
		// 支付异步回调（匿名可达，网关签名校验）
		mux.HandleFunc("POST /api/v1/store/notify/wechat", a.storeNotifyWeChat)
		mux.HandleFunc("POST /api/v1/store/notify/alipay", a.storeNotifyAlipay)
		mux.HandleFunc("POST /api/v1/store/notify/stripe", a.storeNotifyStripe)
		// 管理：商品 CRUD
		mux.HandleFunc("GET /api/v1/admin/store/products", a.adminStoreProducts)
		mux.HandleFunc("POST /api/v1/admin/store/products", a.adminStoreProductCreate)
		mux.HandleFunc("PUT /api/v1/admin/store/products/{id}", a.adminStoreProductUpdate)
		mux.HandleFunc("DELETE /api/v1/admin/store/products/{id}", a.adminStoreProductDelete)
		// 管理：订单（列表/详情/发货/退款/导出）
		mux.HandleFunc("GET /api/v1/admin/store/orders", a.adminStoreOrders)
		mux.HandleFunc("GET /api/v1/admin/store/orders/export", a.adminStoreOrdersExport)
		mux.HandleFunc("GET /api/v1/admin/store/orders/{no}", a.adminStoreOrderDetail)
		mux.HandleFunc("POST /api/v1/admin/store/orders/{no}/fulfill", a.adminStoreOrderFulfill)
		mux.HandleFunc("POST /api/v1/admin/store/orders/{no}/refund", a.adminStoreOrderRefund)
		mux.HandleFunc("GET /api/v1/admin/store/orders/{no}/refunds", a.adminStoreOrderRefunds)
		// 管理：退货审核（列表 + 批准/驳回）
		mux.HandleFunc("GET /api/v1/admin/store/returns", a.adminStoreReturns)
		mux.HandleFunc("POST /api/v1/admin/store/returns/{id}/decide", a.adminStoreReturnDecide)
		// 管理：优惠券 CRUD + 网关配置洞察
		mux.HandleFunc("GET /api/v1/admin/store/coupons", a.adminStoreCoupons)
		mux.HandleFunc("POST /api/v1/admin/store/coupons", a.adminStoreCouponCreate)
		mux.HandleFunc("PUT /api/v1/admin/store/coupons/{id}", a.adminStoreCouponUpdate)
		mux.HandleFunc("DELETE /api/v1/admin/store/coupons/{id}", a.adminStoreCouponDelete)
		mux.HandleFunc("GET /api/v1/admin/store/config", a.adminStoreConfig)
	}
	// API 兜底：未注册的 /api/v1/* 一律返回 404 JSON，不落 SPA HTML 兜底。
	// 语义：404 = 接口不存在或**能力未启用**（CoreModules 字段为 nil 时对应路由不注册）；
	// 若落到静态兜底会返回 200 + index.html，客户端无法区分"能力未启用"。
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "API_NOT_FOUND", "接口不存在或该能力未启用（"+r.Method+" "+r.URL.Path+"）")
	})
	// 静态前端（go:embed 注入；SPA 回退 index.html）
	mux.HandleFunc("GET /favicon.svg", a.serveFavicon)
	// 客服挂件脚本（SPEC-CS-001 M0）：同 favicon 的坑——单段带点路径会被博客通配
	// 路由 GET /{slug} 抢走，故显式注册，直接交 StaticHandler 按 .js 正确出 Content-Type。
	mux.HandleFunc("GET /cs-widget.js", a.serveCsWidget)
	mux.HandleFunc("GET /robots.txt", a.blogRobots)
	mux.Handle("/", StaticHandler())
	// 中间件链：CORS → 鉴权 → 站点解析 → 日志（CORS 最外，OPTIONS 预检不经过鉴权）
	return corsMiddleware(a.authMiddleware(a.siteMiddleware(logMiddleware(mux))), a.cfg.GetString("server.cors_origins"))
}

var version = "dev"

// SetVersion 由 main 在启动时注入（ldflags -X main.version=… 后同步到 handler）。
func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) version(w http.ResponseWriter, _ *http.Request) {
	// version=主系统语义化版本；schema=DB schema 版本（每次结构性迁移递增；插件/第三方判断数据结构演进）
	writeJSON(w, http.StatusOK, map[string]any{"version": version, "schema": a.dbSchemaVersion()})
}

// dbSchemaVersion 返回当前 DB schema 版本（1=基础+files.slug 稳定链接；后续结构迁移递增）。
// 读取 files 表是否含 slug 列做实证，避免常量与库脱节。
func (a *API) dbSchemaVersion() int {
	var n int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('files') WHERE name='slug'`).Scan(&n)
	if err != nil {
		return 0
	}
	if n > 0 {
		return 2 // schema v2：files.slug + 唯一索引（公开稳定链接）
	}
	return 1
}

func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	// H3/C1 修复：全量配置快照（含 smtp.*、pay.notify_secret、security.service_token、
	// blog.authors 等敏感项）仅管理员可读。纵深防御：路由级 /api/v1/admin/ 前缀已统一拦截，
	// 此处再加 handler 层守卫，避免将来该 handler 被挪到别的路径时静默失守。
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	writeJSON(w, http.StatusOK, a.cfg.Snapshot())
}

func (a *API) auditList(w http.ResponseWriter, r *http.Request) {
	// M9 修复：全量审计日志（含各用户操作轨迹）仅管理员可读
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, user_id, action, target, detail, prev_hash, hash, created_at
		 FROM audit_log ORDER BY id DESC LIMIT 200`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "AUDIT_READ_FAILED", err.Error())
		return
	}
	defer rows.Close()
	type row struct {
		ID        int64          `json:"id"`
		UserID    string         `json:"user_id,omitempty"`
		Action    string         `json:"action"`
		Target    string         `json:"target,omitempty"`
		Detail    map[string]any `json:"detail,omitempty"`
		PrevHash  string         `json:"prev_hash"`
		Hash      string         `json:"hash"`
		CreatedAt int64          `json:"created_at"`
	}
	out := []row{}
	for rows.Next() {
		var r row
		var detail string
		if err := rows.Scan(&r.ID, &r.UserID, &r.Action, &r.Target, &detail, &r.PrevHash, &r.Hash, &r.CreatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "AUDIT_READ_FAILED", err.Error())
			return
		}
		_ = json.Unmarshal([]byte(detail), &r.Detail)
		out = append(out, r)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (a *API) auditVerify(w http.ResponseWriter, r *http.Request) {
	// M9 修复：审计链校验结果（含链长度/断点信息）仅管理员可读
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return
	}
	if err := a.aud.Verify(r.Context()); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// 数据接口一律禁止缓存：避免浏览器启发式缓存导致旧列表/旧标签（用户改数据后看不到新内容）
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]any{
		"error": map[string]any{"code": errCode, "message": msg},
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		// 访问日志（运维）：req_id 方法 路径 → 状态码 耗时；不记录请求体/header（无敏感泄漏）
		log.Printf("[%s] %s %s %s → %d %s", time.Now().Format("15:04:05"),
			reqID(r), r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// reqID 请求追踪 id：优先 X-Request-Id，无则生成短随机（关联采集聚合等跨调用）
func reqID(r *http.Request) string {
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return id
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// statusRecorder 捕获响应状态码供访问日志使用。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
