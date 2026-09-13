package mcpserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"websearch/pkg/cache"
	"websearch/pkg/log"
	searchcore "websearch/pkg/search/core"
	"websearch/pkg/search"
)

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchParamsWithIntent LLM 摘要启用时使用的参数（含 intent）。

// smartsearch 工具：handler、搜索编排、缓存、fetch_top_n 与单引擎过滤。
// ── WebSearch 处理函数（两个版本适配不同 Params） ─────────────────────────────

// WebSearchWithIntent LLM 启用时的 tool handler。
func WebSearchWithIntent(ctx context.Context, req *mcp.CallToolRequest, params *SearchParamsWithIntent) (*mcp.CallToolResult, any, error) {
	return doWebSearch(ctx, req, params.Query, params.Intent, params.TimeRange, params.FetchTopN)
}

// WebSearchNoIntent LLM 未启用时的 tool handler。
func WebSearchNoIntent(ctx context.Context, req *mcp.CallToolRequest, params *SearchParamsNoIntent) (*mcp.CallToolResult, any, error) {
	return doWebSearch(ctx, req, params.Query, "", params.TimeRange, params.FetchTopN)
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

// HybridSearchImpl 已在 SearchRaw 内处理，此函数仅用于单引擎模式。
func postSearchFilter(results []search.SearchResult, engineName string) []search.SearchResult {
	if len(results) == 0 {
		return results
	}
	ec := smartSearchConf.Engines[engineName]

	// score 过滤
	results = searchcore.FilterByScore(results, ec.MinScore)

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
		searchcore.SortByScore(results)
		results = results[:smartSearchConf.MaxSize]
	}

	return results
}
