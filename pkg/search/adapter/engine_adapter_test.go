package adapter

import (
	"errors"
	"strings"
	"testing"

	"websearch/pkg/antirobot"
	"websearch/pkg/search/core"
)

func TestEngineSearchAdapterPreservesFailure(t *testing.T) {
	for _, tc := range []struct {
		reason string
		kind   core.FailureKind
	}{
		{"HTTP 202 rate-limited", core.FailureRateLimit},
		{"CAPTCHA challenge", core.FailureChallenge},
		{"request timeout", core.FailureTimeout},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			a := NewEngineSearchAdapter("duckduckgo", &fakeEngine{name: "duckduckgo", err: errors.New(tc.reason)})
			results, err := a.SearchRaw("query")
			if len(results) != 0 || err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("upstream failure lost: results=%v err=%v", results, err)
			}
			if got := core.ClassifyFailure(err); got != tc.kind {
				t.Fatalf("kind=%s, want %s", got, tc.kind)
			}
		})
	}
}

func TestEngineSearchAdapterFallbackStillSucceeds(t *testing.T) {
	a := NewEngineSearchAdapter("web",
		&fakeEngine{name: "first", err: errors.New("HTTP 429")},
		&fakeEngine{name: "second", results: []antirobot.Result{{Title: "Result", URL: "https://example.com"}}},
	)
	results, err := a.SearchRaw("query")
	if err != nil || len(results) != 1 {
		t.Fatalf("fallback failed: results=%v err=%v", results, err)
	}
}

func TestEngineSearchAdapterGenuineEmptyResult(t *testing.T) {
	a := NewEngineSearchAdapter("duckduckgo", &fakeEngine{name: "duckduckgo"})
	results, err := a.SearchRaw("query")
	if len(results) != 0 || err == nil || strings.Contains(err.Error(), "failed") {
		t.Fatalf("unexpected empty result: results=%v err=%v", results, err)
	}
}
