package provider

import (
	"websearch/pkg/search/core"
	"fmt"
	"strings"
	"time"

	"websearch/pkg/client"
)

const exaAPIEndpoint = "https://api.exa.ai/search"

// ExaSearchImpl 实现 core.SearchInf 接口，通过 Exa Web Search API 搜索。
type ExaSearchImpl struct {
	name           string
	keys           *KeyPool
	numResults     int
	lookbackDays   int // 搜索时间范围（天），默认 90
	excludeDomains []string
}

type exaSearchReq struct {
	Query              string      `json:"query"`
	NumResults         int         `json:"numResults,omitempty"`
	StartPublishedDate string      `json:"startPublishedDate,omitempty"`
	EndPublishedDate   string      `json:"endPublishedDate,omitempty"`
	ExcludeDomains     []string    `json:"excludeDomains,omitempty"`
	Type               string      `json:"type,omitempty"`
	Contents           exaContents `json:"contents"`
}

type exaContents struct {
	Highlights bool `json:"highlights"`
}

type exaResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	PublishedDate string   `json:"publishedDate,omitempty"`
	Highlights    []string `json:"highlights,omitempty"`
}

type exaSearchResp struct {
	Results []exaResult `json:"results"`
}

// NewExaSearch 创建 Exa 搜索实例，默认搜索最近 90 天。
func NewExaSearch(keys *KeyPool, excludeDomains []string) *ExaSearchImpl {
	return &ExaSearchImpl{
		name:           "exa",
		keys:           keys,
		numResults:     5,
		lookbackDays:   90,
		excludeDomains: excludeDomains,
	}
}

// NewExaSearchWithResults 创建指定配置的 Exa 搜索实例。
// lookbackDays 控制搜索时间范围（天），<=0 时使用默认 90 天。
func NewExaSearchWithResults(keys *KeyPool, numResults, lookbackDays int, excludeDomains []string) *ExaSearchImpl {
	if numResults <= 0 {
		numResults = 5
	}
	if lookbackDays <= 0 {
		lookbackDays = 90
	}
	return &ExaSearchImpl{
		name:           "exa",
		keys:           keys,
		numResults:     numResults,
		lookbackDays:   lookbackDays,
		excludeDomains: excludeDomains,
	}
}

func (e *ExaSearchImpl) Name() string { return e.name }

func (e *ExaSearchImpl) Search(query string) (string, error) {
	results, err := e.SearchRaw(query)
	if err != nil {
		return "", err
	}
	return e.MergeContent(query, results)
}

// SearchRawWithTimeRange 实现 core.SearchTimeRanger 接口，支持动态时间范围。
func (e *ExaSearchImpl) SearchRawWithTimeRange(query string, lookbackDays int) ([]core.SearchResult, error) {
	if lookbackDays <= 0 {
		return e.SearchRaw(query)
	}
	saved := e.lookbackDays
	e.lookbackDays = lookbackDays
	defer func() { e.lookbackDays = saved }()
	return e.SearchRaw(query)
}

func (e *ExaSearchImpl) SearchRaw(query string) ([]core.SearchResult, error) {
	now := time.Now().UTC()
	startDate := now.AddDate(0, 0, -e.lookbackDays)

	req := exaSearchReq{
		Query:              query,
		NumResults:         e.numResults,
		StartPublishedDate: startDate.Format(time.RFC3339),
		EndPublishedDate:   now.Format(time.RFC3339),
		ExcludeDomains:     e.excludeDomains,
		Type:               "auto",
		Contents:           exaContents{Highlights: true},
	}

	var resp exaSearchResp
	key := e.keys.Next()
	res, err := client.DefaultClient.R().
		SetHeader("x-api-key", key).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&resp).
		Post(exaAPIEndpoint)
	if err != nil {
		return nil, &KeyError{Key: key, Err: fmt.Errorf("exa 搜索 API 调用失败: %w", err)}
	}
	if res.StatusCode() != 200 {
		return nil, &KeyError{Key: key, Err: fmt.Errorf("exa 搜索 API 返回错误状态码: %d", res.StatusCode())}
	}
	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("exa 搜索 API 结果为空")
	}

	ret := make([]core.SearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		content := strings.Join(r.Highlights, "\n")
		ret = append(ret, core.SearchResult{
			Title:       r.Title,
			Url:         strings.TrimSpace(r.URL),
			Content:     content,
			PublishDate: r.PublishedDate,
			Engine:      e.name,
		})
	}
	return ret, nil
}

func (e *ExaSearchImpl) MergeContent(query string, results []core.SearchResult) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("没有搜索结果可以合并")
	}
	var buf strings.Builder
	buf.Grow(1024 * len(results))
	buf.WriteString(core.MDSearchHeader(query, len(results)))
	for i, val := range results {
		if core.ShowMeta {
			buf.WriteString(core.FormatMDScore(i+1, val.Title, val.Url, val.Engine, core.FormatScore(val.Score), val.Content))
		} else {
			buf.WriteString(core.FormatMD(i+1, val.Title, val.Url, val.Content))
		}
	}
	return buf.String(), nil
}
