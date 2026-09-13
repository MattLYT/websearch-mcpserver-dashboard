package provider

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newDoubaoGlobalTestServer(t *testing.T, status int, body string) (*DoubaoSearchImpl, chan doubaoGlobalSearchRequest) {
	t.Helper()
	reqCh := make(chan doubaoGlobalSearchRequest, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req doubaoGlobalSearchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		reqCh <- req
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	pool, _ := NewKeyPool([]string{"doubao-test-key"})
	engine := NewDoubaoSearch(pool, DoubaoOptions{Version: doubaoVersionGlobal})
	engine.globalEndpoint = srv.URL
	return engine, reqCh
}

func newDoubaoCustomTestServer(t *testing.T, status int, body string) (*DoubaoSearchImpl, chan doubaoCustomSearchRequest) {
	t.Helper()
	reqCh := make(chan doubaoCustomSearchRequest, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req doubaoCustomSearchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		reqCh <- req
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	pool, _ := NewKeyPool([]string{"doubao-test-key"})
	engine := NewDoubaoSearch(pool, DoubaoOptions{Version: doubaoVersionCustom})
	engine.customEndpoint = srv.URL
	return engine, reqCh
}

const doubaoGlobalOKResp = `{
  "ResponseMetadata": {"RequestId": "rid-global"},
  "Result": {
    "Documents": [
      {
        "Url": " https://www.volcengine.com/docs/87772/2548026 ",
        "Title": "Doubao Global",
        "Snippet": [
          {"Type": "text", "Text": "Global text one"},
          {"Type": "text", "Text": "Global text two"}
        ],
        "DocumentInfo": {"PublishTime": "2026-09-11T10:00:00+08:00"}
      }
    ],
    "ErrorCode": 0
  }
}`

const doubaoCustomOKResp = `{
  "ResponseMetadata": {"RequestId": "rid-custom"},
  "Result": {
    "WebResults": [
      {
        "Title": "Doubao Custom",
        "Url": "https://www.volcengine.com/docs/87772/2272953",
        "Content": "full content",
        "RankScore": 0.91
      }
    ]
  }
}`

func TestDoubao_NameMatchesEngine(t *testing.T) {
	pool, _ := NewKeyPool([]string{"k"})
	engine := NewDoubaoSearch(pool, DoubaoOptions{Version: "custom"})
	if engine.Name() != "doubao" {
		t.Fatalf("Name() = %q, want doubao", engine.Name())
	}
	if normalizeDoubaoVersion("both") != doubaoVersionGlobal {
		t.Fatalf("both must fall back to global, got %q", normalizeDoubaoVersion("both"))
	}
}

func TestDoubao_Global_HappyPath(t *testing.T) {
	engine, reqCh := newDoubaoGlobalTestServer(t, http.StatusOK, doubaoGlobalOKResp)
	results, err := engine.SearchRaw("current news")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := <-reqCh
	if req.Query != "current news" || req.SearchType != "web" {
		t.Fatalf("unexpected request: %+v", req)
	}
	if len(results) != 1 || results[0].Engine != "doubao" {
		t.Fatalf("Engine must be doubao, got %+v", results)
	}
	if results[0].Url != "https://www.volcengine.com/docs/87772/2548026" {
		t.Fatalf("url = %q", results[0].Url)
	}
}

func TestDoubao_Custom_TimeRange(t *testing.T) {
	engine, reqCh := newDoubaoCustomTestServer(t, http.StatusOK, doubaoCustomOKResp)
	results, err := engine.SearchRawWithTimeRange("current news", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := <-reqCh
	if req.TimeRange != "OneWeek" {
		t.Fatalf("TimeRange = %q, want OneWeek", req.TimeRange)
	}
	if len(results) != 1 || results[0].Engine != "doubao" || results[0].Score != 0.91 {
		t.Fatalf("unexpected result: %+v", results)
	}
}

func TestDoubao_AuthErrorIsKeyError(t *testing.T) {
	engine, _ := newDoubaoGlobalTestServer(t, http.StatusUnauthorized, `{"ResponseMetadata":{"Error":{"Code":"10403","Message":"denied"}}}`)
	_, err := engine.SearchRaw("query")
	var ke *KeyError
	if !errors.As(err, &ke) {
		t.Fatalf("want KeyError, got %v", err)
	}
}

func TestDoubao_Blacklist(t *testing.T) {
	engine, _ := newDoubaoGlobalTestServer(t, http.StatusOK, doubaoGlobalOKResp)
	engine.excludeDomains = []string{"volcengine.com"}
	results, err := engine.SearchRaw("news")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("blacklisted host should be dropped, got %+v", results)
	}
}

func TestLookbackDaysToDoubaoRange(t *testing.T) {
	tests := map[int]string{0: "", 1: "OneDay", 7: "OneWeek", 30: "OneMonth", 90: "OneYear"}
	for days, want := range tests {
		if got := lookbackDaysToDoubaoRange(days); got != want {
			t.Errorf("lookbackDaysToDoubaoRange(%d) = %q, want %q", days, got, want)
		}
	}
}

// ── 集成测试（从 gitignore 的 config.test.yaml 加载 API Key） ──

func TestDoubao_Global_SearchRaw_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试: -short 模式")
	}
	apiKey := loadDoubaoAPIKey(t)
	engine := NewDoubaoSearch(newTestKeyPool(t, apiKey), DoubaoOptions{
		Version:    doubaoVersionGlobal,
		NumResults: 5,
	})
	results, err := engine.SearchRaw("Go 泛型")
	if err != nil {
		t.Fatalf("global SearchRaw failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty global results")
	}
	for i, r := range results {
		t.Logf("[global %d] %s - %s", i+1, r.Title, r.Url)
		if r.Title == "" || r.Url == "" {
			t.Errorf("empty title/url: %+v", r)
		}
		if r.Engine != "doubao" {
			t.Errorf("engine = %q, want doubao", r.Engine)
		}
	}
}

func TestDoubao_Custom_SearchRaw_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试: -short 模式")
	}
	apiKey := loadDoubaoAPIKey(t)
	engine := NewDoubaoSearch(newTestKeyPool(t, apiKey), DoubaoOptions{
		Version:    doubaoVersionCustom,
		NumResults: 5,
	})
	results, err := engine.SearchRaw("Go 泛型")
	if err != nil {
		t.Fatalf("custom SearchRaw failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty custom results")
	}
	for i, r := range results {
		t.Logf("[custom %d] %s - %s", i+1, r.Title, r.Url)
		if r.Engine != "doubao" {
			t.Errorf("engine = %q, want doubao", r.Engine)
		}
	}
}

func TestDoubao_Custom_SearchRawWithTimeRange_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试: -short 模式")
	}
	apiKey := loadDoubaoAPIKey(t)
	engine := NewDoubaoSearch(newTestKeyPool(t, apiKey), DoubaoOptions{
		Version:    doubaoVersionCustom,
		NumResults: 5,
	})
	results, err := engine.SearchRawWithTimeRange("AI", 7)
	if err != nil {
		t.Fatalf("custom SearchRawWithTimeRange(week) failed: %v", err)
	}
	t.Logf("custom week 范围结果数: %d", len(results))
	if len(results) == 0 {
		t.Fatal("expected non-empty custom week results")
	}
}
