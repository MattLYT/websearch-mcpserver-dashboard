package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"websearch/pkg/cache"
	"websearch/pkg/config"
	"websearch/pkg/search"
)

type blockedWebSearch struct {
	mockSearch
	started chan struct{}
	release chan struct{}
}

func (m *blockedWebSearch) SearchRaw(query string) ([]search.SearchResult, error) {
	close(m.started)
	<-m.release
	return m.mockSearch.SearchRaw(query)
}

type blockedAcademicSearch struct {
	mockAcademic
	started chan struct{}
	release chan struct{}
}

func (m *blockedAcademicSearch) SearchAcademicRaw(query string, opts ...search.AcademicSearchOptions) (search.AcademicSearchResult, error) {
	close(m.started)
	<-m.release
	return m.mockAcademic.SearchAcademicRaw(query, opts...)
}

func TestSearchCachesCannotRepopulateAfterClear(t *testing.T) {
	for _, academic := range []bool{false, true} {
		t.Run(map[bool]string{false: "web", true: "academic"}[academic], func(t *testing.T) {
			initTestLogger()
			restoreGlobals(t)
			c, err := cache.New(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			cacheInst, cacheNamespace = c, "source-a"
			summarizerInst, fallbackSearch = nil, nil
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			searchapi = &blockedWebSearch{mockSearch: mockSearch{name: "mock", results: []search.SearchResult{{Title: "web result", Url: "https://example.com"}}}, started: started, release: release}
			academicSearcher = &blockedAcademicSearch{mockAcademic: mockAcademic{engines: []string{"arxiv"}, results: []search.SearchResult{{Title: "paper", Url: "https://example.com/paper", Type: "paper"}}}, started: started, release: release}
			done := make(chan error, 1)
			go func() {
				if academic {
					_, _, err := doAcademicSearch(context.Background(), "q", nil, "", 1)
					done <- err
				} else {
					_, _, err := doWebSearch(context.Background(), nil, "q", "", 3, intPtr(0))
					done <- err
				}
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("search never started")
			}
			if _, err := c.Clear(); err != nil {
				t.Fatal(err)
			}
			releaseOnce.Do(func() { close(release) })
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			view, err := c.ForRequest("source-a")
			if err != nil {
				t.Fatal(err)
			}
			key := "q"
			if academic {
				key = "q||"
			}
			if rec, _, err := view.Lookup(key, "", academic); err != nil || rec != nil {
				t.Fatalf("old in-flight search populated current generation: %+v, %v", rec, err)
			}
			// A new request must still cache successfully after invalidation.
			searchapi = &mockSearch{name: "mock", results: []search.SearchResult{{Title: "current web", Url: "https://example.com"}}}
			academicSearcher = &mockAcademic{engines: []string{"arxiv"}, results: []search.SearchResult{{Title: "current paper", Url: "https://example.com/paper", Type: "paper"}}}
			if academic {
				_, _, err = doAcademicSearch(context.Background(), "q", nil, "", 1)
			} else {
				_, _, err = doWebSearch(context.Background(), nil, "q", "", 3, intPtr(0))
			}
			if err != nil {
				t.Fatal(err)
			}
			if rec, _, err := view.Lookup(key, "", academic); err != nil || rec == nil {
				t.Fatalf("new-generation request did not cache: %+v, %v", rec, err)
			}
		})
	}
}

func TestSearchCacheNamespaceTracksSourcesNotDashboard(t *testing.T) {
	conf := config.Config{Mode: config.ModeHybrid}
	conf.Tavily.APIKey = "synthetic-secret-never-persist"
	base, err := searchCacheNamespace(conf)
	if err != nil || len(base) != 64 || strings.Contains(base, conf.Tavily.APIKey) {
		t.Fatalf("namespace must be a hash, not secret material: %q, %v", base, err)
	}
	changed := conf
	changed.Dashboard.AdminPassword = "different-admin-password"
	if got, err := searchCacheNamespace(changed); err != nil || got != base {
		t.Fatalf("dashboard-only changes should not partition search results: %q, %v", got, err)
	}
	for name, change := range map[string]func(*config.Config){
		"api switch":      func(c *config.Config) { off := false; c.Anysearch.Enabled = &off },
		"web engine":      func(c *config.Config) { c.So360.Enabled = true },
		"academic engine": func(c *config.Config) { c.Academic.DisableArxiv = true },
		"mode":            func(c *config.Config) { c.Mode = config.ModeEngine },
		"doubao filter":   func(c *config.Config) { c.Doubao.ICPHostOnly = true },
		"doubao version":  func(c *config.Config) { c.Doubao.Version = "custom" },
	} {
		changed := conf
		change(&changed)
		if got, err := searchCacheNamespace(changed); err != nil || got == base {
			t.Fatalf("%s change must partition the cache: %q, %v", name, got, err)
		}
	}
}
