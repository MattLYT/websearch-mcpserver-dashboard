package telemetry

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestProjectLabelsAndContext(t *testing.T) {
	for _, label := range []string{"PZXM", "声热制冷", " FP "} {
		if got := NormalizeProject(label); got != strings.TrimSpace(label) {
			t.Fatalf("label %q: %q", label, got)
		}
	}
	for _, label := range []string{"C:/Users/private", "https://secret.example", "token=secret", "bad\nname", strings.Repeat("a", 65)} {
		if got := NormalizeProject(label); got != "" {
			t.Fatalf("accepted private/invalid label %q", label)
		}
	}
	ctx := WithProject(WithClient(context.Background(), "codex"), "PZXM")
	ctx = WithRequestCollector(ctx, &RequestCollector{})
	RecordEventContext(ctx, Event{Kind: "tool"})
	if got := CollectorFromContext(ctx).EventSnapshot()[0]; got.Project != "PZXM" || got.Client != "codex" {
		t.Fatalf("collector: %+v", got)
	}
	if got := ProjectFromContext(WithProject(ctx, "")); got != "" {
		t.Fatal("project leaked after clearing")
	}
}

func TestAttributionUsage(t *testing.T) {
	s := openTestStore(t)
	for _, e := range []Event{
		{Kind: "tool", Client: "codex-mcp-client", Project: "PZXM", Tool: "smartsearch", Success: true, Duration: 10 * time.Millisecond},
		{Kind: "tool", Client: "codex", Project: "FP", Tool: "cleanfetch", Duration: 11 * time.Millisecond},
		{Kind: "tool", Client: "WorkBuddy", Tool: "smartsearch", Success: true},
		{Kind: "tool", Client: "pzxm-upgrade-check", Tool: "smartsearch", Success: true},
		{Kind: "tool", Tool: "file_search", Project: "C:/secret"},
		{Kind: "provider", Client: "codex", Project: "PZXM", Provider: "bing", Success: true},
	} {
		if err := s.Record(e); err != nil {
			t.Fatal(err)
		}
	}
	rows, days, err := s.AttributionUsageStats(30)
	if err != nil || days != 30 || len(rows) != 5 {
		t.Fatalf("usage: %+v days=%d err=%v", rows, days, err)
	}
	codex, diag, unknown := int64(0), int64(0), int64(0)
	for _, r := range rows {
		if r.Application == "codex" {
			codex += r.Requests
		}
		if r.Client == "pzxm-upgrade-check" {
			diag += r.Requests
		}
		if r.Client == "" {
			unknown += r.Requests
			if r.Project != "" {
				t.Fatal("path persisted")
			}
		}
	}
	if codex != 2 || diag != 1 || unknown != 1 {
		t.Fatalf("groups: %d/%d/%d", codex, diag, unknown)
	}
	// Move one record outside the requested window without deleting history.
	if _, err := s.db.Exec("UPDATE usage_events SET occurred_at=? WHERE client='codex'", time.Now().AddDate(0, 0, -2).Unix()); err != nil {
		t.Fatal(err)
	}
	recent, _, err := s.AttributionUsageStats(1)
	if err != nil || len(recent) != 4 {
		t.Fatalf("window: %+v %v", recent, err)
	}
	s.retentionDays = 3
	_, days, err = s.AttributionUsageStats(30)
	if err != nil || days != 3 {
		t.Fatalf("retention: %d %v", days, err)
	}
	events, err := s.RecentFiltered(EventFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var matched bool
	for _, e := range events {
		if e.Client == "codex-mcp-client" {
			matched = e.Project == "PZXM"
		}
	}
	if !matched {
		t.Fatal("event project readback failed")
	}
}

func TestApplicationAliases(t *testing.T) {
	for client, want := range map[string]string{"codex-mcp-client": "codex", "Codex": "codex", "WorkBuddy": "workbuddy", "workbuddy/1.0": "workbuddy", "cherrystudio": "cherry", "pzxm-files-check": "unknown", "restart-check": "unknown", "my-codex-project": "unknown", "": "unknown"} {
		app := Application(client)
		if app != want {
			t.Fatalf("%q: %q want %q", client, app, want)
		}
	}
}
