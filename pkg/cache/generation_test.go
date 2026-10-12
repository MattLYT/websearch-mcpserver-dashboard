package cache

import (
	"path/filepath"
	"testing"
)

func requestView(t *testing.T, c *Cache, namespace string) *Cache {
	t.Helper()
	view, err := c.ForRequest(namespace)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestClearRejectsInFlightStoreAndSummaryAcrossConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	old := requestView(t, c, "source-a")
	if err := old.Store("q", "intent", false, nil, "", 0); err != nil {
		t.Fatal(err)
	}
	if count, err := c.Clear(); err != nil || count != 1 {
		t.Fatalf("Clear = %d, %v", count, err)
	}
	// A second connection models a restarted process while the old request ends.
	restarted, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	current := requestView(t, restarted, "source-a")
	if err := old.Store("q", "intent", false, nil, "stale result", 0); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM search_cache`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("in-flight request reinserted deleted data: count=%d err=%v", count, err)
	}
	if err := current.Store("q", "intent", false, nil, "", 0); err != nil {
		t.Fatal(err)
	}
	if err := old.UpdateSummary("q", "intent", "stale summary"); err != nil {
		t.Fatal(err)
	}
	rec, _, err := current.Lookup("q", "intent", false)
	if err != nil || rec == nil || rec.Summary != "" || rec.Query != "q" {
		t.Fatalf("late summary changed the new generation: %+v, %v", rec, err)
	}
	if rec, _, err := old.Lookup("q", "intent", false); err != nil || rec != nil {
		t.Fatalf("old generation read new data: %+v, %v", rec, err)
	}
}

func TestRequestNamespacesIsolateLegacyAndOtherSources(t *testing.T) {
	c := newTestCache(t)
	if err := c.Store("q", "", false, nil, "legacy", 0); err != nil {
		t.Fatal(err)
	}
	a := requestView(t, c, "source-a")
	if rec, _, err := a.Lookup("q", "", false); err != nil || rec != nil {
		t.Fatalf("unscoped legacy cache was trusted: %+v, %v", rec, err)
	}
	if err := a.Store("q", "", false, nil, "source-a-result", 0); err != nil {
		t.Fatal(err)
	}
	b := requestView(t, c, "source-b")
	if rec, _, err := b.Lookup("q", "", false); err != nil || rec != nil {
		t.Fatalf("different source configuration reused data: %+v, %v", rec, err)
	}
	if rec, _, err := requestView(t, c, "source-a").Lookup("q", "", false); err != nil || rec == nil || rec.Summary != "source-a-result" {
		t.Fatalf("same configuration should reuse current-generation cache: %+v, %v", rec, err)
	}
}
