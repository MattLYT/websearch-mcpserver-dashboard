package dashboard

import (
	"encoding/json"
	"testing"
	"websearch/pkg/telemetry"
)

func TestCallsDefaultLimitAndExplicitCompatibility(t *testing.T) {
	h, mux := newTestHandler(t)
	for i := 0; i < 120; i++ {
		if err := h.store.Record(telemetry.Event{Kind: "tool", Tool: "probe", Success: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/__admin/api/calls", 100},
		{"/__admin/api/calls?limit=20", 20},
		{"/__admin/api/calls?limit=50", 50},
		{"/__admin/api/calls?limit=100", 100},
		{"/__admin/api/calls?limit=invalid", 50},
		{"/__admin/api/events", 50},
	} {
		var rows []telemetry.StoredEvent
		if err := json.Unmarshal(getJSON(t, mux, tc.path), &rows); err != nil || len(rows) != tc.want {
			t.Fatalf("%s: %d rows, want %d, err=%v", tc.path, len(rows), tc.want, err)
		}
	}
}
