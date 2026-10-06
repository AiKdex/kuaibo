// collector_probe.go 实现智能探路（probe_source 工具 + 未来 API 共用）：
// 给定一个官办列表页 URL，自动尝试 http/tls 两级抓取，分析页面结构
// （内容链接模式/标题/日期），生成推荐 CollectTemplate 与抓取模式，
// 结果可直接喂给 source_manage create 建源。
// 与 fetchListPage 的区别：输入无模板、仅裸 URL，做统计分析而非按模板过滤。
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ProbeResult 探路结果（JSON 键名与工具壳契约绑定，定型后不可改名）。
type ProbeResult struct {
	URL           string           `json:"url"`                    // 入参列表页 URL，原样回填
	Recommended   string           `json:"recommended_fetch_mode"` // 推荐抓取模式：http / tls（browser 本仓库未启用不推荐）
	Tried         []string         `json:"tried_modes"`            // 实际尝试过的模式（按顺序），三级全失败时用于排查
	BodyLength    int              `json:"body_length"`            // 实际取到的列表页 HTML 长度（字节）
	TotalLinks    int              `json:"total_links"`            // HTML 中全部去重 <a> 链接数
	ContentLinks  int              `json:"content_links"`          // 判定为"内容条目"的链接数（筛掉导航/空文本/重复后）
	ContentPrefix string           `json:"content_prefix"`         // 内容链接的公共路径前缀（若有明显公共前缀）
	Template      *CollectTemplate `json:"template"`               // 推断出的推荐模板（建议值，最终由人工确认）
	SampleItems   []CollectItem    `json:"sample_items"`           // 抽样条目（≤5），供建源后人工核对
	NextStep      string           `json:"next_step"`              // 下一步引导
}

// probe 常量：超时/正文上限沿用现网采集约定。
const (
	probeTimeout   = 15 * time.Second
	probeMaxBody   = 4 << 20 // 4MiB
	probeMaxSample = 5
)

// ProbeSource 智能探路：给定列表页 URL，自动尝试 http/tls 两级抓取，
// 分析页面结构并生成推荐的 CollectTemplate 与抓取模式。
// 两级均失败时不返回 nil 错误（result 带 Tried 供排查）。
func (c *Collector) ProbeSource(ctx context.Context, pageURL string) (*ProbeResult, error) {
	res := &ProbeResult{URL: pageURL, Tried: []string{}}
	var lastErr error

	// SSRF 防护：探路目标同样必须为公网地址
	if err := ensurePublicURL(pageURL); err != nil {
		return nil, err
	}

	// 抓取顺序：http（直连+Chrome 头）→ tls（TLS/JA3+HTTP2 指纹）。
	// browser 档本仓库未启用（无浏览器渲染抓取），仅在 Tried 记录、不硬失败。
	for _, mode := range []string{"http", "tls", "browser"} {
		res.Tried = append(res.Tried, mode)
		var body []byte
		var status int
		var err error
		switch mode {
		case "http":
			body, status, err = probeFetchHTTP(ctx, pageURL)
		case "tls":
			body, status, err = fetchTLS(ctx, pageURL, probeTimeout)
		case "browser":
			// browser 档本仓库未启用：不覆盖前面 http/tls 的真实失败原因，
			// 仅在没有更实质错误时兜底提示。
			if lastErr == nil {
				lastErr = errors.New("browser 档未启用（本仓库无浏览器渲染抓取）")
			}
			continue
		}
		if err != nil {
			lastErr = err
			continue
		}
		if status != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d: %s", status, pageURL)
			continue
		}
		if len(body) == 0 {
			lastErr = errors.New("空正文")
			continue
		}
		total, contents := probeAnalyze(body, pageURL)
		res.BodyLength = len(body)
		res.TotalLinks = total
		res.ContentLinks = len(contents)
		if len(contents) == 0 {
			lastErr = errors.New("未解析出内容链接（疑似 JS 渲染/反爬页）")
			continue
		}
		// 该模式成功：填充结果并返回（Recommended = Tried 最后一个成功的模式）
		res.Recommended = mode
		res.ContentPrefix = probeCommonPrefix(contents)
		res.Template = probeBuildTemplate(contents, res.ContentPrefix)
		// 模板推断出 date_regex 时给抽样条目回填日期，便于人工核对
		if res.Template.List.DateRegex != "" {
			if dre := regexp.MustCompile(res.Template.List.DateRegex); dre != nil {
				for i := range contents {
					if m := dre.FindString(contents[i].Title); m != "" {
						contents[i].Date = m
					} else if m := dre.FindString(contents[i].Link); m != "" {
						contents[i].Date = m
					}
				}
			}
		}
		if len(contents) > probeMaxSample {
			res.SampleItems = contents[:probeMaxSample]
		} else {
			res.SampleItems = contents
		}
		res.NextStep = "用 source_manage create 创建源（template 和 fetch_mode 用上面的推荐值），再用 collect_jobs 触发采集"
		return res, nil
	}
	if lastErr == nil {
		lastErr = errors.New("两级抓取均未成功")
	}
	return res, lastErr
}

// probeFetchHTTP 直连抓取（与 fetchListPage http 分支同款 client：Chrome 头 + 不走系统代理 + 超时约定）。
func probeFetchHTTP(ctx context.Context, pageURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, 0, err
	}
	setBrowserHeaders(req.Header)
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		DialContext:           SafeDialContext, // H6 收尾：建连前 Control 复验最终 IP，关闭 DNS rebinding TOCTOU
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	cli := &http.Client{Timeout: probeTimeout, Transport: transport, CheckRedirect: safeCheckRedirect}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("抓取失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, probeMaxBody))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// probeAnalyze 遍历 <a> 做统计：返回去重后的全部链接数与内容条目（过滤导航/空文本/重复/站外链接）。
// 站外链接（不同 host）视为导航/友情链接入口——官办站内容条目几乎总在同域，此启发对栏目页安全。
func probeAnalyze(body []byte, pageURL string) (int, []CollectItem) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return 0, nil
	}
	base, _ := url.Parse(pageURL)
	baseHost := ""
	if base != nil {
		baseHost = strings.ToLower(base.Host)
	}
	seenAll := map[string]bool{}
	seenContent := map[string]bool{}
	var contents []CollectItem

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			var href, text string
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href = attr.Val
				}
			}
			if href != "" {
				abs := ResolveCollectLink(base, href, "")
				if !seenAll[abs] {
					seenAll[abs] = true
				}
				if n.FirstChild != nil {
					text = strings.TrimSpace(collectLinkText(n))
				}
				text = reSpace.ReplaceAllString(text, " ")
				// 站外/javascript/空文本/导航词 → 不计内容
				u, uerr := url.Parse(abs)
				if uerr != nil || (u.Scheme != "http" && u.Scheme != "https") || (baseHost != "" && strings.ToLower(u.Host) != baseHost) {
					return
				}
				if text != "" && !reExcludeWords.MatchString(text) && !seenContent[abs] {
					seenContent[abs] = true
					contents = append(contents, CollectItem{Title: text, Link: abs})
				}
			}
			return
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return len(seenAll), contents
}

// probeCommonPrefix 求内容链接的公共路径前缀（排除域名，只算 path 段）。
// 前缀过短（<3 字符，如 "/"）视为无稳定公共前缀，返回空。
func probeCommonPrefix(items []CollectItem) string {
	if len(items) < 2 {
		return ""
	}
	paths := make([]string, 0, len(items))
	for _, it := range items {
		if u, err := url.Parse(it.Link); err == nil {
			p := u.Path
			if p == "" {
				p = "/"
			}
			paths = append(paths, p)
		}
	}
	if len(paths) < 2 {
		return ""
	}
	// 字符级最长公共前缀
	prefix := paths[0]
	for _, p := range paths[1:] {
		i := 0
		for i < len(prefix) && i < len(p) && prefix[i] == p[i] {
			i++
		}
		prefix = prefix[:i]
		if prefix == "" {
			break
		}
	}
	// 截到最后一个 "/"（保证是路径段边界，不把文件名半截当前缀）
	if i := strings.LastIndex(prefix, "/"); i > 0 {
		prefix = prefix[:i+1]
	}
	if len(prefix) < 3 || prefix == "/" {
		return ""
	}
	return prefix
}

// reDateCandidates 常见日期形态（探路检测标题/链接用）。
var reDateCandidates = []struct {
	pattern *regexp.Regexp
	expr    string
}{
	{regexp.MustCompile(`\d{4}-\d{1,2}-\d{1,2}`), `\d{4}-\d{1,2}-\d{1,2}`},
	{regexp.MustCompile(`\d{4}/\d{1,2}/\d{1,2}`), `\d{4}/\d{1,2}/\d{1,2}`},
	{regexp.MustCompile(`\d{4}年\d{1,2}月`), `\d{4}年\d{1,2}月`},
	{regexp.MustCompile(`\d{8}`), `\d{8}`},
}

// probeBuildTemplate 基于内容链接统计生成推荐模板（建议值，允许为空字段）。
// 原则：模板是"建议"而非"铁定结果"；无法可靠识别的一律留空，不臆造。
func probeBuildTemplate(items []CollectItem, prefix string) *CollectTemplate {
	tpl := &CollectTemplate{}
	if prefix != "" {
		tpl.List.LinkPrefix = prefix
	}
	// date_regex：统计各形态命中数，取命中最多的（≥2 才给，否则留空=采集端取当日）
	bestExpr, bestHits := "", 0
	for _, cand := range reDateCandidates {
		hits := 0
		for _, it := range items {
			if cand.pattern.MatchString(it.Title) || cand.pattern.MatchString(it.Link) {
				hits++
			}
		}
		if hits >= 2 && hits > bestHits {
			bestExpr, bestHits = cand.expr, hits
		}
	}
	if bestExpr != "" {
		tpl.List.DateRegex = bestExpr
	}
	// status_rules：标题常见词给初步候选（ongoing/upcoming/expired），最终由业务人工确认。
	tpl.StatusRules = []struct {
		Match  string `json:"match"`
		Status string `json:"status"`
	}{
		{Match: "公示", Status: "expired"},
		{Match: "结果", Status: "expired"},
		{Match: "招聘", Status: "ongoing"},
		{Match: "诚聘", Status: "ongoing"},
		{Match: "招录", Status: "ongoing"},
		{Match: "预告", Status: "upcoming"},
		{Match: "计划", Status: "upcoming"},
	}
	return tpl
}
