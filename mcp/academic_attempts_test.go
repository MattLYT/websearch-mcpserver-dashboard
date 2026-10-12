package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"websearch/pkg/cache"
	"websearch/pkg/search"
	"websearch/pkg/telemetry"
)

type attemptedAcademic struct {
	registered []string
	result     search.AcademicSearchResult
	err        error
	calls      int
}

func (a *attemptedAcademic) AcademicEngines() []string { return a.registered }
func (a *attemptedAcademic) SearchAcademicRaw(string, ...search.AcademicSearchOptions) (search.AcademicSearchResult, error) {
	a.calls++
	return a.result, a.err
}

func TestAcademicProviderEventsRespectActualAttempts(t *testing.T) {
	for _, tc := range []struct {
		name, query                      string
		registered, requested, attempted []string
		resultEngine                     string
		failure                          bool
		want                             map[string]bool
	}{
		{"doi exact", "10.1234/test", []string{"crossref", "arxiv"}, nil, []string{"crossref"}, "crossref", false, map[string]bool{"crossref": true}},
		{"arxiv exact", "arXiv:1706.03762", []string{"crossref", "arxiv"}, nil, []string{"arxiv"}, "arxiv", false, map[string]bool{"arxiv": true}},
		{"requested openalex", "10.1234/test", []string{"crossref", "openalex", "arxiv"}, []string{"openalex"}, []string{"openalex"}, "openalex", false, map[string]bool{"openalex": true}},
		{"disabled openalex", "10.1234/test", []string{"crossref", "arxiv"}, nil, []string{"crossref"}, "crossref", false, map[string]bool{"crossref": true}},
		{"exact generic failure", "10.1234/test", []string{"crossref", "arxiv"}, nil, []string{"crossref"}, "", true, map[string]bool{"crossref": false}},
		{"known empty attempts", "10.1234/test", []string{"crossref", "arxiv"}, nil, []string{}, "", true, map[string]bool{}},
		{"ordinary unknown attempts", "keyword query", []string{"crossref", "arxiv"}, nil, nil, "", false, map[string]bool{"crossref": true, "arxiv": true}},
		{"ordinary requested unknown attempts", "keyword query", []string{"crossref", "arxiv"}, []string{"arxiv"}, nil, "", false, map[string]bool{"arxiv": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initTestLogger()
			restoreGlobals(t)
			store, err := telemetry.Open(filepath.Join(t.TempDir(), "telemetry.db"), 30)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			previous := telemetry.Default()
			telemetry.SetDefault(store)
			defer telemetry.SetDefault(previous)
			searcher := &attemptedAcademic{registered: tc.registered, result: search.AcademicSearchResult{AttemptedEngines: tc.attempted}}
			if tc.failure {
				searcher.err = fmt.Errorf("学术引擎搜索无结果")
			} else {
				searcher.result.Results = []search.SearchResult{{Title: "paper", Engine: tc.resultEngine}}
				if tc.attempted != nil {
					// Unrelated merged metadata must not expand an authoritative set.
					searcher.result.Results[0].Engines = []string{tc.resultEngine, "unattempted_source"}
				}
			}
			academicSearcher, searchapi, cacheInst = searcher, fakeSearchAPI{}, nil
			_, _, err = doAcademicSearch(context.Background(), tc.query, tc.requested, "", 1)
			if (err != nil) != tc.failure {
				t.Fatalf("unexpected search error: %v", err)
			}
			rows, err := store.Recent(20)
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]bool)
			for _, row := range rows {
				if row.Kind != "provider" {
					continue
				}
				got[row.Provider] = row.Success
				if tc.failure && row.ErrorKind != telemetry.ErrorKindNoResult {
					t.Fatalf("unknown exact failure must stay generic no_result: %+v", row)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("provider attempts=%v, want %v; rows=%+v", got, tc.want, rows)
			}
		})
	}
}

func TestAcademicCacheHitDoesNotEmitNewProviderAttempts(t *testing.T) {
	initTestLogger()
	restoreGlobals(t)
	store, err := telemetry.Open(filepath.Join(t.TempDir(), "telemetry.db"), 30)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	previous := telemetry.Default()
	telemetry.SetDefault(store)
	defer telemetry.SetDefault(previous)
	c, err := cache.New(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	searcher := &attemptedAcademic{registered: []string{"crossref", "arxiv"}, result: search.AcademicSearchResult{
		AttemptedEngines: []string{"crossref"}, Results: []search.SearchResult{{Title: "paper", Engine: "crossref"}},
	}}
	academicSearcher, searchapi, cacheInst, cacheNamespace = searcher, fakeSearchAPI{}, c, "test-sources"
	for range 2 {
		if _, _, err := doAcademicSearch(context.Background(), "10.1234/test", nil, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	providers, cachedTools := 0, 0
	for _, row := range rows {
		if row.Kind == "provider" {
			providers++
		}
		if row.Kind == "tool" && row.CacheHit {
			cachedTools++
		}
	}
	if searcher.calls != 1 || providers != 1 || cachedTools != 1 {
		t.Fatalf("cache hit invented provider activity: calls=%d providers=%d cached_tools=%d rows=%+v", searcher.calls, providers, cachedTools, rows)
	}
}
