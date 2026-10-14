package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"websearch/pkg/search"
	"websearch/pkg/telemetry"
)

func TestAcademicSearchRecordsProviderFailureWhenAggregateFails(t *testing.T) {
	store, err := telemetry.Open(filepath.Join(t.TempDir(), "telemetry.db"), 30)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	previous := telemetry.Default()
	telemetry.SetDefault(store)
	t.Cleanup(func() { telemetry.SetDefault(previous) })

	oldSearcher, oldCache, oldSearchAPI := academicSearcher, cacheInst, searchapi
	searchapi = fakeSearchAPI{}
	academicSearcher = &failureAcademic{}
	cacheInst = nil
	t.Cleanup(func() {
		academicSearcher, cacheInst, searchapi = oldSearcher, oldCache, oldSearchAPI
	})

	if _, _, err := AcademicSearchHandler(context.Background(), nil, &AcademicSearchParams{Query: "thermoacoustic"}); err == nil {
		t.Fatal("expected aggregate search failure")
	}
	rows, err := store.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.Kind == "provider" && row.Provider == "dblp" && !row.Success {
			found = true
			if row.ErrorSummary == "" {
				t.Fatal("provider failure event should keep its error summary")
			}
		}
	}
	if !found {
		t.Fatalf("dblp failure was not recorded: %+v", rows)
	}
	academicSearcher = &failureAcademic{structured: true}
	if _, _, err := AcademicSearchHandler(context.Background(), nil, &AcademicSearchParams{Query: "consensus"}); err == nil || strings.Contains(err.Error(), "搜索无结果") {
		t.Fatalf("challenge must not be presented as normal empty results: %v", err)
	}
	rows, err = store.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	challengeEvents := 0
	for _, row := range rows {
		if row.ErrorKind == telemetry.ErrorKindCaptcha && !row.Success && (row.Tool == "academicsearch" || row.Provider == "dblp") {
			challengeEvents++
		}
	}
	if challengeEvents != 2 {
		t.Fatalf("expected tool and provider challenge events: %+v", rows)
	}
}

type failureAcademic struct{ structured bool }

func (f *failureAcademic) AcademicEngines() []string { return []string{"dblp"} }
func (f *failureAcademic) SearchAcademicRaw(query string, opts ...search.AcademicSearchOptions) (search.AcademicSearchResult, error) {
	if f.structured {
		return search.AcademicSearchResult{EngineErrors: map[string]string{"dblp": "dblp: Anubis challenge required"}}, fmt.Errorf("学术检索未返回可用结果，存在引擎失败（dblp: Anubis challenge required）")
	}
	return search.AcademicSearchResult{}, fmt.Errorf("学术引擎搜索无结果（dblp: dblp parse: invalid character '<'）")
}

type fakeSearchAPI struct{}

func (fakeSearchAPI) Name() string { return "fake" }
func (fakeSearchAPI) Search(string) (string, error) {
	return "", nil
}
func (fakeSearchAPI) SearchRaw(string) ([]search.SearchResult, error) {
	return nil, nil
}
func (fakeSearchAPI) MergeContent(query string, results []search.SearchResult) (string, error) {
	var sb strings.Builder
	for _, result := range results {
		sb.WriteString(result.Title)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func TestAcademicSearchRecordsProviderResultCounts(t *testing.T) {
	store, err := telemetry.Open(filepath.Join(t.TempDir(), "telemetry.db"), 30)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	previous := telemetry.Default()
	telemetry.SetDefault(store)
	t.Cleanup(func() { telemetry.SetDefault(previous) })

	oldSearcher, oldCache, oldSearchAPI := academicSearcher, cacheInst, searchapi
	searchapi = fakeSearchAPI{}
	academicSearcher = &mockAcademic{
		engines: []string{"arxiv", "crossref", "openalex"},
		results: []search.SearchResult{
			{Title: "one", Engine: "arxiv", Engines: []string{"arxiv", "crossref"}},
			{Title: "two", Engine: "openalex", Engines: []string{"openalex"}},
			{Title: "three", Engine: "arxiv", Engines: []string{"arxiv"}},
		},
	}
	cacheInst = nil
	t.Cleanup(func() {
		academicSearcher, cacheInst, searchapi = oldSearcher, oldCache, oldSearchAPI
	})

	result, _, err := AcademicSearchHandler(context.Background(), nil, &AcademicSearchParams{Query: "thermoacoustic"})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected a tool result")
	}

	rows, err := store.Recent(20)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("got %d events, want 1 tool event + 3 provider events: %+v", len(rows), rows)
	}
	counts := make(map[string]int)
	for _, row := range rows {
		switch row.Kind {
		case "tool":
			if row.Tool != "academicsearch" {
				t.Fatalf("tool event = %q, want academicsearch", row.Tool)
			}
			if row.ResultCount != 3 {
				t.Fatalf("tool result_count = %d, want 3", row.ResultCount)
			}
		case "provider":
			counts[row.Provider] = row.ResultCount
		}
	}
	want := map[string]int{"arxiv": 2, "crossref": 1, "openalex": 1}
	for name, wantCount := range want {
		if counts[name] != wantCount {
			t.Fatalf("provider %s result_count = %d, want %d (all=%v)", name, counts[name], wantCount, counts)
		}
	}
}
