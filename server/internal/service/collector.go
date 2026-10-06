// Package service 的 collector.go 实现采集执行器（频道化 P10：城市级官办源聚合）。
// 设计要点：
//   - "探路一次·模板复用"：每个源 = 一份站点模板（链接/标题/日期/单位提取规则 + 状态规则），
//     采集器按模板抓取列表页 → 解析条目 → 组装 markdown（front matter 元数据 + 溯源）
//     → 以文件形式入库知识库（图谱/双链/智能集合/语义检索/博客化发布全部直接生效）；
//   - 去重：按"目标目录内同名文件"跳过（文件名含日期+标题，重跑幂等）；
//   - 打标：城市 + 类型标签自动关联（情报/城市/{city}、情报/就业）；
//   - 运行记录落库（collect_runs），每次采集可审计；
//   - 同一执行器被 Tool（ai/tools_collect.go collect_jobs）与 API（handler/source.go）共用。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	fhttp "github.com/bogdanfinn/fhttp"
)

// CollectTemplate 站点模板（来源：探路流程生成，见 docs/采集对接API协议.md）。
type CollectTemplate struct {
	List struct {
		LinkRegex    string   `json:"link_regex"`    // 链接匹配正则（空=全部 http(s)/相对链接）
		TitleRegex   string   `json:"title_regex"`   // 标题匹配正则（空=全部非空标题）
		ExcludeRegex string   `json:"exclude_regex"` // 排除正则（导航/通用词）
		ExcludeWords []string `json:"exclude_words"` // 排除关键词（标题命中任一即跳过；比正则更易配置）
		DateRegex    string   `json:"date_regex"`    // 从标题/链接提取日期（空=用当日）
		OrgRegex     string   `json:"org_regex"`     // 从标题提取单位（空=不提取）
		LinkPrefix   string   `json:"link_prefix"`   // 相对链接补全前缀
		URLs         []string `json:"urls"`          // 多列表页 URL（空=用 src.URL 单页）；JS 分页/归档栏目可把可达子页都列进来
	} `json:"list"`
	StatusRules []struct {
		Match  string `json:"match"`  // 标题关键词
		Status string `json:"status"` // ongoing|upcoming|expired
	} `json:"status_rules"`
	// DirectItems 种子条目（低频高价值页面翻不了页时用）：直接作为采集条目，
	// 走同一管线（详情抓取→把关→入库），title/link/date 显式给出。
	DirectItems []CollectItem `json:"direct_items"`
	// CityResolver 条目级城市归属（一源多城）：省级平台"热点招聘"栏目天然跨城市，
	// 配置后每条目按标题/正文解析真实城市 → 入库目录 `采集/就业情报/{城市}` 分流、
	// front matter 写入 city_resolved/confidence/basis、标签按真实城市打。
	CityResolver *CityResolver `json:"city_resolver"`
}

// CityResolver 城市归属解析规则（一源多城，详见 docs/多城市-一源多城方案.md）。
// 白名单约束（防逃逸）：解析结果必须命中 CityWhitelist（地级市标准名）或 CountyMap（县/区/县级市）
// 才采信；"XX市"归一化后比对白名单；未命中一律 fallback（全省/未识别），绝不硬猜——
// 否则"上海证券交易所上市""马鞍山市"这类捕获会直接生成垃圾城市目录（第三方复验发现）。
type CityResolver struct {
	FromTitleRegex string            `json:"from_title_regex"` // 标题城市名（高置信）；需捕获组，如 "^(马鞍山|滁州|合肥|...)"
	FromBodyRegex  string            `json:"from_body_regex"`  // 正文地址（中置信）；需捕获组，如 "位于\\s*([^\\s，。]{2,10}?)(?:市|县)"
	CountyMap      map[string]string `json:"county_map"`       // 县/区/县级市→地级市 映射（正文兜底）：{"怀宁县":"安庆","天长市":"滁州"}
	CityWhitelist  []string          `json:"city_whitelist"`   // 已知地级市标准名（安徽16市）：正文"市名"必须命中才采信
	Fallback       string            `json:"fallback"`         // 识别不出时：province（全省）| keep_source_city（沿用源城市）
}

// normalizeCityName "XX市/XX地区" → "XX"（县级市名如"天长市"不在此列——它们走 county_map 原始名）。
func normalizeCityName(s string) string {
	for _, suf := range []string{"市", "地区"} {
		if strings.HasSuffix(s, suf) {
			if c := strings.TrimSuffix(s, suf); c != "" && c != s {
				return c
			}
		}
	}
	return s
}

// inCityWhitelist 地级市标准名白名单命中？
func inCityWhitelist(r *CityResolver, name string) bool {
	for _, c := range r.CityWhitelist {
		if c == name {
			return true
		}
	}
	return false
}

// cityFallback 识别不出时的兜底。
func (c *Collector) cityFallback(r *CityResolver) (string, string, string) {
	if r != nil && r.Fallback == "province" {
		return "全省", "低", "未识别"
	}
	return "", "", ""
}

// rePubStart / rePubStop 剥离发布机构片段（两步截断，兼容 Go RE2 无 lookahead）：
// 正文常以"发布机构：肥东县公共就业（人才）服务中心"开头，若不剥离会污染城市归属识别
// （机构所在地 ≠ 雇主所在地）。只删"发布机构：..."起、到"发布日期/阅读数"等结构词为止的
// 一小段——不贪心剥整行（正文常挤成一行，剥多了会吃掉后面的企业地址）。
var rePubStart = regexp.MustCompile(`(?:发布机构|发布单位|供稿单位|信息来源|来源单位)\s*[:：]`)
var rePubStop = regexp.MustCompile(`发布日期|阅读数|点赞|分享|收藏|发布时间|[\n。；]`)

func stripPublisher(s string) string {
	m := rePubStart.FindStringIndex(s)
	if m == nil {
		return s
	}
	rest := s[m[1]:]
	stop := rePubStop.FindStringIndex(rest)
	end := len(s)
	if stop != nil {
		end = m[1] + stop[0]
	}
	return s[:m[0]] + s[end:]
}

// CollectItem 解析出的单个列表条目。
type CollectItem struct {
	Title string `json:"title"`
	Link  string `json:"link"`
	Org   string `json:"org,omitempty"`
	Date  string `json:"date,omitempty"`
	Body  string `json:"body,omitempty"` // 详情页正文（采集把关抓取）
}

// CollectResult 单源采集结果。
type CollectResult struct {
	Source   string   `json:"source"`
	Fetched  int      `json:"fetched"`
	Created  int      `json:"created"`
	Skipped  int      `json:"skipped"`
	Rejected int      `json:"rejected"`          // 把关剔除（无效/空正文/命中排除词）
	Rejects  []string `json:"rejects,omitempty"` // 剔除明细（标题 + 原因），限量保留
	Error    string   `json:"error,omitempty"`
}

// CollectSummary 一次采集运行汇总。
type CollectSummary struct {
	City     string          `json:"city"`
	RunID    int64           `json:"run_id"`
	Status   string          `json:"status"`
	Fetched  int             `json:"fetched"`
	Created  int             `json:"created"`
	Skipped  int             `json:"skipped"`
	Rejected int             `json:"rejected"`
	Failed   int             `json:"failed"`
	Target   string          `json:"target"`
	Results  []CollectResult `json:"results"`
	Tip      string          `json:"tip"`
}

var (
	reDefaultLink  = regexp.MustCompile(`(?i)^(https?://|/)`)
	reExcludeWords = regexp.MustCompile(`(?i)^\s*(首页|登录|注册|设为首页|加入收藏|网站地图|联系我们|关于我们|更多|more)\s*$`)
	reSpace        = regexp.MustCompile(`\s+`)
	reBadName      = regexp.MustCompile(`[\\/:*?"<>|\r\n]+`)
)

// Collector 采集执行器（Tool 与 API 共用）。
type Collector struct {
	sources  *SourceStore
	files    *FileStore
	tags     *TagStore
	runMu    sync.Mutex                                // 采集运行互斥：同一时刻只允许一个 run（防并发双写同名冲突→-N 副本）
	OnFinish func(runID int64, status, summary string) // 终态回调（通知中心挂载点，可选）
}

// NewCollector 创建采集执行器。
func NewCollector(sources *SourceStore, files *FileStore, tags *TagStore) *Collector {
	return &Collector{sources: sources, files: files, tags: tags}
}

// TombstoneSource 记录删除记忆（供删除路径调用）：采集产物被删时记 source_url，
// 后续采集跳过重建。见 source.go TombstoneSource。
func (c *Collector) TombstoneSource(ctx context.Context, sourceURL, reason string) error {
	return c.sources.TombstoneSource(ctx, sourceURL, reason)
}

// StartRunAsync 异步启动一次采集（立即返回真实 run_id；后台 goroutine 逐源抓取入库，
// 状态经 ListRuns 查询；适合慢源/批量场景，不阻塞调用方）。
// 兜底：goroutine panic 也会落终态（FinishRun failed）；进程崩溃由启动时 OrphanRuns 收尾。
// 并发防护：采集互斥——已有 run 在跑时拒绝新 run（防重叠双写产生同名 -N 副本）。
func (c *Collector) StartRunAsync(ctx context.Context, city string, sourceIDs []string, limit int, force bool, trigger string) (int64, error) {
	if trigger == "" {
		trigger = "api"
	}
	if !c.runMu.TryLock() {
		return 0, fmt.Errorf("已有采集任务在运行，请稍后重试（单实例串行，防并发双写）")
	}
	runID, err := c.sources.StartRun(ctx, city, trigger)
	if err != nil {
		c.runMu.Unlock()
		return 0, fmt.Errorf("记录采集开始失败: %w", err)
	}
	bgCtx := context.WithoutCancel(ctx)
	go func() {
		defer c.runMu.Unlock()
		fired := false
		fire := func(status, summary string) {
			if fired {
				return
			}
			fired = true
			c.fireFinish(runID, status, summary)
		}
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("collect panic: %v", r)
				_ = c.sources.FinishRun(bgCtx, runID, "failed", 0, 0, 0, 0, 0, msg)
				fire("failed", msg)
			}
		}()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, err := c.runWithID(bgCtx, runID, city, sourceIDs, limit, force, trigger)
			if err != nil {
				_ = c.sources.FinishRun(bgCtx, runID, "failed", 0, 0, 0, 0, 0, err.Error())
			}
		}()
		// 收尾兜底：主体 10 分钟未完成（网络挂起/写锁拖死等）→ 强制落终态，
		// 避免 run 永久 running（缺陷 1 复现场景：fetched/created 恒 0 但文件已入库）。
		select {
		case <-done:
		case <-time.After(10 * time.Minute):
			msg := "采集超时未收尾（10min watchdog 强制落终态；文件可能已部分入库）"
			_ = c.sources.FinishRun(bgCtx, runID, "failed", 0, 0, 0, 0, 0, msg)
			fire("failed", msg)
		}
		// 主体正常完成后，统一读一次终态（runWithID 内部已 FinishRun 落终态）→ 通知。
		if st, ok := c.sources.RunStatus(bgCtx, runID); ok {
			fire(st.Status, st.Error)
		}
	}()
	return runID, nil
}

// fireFinish 触发终态回调（通知中心挂载：采集完成/失败 → 未读通知）。
func (c *Collector) fireFinish(runID int64, status, summary string) {
	if c.OnFinish != nil {
		c.OnFinish(runID, status, summary)
	}
}

// Run 执行一次采集（同步路径自建 run 记录）：按城市（或指定源）抓取解析入库。
// sourceIDs 为空 = 该城市全部启用源；limit<=0 默认 50；force=true 忽略去重。
func (c *Collector) Run(ctx context.Context, city string, sourceIDs []string, limit int, force bool, trigger string) (*CollectSummary, error) {
	if !c.runMu.TryLock() {
		return nil, fmt.Errorf("已有采集任务在运行，请稍后重试（单实例串行，防并发双写）")
	}
	defer c.runMu.Unlock()
	runID, err := c.sources.StartRun(ctx, city, trigger)
	if err != nil {
		return nil, fmt.Errorf("记录采集开始失败: %w", err)
	}
	return c.runWithID(ctx, runID, city, sourceIDs, limit, force, trigger)
}

// runWithID 采集主体（run 记录已建，仅落结果；供 Run 与 StartRunAsync 共用）。
func (c *Collector) runWithID(ctx context.Context, runID int64, city string, sourceIDs []string, limit int, force bool, trigger string) (*CollectSummary, error) {
	if city == "" {
		return nil, fmt.Errorf("city 不能为空")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if trigger == "" {
		trigger = "api"
	}

	// 全 kind 查询（job/training/…多频道共存）；源级 kind 决定入库目录与标签
	srcs, err := c.sources.ListSources(ctx, city, "")
	if err != nil {
		return nil, err
	}
	var enabled []*Source
	for _, s := range srcs {
		if !s.Enabled {
			continue
		}
		if len(sourceIDs) > 0 && !contains(sourceIDs, s.ID) {
			continue
		}
		enabled = append(enabled, s)
	}
	if len(enabled) == 0 {
		_ = c.sources.FinishRun(ctx, runID, "failed", 0, 0, 0, 0, 0, "该城市没有启用的采集源")
		return nil, fmt.Errorf("城市 %s 没有启用的采集源（请先创建源配置）", city)
	}

	summary := &CollectSummary{City: city, RunID: runID, Target: "采集/就业情报/" + city}
	for _, src := range enabled {
		res := CollectResult{Source: src.Name}
		// 把关 L0：泛栏目源（generic）与公告栏目（job_notice）必须配白名单（title_regex）
		// 才采集——防止"接错栏目"混装公文/公告等噪声直接进频道目录；未配白名单则跳过
		// （可配置可回滚，改配置即生效）。job_notice 是"含岗位的公告栏目"，同样要求白名单，
		// 否则一条"会议通知"就能灌进来（验证报告语义缺口）。
		if src.ChannelType == "generic" || src.ChannelType == "job_notice" {
			tpl, _ := ParseCollectTemplate(src.Template)
			if strings.TrimSpace(tpl.List.TitleRegex) == "" {
				summary.Skipped++
				res.Error = src.ChannelType + " 栏目源未配置白名单（template.list.title_regex），已跳过——请声明栏目类型或加白名单"
				summary.Results = append(summary.Results, res)
				continue
			}
		}
		items, ferr := c.fetchList(ctx, src)
		if ferr != nil {
			res.Error = ferr.Error()
			summary.Failed++
			_ = c.sources.MarkSourceRun(ctx, src.ID, "failed", 0)
			summary.Results = append(summary.Results, res)
			continue
		}
		if len(items) > limit {
			items = items[:limit]
		}
		// 把关①：模板排除关键词过滤（标题命中任一即剔除；剔除数计入 rejected——验证报告缺陷 2）
		items, rej := filterCollectByExcludeWords(items, src)
		for _, r := range rej {
			res.Rejects = append(res.Rejects, r)
		}
		rejCount := len(rej)
		res.Fetched = len(items)
		summary.Fetched += len(items)

		// 把关②：详情页正文抓取 + 有效性校验（404/超时/空正文剔除）——
		// 入库按条目归属城市分流（一源多城）：目标目录/标签/front matter 全部按解析城市落
		created, skipped, rejected, rejects, cerr := c.ingestItems(ctx, src, items, force)
		if cerr != nil {
			res.Error = cerr.Error()
			summary.Failed++
			_ = c.sources.MarkSourceRun(ctx, src.ID, "failed", res.Fetched)
			summary.Results = append(summary.Results, res)
			continue
		}
		res.Created, res.Skipped = created, skipped
		res.Rejected = rejected + rejCount // 排除词剔除 + 正文校验剔除
		res.Rejects = append(res.Rejects, rejects...)
		summary.Created += created
		summary.Skipped += skipped
		summary.Rejected += rejected + rejCount
		status := "ok"
		if rejected+rejCount > 0 {
			status = "partial"
		}
		if summary.Failed > 0 {
			status = "partial"
		}
		_ = c.sources.MarkSourceRun(ctx, src.ID, status, created)
		summary.Results = append(summary.Results, res)
	}

	if summary.Failed > 0 {
		summary.Status = "partial"
	} else {
		summary.Status = "ok"
	}
	// 把关 L3 可观测：拒绝明细（标题+原因）随运行记录落库，供"这轮拒了多少、为什么拒、拒了哪些"
	detail := ""
	var lines []string
	for _, r := range summary.Results {
		for _, rej := range r.Rejects {
			lines = append(lines, rej)
		}
	}
	if len(lines) > 0 {
		if b, err := json.Marshal(lines); err == nil {
			detail = string(b)
		}
	}
	_ = c.sources.FinishRunDetail(ctx, runID, summary.Status, summary.Fetched, summary.Created, summary.Skipped, summary.Rejected, summary.Failed, "", detail)
	summary.Tip = "已入库文件自动进知识库（图谱/智能集合/语义检索可用）；重新采集同源幂等（同名跳过），force=true 可强制重入。"
	return summary, nil
}

// ownerFor / spaceFor 单用户阶段固定系统 owner 与 home 空间（与 main.go homeSpaceID 一致）。
// 注：多用户阶段由调用方（handler/tool）显式传入，此处保留系统兜底。
func ownerFor(_ string) string { return SystemOwnerID }
func spaceFor(_ string) string { return SystemHomeSpaceID }

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// fetchList 按模板抓取并解析列表页。
func (c *Collector) fetchList(ctx context.Context, src *Source) ([]CollectItem, error) {
	tpl, err := ParseCollectTemplate(src.Template)
	if err != nil {
		return nil, fmt.Errorf("模板解析失败: %w", err)
	}
	// 种子条目：低频高价值页面（如培训目录通知）翻页不可达时直接作为采集条目
	items := []CollectItem{}
	if len(tpl.DirectItems) > 0 {
		items = append(items, tpl.DirectItems...)
	}
	// 多列表页 URL（默认 src.URL 单页）
	urls := []string{src.URL}
	if len(tpl.List.URLs) > 0 {
		urls = tpl.List.URLs
	}
	seen := map[string]bool{}
	for _, u := range urls {
		// SSRF 防护：采集目标必须为公网地址（防源配置指向内网/保留地址形成跳板）
		if err := ensurePublicURL(u); err != nil {
			if len(urls) == 1 {
				return nil, err
			}
			continue
		}
		pageItems, perr := c.fetchListPage(ctx, u, tpl, src.FetchMode)
		if perr != nil {
			if errors.Is(perr, errFetchSkipped) {
				continue // 整源 skip：不报错、不产出条目
			}
			if len(urls) == 1 {
				return nil, perr
			}
			continue // 多页场景：单页失败不阻断其余页面
		}
		for _, it := range pageItems {
			if !seen[it.Link] {
				seen[it.Link] = true
				items = append(items, it)
			}
		}
	}
	return items, nil
}

// errFetchSkipped 整源跳过标记（fetch_mode=skip）：不视为失败，不产出条目。
var errFetchSkipped = errors.New("fetch_mode=skip：该源已标记放弃")

// errFetchModeNotReady 未启用的分级 fetch 模式（tls/trawl 需接入对应组件）。
func errFetchModeNotReady(mode string) error {
	switch mode {
	case "tls":
		return errors.New("fetch_mode=tls 未启用：需接入 tls-client（TLS 指纹模拟）")
	case "trawl":
		return errors.New("fetch_mode=trawl 未启用：需部署 TRAWL 旁路服务")
	case "browser":
		return errors.New("fetch_mode=browser 未启用：需接入真浏览器（playwright/rod）驱动")
	default:
		return fmt.Errorf("fetch_mode=%s 不支持", mode)
	}
}

// setBrowserHeaders 设置完整 Chrome 浏览器头（http/tls 模式通用）：大幅降低被轻防护/UA 类反爬拦截的概率。
func setBrowserHeaders(h http.Header) {
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	// 不设置 Accept-Encoding：Go transport 默认请求 gzip 并自动解压；手动设置会禁用自动解压导致正文乱码
	h.Set("Cache-Control", "no-cache")
	h.Set("Pragma", "no-cache")
	h.Set("sec-ch-ua", `"Not/A)Brand";v="8", "Chromium";v="126", "Google Chrome";v="126"`)
	h.Set("sec-ch-ua-mobile", "?0")
	h.Set("sec-ch-ua-platform", `"Windows"`)
	h.Set("Sec-Fetch-Dest", "document")
	h.Set("Sec-Fetch-Mode", "navigate")
	h.Set("Sec-Fetch-Site", "none")
	h.Set("Sec-Fetch-User", "?1")
	h.Set("Upgrade-Insecure-Requests", "1")
}

// tlsClientOnce 惰性初始化 tls-client 单例：TLS/JA3 指纹 + HTTP/2 指纹模拟 Chrome，
// 用于 fetch_mode=tls 档（目标：蚌埠市级 WAF 这类"请求栈差异"站点；CivicPlus 类重防护仍走 TRAWL/放弃）。
var (
	tlsClientOnce sync.Once
	tlsClient     tlsclient.HttpClient
	tlsClientErr  error
)

func getTLSClient() (tlsclient.HttpClient, error) {
	tlsClientOnce.Do(func() {
		opts := []tlsclient.HttpClientOption{
			tlsclient.WithClientProfile(profiles.Chrome_133),
			tlsclient.WithTimeoutSeconds(15),
		}
		tlsClient, tlsClientErr = tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), opts...)
	})
	return tlsClient, tlsClientErr
}

// fetchTLS 用 tls-client 抓取页面：TLS/JA3+HTTP2 指纹模拟 Chrome，返回去压缩后的 body 与 HTTP 状态码。
func fetchTLS(ctx context.Context, pageURL string, timeout time.Duration) ([]byte, int, error) {
	cli, err := getTLSClient()
	if err != nil {
		return nil, 0, fmt.Errorf("tls-client 初始化失败: %w", err)
	}
	req, err := fhttp.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, 0, err
	}
	setBrowserHeaders(http.Header(req.Header))
	resp, err := cli.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("抓取失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// fetchListPage 抓取单个列表页并解析条目。
func (c *Collector) fetchListPage(ctx context.Context, pageURL string, tpl *CollectTemplate, fetchMode string) ([]CollectItem, error) {
	switch fetchMode {
	case "skip":
		return nil, errFetchSkipped
	case "tls":
		body, status, err := fetchTLS(ctx, pageURL, 15*time.Second)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d: %s", status, pageURL)
		}
		return ParseCollectList(body, pageURL, tpl)
	case "trawl":
		return nil, errFetchModeNotReady(fetchMode)
	case "browser":
		return nil, errFetchModeNotReady(fetchMode)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	setBrowserHeaders(req.Header)
	// 直连（不走系统代理）：采集目标是公网官办源，本机代理可能导致连接挂起
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		DialContext:           SafeDialContext, // H6 收尾：建连前 Control 复验最终 IP，关闭 DNS rebinding TOCTOU
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	cli := &http.Client{Timeout: 15 * time.Second, Transport: transport, CheckRedirect: safeCheckRedirect}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("抓取失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, pageURL)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return ParseCollectList(body, pageURL, tpl)
}

// ParseCollectTemplate 解析源模板 JSON（空模板给默认规则：任一非导航链接）。
func ParseCollectTemplate(raw json.RawMessage) (*CollectTemplate, error) {
	tpl := &CollectTemplate{}
	if len(raw) > 0 && string(raw) != "{}" {
		if err := json.Unmarshal(raw, tpl); err != nil {
			return nil, err
		}
	}
	return tpl, nil
}

// ParseCollectList 用 x/net/html 遍历 <a> 链接，按模板过滤并提取字段。
func ParseCollectList(body []byte, pageURL string, tpl *CollectTemplate) ([]CollectItem, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("HTML 解析失败: %w", err)
	}
	base, _ := url.Parse(pageURL)

	var linkRe, titleRe, exRe, dateRe, orgRe *regexp.Regexp
	if tpl.List.LinkRegex != "" {
		linkRe = regexp.MustCompile(tpl.List.LinkRegex)
	}
	if tpl.List.TitleRegex != "" {
		titleRe = regexp.MustCompile(tpl.List.TitleRegex)
	}
	if tpl.List.ExcludeRegex != "" {
		exRe = regexp.MustCompile(tpl.List.ExcludeRegex)
	}
	if tpl.List.DateRegex != "" {
		dateRe = regexp.MustCompile(tpl.List.DateRegex)
	}
	if tpl.List.OrgRegex != "" {
		orgRe = regexp.MustCompile(tpl.List.OrgRegex)
	}

	seen := map[string]bool{}
	var out []CollectItem

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			var href, text string
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href = attr.Val
				}
			}
			if n.FirstChild != nil {
				text = strings.TrimSpace(collectLinkText(n))
			}
			text = reSpace.ReplaceAllString(text, " ")
			if href != "" && text != "" && !reExcludeWords.MatchString(text) {
				// 模板正则匹配 href 原始值（相对路径写法，如 ^/cms/web/05byuip6/）；
				// 排除正则同时检查 href 与标题。
				if linkRe != nil && !linkRe.MatchString(href) {
					return
				}
				if titleRe != nil && !titleRe.MatchString(text) {
					return
				}
				if exRe != nil && (exRe.MatchString(href) || exRe.MatchString(text)) {
					return
				}
				abs := ResolveCollectLink(base, href, tpl.List.LinkPrefix)
				if seen[abs] {
					return
				}
				seen[abs] = true
				it := CollectItem{Title: text, Link: abs}
				if dateRe != nil {
					if m := dateRe.FindString(text); m != "" {
						it.Date = m
					} else if m := dateRe.FindString(abs); m != "" {
						it.Date = m
					}
				}
				if orgRe != nil {
					if m := orgRe.FindString(text); m != "" {
						it.Org = m
					}
				}
				out = append(out, it)
			}
			return
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return out, nil
}

// collectLinkText 取 <a> 可见文本（跳过嵌套 <a>）。
func collectLinkText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(ch *html.Node) {
		if ch.Type == html.TextNode {
			sb.WriteString(ch.Data)
		}
		if ch.Type == html.ElementNode && ch.Data == "a" {
			return
		}
		for gc := ch.FirstChild; gc != nil; gc = gc.NextSibling {
			walk(gc)
		}
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		walk(ch)
	}
	return sb.String()
}

// ResolveCollectLink 相对链接补全。
func ResolveCollectLink(base *url.URL, href, prefix string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	if prefix != "" {
		return strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(href, "/")
	}
	if base != nil {
		if u, err := url.Parse(href); err == nil {
			return base.ResolveReference(u).String()
		}
	}
	return href
}

// filterCollectByExcludeWords 把关①：标题命中模板 exclude_words 任一关键词即剔除（返回剩余条目 + 剔除明细）。
// 剔除只发生在入库前——不创建文件，不留残留；明细进运行摘要可审计，改配置后 force 重采可补救。
func filterCollectByExcludeWords(items []CollectItem, src *Source) ([]CollectItem, []string) {
	tpl, err := ParseCollectTemplate(src.Template)
	if err != nil || len(tpl.List.ExcludeWords) == 0 {
		return items, nil
	}
	out := items[:0]
	var rejects []string
	for _, it := range items {
		hit := ""
		for _, kw := range tpl.List.ExcludeWords {
			if kw != "" && strings.Contains(it.Title, kw) {
				hit = kw
				break
			}
		}
		if hit != "" {
			rejects = append(rejects, fmt.Sprintf("%s（排除词：%s）", it.Title, hit))
			continue
		}
		out = append(out, it)
	}
	return out, rejects
}

// minCollectBodyRunes 详情页正文最少有效字数（去标签后）；低于视为无效/空壳页，把关剔除。
const minCollectBodyRunes = 40

// fetchDetail 把关②：抓取详情页并提取正文。失败（404/超时/网络错误）或正文过短返回 error，
// 由 ingestItems 计为剔除——无效条目不创建文件。
func (c *Collector) fetchDetail(ctx context.Context, link, fetchMode string) (string, error) {
	// SSRF 防护：详情链接同样必须为公网地址
	if err := ensurePublicURL(link); err != nil {
		return "", err
	}
	switch fetchMode {
	case "skip":
		return "", errFetchSkipped
	case "tls":
		body, status, err := fetchTLS(ctx, link, 12*time.Second)
		if err != nil {
			return "", err
		}
		if status == http.StatusNotFound {
			return "", errors.New("HTTP 404（页面不存在）")
		}
		if status != http.StatusOK {
			return "", fmt.Errorf("HTTP %d", status)
		}
		doc, err := html.Parse(strings.NewReader(string(body)))
		if err != nil {
			return "", fmt.Errorf("HTML 解析失败: %w", err)
		}
		text := extractCollectBody(doc)
		if text == "" {
			return "", errors.New("正文提取为空（疑似验证码/JS 渲染/反爬页）")
		}
		return text, nil
	case "trawl":
		return "", errFetchModeNotReady(fetchMode)
	case "browser":
		return "", errFetchModeNotReady(fetchMode)
	}
	cli := &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		DialContext:           SafeDialContext, // H6 收尾：建连前 Control 复验最终 IP，关闭 DNS rebinding TOCTOU
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
	}, CheckRedirect: safeCheckRedirect}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	setBrowserHeaders(http.Header(req.Header))
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("抓取失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", errors.New("HTTP 404（页面不存在）")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("HTML 解析失败: %w", err)
	}
	text := extractCollectBody(doc)
	if text == "" {
		return "", errors.New("正文提取为空（疑似验证码/JS 渲染/反爬页）")
	}
	return text, nil
}

// extractCollectBody 通用正文提取：递归取 div/article/section 中文本最长的块，
// 跳过 script/style/nav/a（导航与脚本不参与），去空白后返回。
func extractCollectBody(root *html.Node) string {
	var best string
	bestLen := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "div" || n.Data == "article" || n.Data == "section") {
			var sb strings.Builder
			var inner func(*html.Node)
			inner = func(ch *html.Node) {
				if ch.Type == html.ElementNode {
					switch ch.Data {
					case "script", "style", "nav", "a", "form", "iframe", "button":
						return
					case "table":
						// 表格型政务页面（培训目录等）：转 Markdown 表格保结构，避免单元格文本粘连
						sb.WriteString("\n" + tableToMarkdown(ch) + "\n")
						return
					}
				}
				if ch.Type == html.TextNode {
					sb.WriteString(ch.Data)
				}
				for cc := ch.FirstChild; cc != nil; cc = cc.NextSibling {
					inner(cc)
				}
			}
			inner(n)
			txt := reSpace.ReplaceAllString(strings.TrimSpace(sb.String()), " ")
			if ln := len([]rune(txt)); ln > bestLen {
				bestLen = ln
				best = txt
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	if bestLen < minCollectBodyRunes {
		return ""
	}
	return best
}

// tableToMarkdown HTML 表格 → Markdown 表格（| 分隔，首行表头，单元格内 | 转义）。
func tableToMarkdown(t *html.Node) string {
	cellTxt := func(n *html.Node) string {
		var sb strings.Builder
		var w func(*html.Node)
		w = func(x *html.Node) {
			if x.Type == html.TextNode {
				sb.WriteString(x.Data)
			}
			for cc := x.FirstChild; cc != nil; cc = cc.NextSibling {
				w(cc)
			}
		}
		w(n)
		return reSpace.ReplaceAllString(strings.TrimSpace(sb.String()), " ")
	}
	var rows [][]string
	var cur []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch n.Data {
		case "tr":
			if len(cur) > 0 {
				rows = append(rows, cur)
				cur = nil
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					cur = append(cur, cellTxt(c))
				}
			}
		default:
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
	}
	walk(t)
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	if len(rows) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, r := range rows {
		for j := range r {
			r[j] = strings.ReplaceAll(r[j], "|", "\\|")
		}
		sb.WriteString("| " + strings.Join(r, " | ") + " |\n")
		if i == 0 {
			sb.WriteString("|" + strings.Repeat("---|", len(r)) + "\n")
		}
	}
	return sb.String()
}

// resolveCollectCity 解析条目归属城市（一源多城）。信号优先级：
// 标题城市名（强）＞ 正文"位于/地处 XX"地址（中）＞ 正文县名（弱，过 county_map）。
// 返回 (city, confidence, basis)；city 为空 = 沿用源城市（fallback=keep_source_city 或未命中）。
func (c *Collector) resolveCollectCity(title, body string, r *CityResolver) (string, string, string) {
	if r == nil {
		return "", "", ""
	}
	// 标题城市名（高置信）：捕获须命中白名单/县映射才采信（防"上海证券交易所上市"式逃逸）
	if r.FromTitleRegex != "" {
		if re, err := regexp.Compile(r.FromTitleRegex); err == nil {
			if m := re.FindStringSubmatch(title); len(m) > 1 && strings.TrimSpace(m[1]) != "" {
				name := normalizeCityName(strings.TrimSpace(m[1]))
				if inCityWhitelist(r, name) {
					return name, "高", "标题"
				}
			}
		}
	}
	body2 := stripPublisher(body)
	if r.FromBodyRegex != "" {
		if re, err := regexp.Compile(r.FromBodyRegex); err == nil {
			if m := re.FindStringSubmatch(body2); len(m) > 1 {
				name := strings.TrimSpace(m[1])
				if name == "" {
					return c.cityFallback(r)
				}
				// ① 县/区/县级市名：原始名直接查 county_map（"天长市"等县级市也在此列）
				if city, ok := r.CountyMap[name]; ok {
					return city, "中", "正文县名"
				}
				// ② 归一化（"马鞍山市"→"马鞍山"）后比对地级市白名单
				norm := normalizeCityName(name)
				if inCityWhitelist(r, norm) {
					return norm, "中", "正文地址"
				}
				// ③ 未命中白名单/映射 → 全省/未识别（诚实不硬猜）
				return c.cityFallback(r)
			}
		}
	}
	return c.cityFallback(r)
}

// existingNames 列出目录现有文件名 → 文件 id（去重 + force 覆盖更新定位用）。
func (c *Collector) existingNames(ctx context.Context, dirID string) (map[string]string, error) {
	list, err := c.files.ListDir(ctx, spaceFor(""), dirID, DefaultSiteID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(list))
	for _, f := range list {
		m[f.Name] = f.ID
	}
	return m, nil
}

// ingestItems 组装 markdown 入库 + 打标；返回 (created, skipped, rejected, rejects)。
// 把关②：每条目入库前抓详情页正文并校验——404/超时/空正文剔除（不入库、留痕），
// 正文有效则写入 md（AI 解读与检索有真实内容可用）。
// 一源多城：配置 city_resolver 时按解析城市分流到 `采集/就业情报/{城市}/`（识别不出→`全省/`），
// front matter 写 city_resolved/confidence/basis，标签按真实城市打；未配置 → 沿用源城市目录。
// 幂等（缺陷 1 修复）：同名文件 = 同一岗位——非 force 跳过；force 覆盖更新内容（保留 id/标签/图谱关系），
// 绝不产生 -N 副本。file.go Upload 的"自动加后缀"策略不再被采集路径触发。
func (c *Collector) ingestItems(ctx context.Context, src *Source, items []CollectItem, force bool) (int, int, int, []string, error) {
	created, skipped, rejected := 0, 0, 0
	var rejects []string
	ownerID := ownerFor(src.City)
	spaceID := spaceFor(src.City)
	// 频道目录/标签按 kind 区分：job→采集/就业情报（情报/就业），training→采集/培训补贴（情报/培训）；
	// 源配置 target_dir 可覆盖入库根目录（可设置：如 采集/求职/蚌埠），空=按 kind 默认
	kind := src.Kind
	if kind == "" {
		kind = "job"
	}
	baseDir := src.TargetDir
	tagRoot := "情报/就业"
	if kind == "training" {
		tagRoot = "情报/培训"
	}
	if baseDir == "" {
		baseDir = "采集/就业情报"
		if kind == "training" {
			baseDir = "采集/培训补贴"
		}
	}
	tpl, _ := ParseCollectTemplate(src.Template)
	resolver := tpl.CityResolver
	hasResolver := resolver != nil && (resolver.FromTitleRegex != "" || resolver.FromBodyRegex != "")

	// 目标目录与快照按城市懒加载（一源多城：一个源可同时写多个城市目录）
	dirCache := map[string]string{}
	snapCache := map[string]map[string]string{}
	getTarget := func(sub string) (string, map[string]string, error) {
		if d, ok := dirCache[sub]; ok {
			return d, snapCache[sub], nil
		}
		d, err := c.files.EnsurePath(ctx, ownerID, spaceID, "", baseDir+"/"+sub)
		if err != nil {
			return "", nil, err
		}
		snap, _ := c.existingNames(ctx, d)
		dirCache[sub], snapCache[sub] = d, snap
		return d, snap, nil
	}

	today := time.Now().Format("2006-01-02")
	for _, it := range items {
		date := it.Date
		if date == "" {
			date = today
		}
		name := CleanCollectName(date + "-" + it.Title + ".md")
		if name == ".md" {
			name = date + "-未命名.md"
		}
		// 把关②：先抓正文 + 有效性校验（404/超时/空正文剔除）——
		// 一源多城解析必须用详情正文（列表页 it.Body 恒为空，用它会永远"未识别"）
		body, derr := c.fetchDetail(ctx, it.Link, src.FetchMode)
		if derr != nil {
			rejected++
			rejects = append(rejects, fmt.Sprintf("%s（正文校验：%v）", it.Title, derr))
			continue
		}
		// 一源多城：解析归属城市 → 目标子目录（空=沿用源城市）
		rCity, rConf, rBasis := "", "", ""
		if hasResolver {
			rCity, rConf, rBasis = c.resolveCollectCity(it.Title, body, resolver)
		}
		sub := src.City
		if rCity != "" {
			sub = rCity
		}
		md := BuildCollectMarkdown(src, it, date, today, body, rCity, rConf, rBasis)

		dirID, snap, err := getTarget(sub)
		if err != nil {
			return created, skipped, rejected, rejects, fmt.Errorf("创建目录 %s 失败: %w", sub, err)
		}
		// 幂等：同名命中 → 非 force 跳过 / force 覆盖更新（保留 id 与标签，不新增副本）。
		// 注意：墓碑检查放在幂等之后——文件在册（含回收站恢复后）走正常幂等；
		// 墓碑只拦"不在册却命中删除记忆"的条目（清理→重采→又回来的死循环）。
		if fid, ok := snap[name]; ok {
			if !force {
				skipped++
				continue
			}
			if _, err := c.files.UpdateContent(ctx, ownerID, fid, md); err != nil {
				return created, skipped, rejected, rejects, fmt.Errorf("覆盖更新失败 %s: %w", name, err)
			}
			created++ // 计入"本次生效"数（覆盖视为一次有效写入）
			continue
		}
		// 删除记忆（tombstone）：用户删过/人工剔除过的条目不重建
		if c.sources.IsTombstoned(ctx, it.Link) {
			rejected++
			rejects = append(rejects, fmt.Sprintf("%s（已删除/人工剔除，跳过重建）", it.Title))
			continue
		}

		f, err := c.files.CreateDoc(ctx, ownerID, spaceID, dirID, name, md, DefaultSiteID)
		if err != nil {
			return created, skipped, rejected, rejects, fmt.Errorf("入库失败 %s: %w", name, err)
		}
		// 标签：真实城市（情报/城市/{city}）+ 频道类型（情报/就业 或 情报/培训）
		cityLabel := src.City
		if rCity != "" {
			cityLabel = rCity
		}
		var tagIDs []string
		if tid, err := c.tags.EnsurePath(ctx, ownerID, "情报/城市/"+cityLabel); err == nil {
			tagIDs = append(tagIDs, tid)
		}
		if tid, err := c.tags.EnsurePath(ctx, ownerID, tagRoot); err == nil {
			tagIDs = append(tagIDs, tid)
		}
		if len(tagIDs) > 0 {
			_ = c.tags.MergeFileTags(ctx, ownerID, f.ID, tagIDs)
		}
		created++
	}
	return created, skipped, rejected, rejects, nil
}

// BuildCollectMarkdown 组装岗位 markdown：front matter（溯源元数据 + 城市归属）+ 正文。
func BuildCollectMarkdown(src *Source, it CollectItem, date, today, body, rCity, rConf, rBasis string) string {
	status := inferCollectStatus(it.Title, src.Template)
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("source_url: " + it.Link + "\n")
	sb.WriteString("source_name: " + src.Name + "\n")
	sb.WriteString("city: " + src.City + "\n")
	if rCity != "" {
		sb.WriteString("city_resolved: " + rCity + "\n")
		sb.WriteString("city_confidence: " + rConf + "\n")
		sb.WriteString("city_basis: " + rBasis + "\n")
	}
	// kind 由源配置（job|training|bid|notice…），默认 job——频道化：培训/招标等频道入库即区分
	kind := src.Kind
	if kind == "" {
		kind = "job"
	}
	sb.WriteString("kind: " + kind + "\n")
	sb.WriteString("status: " + status + "\n")
	sb.WriteString("collected_at: " + today + "\n")
	if it.Org != "" {
		sb.WriteString("org: " + it.Org + "\n")
	}
	if date != today {
		sb.WriteString("publish_date: " + date + "\n")
	}
	sb.WriteString("---\n\n")
	sb.WriteString("# " + it.Title + "\n\n")
	sb.WriteString("- 来源：" + src.Name + "\n")
	sb.WriteString("- 原文：[原文链接](" + it.Link + ")\n")
	if it.Org != "" {
		sb.WriteString("- 单位：" + it.Org + "\n")
	}
	sb.WriteString("- 收录时间：" + today + "\n\n")
	if body != "" {
		// 正文截断至 16KB，防超大页入库
		if r := []rune(body); len(r) > 16000 {
			body = string(r[:16000]) + "\n\n（正文超长已截断）"
		}
		sb.WriteString("## 正文\n\n")
		sb.WriteString(body)
		sb.WriteString("\n")
	} else {
		sb.WriteString("## 摘要\n\n（待 AI 解读补充）\n")
	}
	return sb.String()
}

// inferCollectStatus 按源模板状态规则推断岗位状态（ongoing|upcoming|expired，默认 ongoing）。
func inferCollectStatus(title string, raw json.RawMessage) string {
	tpl, err := ParseCollectTemplate(raw)
	if err != nil {
		return "ongoing"
	}
	for _, r := range tpl.StatusRules {
		if strings.Contains(title, r.Match) {
			return r.Status
		}
	}
	return "ongoing"
}

// CleanCollectName 清理 Windows 非法文件名字符。
func CleanCollectName(s string) string {
	s = reBadName.ReplaceAllString(s, "-")
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
