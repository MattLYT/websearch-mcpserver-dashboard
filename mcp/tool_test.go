package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"websearch/pkg/config"
	"websearch/pkg/fetch/jina"
	"websearch/pkg/search"
	"websearch/pkg/fetch/webfetch"
)

import (
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ── CleanFetch handler 回退逻辑测试 ──────────────────────────────────────────

func TestCleanFetch_EmptyURL(t *testing.T) {
	_, _, err := CleanFetch(context.Background(), nil, &CleanFetchParams{URL: ""})
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestCleanFetch_BothNil(t *testing.T) {
	oldWF := webfetchInst
	oldJina := jinaInst
	webfetchInst = nil
	jinaInst = nil
	defer func() { webfetchInst = oldWF; jinaInst = oldJina }()

	_, _, err := CleanFetch(context.Background(), nil, &CleanFetchParams{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error when both nil")
	}
	t.Logf("error: %v", err)
}

func TestCleanFetch_WebFetchSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test")
	}
	fetcher, err := webfetch.NewFromConfig(config.CleanFetchConfig{
		Enabled:        true,
		FileTTL:        1,
		MaxInlineLines: 100,
	}, config.PDFParserConfig{}, "")
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	defer fetcher.Close()

	oldWF := webfetchInst
	oldJina := jinaInst
	webfetchInst = fetcher
	jinaInst = nil
	defer func() { webfetchInst = oldWF; jinaInst = oldJina }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, _, err := CleanFetch(ctx, nil, &CleanFetchParams{URL: "https://wmyskxz.cn/weekly/177/"})
	if err != nil {
		t.Fatalf("CleanFetch failed: %v", err)
	}
	if result == nil || len(result.Content) == 0 {
		t.Fatal("expected non-empty result")
	}
	t.Logf("Result content length: %d", len(fmt.Sprintf("%v", result.Content)))
}

func TestCleanFetch_WebFetchFail_JinaNil(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test")
	}
	fetcher, err := webfetch.NewFromConfig(config.CleanFetchConfig{
		Enabled:        true,
		FileTTL:        1,
		MaxInlineLines: 100,
	}, config.PDFParserConfig{}, "")
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	defer fetcher.Close()

	oldWF := webfetchInst
	oldJina := jinaInst
	webfetchInst = fetcher
	jinaInst = nil
	defer func() { webfetchInst = oldWF; jinaInst = oldJina }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 不存在的 URL，webfetch 会失败，jina 为 nil
	_, _, err = CleanFetch(ctx, nil, &CleanFetchParams{URL: "https://this-domain-does-not-exist-12345.com"})
	if err == nil {
		t.Fatal("expected error when webfetch fails and jina is nil")
	}
	t.Logf("error: %v", err)
}

func TestCleanFetch_OnlyJina(t *testing.T) {
	// 测试 webfetchInst 为 nil，仅 jinaInst 的路径
	// 由于 jinaInst 需要真实 API key，这里只验证逻辑分支
	oldWF := webfetchInst
	oldJina := jinaInst
	webfetchInst = nil
	// jinaInst 仍为 nil（无真实 key），所以两者都 nil
	jinaInst = nil
	defer func() { webfetchInst = oldWF; jinaInst = oldJina }()

	_, _, err := CleanFetch(context.Background(), nil, &CleanFetchParams{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error when both are nil")
	}
}

// ── formatWebFetchResult 测试 ─────────────────────────────────────────────────

func TestFormatWebFetchResult_Inline(t *testing.T) {
	result := &webfetch.Result{
		Title:    "Test Title",
		Mode:     "inline",
		Markdown: "Hello **world**",
	}
	r := formatWebFetchResult(result)
	if r == "" {
		t.Fatal("expected non-empty text")
	}
}

func TestFormatWebFetchResult_SavedToFile(t *testing.T) {
	result := &webfetch.Result{
		Title:      "Big Doc",
		Mode:       "saved_to_file",
		FilePath:   "/tmp/webfetch/test.md",
		TotalLines: 500,
		TotalChars: 50000,
		AgentHint:  "Use read_file to read",
	}
	r := formatWebFetchResult(result)
	if r == "" {
		t.Fatal("expected non-empty text")
	}
}

// ── formatJinaResult 测试 ─────────────────────────────────────────────────────

func TestFormatJinaResult(t *testing.T) {
	result := &jina.FetchResult{
		Title:         "Jina Title",
		Description:   "A description",
		PublishedTime: "2026-01-01",
		Content:       "Content body",
	}
	r := formatJinaResult(result)
	if r == "" {
		t.Fatal("expected non-empty text")
	}
}

// ── PDFParserHandler 测试 ─────────────────────────────────────────────────────

func TestPDFParserHandler_EmptyPath(t *testing.T) {
	_, _, err := PDFParserHandler(context.Background(), nil, &PDFParserParams{Path: ""})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestPDFParserHandler_NotInitialized(t *testing.T) {
	oldWF := webfetchInst
	webfetchInst = nil
	defer func() { webfetchInst = oldWF }()

	_, _, err := PDFParserHandler(context.Background(), nil, &PDFParserParams{Path: "/tmp/test.pdf"})
	if err == nil {
		t.Fatal("expected error when webfetch not initialized")
	}
}

func TestResolvePDFPath(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		remote bool
	}{
		{"https://example.com/a.pdf", "https://example.com/a.pdf", true},
		{"HTTP://example.com/a.pdf", "HTTP://example.com/a.pdf", true},
		{"file:///C:/docs/a.pdf", "file:///C:/docs/a.pdf", false},
		{`C:\docs\a.pdf`, "file:///C:/docs/a.pdf", false},
		{"/tmp/a.pdf", "file:////tmp/a.pdf", false},
	}
	for _, tt := range tests {
		got, remote := resolvePDFPath(tt.in)
		if got != tt.want || remote != tt.remote {
			t.Errorf("resolvePDFPath(%q) = %q,%v want %q,%v", tt.in, got, remote, tt.want, tt.remote)
		}
	}
}

func TestPDFParserHandler_RejectsPrivateURL(t *testing.T) {
	oldWF := webfetchInst
	webfetchInst = &webfetch.Fetcher{}
	defer func() { webfetchInst = oldWF }()

	_, _, err := PDFParserHandler(context.Background(), nil, &PDFParserParams{Path: "https://127.0.0.1/secret.pdf"})
	if err == nil {
		t.Fatal("expected intranet URL to be rejected")
	}
	if !strings.Contains(err.Error(), "内网") && !strings.Contains(err.Error(), "不允许") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClampFetchTopN(t *testing.T) {
	if clampFetchTopN(-1) != 0 || clampFetchTopN(0) != 0 || clampFetchTopN(3) != 3 || clampFetchTopN(9) != 5 {
		t.Fatalf("clampFetchTopN unexpected")
	}
}

func TestEnrichTopN(t *testing.T) {
	in := []search.SearchResult{
		{Title: "a", Url: "https://a.example/x", Content: "snip-a"},
		{Title: "b", Url: "https://b.example/x", Content: "snip-b"},
		{Title: "c", Url: "https://c.example/x", Content: "snip-c"},
	}
	fetch := func(_ context.Context, rawURL string) (string, error) {
		if strings.Contains(rawURL, "b.example") {
			return "", fmt.Errorf("fail")
		}
		return "BODY-" + rawURL, nil
	}
	out := enrichTopN(context.Background(), in, 2, fetch)
	if out[0].Content != "BODY-https://a.example/x" {
		t.Fatalf("first content = %q", out[0].Content)
	}
	if out[1].Content != "snip-b" {
		t.Fatalf("second should keep snippet, got %q", out[1].Content)
	}
	if out[2].Content != "snip-c" {
		t.Fatalf("third should be untouched, got %q", out[2].Content)
	}
	n0 := enrichTopN(context.Background(), in, 0, fetch)
	if n0[0].Content != "snip-a" {
		t.Fatal("n=0 must not fetch")
	}
}

func TestEnrichTopN_SkipsPrivate(t *testing.T) {
	old := webfetchInst
	webfetchInst = &webfetch.Fetcher{}
	defer func() { webfetchInst = old }()

	in := []search.SearchResult{{Url: "https://127.0.0.1/x", Content: "snip"}}
	out := enrichTopN(context.Background(), in, 1, fetchPageContent)
	if out[0].Content != "snip" {
		t.Fatalf("private URL must keep snippet, got %q", out[0].Content)
	}
}

// ── postSearchFilter 测试 ────────────────────────────────────────────────────

func TestPostSearchFilter_Empty(t *testing.T) {
	old := smartSearchConf
	smartSearchConf = config.SmartSearchConfig{}
	defer func() { smartSearchConf = old }()

	out := postSearchFilter(nil, "bing")
	if len(out) != 0 {
		t.Fatalf("expected 0, got %d", len(out))
	}
}

func TestPostSearchFilter_ScoreFilter(t *testing.T) {
	old := smartSearchConf
	smartSearchConf = config.SmartSearchConfig{
		Engines: map[string]config.SmartSearchEngine{
			"tavily_api": {MinScore: 0.5},
		},
	}
	defer func() { smartSearchConf = old }()

	results := []search.SearchResult{
		{Title: "high", Score: 0.9, Engine: "tavily_api"},
		{Title: "low", Score: 0.2, Engine: "tavily_api"},
		{Title: "mid", Score: 0.6, Engine: "tavily_api"},
	}
	out := postSearchFilter(results, "tavily_api")
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
}

func TestPostSearchFilter_NoScore_IgnoresMinScore(t *testing.T) {
	old := smartSearchConf
	smartSearchConf = config.SmartSearchConfig{
		Engines: map[string]config.SmartSearchEngine{
			"bing": {MinScore: 0.9},
		},
	}
	defer func() { smartSearchConf = old }()

	results := []search.SearchResult{
		{Title: "b1", Score: 0, Engine: "bing"},
		{Title: "b2", Score: 0, Engine: "bing"},
	}
	out := postSearchFilter(results, "bing")
	if len(out) != 2 {
		t.Fatalf("no-score engine should ignore minScore, got %d", len(out))
	}
}

func TestPostSearchFilter_PerEngineMaxSize(t *testing.T) {
	old := smartSearchConf
	smartSearchConf = config.SmartSearchConfig{
		Engines: map[string]config.SmartSearchEngine{
			"bing": {MaxSize: 2},
		},
	}
	defer func() { smartSearchConf = old }()

	results := []search.SearchResult{
		{Title: "b1", Score: 0, Engine: "bing"},
		{Title: "b2", Score: 0, Engine: "bing"},
		{Title: "b3", Score: 0, Engine: "bing"},
		{Title: "b4", Score: 0, Engine: "bing"},
		{Title: "b5", Score: 0, Engine: "bing"},
	}
	out := postSearchFilter(results, "bing")
	if len(out) != 2 {
		t.Fatalf("expected 2 (per-engine max), got %d", len(out))
	}
}

func TestPostSearchFilter_GlobalMaxSize_NoScore(t *testing.T) {
	old := smartSearchConf
	smartSearchConf = config.SmartSearchConfig{
		MaxSize: 3,
		Engines: map[string]config.SmartSearchEngine{
			"bing": {MaxSize: 10},
		},
	}
	defer func() { smartSearchConf = old }()

	results := []search.SearchResult{
		{Title: "b1", Score: 0, Engine: "bing"},
		{Title: "b2", Score: 0, Engine: "bing"},
		{Title: "b3", Score: 0, Engine: "bing"},
		{Title: "b4", Score: 0, Engine: "bing"},
	}
	// no-score: perEngineCap = min(10, ceil(3/1)) = 3 → 3 results → global max 3
	out := postSearchFilter(results, "bing")
	if len(out) != 3 {
		t.Fatalf("expected 3, got %d", len(out))
	}
}

func TestPostSearchFilter_GlobalMaxSize_WithScore(t *testing.T) {
	old := smartSearchConf
	smartSearchConf = config.SmartSearchConfig{
		MaxSize: 2,
		Engines: map[string]config.SmartSearchEngine{
			"tavily_api": {MaxSize: 10},
		},
	}
	defer func() { smartSearchConf = old }()

	results := []search.SearchResult{
		{Title: "low", Score: 0.3, Engine: "tavily_api"},
		{Title: "high", Score: 0.9, Engine: "tavily_api"},
		{Title: "mid", Score: 0.6, Engine: "tavily_api"},
	}
	out := postSearchFilter(results, "tavily_api")
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
	// sorted by score: high(0.9), mid(0.6)
	if out[0].Title != "high" || out[1].Title != "mid" {
		t.Errorf("expected high,mid by score, got %s,%s", out[0].Title, out[1].Title)
	}
}

func TestPostSearchFilter_NoPerEngineMaxSize_NoGlobal(t *testing.T) {
	// 显式配置了引擎但未设 max_size（=0）：不再回落默认 4，也不截断
	old := smartSearchConf
	smartSearchConf = config.SmartSearchConfig{
		Engines: map[string]config.SmartSearchEngine{
			"bing": {}, // no MaxSize set
		},
	}
	defer func() { smartSearchConf = old }()

	results := []search.SearchResult{
		{Title: "b1", Score: 0, Engine: "bing"},
		{Title: "b2", Score: 0, Engine: "bing"},
		{Title: "b3", Score: 0, Engine: "bing"},
		{Title: "b4", Score: 0, Engine: "bing"},
		{Title: "b5", Score: 0, Engine: "bing"},
	}
	out := postSearchFilter(results, "bing")
	if len(out) != 5 {
		t.Fatalf("expected 5 (no per-engine default truncation), got %d", len(out))
	}
}

func TestPostSearchFilter_EmptyEngines_GlobalMaxSize(t *testing.T) {
	// 空 engines map：无 per-engine 配置时不回落默认 4，只应用全局 max_size
	old := smartSearchConf
	smartSearchConf = config.SmartSearchConfig{
		MaxSize: 10,
	}
	defer func() { smartSearchConf = old }()

	results := make([]search.SearchResult, 10)
	for i := range results {
		results[i] = search.SearchResult{Title: fmt.Sprintf("r%d", i), Score: 0, Engine: "bing"}
	}
	out := postSearchFilter(results, "bing")
	if len(out) != 10 {
		t.Fatalf("expected 10 (global max only), got %d", len(out))
	}
}

func TestPostSearchFilter_ApipoolNoConfig_GlobalMaxSize(t *testing.T) {
	// apipool 不在 smartsearch.engines 中：同样不应被默默截成 4
	old := smartSearchConf
	smartSearchConf = config.SmartSearchConfig{
		MaxSize: 10,
	}
	defer func() { smartSearchConf = old }()

	results := make([]search.SearchResult, 10)
	for i := range results {
		results[i] = search.SearchResult{Title: fmt.Sprintf("r%d", i), Score: 0, Engine: "apipool"}
	}
	out := postSearchFilter(results, "apipool")
	if len(out) != 10 {
		t.Fatalf("expected 10 (global max only), got %d", len(out))
	}
}

// ── parsePagesSpec 测试（T19）────────────────────────────────────────────────

func TestParsePagesSpec(t *testing.T) {
	tests := []struct {
		in   string
		want []int
	}{
		{"", nil},
		{"3", []int{3}},
		{"1-3", []int{1, 2, 3}},
		{"5,1,3-4", []int{1, 3, 4, 5}},
		{" 2 , 1 - 2 ", []int{1, 2}},
	}
	for _, tt := range tests {
		got, err := parsePagesSpec(tt.in)
		if err != nil {
			t.Errorf("parsePagesSpec(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("parsePagesSpec(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
	for _, bad := range []string{"abc", "0", "5-2", "1-", "-3", "1..3", "-1"} {
		if _, err := parsePagesSpec(bad); err == nil {
			t.Errorf("parsePagesSpec(%q) expected error", bad)
		}
	}
}

func TestPDFParserHandler_PagesOverMax(t *testing.T) {
	old := pdfMaxPages
	pdfMaxPages = 5
	defer func() { pdfMaxPages = old }()

	_, _, err := PDFParserHandler(context.Background(), nil, &PDFParserParams{Path: "x.pdf", Pages: "1-6"})
	if err == nil || !strings.Contains(err.Error(), "超过单次上限") {
		t.Fatalf("expected pages-over-max error, got %v", err)
	}
}

// ── mergeFetchURLs / 批量抓取测试（T20）──────────────────────────────────────

func TestMergeFetchURLs(t *testing.T) {
	got := mergeFetchURLs(" https://a.com/1 ", []string{"https://a.com/1", "", "https://b.com/2"})
	if fmt.Sprint(got) != "[https://a.com/1 https://b.com/2]" {
		t.Fatalf("mergeFetchURLs = %v", got)
	}
	if len(mergeFetchURLs("", nil)) != 0 {
		t.Fatal("empty inputs should merge to empty")
	}
}

func TestCleanFetch_BatchValidation(t *testing.T) {
	if _, _, err := CleanFetch(context.Background(), nil, &CleanFetchParams{}); err == nil {
		t.Fatal("expected error when url and urls both empty")
	}
	tooMany := &CleanFetchParams{URLs: []string{
		"https://a.com/1", "https://a.com/2", "https://a.com/3",
		"https://a.com/4", "https://a.com/5", "https://a.com/6",
	}}
	if _, _, err := CleanFetch(context.Background(), nil, tooMany); err == nil || !strings.Contains(err.Error(), "最多") {
		t.Fatalf("expected over-limit error, got %v", err)
	}
}

func TestCleanFetch_BatchPartialFailure(t *testing.T) {
	// 三个 URL：一个内网（SSRF 拒绝），一个非法协议，全部失败也应分节返回错误
	urls := []string{"https://127.0.0.1/secret", "ftp://example.com/x", "https://127.0.0.2/y"}
	res, _, err := CleanFetch(context.Background(), nil, &CleanFetchParams{URLs: urls})
	if err != nil {
		t.Fatalf("batch must not fail wholesale: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatal("expected sectioned output")
	}
	text := res.Content[0].(*mcp.TextContent).Text
	for _, u := range urls {
		if !strings.Contains(text, u) {
			t.Errorf("output missing section for %s", u)
		}
	}
	if !strings.Contains(text, "抓取失败") {
		t.Errorf("output should mark failed items:\n%s", text)
	}
}
