package telemetry

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProviderNeedsThreeConsecutiveFailuresToBeDown(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "telemetry.db"), 30)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 1; i <= 3; i++ {
		if err := s.Record(Event{Kind: "provider", Provider: "demo", Query: "private customer 123456 test", Success: false, Duration: time.Millisecond, Error: errors.New("upstream failed")}); err != nil {
			t.Fatal(err)
		}
		o, err := s.Overview()
		if err != nil {
			t.Fatal(err)
		}
		got := o.Providers[0].Status
		if i < 3 && got == "down" {
			t.Fatalf("failure %d must not be down", i)
		}
		if i == 3 && got != "down" {
			t.Fatalf("third consecutive failure = %q, want down", got)
		}
	}
}

func TestRawQueryIsNeverStored(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "telemetry.db"), 30)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	query := "alice@example.com secret-token-abcdefgh customer 123456"
	if err := s.Record(Event{Kind: "tool", Tool: "smartsearch", Query: query, Success: true}); err != nil {
		t.Fatal(err)
	}
	events, err := s.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events", len(events))
	}
	joined := events[0].QueryKeywords + events[0].ErrorSummary + events[0].QueryTopic
	for _, secret := range []string{"alice@example.com", "secret-token-abcdefgh", "123456"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("stored sensitive query fragment %q", secret)
		}
	}
	if events[0].QueryHash == "" {
		t.Fatal("expected irreversible query hash")
	}
}
