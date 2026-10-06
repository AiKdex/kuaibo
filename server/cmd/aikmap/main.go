// AiKmap 单二进制入口：serve / migrate / seed。
// 唯一制品原则（实施文档 §2.5）：源码即运行，CI 构建产物哈希校验。
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/config"
	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/handler"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/service"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

var version = "dev" // CI 注入构建哈希

func main() {
	handler.SetVersion(version)
	if err := run(); err != nil {
		log.Fatalf("aiklog: %v", err)
	}
}

func run() error {
	fs := flag.NewFlagSet("aikmap", flag.ExitOnError)
	dbPath := fs.String("db", "./data/aikmap.db", "SQLite 数据库路径")
	addr := fs.String("addr", ":8080", "HTTP 监听地址")
	// 子命令：`aikmap serve -db x`（位置参数优先）；缺省即 serve
	sub := "serve"
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "serve" || args[0] == "migrate" || args[0] == "seed") {
		sub = args[0]
		args = args[1:]
	}
	_ = fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := repo.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	b := bus.New()
	cfg := config.New(db, b)
	if err := cfg.LoadFromDB(ctx); err != nil {
		return err
	}

	aud := service.New(db)
	// 存储根解析：相对路径按「可执行文件所在目录的上级」解析（bin\..\data\files），
	// 不依赖进程工作目录——防止不同启动方式（cwd 变化）把数据写散到错误位置。
	root := cfg.GetString("storage.local.root")
	if root != "" && !filepath.IsAbs(root) {
		if exe, err := os.Executable(); err == nil {
			root = filepath.Join(filepath.Dir(exe), "..", root)
		}
	}
	st, err := storage.NewLocal(root)
	if err != nil {
		return err
	}
	files := service.NewFileStore(db, st, b, aud)
	tags := service.NewTagStore(db, b, aud)
	kb := service.NewKBStore(db)
	impex := service.NewImpexStore(db, files, tags, b, aud)
	// 爱库录精简发行：裁剪采集源/执行器/IM（详见 AIKLOG-精简发行方案）
	notify := service.NewNotifyStore(db)

	// AI 网关 + 多 provider 注册（secrets/providers.json 内置种子 → DB 持久化 → 网关注册）
	gate := ai.NewGateway(cfg, b)
	// A1 用量记账（WeKnora 借鉴，B27 移植）：LLM/Embedding 调用落 llm_usage，token 计费前置账本
	gate.SetUsageRecorder(ai.NewUsageDB(db))
	// 用量管控（配额 + 增值 token 计费）：newReq 前查、recordUsage 后扣；nil=不管控
	gate.SetMeter(ai.NewMeter(db))
	// 加载 provider：内置种子（仅缺失时写 DB）→ DB 全量注册到网关 → 注入能力级默认。
	// DB 为运行期权威源，支持后台热增删改任意大模型，无需改代码/重启。
	loadProviders(ctx, cfg, gate, db)
	// DB 与 JSON 均无 provider 时，回退旧单文件 token 池兼容路径
	if len(gate.Providers()) == 0 {
		if p, ok := loadTokenPool(); ok {
			gate.SetPool(p)
			// 兼容旧单文件 secrets（ai-provider.json）
			if cfg.GetString("ai.llm.endpoint") == "" {
				if ep := providerEndpoint(); ep != "" {
					_, _ = cfg.Set(ctx, "ai.llm.endpoint", ep, "string", "模型网关端点（来自 secrets/ai-provider.json）", "system")
				}
			}
			if cfg.GetString("ai.llm.model") == "" {
				if m := providerModel(); m != "" {
					_, _ = cfg.Set(ctx, "ai.llm.model", m, "string", "模型名（来自 secrets/ai-provider.json）", "system")
				}
			}
		}
	}

	// 工具注册表 + Agent（阶段 2：search_files 检索 + read_file 精读，工具可插拔扩展）
	reg := ai.NewRegistry()
	// home 空间兜底：单用户阶段固定系统空间（settings 未显式配置时）
	homeSpaceID := cfg.GetString("system.home_space_id")
	if homeSpaceID == "" {
		homeSpaceID = service.SystemHomeSpaceID
	}
	reg.Register(ai.NewSearchFilesTool(files, homeSpaceID, gate, db, gate.EmbeddingModel()))
	// 入库 AI 解读（后台串行分批，摘要进库；read_file 优先复用）
	summarizer := ai.NewSummarizer(db, files, tags, gate, cfg.GetString("ai.summarize.exts"), cfg.GetBool("ai.entity_extract.enabled"))
	reg.Register(ai.NewReadFileTool(files, summarizer))
	// 批量工具（协作方 Partner-A 交付：查重/批量打标/批量整理/批量重命名）
	reg.Register(ai.NewDedupFilesTool(db, homeSpaceID))
	reg.Register(ai.NewBatchTagFilesTool(files, tags))
	reg.Register(ai.NewBatchOrganizeTool(db, files))
	reg.Register(ai.NewBatchRenameTool(files))
	// 官方工具矩阵：web_clip 网页剪藏（对齐上游 AgentTools 规范；复用 impex.CreateURLImport）
	reg.Register(ai.NewWebClipTool(impex, files, service.SystemOwnerID, homeSpaceID))
	// A2 FAQ 检索工具（WeKnora 借鉴，B27 移植）：问题即检索单元，答案直接注入回答上下文
	reg.Register(ai.NewFAQSearchTool(kb, homeSpaceID))
	// A3 空间长期记忆（WeKnora 借鉴 + 云雇工叙事，B27 移植）：Agent 开工读上下文、任务中沉淀事实
	reg.Register(ai.NewMemoryContextTool(kb, homeSpaceID))
	reg.Register(ai.NewMemorySetTool(kb, homeSpaceID, service.SystemOwnerID))
	// A4 预生成问题索引（WeKnora 借鉴，B27 移植）：文档入库自动生成 Q&A 变体 → kb_faq(source=import:)
	// 开关缺省开：显式设置 ai.question_gen.enabled=false 才关
	qgenEnabled := !cfg.IsExplicit("ai.question_gen.enabled") || cfg.GetBool("ai.question_gen.enabled")
	qgen := ai.NewQuestionGen(db, files, kb, gate, cfg.GetString("ai.summarize.exts"), qgenEnabled)
	qgen.Subscribe(b)
	agent := ai.NewAgent(gate, reg, cfg.GetInt("ai.agent.max_rounds"))
	// 规则打标（L0：扩展名/文件名关键词 → 标签；上传即时执行，零 AI 成本）
	tags.SubscribeRuleTags(b)

	// 种子：确保系统 owner 与 home 空间存在
	if err := service.EnsureSystem(ctx, db, aud); err != nil {
		return err
	}
	if err := service.EnsureBlogSpace(ctx, db); err != nil {
		return err
	}
	// 多站点：确保默认站 + 默认站配置存在（幂等；存量单站数据归默认站）
	if err := service.NewSiteStore(db).EnsureDefaultSite(ctx); err != nil {
		return err
	}
	// 多租户 SaaS 计费（SPEC-BILLING）：幂等写入内置套餐目录（免费/专业/团队）。
	// 免费档是订阅到期降档的兜底目标，必须保证存在且启用。
	if err := service.NewBilling(db).SeedDefaultPlans(ctx); err != nil {
		return err
	}
	// 多站点数据访问（M1 路由解析 + 配置/主题按站点）
	siteStore := service.NewSiteStore(db)

	switch sub {
	case "migrate":
		fmt.Println("migrate: ok")
		return nil
	case "seed":
		fmt.Println("seed: ok")
		return nil
	case "serve":
		// 后台全量补量（幂等）全部异步：HTTP 服务立即可用，大库启动不被阻塞。
		// 存量文本补索引 / AI 入库解读队列 / 向量模型重建成都放 goroutine 串行执行。
		summarizer.Subscribe(b)
		go func() {
			time.Sleep(2 * time.Second) // 让服务先监听，避免启动窗口内请求排队
			if n, err := files.ReindexSpace(ctx, homeSpaceID); err == nil && n > 0 {
				log.Printf("reindex: %d files indexed", n)
			}
		}()
		go func() {
			time.Sleep(2 * time.Second)
			if n, err := summarizer.RequeueSpace(ctx, homeSpaceID); err == nil && n > 0 {
				log.Printf("ai-summarize: %d files queued", n)
			}
		}()
		go summarizer.Run(ctx)
		// 向量化后台 worker：文本块 → embedding → vectors 表（启动即补量 + 事件增量）
		vectorizer := ai.NewVectorizer(db, gate, b, cfg.GetInt("ai.embedding.dim"))
		go func() {
			time.Sleep(2 * time.Second)
			if n, err := vectorizer.CleanupStaleModels(ctx); err == nil && n > 0 {
				log.Printf("vectorizer: cleared %d stale vectors (model changed)", n)
			}
		}()
		go vectorizer.Run(ctx)
		mods := buildCoreModules(db, cfg, b, aud, files, tags, notify)
		h := handler.New(db, cfg, b, aud, gate, files, tags, kb, agent, summarizer, impex, notify, mods, siteStore)
		// [family] AI 人生参谋工具（家族传承二期①；mods.Family nil=模块未装配则不注册）
		if mods.Family != nil {
			reg.Register(ai.NewFamilyAdvisorTool(mods.Family))
		}
		// B2：把内核工具注册表交给 handler，供 workflow 的 Type=tool 步骤执行通用工具
		// （采集 web_clip / 检索 search_files / 批量打标签等）。注入后"采集→整理→出草稿"
		// 即可由一条 workflow 编排、经 IM 指令或 API 触发。
		h.SetRegistry(reg)
		// A09+A5：概念页 AI 摘要器（概念概述缓存 + 来源文档引用链）
		h.SetConceptSummarizer(ai.NewConceptSummarizer(db, gate))
		// 2.3 定时发布调度：每分钟将到期文章自动公开（publish_at<=now → NULL；不改变更新时间序）
		go func() {
			tk := time.NewTicker(time.Minute)
			defer tk.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tk.C:
					if _, err := db.ExecContext(ctx,
						`UPDATE files SET publish_at=NULL WHERE publish_at IS NOT NULL AND publish_at<=?`,
						time.Now().UnixMilli()); err == nil {
						// 静默：定时发布到期自动公开
					}
				}
			}
		}()
		// [family] 家族纪念日提醒（每小时扫描；family.enabled 关或表空时零开销；anniv_key 幂等去重）。
		// mods.Family 为 nil（capability.family 未装配）时不启动。
		if mods.Family != nil {
			familyStore := mods.Family
			go func() {
				ftk := time.NewTicker(time.Hour)
				defer ftk.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case now := <-ftk.C:
						if familyStore.Enabled() {
							if err := familyStore.DailyReminders(ctx, now); err != nil {
								log.Printf("family reminders: %v", err)
							}
						}
					}
				}
			}()
		}
		// B18：孤儿派生对象定时兜底清理（源文件被直删 / 回滚库后留下的无主缓存）。
		// 默认 24h 一次（media.derived_sweep_interval_min）；0 = 关闭，行为与 B16 一致。
		go func() {
			interval := cfg.GetInt("media.derived_sweep_interval_min")
			if interval <= 0 {
				return
			}
			d := time.Duration(interval) * time.Minute
			time.Sleep(2 * time.Second) // 让服务先监听，避免启动窗口内请求排队
			sweep := func() {
				n, freed, err := files.SweepOrphans(ctx)
				if err != nil {
					log.Printf("derived-sweep: error: %v", err)
					return
				}
				if n > 0 {
					log.Printf("derived-sweep: removed %d orphan objects, freed %d bytes", n, freed)
				}
			}
			sweep() // 启动时先兜底清一次
			tk := time.NewTicker(d)
			defer tk.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tk.C:
					sweep()
				}
			}
		}()
		// 一致性备份（ops.backup_enabled 后台可动态开关；周期 ops.backup_schedule=HH:MM 每分钟检查，改设置≤1min 生效）
		// 保留份数 ops.backup_keep_local/keep_remote 动态读取；VACUUM INTO 快照 → storage 后端推远端。
		// 调度器**常驻**（不套在启动时开关判断内）：运行期在后台开启开关后 ≤1 分钟自动开始，无需重启进程；
		// 关闭开关即停做（不退出调度器，便于再次开启时无需重启）。上游把 goroutine 套在 enabled 判断里，
		// 导致「后台首次开启后不重启就不备份」——本仓改为常驻 + 「启用即备份一次」。
		go func() {
			dataDir := filepath.Dir(*dbPath)
			runBackup := func() {
				kl, kr := cfg.GetInt("ops.backup_keep_local"), cfg.GetInt("ops.backup_keep_remote")
				if err := backupOnce(context.Background(), db, st, dataDir, kl, kr); err != nil {
					log.Printf("backup failed: %v", err)
					return
				}
				_, _ = cfg.Set(context.Background(), "ops.backup_last_at", time.Now().Format(time.RFC3339), "string", "最近一次成功备份时间", "system")
			}
			wasEnabled := false
			for {
				enabled := cfg.GetBool("ops.backup_enabled")
				if enabled && !wasEnabled {
					runBackup() // 启用即备份一次（含进程启动时已启用 → 「启动即备份」）
				} else if enabled && backupDue(cfg.GetString("ops.backup_schedule")) {
					runBackup()
				}
				wasEnabled = enabled
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Minute):
				}
			}
		}()
		// bus 订阅：导入导出终态 → 通知（done/failed 都提醒，用户在导入导出中心处理）
		_ = b.Subscribe("impex.done", func(_ context.Context, e bus.Event) error {
			st, _ := e.Data["status"].(string)
			return notify.Add(context.Background(), "impex", map[string]any{
				"title": "导入导出完成", "message": "任务已处理完成（状态：" + st + "）", "link": "/#/impex",
			})
		})
		_ = b.Subscribe("impex.failed", func(_ context.Context, e bus.Event) error {
			st, _ := e.Data["status"].(string)
			return notify.Add(context.Background(), "impex", map[string]any{
				"title": "导入导出失败", "message": "任务未完成（状态：" + st + "），请到导入导出中心重试", "link": "/#/impex",
			})
		})
		srv := &http.Server{
			Addr:              *addr,
			Handler:           h.Routes(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		log.Printf("AiKlog 爱库录 %s listening on %s (db=%s)", version, *addr, *dbPath)
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown cmd %q (serve|migrate|seed)", sub)
	}
}

// loadProvidersJSON 读取多 provider 注册表（secrets/providers.json）。
// 返回 (defs, active)；文件缺失时返回 (nil, nil) 走旧单文件兼容路径。
func loadProvidersJSON() (map[string]*ai.ProviderDef, map[string]string) {
	type providersFile struct {
		Active    map[string]string           `json:"active"`
		Providers map[string]*ai.ProviderDef `json:"providers"`
	}
	for _, p := range []string{"server/secrets/providers.json", "secrets/providers.json"} {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var f providersFile
		if json.Unmarshal(raw, &f) != nil || len(f.Providers) == 0 {
			return nil, nil
		}
		return f.Providers, f.Active
	}
	return nil, nil
}

// loadProviders 加载模型提供方（「可任意添加/配置大模型」底座）：
//  1. providers.json 内置种子：仅当 DB 缺失该 name 时写入（builtin=1），保留用户在 UI 的编辑；
//  2. 从 DB 全量加载（内置 + 用户自定义）注册到网关注册表；DB 读取失败回退 JSON 内置；
//  3. 注入能力级默认 provider（尊重 DB 已显式设置的用户选择）。
//
// DB 为运行期权威源，后台增删改即时热更新网关注册表，无需改代码或重启。
func loadProviders(ctx context.Context, cfg *config.Store, gate *ai.Gateway, db *sql.DB) {
	builtins, active := loadProvidersJSON()
	if builtins == nil {
		builtins = map[string]*ai.ProviderDef{}
	}
	// 1) 内置种子（缺失才插入）
	for name, d := range builtins {
		d.Builtin = true
		exists, err := ai.ProviderDefExists(db, name)
		if err != nil {
			log.Printf("check provider %s failed: %v", name, err)
			continue
		}
		if !exists {
			if err := ai.SaveProviderDef(db, name, d); err != nil {
				log.Printf("seed provider %s failed: %v", name, err)
			}
		}
	}
	// 2) DB 全量加载并注册到网关
	all, err := ai.LoadProviderDefs(db)
	if err != nil {
		log.Printf("load providers from db failed: %v; fallback to json builtins", err)
		all = builtins
	}
	for n, d := range all {
		gate.UpsertProvider(n, d)
	}
	// 3) 注入能力级默认 provider（未显式配置时按 active 注入；DB 已设置则尊重）
	gate.SetActiveDefaults(active)
	for _, cap := range []string{"llm", "embedding", "asr", "tts", "image", "rerank"} {
		if !cfg.IsExplicit("ai." + cap + ".provider") {
			if name := active[cap]; name != "" {
				if _, ok := gate.Providers()[name]; ok {
					_, _ = cfg.Set(ctx, "ai."+cap+".provider", name, "string", "能力 provider（secrets/providers.json active）", "system")
				}
			}
		}
	}
}

// ---- token 池加载（secrets/ai-provider.json 旧单文件兼容）----

// aiProviderJSON secrets 文件结构（git 忽略，不入库）。
type aiProviderJSON struct {
	Endpoint string   `json:"endpoint"`
	Model    string   `json:"model"`
	Keys     []string `json:"keys"`
}

// providerFile 定位 secrets 文件：优先 server/secrets，兼容工作目录 secrets。
func providerFile() string {
	for _, p := range []string{"server/secrets/ai-provider.json", "secrets/ai-provider.json"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// loadTokenPool 读取 secrets 并构造 token 池；缺失/空时返回 (nil,false)。
func loadTokenPool() (*ai.TokenPool, bool) {
	prov, ok := loadProviderJSON()
	if !ok || len(prov.Keys) == 0 {
		return nil, false
	}
	pool := ai.NewTokenPool(prov.Keys, 30*time.Second)
	if pool.Len() == 0 {
		return nil, false
	}
	return pool, true
}

// providerEndpoint 返回 secrets 中的模型网关端点（供启动注入配置）。
func providerEndpoint() string {
	prov, ok := loadProviderJSON()
	if !ok {
		return ""
	}
	return prov.Endpoint
}

// providerModel 返回 secrets 中的默认模型名。
func providerModel() string {
	prov, ok := loadProviderJSON()
	if !ok {
		return ""
	}
	return prov.Model
}

// loadProviderJSON 读取 secrets 文件（endpoint/model/keys）。
func loadProviderJSON() (aiProviderJSON, bool) {
	p := providerFile()
	if p == "" {
		return aiProviderJSON{}, false
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return aiProviderJSON{}, false
	}
	var prov aiProviderJSON
	if err := json.Unmarshal(raw, &prov); err != nil {
		return aiProviderJSON{}, false
	}
	return prov, true
}
