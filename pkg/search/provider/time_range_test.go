package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"websearch/pkg/client"
	"websearch/pkg/search/core"

	"resty.dev/v3"
)

type timeRangeTransport func(*http.Request) (*http.Response, error)

func (f timeRangeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type timeRangeCase struct {
	name, field, defaultWindow, week, month string
	newEngine                               func() core.SearchInf
}

func timeRangeCases() []timeRangeCase {
	return []timeRangeCase{
		{"exa", "startPublishedDate", "90", "7", "30", func() core.SearchInf { return NewExaSearch(mustNewKeyPool("test-key"), nil) }},
		{"tavily", "time_range", "", "week", "month", func() core.SearchInf { return NewTavilySearch(mustNewKeyPool("test-key"), nil) }},
		{"baidu", "search_recency_filter", "semiyear", "week", "month", func() core.SearchInf { return NewBaiduSeach(mustNewKeyPool("test-key"), nil) }},
		{"baidu_ai", "search_recency_filter", "semiyear", "week", "month", func() core.SearchInf {
			return NewBaiduAISearch(mustNewKeyPool("test-key"), nil, "", "", false, false, "")
		}},
		{"doubao", "TimeRange", "OneYear", "OneWeek", "OneMonth", func() core.SearchInf {
			return NewDoubaoSearch(mustNewKeyPool("test-key"), DoubaoOptions{Version: "custom", TimeRange: "OneYear"})
		}},
	}
}

// The transport captures the serialized API boundary and never contacts an upstream.
func installTimeRangeTransport(t *testing.T, capture func(string, map[string]any)) {
	t.Helper()
	saved := client.DefaultClient
	client.DefaultClient = resty.New().SetTimeout(5 * time.Second).SetTransport(timeRangeTransport(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		query, _ := body["query"].(string)
		if q, ok := body["Query"].(string); ok {
			query = q
		}
		if messages, ok := body["messages"].([]any); ok {
			query, _ = messages[0].(map[string]any)["content"].(string)
		}
		capture(query, body)
		response := `{"results":[{"title":"test","url":"https://example.com","content":"test"}],"references":[{"title":"test","url":"https://example.com","content":"test"}],"Result":{"WebResults":[{"Title":"test","Url":"https://example.com","Content":"test"}]}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
	}))
	t.Cleanup(func() { client.DefaultClient = saved })
}

func (tc timeRangeCase) assertWindow(t *testing.T, body map[string]any, want string) {
	t.Helper()
	value, present := body[tc.field]
	if want == "" {
		if present {
			t.Errorf("%s must be omitted, got %v", tc.field, value)
		}
		if tc.name == "exa" {
			if _, ok := body["endPublishedDate"]; ok {
				t.Error("endPublishedDate must be omitted")
			}
		}
		return
	}
	got, _ := value.(string)
	if tc.name == "exa" {
		start, err := time.Parse(time.RFC3339, got)
		if err != nil {
			t.Fatal(err)
		}
		end, err := time.Parse(time.RFC3339, fmt.Sprint(body["endPublishedDate"]))
		if err != nil {
			t.Fatal(err)
		}
		got = fmt.Sprintf("%.0f", end.Sub(start).Hours()/24)
	}
	if got != want {
		t.Errorf("window = %q, want %q", got, want)
	}
}

func TestProviderTimeRangeZeroOmitsFilter(t *testing.T) {
	for _, tc := range timeRangeCases() {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan map[string]any, 2)
			installTimeRangeTransport(t, func(_ string, body map[string]any) { requests <- body })
			engine := tc.newEngine()
			if _, err := engine.(core.SearchTimeRanger).SearchRawWithTimeRange("unlimited", 0); err != nil {
				t.Fatal(err)
			}
			tc.assertWindow(t, <-requests, "")
			if _, err := engine.SearchRaw("default"); err != nil {
				t.Fatal(err)
			}
			tc.assertWindow(t, <-requests, tc.defaultWindow)
		})
	}
}

func TestProviderTimeRangeConcurrentDefaults(t *testing.T) {
	for _, tc := range timeRangeCases() {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan map[string]any, 4)
			releaseA, releaseB := make(chan struct{}), make(chan struct{})
			var onceA, onceB sync.Once
			unblockA := func() { onceA.Do(func() { close(releaseA) }) }
			unblockB := func() { onceB.Do(func() { close(releaseB) }) }
			installTimeRangeTransport(t, func(query string, body map[string]any) {
				requests <- body
				if query == "A" {
					<-releaseA
				}
				if query == "B" {
					<-releaseB
				}
			})
			var workers sync.WaitGroup
			t.Cleanup(func() { unblockA(); unblockB(); workers.Wait() })
			engine := tc.newEngine()
			ranged := engine.(core.SearchTimeRanger)
			doneA, doneB := make(chan error, 1), make(chan error, 1)
			workers.Add(1)
			go func() { defer workers.Done(); _, err := ranged.SearchRawWithTimeRange("A", 7); doneA <- err }()
			tc.assertWindow(t, <-requests, tc.week)
			workers.Add(1)
			go func() { defer workers.Done(); _, err := ranged.SearchRawWithTimeRange("B", 30); doneB <- err }()
			tc.assertWindow(t, <-requests, tc.month)
			if _, err := engine.SearchRaw("during"); err != nil {
				t.Fatal(err)
			}
			tc.assertWindow(t, <-requests, tc.defaultWindow)
			unblockA()
			if err := <-doneA; err != nil {
				t.Fatal(err)
			}
			unblockB()
			if err := <-doneB; err != nil {
				t.Fatal(err)
			}
			if _, err := engine.SearchRaw("after"); err != nil {
				t.Fatal(err)
			}
			tc.assertWindow(t, <-requests, tc.defaultWindow)
		})
	}
}
