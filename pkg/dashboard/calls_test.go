package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"websearch/pkg/telemetry"
)

func TestCallsAndCompleteRequestEventsAPI(t *testing.T) {
	h, mux := newTestHandler(t)
	for _, event := range []telemetry.Event{
		{Kind: "tool", Tool: "smartsearch", RequestID: "complete", Success: true},
		{Kind: "provider", Provider: "failed-source", RequestID: "complete", ErrorKind: "timeout"},
		{Kind: "tool", Tool: "cleanfetch", RequestID: "different", ErrorKind: "parse"},
		{Kind: "tool", Tool: "smartsearch", Success: true},
		{Kind: "provider", Provider: "failed-source", ErrorKind: "timeout"},
	} {
		if err := h.store.Record(event); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 501; i++ {
		if err := h.store.Record(telemetry.Event{Kind: "provider", Provider: "synthetic", RequestID: "complete", Success: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"", 3},
		{"?tool=smartsearch&provider=failed-source&status=success&error_kind=timeout", 1},
		{"?provider=failed-source&status=failure", 0},
		{"?tool=cleanfetch&error_kind=parse", 1},
	} {
		var rows []telemetry.StoredEvent
		if err := json.Unmarshal(getJSON(t, mux, "/__admin/api/calls"+tc.query), &rows); err != nil || len(rows) != tc.want {
			t.Fatalf("%s: %+v %v", tc.query, rows, err)
		}
		for _, row := range rows {
			if row.Kind != "tool" {
				t.Fatalf("not a main call: %+v", row)
			}
		}
	}
	var details []telemetry.StoredEvent
	// List filters must not remove source evidence from the expanded request.
	if err := json.Unmarshal(getJSON(t, mux, "/__admin/api/request-events?request_id=complete&limit=20&status=failure&provider=failed-source"), &details); err != nil || len(details) != 503 {
		t.Fatalf("complete request details: %d, %v", len(details), err)
	}
	for _, event := range details {
		if event.RequestID != "complete" {
			t.Fatalf("unrelated request detail: %+v", event)
		}
	}
	for _, query := range []string{"", "?request_id=", "?request_id=%20%09"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/__admin/api/request-events"+query, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("empty request %q: %d", query, rec.Code)
		}
	}
	for _, path := range []string{"/__admin/api/calls", "/__admin/api/request-events?request_id=complete"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("method boundary %s: %d", path, rec.Code)
		}
	}
}
