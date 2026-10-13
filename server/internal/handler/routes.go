// Package handler 提供 HTTP/WS 入口：参数校验、鉴权、路由（实施文档 §8）。
// 阶段 0 最小面：健康检查、版本、配置快照、审计查询；业务端点随阶段逐步开启。
package handler

import (
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
	tags       *service.TagStore
	kb         *service.KBStore
	agent      *ai.Agent
	summarizer *ai.Summarizer
	impex      *service.ImpexStore
	notify     *service.NotifyStore
	hub        *webhookHub // 事件投递器（webhook 出站）
}

// New 创建 API。
func New(db *sql.DB, cfg *config.Store, b *bus.Bus, aud *service.AuditStore, gate *ai.Gateway, files *service.FileStore, tags *service.TagStore, kb *service.KBStore, agent *ai.Agent, summarizer *ai.Summarizer, impex *service.ImpexStore, notify *service.NotifyStore) *API {
	a := &API{db: db, cfg: cfg, b: b, aud: aud, ai: gate, files: files, tags: tags, kb: kb, agent: agent, summarizer: summarizer, impex: impex, notify: notify}
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
	mux.HandleFunc("GET /api/v1/admin/settings", a.settings)
	mux.HandleFunc("PUT /api/v1/admin/settings", a.updateSettings)
	mux.HandleFunc("GET /api/v1/admin/ai/providers", a.aiProvidersList)
	mux.HandleFunc("GET /api/v1/admin/ai/usage", a.adminAIUsage)
	mux.HandleFunc("POST /api/v1/admin/ai/providers/switch", a.aiProvidersSwitch)
	mux.HandleFunc("POST /api/v1/admin/ai/providers/custom", a.aiCustomProviderSave)
	mux.HandleFunc("DELETE /api/v1/admin/ai/providers/custom", a.aiCustomProviderClear)
	mux.HandleFunc("POST /api/v1/admin/tags/apply-rules", a.ruleTagsApply)
	mux.HandleFunc("GET /api/v1/admin/audit", a.auditList)
	mux.HandleFunc("GET /api/v1/admin/audit/verify", a.auditVerify)
	// 文件 API（阶段 1）
	mux.HandleFunc("GET /api/v1/files", a.filesList)
	mux.HandleFunc("GET /api/v1/files/resolve", a.filesResolveWiki)
	mux.HandleFunc("GET /api/v1/search", a.search)
	mux.HandleFunc("GET /api/v1/files/tree", a.filesTree)
	mux.HandleFunc("GET /api/v1/files/{id}/summary", a.filesSummary)
	mux.HandleFunc("GET /api/v1/files/{id}/links", a.fileLinks)
	mux.HandleFunc("GET /api/v1/files/{id}/locate", a.locateChunk)
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
	mux.HandleFunc("GET /api/v1/kb/concepts/{id}", a.kbConcept)
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
	mux.HandleFunc("POST /api/v1/public/blog/ask", a.publicBlogAsk)
	mux.HandleFunc("GET /api/v1/public/blog/ask/quota", a.publicBlogAskQuota)
	mux.HandleFunc("GET /api/v1/public/media/{id}", a.publicMediaGet)
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
	mux.HandleFunc("POST /api/v1/public/unlock", a.publicUnlock)
	// 博客功能插件协议 + RSS
	mux.HandleFunc("GET /api/v1/blog/plugins", a.blogPluginList)
	mux.HandleFunc("POST /api/v1/blog/plugins", a.blogPluginUpsert)
	mux.HandleFunc("POST /api/v1/admin/apps/install-zip", a.appInstallZip)
	// 应用中心（在线目录）：浏览远程市场 + 一键安装
	mux.HandleFunc("GET /api/v1/admin/apps/market", a.marketList)
	mux.HandleFunc("POST /api/v1/admin/apps/market/install", a.marketInstall)
	// 站长博客侧市场（上游 blog_market.go 三端点移植，2026-09-18 授权；与 admin 侧同构同管线）
	mux.HandleFunc("GET /api/v1/blog/market", a.marketList)
	mux.HandleFunc("POST /api/v1/blog/market/install", a.marketInstall)
	// 第三方开发者体系（devmarket）：开发者自管 + 平台审核闸门
	mux.HandleFunc("POST /api/v1/developer/apply", a.devApply)
	mux.HandleFunc("GET /api/v1/developer/me", a.devMe)
	mux.HandleFunc("GET /api/v1/developer/listings", a.devListings)
	mux.HandleFunc("POST /api/v1/developer/listings", a.devListingCreate)
	mux.HandleFunc("GET /api/v1/developer/listings/{id}", a.devListingGet)
	mux.HandleFunc("PUT /api/v1/developer/listings/{id}", a.devListingUpdate)
	mux.HandleFunc("DELETE /api/v1/developer/listings/{id}", a.devListingDelete)
	mux.HandleFunc("POST /api/v1/developer/listings/{id}/submit", a.devListingSubmit)
	mux.HandleFunc("POST /api/v1/developer/listings/{id}/upload", a.devListingUpload)
	mux.HandleFunc("GET /api/v1/admin/market/developers", a.adminDevelopers)
	mux.HandleFunc("POST /api/v1/admin/market/developers/{id}/approve", a.adminDeveloperApprove)
	mux.HandleFunc("POST /api/v1/admin/market/developers/{id}/reject", a.adminDeveloperReject)
	mux.HandleFunc("GET /api/v1/admin/market/listings", a.adminMarketListings)
	mux.HandleFunc("POST /api/v1/admin/market/listings/{id}/approve", a.adminMarketApprove)
	mux.HandleFunc("POST /api/v1/admin/market/listings/{id}/reject", a.adminMarketReject)
	mux.HandleFunc("GET /market/uploads/{path...}", a.marketUploadFile)
	// 公开 /market 门户（无需登录浏览；index.json 为本实例合并视图）
	mux.HandleFunc("GET /market", a.marketPortal)
	mux.HandleFunc("GET /market/", a.marketPortal)
	mux.HandleFunc("GET /market/index.json", a.marketIndexFile)
	mux.HandleFunc("GET /market/official/{file}", a.officialMarketFile)
	// 实例许可证（0.5.x 市场多壳模型配套；付费安装门禁用）
	mux.HandleFunc("GET /api/v1/license", a.licenseStatus)
	mux.HandleFunc("POST /api/v1/license", a.licenseActivate)
	mux.HandleFunc("DELETE /api/v1/license", a.licenseDeactivate)
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
	// 公开博客服务端 HTML（SEO：无哈希路径，爬虫可读）
	// 中文根路径 /{slug} 为主链；/blog/{slug} 兼容保留
	mux.HandleFunc("GET /blog", a.blogHTMLIndex)
	mux.HandleFunc("GET /blog/{slug}", a.blogHTMLPost)
	mux.HandleFunc("GET /{slug}", a.blogHTMLPost)
	// 静态前端（go:embed 注入；SPA 回退 index.html）
	mux.HandleFunc("GET /robots.txt", a.blogRobots)
	mux.Handle("/", StaticHandler())
	// 中间件链：CORS → 鉴权 → 日志（CORS 最外，OPTIONS 预检不经过鉴权）
	return corsMiddleware(a.authMiddleware(logMiddleware(mux)), a.cfg.GetString("server.cors_origins"))
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

func (a *API) settings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.cfg.Snapshot())
}

func (a *API) auditList(w http.ResponseWriter, r *http.Request) {
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
