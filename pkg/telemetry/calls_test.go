package telemetry

import "testing"

func TestRecentCallsMatchesRetainedRequestEvidence(t *testing.T) {
	s := openTestStore(t)
	for _, e := range []Event{
		{Kind: "tool", Tool: "smartsearch", RequestID: "fallback", Success: true},
		{Kind: "provider", Provider: "engine-a", RequestID: "fallback", ErrorKind: "timeout"},
		{Kind: "provider", Provider: "engine-b", RequestID: "fallback", Success: true},
		{Kind: "tool", Tool: "cleanfetch", RequestID: "failed", ErrorKind: "parse"},
		{Kind: "provider", Provider: "engine-a", RequestID: "failed", Success: true},
		{Kind: "tool", Tool: "smartsearch", RequestID: "other", Success: true},
		{Kind: "provider", Provider: "engine-b", RequestID: "other", Success: true},
		{Kind: "tool", Tool: "smartsearch", Success: true},
		{Kind: "provider", Provider: "engine-a", ErrorKind: "timeout"},
	} {
		if err := s.Record(e); err != nil {
			t.Fatal(err)
		}
	}
	// More than a list-sized event sample separates the main call from its evidence.
	for i := 0; i < 120; i++ {
		if err := s.Record(Event{Kind: "provider", Provider: "noise", RequestID: "noise", Success: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name    string
		filter  CallFilter
		want    int
		request string
	}{
		{"all tool calls", CallFilter{}, 4, ""},
		{"provider", CallFilter{Provider: "engine-a"}, 2, ""},
		{"source failure with tool success", CallFilter{Tool: "smartsearch", Provider: "engine-a", Status: "success", ErrorKind: "timeout"}, 1, "fallback"},
		{"request-level combined evidence", CallFilter{Provider: "engine-b", ErrorKind: "timeout"}, 1, "fallback"},
		{"tool error", CallFilter{Provider: "engine-a", Status: "failure", ErrorKind: "parse"}, 1, "failed"},
		{"mismatched tool", CallFilter{Tool: "cleanfetch", ErrorKind: "timeout"}, 0, ""},
		{"unknown source", CallFilter{Provider: "missing"}, 0, ""},
		{"main outcome", CallFilter{Tool: "smartsearch", Status: "failure"}, 0, ""},
		{"list limit", CallFilter{Limit: 1}, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := s.RecentCalls(tc.filter)
			if err != nil || len(rows) != tc.want {
				t.Fatalf("calls: %+v, err=%v, want %d", rows, err, tc.want)
			}
			for _, row := range rows {
				if row.Kind != "tool" || tc.request != "" && row.RequestID != tc.request {
					t.Fatalf("unexpected main record: %+v", row)
				}
			}
		})
	}
	legacy, err := s.RecentCalls(CallFilter{Limit: 1})
	if err != nil || legacy[0].RequestID != "" {
		t.Fatalf("newest legacy call not retained separately: %+v %v", legacy, err)
	}
}

func TestRequestEventsCompleteAndExact(t *testing.T) {
	s := openTestStore(t)
	if err := s.Record(Event{Kind: "tool", Tool: "smartsearch", RequestID: "large", Success: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<600)
		INSERT INTO usage_events(occurred_at,day,kind,provider,success,request_id)
		SELECT 0,'1970-01-01','provider','synthetic',1,'large' FROM seq`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "large-other"} {
		if err := s.Record(Event{Kind: "provider", Provider: "unrelated", RequestID: id, Success: true}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.RequestEvents("large")
	if err != nil || len(rows) != 601 {
		t.Fatalf("complete details: %d rows, %v", len(rows), err)
	}
	for i, row := range rows {
		if row.RequestID != "large" || i > 0 && rows[i-1].ID > row.ID && rows[i-1].OccurredAt == row.OccurredAt {
			t.Fatalf("non-exact or unordered details: %+v", row)
		}
	}
	if rows, err := s.RequestEvents("missing"); err != nil || len(rows) != 0 {
		t.Fatalf("unknown request: %+v %v", rows, err)
	}
	for _, id := range []string{"", " \t "} {
		if _, err := s.RequestEvents(id); err == nil {
			t.Fatal("empty request id accepted")
		}
	}
}
