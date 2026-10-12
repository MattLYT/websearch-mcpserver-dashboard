package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"websearch/pkg/config"
	"websearch/pkg/search"
	"websearch/pkg/telemetry"
)

func TestSingleEngineSearchRetainsProviderAttribution(t *testing.T) {
	initTestLogger()
	restoreGlobals(t)
	cacheInst, summarizerInst, fallbackSearch = nil, nil, nil
	smartSearchConf = config.SmartSearchConfig{}
	store, err := telemetry.Open(filepath.Join(t.TempDir(), "telemetry.db"), 30)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	previous := telemetry.Default()
	telemetry.SetDefault(store)
	defer telemetry.SetDefault(previous)
	for _, failure := range []bool{false, true} {
		for _, labels := range []struct{ client, project string }{{"codex", "Test"}, {"dsh", "Other"}, {"", ""}} {
			searcher := &mockSearch{name: "mock", results: []search.SearchResult{{Title: "synthetic", Url: "https://example.com", Content: strings.Repeat("synthetic content ", 100)}}}
			if failure {
				searcher.err = fmt.Errorf("synthetic upstream failure")
			}
			searchapi = searcher
			requestID := telemetry.NewRequestID()
			ctx := telemetry.WithRequestID(telemetry.WithProject(telemetry.WithClient(context.Background(), labels.client), labels.project), requestID)
			fetch := 0
			_, _, err := doWebSearch(ctx, nil, "synthetic query", "", 3, &fetch)
			if (err != nil) != failure {
				t.Fatalf("unexpected search error: %v", err)
			}
			rows, err := store.RecentFiltered(telemetry.EventFilter{RequestID: requestID, Limit: 20})
			if err != nil || len(rows) != 2 {
				t.Fatalf("request events: %+v %v", rows, err)
			}
			for _, row := range rows {
				if row.Client != labels.client || row.Project != labels.project || row.Success == failure {
					t.Errorf("%s event lost per-call attribution or status: %+v", row.Kind, row)
				}
			}
		}
	}
}
