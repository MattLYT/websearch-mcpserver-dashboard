package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"websearch/pkg/log"
	"websearch/pkg/search"
)

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchParamsWithIntent LLM 摘要启用时使用的参数（含 intent）。

// academicsearch 工具：handler、学术搜索与结果合并。
// AcademicSearchHandler 学术搜索 tool handler。
func AcademicSearchHandler(ctx context.Context, req *mcp.CallToolRequest, params *AcademicSearchParams) (*mcp.CallToolResult, any, error) {
	return doAcademicSearch(params.Query, params.Engines, params.TimeRange, params.Page)
}

// doWebSearch 通用网页搜索逻辑。
// timeRangeMonths 控制搜索时间范围（月），默认 3，0 表示不限。

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

func formatAcademicResults(query string, res search.AcademicSearchResult) (string, error) {
	if adapter, ok := academicSearcher.(*search.AcademicAdapter); ok {
		return adapter.MergeContentWithErrors(query, res.Results, res.EngineErrors)
	}
	return searchapi.MergeContent(query, res.Results)
}

func formatRawResults(query string, results []search.SearchResult) (string, error) {
	return searchapi.MergeContent(query, results)
}
