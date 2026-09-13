package provider

import (
	"testing"
)

// ── 集成测试（从 config.test.yaml 加载 API Key） ──

func TestTavilySearchImpl_SearchRaw_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试: -short 模式")
	}
	apiKey := loadTavilyAPIKey(t)

	tavily := NewTavilySearch(newTestKeyPool(t, apiKey), []string{"csdn.net"})
	results, err := tavily.SearchRaw("Go programming language")
	if err != nil {
		t.Fatalf("SearchRaw failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}
	for i, r := range results {
		t.Logf("[%d] %s (score: %.4f) - %s", i+1, r.Title, r.Score, r.Url)
		if r.Title == "" {
			t.Error("result title should not be empty")
		}
		if r.Url == "" {
			t.Error("result url should not be empty")
		}
		if r.Engine != "tavily_api" {
			t.Errorf("expected engine 'tavily_api', got %s", r.Engine)
		}
	}
}

func TestTavilySearchImpl_Search_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试: -short 模式")
	}
	apiKey := loadTavilyAPIKey(t)

	tavily := NewTavilySearch(newTestKeyPool(t, apiKey), nil)
	output, err := tavily.Search("latest AI news")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	t.Logf("Search output:\n%s", output)
}

func TestTavilySearchImpl_SearchRawWithTimeRange_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试: -short 模式")
	}
	apiKey := loadTavilyAPIKey(t)

	tavily := NewTavilySearch(newTestKeyPool(t, apiKey), nil)

	// 测试时间范围: day（短时间可能无结果，允许空）
	results, err := tavily.SearchRawWithTimeRange("technology", 1)
	if err != nil {
		t.Logf("day 范围搜索返回错误（可能无结果）: %v", err)
	} else {
		t.Logf("day 范围结果数: %d", len(results))
	}

	// 测试时间范围: week
	results, err = tavily.SearchRawWithTimeRange("technology", 7)
	if err != nil {
		t.Fatalf("SearchRawWithTimeRange(week) failed: %v", err)
	}
	t.Logf("week 范围结果数: %d", len(results))

	// 测试时间范围: month
	results, err = tavily.SearchRawWithTimeRange("technology", 30)
	if err != nil {
		t.Fatalf("SearchRawWithTimeRange(month) failed: %v", err)
	}
	t.Logf("month 范围结果数: %d", len(results))

	// 测试默认时间范围（不限）
	results, err = tavily.SearchRawWithTimeRange("technology", 0)
	if err != nil {
		t.Fatalf("SearchRawWithTimeRange(default) failed: %v", err)
	}
	t.Logf("默认范围结果数: %d", len(results))
}
