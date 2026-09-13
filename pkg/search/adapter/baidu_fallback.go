package adapter

import (
	"websearch/pkg/search/core"
)

// ──────────────────────────────────────────────────────────────────────────────
// BaiduWithFallback SK 调用失败时自动回退到百度网页搜索
// ──────────────────────────────────────────────────────────────────────────────

// BaiduWithFallback 包装百度千帆 SK 引擎，SK 调用失败时自动回退到百度网页搜索引擎。
type BaiduWithFallback struct {
	primary  core.SearchInf // 千帆 SK 引擎
	fallback core.SearchInf // 百度网页搜索引擎
}

// NewBaiduWithFallback 创建带回退的百度搜索引擎。
func NewBaiduWithFallback(primary, fallback core.SearchInf) *BaiduWithFallback {
	return &BaiduWithFallback{primary: primary, fallback: fallback}
}

func (b *BaiduWithFallback) Name() string { return "baidu_api" }

func (b *BaiduWithFallback) Search(query string) (string, error) {
	results, err := b.SearchRaw(query)
	if err != nil {
		return "", err
	}
	return b.MergeContent(query, results)
}

func (b *BaiduWithFallback) SearchRaw(query string) ([]core.SearchResult, error) {
	results, err := b.primary.SearchRaw(query)
	if err != nil {
		// SK 调用失败，回退到百度网页搜索
		return b.fallback.SearchRaw(query)
	}
	return results, nil
}

func (b *BaiduWithFallback) MergeContent(query string, results []core.SearchResult) (string, error) {
	return b.primary.MergeContent(query, results)
}
