// AiKmap 单二进制入口：serve / migrate / seed。
// 唯一制品原则（实施文档 §2.5）：源码即运行，CI 构建产物哈希校验。
package main

import (
	"context"
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

	// AI 网关 + 多 provider 注册（secrets/providers.json：agnes/bailian/mimo，能力级热切换）
	gate := ai.NewGateway(cfg, b)
	if defs, active := loadProvidersJSON(); defs != nil {
		gate.RegisterProviders(defs)
		gate.SetActiveDefaults(active)
		// 未显式配置时按 active 注入各能力 provider（DB 显式设置过则尊重用户选择）
		for _, cap := range []string{"llm", "embedding", "asr", "tts", "image", "rerank"} {
			if !cfg.IsExplicit("ai." + cap + ".provider") {
				if name := active[cap]; name != "" {
					if _, ok := defs[name]; ok {
						_, _ = cfg.Set(ctx, "ai."+cap+".provider", name, "string", "能力 provider（secrets/providers.json active）", "system")
					}
				}
			}
		}
		// embedding 维度从 provider models 推断默认 512（qwen3.7-text-embedding 支持）
	} else if p, ok := loadTokenPool(); ok {
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
		h := handler.New(db, cfg, b, aud, gate, files, tags, kb, agent, summarizer, impex, notify)
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
