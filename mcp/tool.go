package mcpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"websearch/pkg/cache"
	"websearch/pkg/config"
	"websearch/pkg/jina"
	"websearch/pkg/log"
	"websearch/pkg/search"
	"websearch/pkg/summarizer"
	"websearch/pkg/webfetch"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchParamsWithIntent LLM 摘要启用时使用的参数（含 intent）。
type SearchParamsWithIntent struct {
	Query      string `json:"query" jsonschema:"description,搜索关键词：需精准凝练地表达核心检索意图（建议 2-6 个关键词或一个短句），只保留最能定位目标的词；不要把同义词、过程词、修饰词一股脑堆砌成关键词列表，否则会稀释相关性。例如用 'Go 泛型 性能' 而非 'Go 泛型 类型参数 编译 运行时 性能 基准 对比 优化 使用方法'"`
	Intent     string `json:"intent" jsonschema:"description,搜索意图，描述你希望通过搜索解决什么问题或获取什么信息。例如 '了解goroutine调度原理' '对比React和Vue的生态差异' '查找某API的用法示例'。提供意图后可获得更精准的结构化摘要"`
	TimeRange  int    `json:"time_range,omitempty" jsonschema:"description,搜索时间范围（月），限制搜索最近N个月的内容。例如 1=近1个月，3=近3个月，6=近半年，12=近一年。默认3，0表示不限"`
	FetchTopN  int    `json:"fetch_top_n,omitempty" jsonschema:"description,搜索并评分后对前 N 条并发抓取正文（默认 0 不抓，上限 5）。单条失败保留原 snippet，不导致整次搜索失败"`
}

// SearchParamsNoIntent LLM 摘要未启用时使用的参数（无 intent，节省上下文 token）。
type SearchParamsNoIntent struct {
	Query     string `json:"query" jsonschema:"description,搜索关键词：需精准凝练地表达核心检索意图（建议 2-6 个关键词或一个短句），只保留最能定位目标的词；不要把同义词、过程词、修饰词一股脑堆砌成关键词列表，否则会稀释相关性。例如用 'Go 泛型 性能' 而非 'Go 泛型 类型参数 编译 运行时 性能 基准 对比 优化 使用方法'"`
	TimeRange int    `json:"time_range,omitempty" jsonschema:"description,搜索时间范围（月），限制搜索最近N个月的内容。例如 1=近1个月，3=近3个月，6=近半年，12=近一年。默认3，0表示不限"`
	FetchTopN int    `json:"fetch_top_n,omitempty" jsonschema:"description,搜索并评分后对前 N 条并发抓取正文（默认 0 不抓，上限 5）。单条失败保留原 snippet，不导致整次搜索失败"`
}

// AcademicSearchParams 学术搜索参数。
type AcademicSearchParams struct {
	Query     string   `json:"query" jsonschema:"description,学术搜索关键词，例如 'transformer attention mechanism' 或 'CRISPR gene editing'"`
	Engines   []string `json:"engines,omitempty" jsonschema:"description,指定引擎子集（为空则使用全部已启用引擎）。示例: 医学论文用 [\"pubmed\"], CS预印本用 [\"arxiv\"], 物理/数学用 [\"arxiv\",\"crossref\"]"`
	TimeRange string   `json:"time_range,omitempty" jsonschema:"description,时间范围过滤。可选值: year（近一年）, month（近一月）, week（近一周）, day（近一天）。为空则不限"`
	Page      int      `json:"page,omitempty" jsonschema:"description,结果页码（默认 1），每页约 10 条"`
}

// CleanFetchParams cleanfetch 工具参数。
type CleanFetchParams struct {
	URL  string   `json:"url" jsonschema:"description,要抓取的网页 URL，例如 'https://example.com/article'"`
	URLs []string `json:"urls,omitempty" jsonschema:"description,可选批量抓取：与 url 合并去重后最多 5 个。每条独立预检与抓取，单条失败不影响其它，结果按 URL 分节返回"`
}

// maxBatchFetchURLs 单次批量抓取的 URL 上限。
const maxBatchFetchURLs = 5

var (
	searchapi           search.SearchInf
	searchGroup         *search.SearchGroup
	fallbackSearch      *search.BingSearchAdapter
	summarizerInst      *summarizer.Summarizer
	cacheInst           *cache.Cache
	jinaInst            *jina.Reader
	webfetchInst        *webfetch.Fetcher
	academicSearcher    search.AcademicSearcher
	smartSearchConf     config.SmartSearchConfig
	cleanFetchMaxSizeMB int
	pdfMaxPages         int
)

// Init 初始化 MCP 服务组件，通过 Option 模式按需加载。
func Init(conf config.Config, opts ...ServerOption) error {
	for _, opt := range opts {
		opt()
	}

	if searchapi == nil {
		return fmt.Errorf("搜索引擎未初始化，请检查配置")
	}
	return nil
}

func GetCache() *cache.Cache {
	return cacheInst
}

// GetSearchGroup 返回搜索引擎组（供 server 包与 SearXNG 共用同一套引擎）。
func GetSearchGroup() *search.SearchGroup {
	return searchGroup
}

// GetWebFetch 返回 WebFetch 引擎实例（供 server 包关闭时清理）。
func GetWebFetch() *webfetch.Fetcher {
	return webfetchInst
}

// ── WebSearch 处理函数（两个版本适配不同 Params） ─────────────────────────────

// WebSearchWithIntent LLM 启用时的 tool handler。
func WebSearchWithIntent(ctx context.Context, req *mcp.CallToolRequest, params *SearchParamsWithIntent) (*mcp.CallToolResult, any, error) {
	return doWebSearch(ctx, req, params.Query, params.Intent, params.TimeRange, params.FetchTopN)
}

// WebSearchNoIntent LLM 未启用时的 tool handler。
func WebSearchNoIntent(ctx context.Context, req *mcp.CallToolRequest, params *SearchParamsNoIntent) (*mcp.CallToolResult, any, error) {
	return doWebSearch(ctx, req, params.Query, "", params.TimeRange, params.FetchTopN)
}

// AcademicSearchHandler 学术搜索 tool handler。
func AcademicSearchHandler(ctx context.Context, req *mcp.CallToolRequest, params *AcademicSearchParams) (*mcp.CallToolResult, any, error) {
	return doAcademicSearch(params.Query, params.Engines, params.TimeRange, params.Page)
}

// doWebSearch 通用网页搜索逻辑。
// timeRangeMonths 控制搜索时间范围（月），默认 3，0 表示不限。
// 摘要阶段优先流式推送（MCP progress notification），客户端可实时看到生成过程。
func doWebSearch(ctx context.Context, req *mcp.CallToolRequest, query, intent string, timeRangeMonths, fetchTopN int) (*mcp.CallToolResult, any, error) {
	if searchapi == nil {
		return nil, nil, fmt.Errorf("api 初始化未完成")
	}

	fetchTopN = clampFetchTopN(fetchTopN)

	// 默认3个月
	if timeRangeMonths == 0 {
		timeRangeMonths = 3
	}
	lookbackDays := timeRangeMonths * 30
	cacheQuery := webSearchCacheQuery(query, fetchTopN)

	// ---- 缓存查询 ----
	if cacheInst != nil {
		rec, hitType, err := cacheInst.Lookup(cacheQuery, intent, false)
		if err != nil {
			log.Errf("缓存查询异常，跳过缓存: %v", err)
		} else if rec != nil && !rec.Academic {
			if result, ok := finishCachedWebSearch(ctx, rec, hitType, query, intent, fetchTopN); ok {
				return result, nil, nil
			}
		}
		if fetchTopN > 0 {
			// n 专用 key 未命中时，用未抽取的缓存补抽，避免把无正文结果当成完整命中
			rec, hitType, err := cacheInst.Lookup(query, intent, false)
			if err == nil && rec != nil && !rec.Academic && hitType == "query_only" {
				results, parseErr := rec.GetRawResults()
				if parseErr == nil {
					results = enrichFetchedTopN(ctx, results, fetchTopN)
					return finishWebSearch(ctx, req, query, intent, cacheQuery, results)
				}
			}
		}
	}

	// ---- 搜索 ----
	engineName := searchapi.Name()
	var results []search.SearchResult
	var err error

	// 优先使用支持时间范围的接口
	if timeRanger, ok := searchapi.(search.SearchTimeRanger); ok {
		results, err = timeRanger.SearchRawWithTimeRange(query, lookbackDays)
	} else {
		results, err = searchapi.SearchRaw(query)
	}
	if err != nil {
		if fallbackSearch != nil && searchapi != fallbackSearch {
			log.Errf("主搜索引擎失败(%v)，回退到 Bing 引擎", err)
			results, err = fallbackSearch.SearchRaw(query)
			engineName = "bing"
		}
		if err != nil {
			return nil, nil, err
		}
	}

	// 单引擎模式下应用 smartsearch 过滤（HybridSearchImpl 已在 SearchRaw 内处理）
	if _, isHybrid := searchapi.(*search.HybridSearchImpl); !isHybrid {
		results = postSearchFilter(results, engineName)
	}

	results = enrichFetchedTopN(ctx, results, fetchTopN)
	return finishWebSearch(ctx, req, query, intent, cacheQuery, results)
}

const maxFetchTopN = 5

func clampFetchTopN(n int) int {
	if n < 0 {
		return 0
	}
	if n > maxFetchTopN {
		return maxFetchTopN
	}
	return n
}

func webSearchCacheQuery(query string, n int) string {
	if n <= 0 {
		return query
	}
	return query + "|fetch_top_n=" + strconv.Itoa(n)
}

func finishCachedWebSearch(_ context.Context, rec *cache.CacheRecord, hitType, query, intent string, fetchTopN int) (*mcp.CallToolResult, bool) {
	switch hitType {
	case "exact_intent":
		if rec.Summary != "" {
			log.Infof("缓存命中(exact_intent+summary): query=%s", query)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: rec.Summary}}}, true
		}
		fallthrough
	case "query_only":
		results, parseErr := rec.GetRawResults()
		if parseErr != nil {
			return nil, false
		}
		log.Infof("缓存命中(query_only): query=%s", query)
		ret, mergeErr := formatRawResults(query, results)
		if mergeErr != nil {
			return nil, false
		}
		if intent != "" && summarizerInst != nil && rec.Summary == "" {
			cacheQuery := webSearchCacheQuery(query, fetchTopN)
			go asyncSummarize(query, cacheQuery, intent, results)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: ret}}}, true
	}
	return nil, false
}

func asyncSummarize(query, cacheQuery, intent string, results []search.SearchResult) {
	defer func() {
		if r := recover(); r != nil {
			log.Errf("异步摘要 panic: %v", r)
		}
	}()
	output, sumErr := summarizerInst.Summarize(query, intent, results)
	if sumErr == nil && cacheInst != nil {
		_ = cacheInst.UpdateSummary(cacheQuery, intent, output)
		log.Infof("后台异步摘要完成: query=%s, intent=%s", query, intent)
	}
}

func finishWebSearch(ctx context.Context, req *mcp.CallToolRequest, query, intent, cacheQuery string, results []search.SearchResult) (*mcp.CallToolResult, any, error) {
	if intent != "" && summarizerInst != nil {
		var output string
		var sumErr error
		if req != nil && req.Session != nil {
			output, sumErr = streamSummarize(ctx, req, query, intent, results)
			if sumErr != nil {
				log.Errf("LLM 流式摘要失败，回退到非流式摘要: %v", sumErr)
			}
		}
		if sumErr != nil {
			output, sumErr = summarizerInst.Summarize(query, intent, results)
		}
		if sumErr == nil {
			if cacheInst != nil {
				_ = cacheInst.Store(cacheQuery, intent, false, results, output)
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: output}}}, nil, nil
		}
		log.Errf("LLM 摘要失败，回退到原始结果: %v", sumErr)
	}

	ret, err := formatRawResults(query, results)
	if err != nil {
		return nil, nil, err
	}
	if cacheInst != nil {
		_ = cacheInst.Store(cacheQuery, intent, false, results, "")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: ret}}}, nil, nil
}

type pageFetcher func(ctx context.Context, rawURL string) (string, error)

func enrichTopN(ctx context.Context, results []search.SearchResult, n int, fetch pageFetcher) []search.SearchResult {
	if n <= 0 || fetch == nil || len(results) == 0 {
		return results
	}
	if n > len(results) {
		n = len(results)
	}
	out := make([]search.SearchResult, len(results))
	copy(out, results)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		u := strings.TrimSpace(out[i].Url)
		if u == "" {
			continue
		}
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			if ctx.Err() != nil {
				return
			}
			body, err := fetch(ctx, u)
			if err != nil || strings.TrimSpace(body) == "" {
				return
			}
			out[i].Content = body
		}(i, u)
	}
	wg.Wait()
	return out
}

func fetchPageContent(ctx context.Context, rawURL string) (string, error) {
	if err := validateURLSecurity(rawURL); err != nil {
		return "", err
	}
	if webfetchInst == nil {
		return "", fmt.Errorf("webfetch 未初始化")
	}
	res, err := webfetchInst.Fetch(ctx, rawURL)
	if err != nil {
		return "", err
	}
	if res == nil || strings.TrimSpace(res.Markdown) == "" {
		return "", fmt.Errorf("empty content")
	}
	return res.Markdown, nil
}

func enrichFetchedTopN(ctx context.Context, results []search.SearchResult, n int) []search.SearchResult {
	if n <= 0 {
		return results
	}
	fetchCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		fetchCtx, cancel = context.WithTimeout(ctx, 15*time.Second)
	}
	defer cancel()
	return enrichTopN(fetchCtx, results, n, fetchPageContent)
}

// doAcademicSearch 学术搜索逻辑。
func doAcademicSearch(query string, engines []string, timeRange string, page int) (*mcp.CallToolResult, any, error) {
	if academicSearcher == nil {
		return nil, nil, fmt.Errorf("学术搜索引擎未启用，请检查配置 bing.academic 是否为 true")
	}

	// ---- 缓存查询 ----
	enginesKey := strings.Join(engines, ",")
	cacheKey := query + "|" + timeRange + "|" + enginesKey
	if cacheInst != nil {
		rec, hitType, err := cacheInst.Lookup(cacheKey, "", true)
		if err != nil {
			log.Errf("缓存查询异常，跳过缓存: %v", err)
		} else if rec != nil && rec.Academic && hitType == "query_only" {
			results, parseErr := rec.GetRawResults()
			if parseErr == nil {
				log.Infof("学术缓存命中: query=%s", query)
				ret, mergeErr := formatAcademicResults(query, search.AcademicSearchResult{Results: results})
				if mergeErr == nil {
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: ret}}}, nil, nil
				}
			}
		}
	}

	// ---- 学术搜索 ----
	opts := search.AcademicSearchOptions{
		Page:      page,
		TimeRange: timeRange,
		Engines:   engines,
	}

	log.Infof("学术搜索: query=%s, engines=%v, timeRange=%s, page=%d", query, engines, timeRange, page)
	res, err := academicSearcher.SearchAcademicRaw(query, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("学术搜索失败: %w", err)
	}

	ret, err := formatAcademicResults(query, res)
	if err != nil {
		return nil, nil, err
	}
	if cacheInst != nil {
		// 逐引擎错误不写入缓存（Store 仍只存干净结果），命中路径展示的是上次结果
		_ = cacheInst.Store(cacheKey, "", true, res.Results, "")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: ret}}}, nil, nil
}

// streamSummarize 流式生成摘要：通过 MCP progress notification 逐 token 推送，
// 同时累积全文，流结束后返回完整格式化摘要（含引用）。
// 客户端断开（ctx 取消）时自动中止。
func streamSummarize(ctx context.Context, req *mcp.CallToolRequest, query, intent string, results []search.SearchResult) (string, error) {
	notifyProgress(ctx, req, 1, 2, fmt.Sprintf("搜索完成，共 %d 条结果，正在生成摘要...", len(results)))

	ch := make(chan string, 64)
	errCh := make(chan error, 1)
	go summarizerInst.SummarizeStream(ctx, query, intent, results, ch, errCh)

	var sb strings.Builder
	for {
		select {
		case token, ok := <-ch:
			if !ok {
				// 流结束；errCh 可能已发送结果（缓冲 1，非阻塞读取）
				select {
				case err := <-errCh:
					if err != nil {
						return "", err
					}
				default:
				}
				return summarizer.FormatCitation(query, sb.String(), results), nil
			}
			sb.WriteString(token)
			notifyProgress(ctx, req, 2, 2, token)
		case err := <-errCh:
			return "", err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// notifyProgress 向 MCP 客户端推送进度通知（StreamableHTTP 传输下实时到达）。
// 推送失败仅记录日志，不影响主流程。
func notifyProgress(ctx context.Context, req *mcp.CallToolRequest, progress, total float64, message string) {
	if req == nil || req.Session == nil {
		return
	}
	params := &mcp.ProgressNotificationParams{
		ProgressToken: req.Params.GetProgressToken(),
		Progress:      progress,
		Total:         total,
		Message:       message,
	}
	if err := req.Session.NotifyProgress(ctx, params); err != nil {
		log.Debugf("progress notification 推送失败: %v", err)
	}
}

func formatAcademicResults(query string, res search.AcademicSearchResult) (string, error) {
	if adapter, ok := academicSearcher.(*search.AcademicAdapter); ok {
		return adapter.MergeContentWithErrors(query, res.Results, res.EngineErrors)
	}
	return searchapi.MergeContent(query, res.Results)
}

func formatRawResults(query string, results []search.SearchResult) (string, error) {
	return searchapi.MergeContent(query, results)
}

// ── CleanFetch 工具 ──────────────────────────────────────────────────────────

// PDFParserParams pdf_parser 工具参数。
type PDFParserParams struct {
	Path  string `json:"path" jsonschema:"description,本地 PDF 文件路径（绝对路径或 file://）或远程 http(s) PDF URL。学术搜索结果中的 pdf_url 直接作为本参数传入"`
	Pages string `json:"pages,omitempty" jsonschema:"description,可选页码范围（1-based），如 '1-10'、'1,3,5-7'。省略时仅解析前 max_pages 页（默认 20），超长文档会提示已截断"`
}

// CleanFetch 通过 go-webfetch 抓取网页，失败时回退到 Jina Reader。
// 支持 url + urls 批量（合并去重，最多 5 个）：并发抓取，单条失败不影响其它。
// 只传一个 URL 时输出与旧版完全一致。
func CleanFetch(ctx context.Context, req *mcp.CallToolRequest, params *CleanFetchParams) (*mcp.CallToolResult, any, error) {
	urls := mergeFetchURLs(params.URL, params.URLs)
	if len(urls) == 0 {
		return nil, nil, fmt.Errorf("url 和 urls 参数至少填一个")
	}
	if len(urls) > maxBatchFetchURLs {
		return nil, nil, fmt.Errorf("批量抓取最多 %d 个 URL（当前 %d 个），请拆分多次调用", maxBatchFetchURLs, len(urls))
	}

	if len(urls) == 1 {
		text, err := fetchCleanPage(ctx, urls[0])
		if err != nil {
			return nil, nil, err
		}
		return textResult(text), nil, nil
	}

	type batchItem struct {
		text string
		err  error
	}
	items := make([]batchItem, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			text, err := fetchCleanPage(ctx, u)
			items[i] = batchItem{text: text, err: err}
		}(i, u)
	}
	wg.Wait()

	var sb strings.Builder
	okCount := 0
	for i, it := range items {
		if i > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		fmt.Fprintf(&sb, "## [%d] %s\n\n", i+1, urls[i])
		if it.err != nil {
			fmt.Fprintf(&sb, "**抓取失败**: %v", it.err)
			continue
		}
		okCount++
		sb.WriteString(it.text)
	}
	log.Infof("批量抓取完成: %d/%d 成功", okCount, len(urls))
	return textResult(sb.String()), nil, nil
}

// mergeFetchURLs 合并 url 与 urls：trim、去重、保持顺序。
func mergeFetchURLs(url string, urls []string) []string {
	seen := make(map[string]bool, len(urls)+1)
	out := make([]string, 0, len(urls)+1)
	for _, u := range append([]string{url}, urls...) {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// fetchCleanPage 抓取单个 URL：SSRF 预检 + HEAD 预检 + webfetch（Jina 兜底），
// 返回格式化后的 Markdown 文本。
func fetchCleanPage(ctx context.Context, rawURL string) (string, error) {
	// ── 安全预检：DNS rebinding 防护 ──
	if err := validateURLSecurity(rawURL); err != nil {
		return "", err
	}

	// ── HEAD 预检：检测文件大小和类型 ──
	if err := headCheck(ctx, rawURL); err != nil {
		return "", err
	}

	// ── 第一层：go-webfetch（无需代理）──
	if webfetchInst != nil {
		result, err := webfetchInst.Fetch(ctx, rawURL)
		if err == nil {
			return formatWebFetchResult(result), nil
		}
		log.Infof("webfetch 抓取失败(%v)，尝试回退到 Jina Reader", err)

		// ── 第二层：Jina Reader（需代理，jinaInst != nil 即表示代理已开启）──
		if jinaInst != nil {
			jinaResult, jinaErr := jinaInst.Fetch(rawURL)
			if jinaErr == nil {
				return formatJinaResult(jinaResult), nil
			}
			return "", fmt.Errorf("webfetch: %v; Jina 兜底: %w", err, jinaErr)
		}
		return "", fmt.Errorf("webfetch 抓取失败: %v", err)
	}

	// webfetch 未初始化，仅用 Jina（兼容旧模式：仅代理+Jina Key）
	if jinaInst != nil {
		jinaResult, jinaErr := jinaInst.Fetch(rawURL)
		if jinaErr != nil {
			return "", fmt.Errorf("jina reader 抓取失败: %w", jinaErr)
		}
		return formatJinaResult(jinaResult), nil
	}

	return "", fmt.Errorf("webfetch 和 jina reader 均未初始化")
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// resolvePDFPath 将 pdf_parser 的 path 规范为 webfetch 可消费的 URL。
// http(s) 远程地址原样返回且 remote=true，禁止再拼 file://。
func resolvePDFPath(path string) (fetchURL string, remote bool) {
	p := strings.TrimSpace(path)
	lower := strings.ToLower(p)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return p, true
	}
	if strings.HasPrefix(p, "file://") {
		return p, false
	}
	return "file:///" + strings.ReplaceAll(p, `\`, "/"), false
}

// PDFParserHandler PDF 解析 tool handler：本地路径 / file:// / 远程 http(s) URL。
// 支持可选 pages 页码范围；省略时受 pdf_parser.max_pages（默认 20）约束并提示截断。
func PDFParserHandler(ctx context.Context, req *mcp.CallToolRequest, params *PDFParserParams) (*mcp.CallToolResult, any, error) {
	if params.Path == "" {
		return nil, nil, fmt.Errorf("path 参数不能为空")
	}

	pages, err := parsePagesSpec(params.Pages)
	if err != nil {
		return nil, nil, err
	}
	maxPages := pdfMaxPages
	if maxPages <= 0 {
		maxPages = 20
	}
	if len(pages) > maxPages {
		return nil, nil, fmt.Errorf("pages 指定了 %d 页，超过单次上限 max_pages=%d，请拆分多次调用（如 pages=\"1-%d\"）", len(pages), maxPages, maxPages)
	}

	if webfetchInst == nil {
		return nil, nil, fmt.Errorf("webfetch 未初始化，请先启用 cleanfetch")
	}

	pdfPath, remote := resolvePDFPath(params.Path)
	if remote {
		if err := validateURLSecurity(pdfPath); err != nil {
			return nil, nil, err
		}
		if err := headCheck(ctx, pdfPath); err != nil {
			return nil, nil, err
		}
	}

	result, err := webfetchInst.FetchPDFWithPages(ctx, pdfPath, pages, maxPages)
	if err != nil {
		return nil, nil, fmt.Errorf("PDF 解析失败: %v", err)
	}
	return textResult(formatWebFetchResult(result)), nil, nil
}

// parsePagesSpec 解析页码表达式："3"、"1-10"、"1,3,5-7"（1-based）。
// 返回去重升序页码列表；非法格式返回错误。
func parsePagesSpec(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	seen := map[int]bool{}
	var out []int
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			from, err1 := strconv.Atoi(strings.TrimSpace(lo))
			to, err2 := strconv.Atoi(strings.TrimSpace(hi))
			if err1 != nil || err2 != nil || from < 1 || to < from {
				return nil, fmt.Errorf("pages 参数格式非法: %q，示例：1-10 或 1,3,5-7", spec)
			}
			for p := from; p <= to; p++ {
				if !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
			}
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil || p < 1 {
			return nil, fmt.Errorf("pages 参数格式非法: %q，示例：1-10 或 1,3,5-7", spec)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out, nil
}

// formatJinaResult 将 Jina Reader 结果格式化为 Markdown 文本。
func formatJinaResult(result *jina.FetchResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", result.Title)
	if result.Description != "" {
		fmt.Fprintf(&sb, "> %s\n\n", result.Description)
	}
	if result.PublishedTime != "" {
		fmt.Fprintf(&sb, "**发布时间**: %s\n\n", result.PublishedTime)
	}
	sb.WriteString(result.Content)
	return sb.String()
}

// formatWebFetchResult 将 go-webfetch 结果格式化为 Markdown 文本。
func formatWebFetchResult(result *webfetch.Result) string {
	var sb strings.Builder
	if result.Preamble != "" {
		fmt.Fprintf(&sb, "%s\n\n", result.Preamble)
	}
	if result.Mode == "inline" {
		if result.Title != "" {
			fmt.Fprintf(&sb, "# %s\n\n", result.Title)
		}
		sb.WriteString(result.Markdown)
	} else {
		// 大文本已存储到文件
		if result.Title != "" {
			fmt.Fprintf(&sb, "# %s\n\n", result.Title)
		}
		fmt.Fprintf(&sb, "内容已保存到文件（共 %d 行，%d 字符）\n\n", result.TotalLines, result.TotalChars)
		fmt.Fprintf(&sb, "**文件路径**: `%s`\n\n", result.FilePath)
		if result.AgentHint != "" {
			fmt.Fprintf(&sb, "**读取提示**: %s\n", result.AgentHint)
		}
	}
	return sb.String()
}

// postSearchFilter 对单引擎搜索结果应用 smartsearch 配置的 score 过滤和 maxsize 截断。
// HybridSearchImpl 已在 SearchRaw 内处理，此函数仅用于单引擎模式。
func postSearchFilter(results []search.SearchResult, engineName string) []search.SearchResult {
	if len(results) == 0 {
		return results
	}
	ec := smartSearchConf.Engines[engineName]

	// score 过滤
	results = search.FilterByScore(results, ec.MinScore)

	// 单引擎 maxsize 截断：仅应用显式配置的 per-engine max_size。
	// 未配置（MaxSize<=0）时不再回落默认 4 —— defaultEngineMaxSize 只属于 hybrid
	// 编排层（pkg/search/hybrid.go）的 per-engine 缺省过滤；tool 层若也回落 4，
	// 会覆盖 apipool / baidu_ai 等未在 smartsearch.engines 配置的引擎在
	// factory.go 里 SetMaxSize 的全局截断。未配置时交给全局 max_size 或引擎自身截断。
	engineMax := ec.MaxSize
	if engineMax <= 0 {
		engineMax = 0
	}
	// 引擎不回传 score 时，取 min(engineMax, ceil(globalMax/1))
	if smartSearchConf.MaxSize > 0 {
		hasScore := false
		for _, r := range results {
			if r.Score > 0 {
				hasScore = true
				break
			}
		}
		if !hasScore {
			perEngineCap := smartSearchConf.MaxSize // 单引擎时 ceil(maxSize/1) = maxSize
			if perEngineCap < engineMax {
				engineMax = perEngineCap
			}
		}
	}
	if engineMax > 0 && len(results) > engineMax {
		results = results[:engineMax]
	}

	// 全局 maxsize 截断
	if smartSearchConf.MaxSize > 0 && len(results) > smartSearchConf.MaxSize {
		search.SortByScore(results)
		results = results[:smartSearchConf.MaxSize]
	}

	return results
}

// ── CleanFetch 安全预检 ──────────────────────────────────────────────────────

// validateURLSecurity DNS rebinding 防护：解析域名并检查所有 IP 是否为内网地址。
// 与 go-webfetch 的 BlockPrivateIP 形成双重防护（MCP 层预检 + 库层连接时检查）。
func validateURLSecurity(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL 格式错误: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("不支持的协议: %s（仅支持 http/https）", scheme)
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("URL 缺少主机名")
	}

	// 已知内网主机名直接拒绝
	if isPrivateHostFast(host) {
		return fmt.Errorf("不允许访问内网地址: %s", host)
	}

	// DNS 解析后检查 IP（防 DNS rebinding）
	ips, err := net.LookupHost(host)
	if err != nil {
		// DNS 解析失败不阻断（可能是临时 DNS 问题，由后续 fetch 报具体错误）
		log.Infof("DNS 解析失败（跳过安全检查）: %s: %v", host, err)
		return nil
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() ||
			isCloudMetadata(ip) {
			return fmt.Errorf("不允许访问内网地址: %s → %s", host, ipStr)
		}
	}
	return nil
}

// isPrivateHostFast 快速检查主机名是否为已知内网地址（无需 DNS 解析）。
func isPrivateHostFast(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0",
		"169.254.169.254", "metadata.google.internal":
		return true
	}
	// IPv6 回环
	if host == "[::1]" {
		return true
	}
	return false
}

// isCloudMetadata 检查 IP 是否为云厂商元数据地址。
func isCloudMetadata(ip net.IP) bool {
	// 169.254.169.254 (AWS/GCP/Azure/阿里云等)
	if ip.Equal(net.IPv4(169, 254, 169, 254)) {
		return true
	}
	// fd00::ec2:e4a:c2fe (AWS IPv6 元数据)
	if ip.IsLinkLocalUnicast() && ip.To4() == nil {
		return true
	}
	return false
}

// headCheck HEAD 预检：检查 Content-Length 防止下载过大文件。
func headCheck(ctx context.Context, rawURL string) error {
	maxSizeMB := cleanFetchMaxSizeMB
	if maxSizeMB <= 0 {
		maxSizeMB = 10
	}

	req, err := http.NewRequestWithContext(ctx, "HEAD", rawURL, nil)
	if err != nil {
		return nil // URL 构造失败不阻断，由后续 fetch 报错
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// HEAD 失败不阻断（某些服务器不支持 HEAD）
		return nil
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil // 状态码异常不阻断，由后续 fetch 报具体错误
	}

	if cl := resp.Header.Get("Content-Length"); cl != "" {
		size, err := strconv.ParseInt(cl, 10, 64)
		if err == nil && size > int64(maxSizeMB)*1024*1024 {
			return fmt.Errorf("文件过大（%.1fMB），超过限制（%dMB），如需抓取请调大 cleanfetch.max_fetch_size_mb",
				float64(size)/1024/1024, maxSizeMB)
		}
	}
	return nil
}
