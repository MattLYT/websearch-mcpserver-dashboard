package dashboard

import (
	"encoding/json"
	"testing"
	"websearch/pkg/telemetry"
)

func TestCallAttributionMatchesUsage(t *testing.T) {
	h, mux := newTestHandler(t)
	h.conf.Dashboard.ClientProjects = map[string]string{"codex-mcp-client": "Mapped", "workbuddy": "Fallback"}
	for _, e := range []telemetry.Event{
		{Kind: "tool", Tool: "probe", Client: "codex-mcp-client", RequestID: "codex-request", Success: true},
		{Kind: "provider", Provider: "synthetic", Client: "codex-mcp-client", RequestID: "codex-request", Success: true},
		{Kind: "tool", Tool: "probe", Client: "workbuddy", Project: "Explicit", RequestID: "workbuddy-request", Success: true},
		{Kind: "tool", Tool: "probe", Client: "websearch", RequestID: "unknown-request", Success: true},
		{Kind: "tool", Tool: "probe", Client: "dsh-mcp-client", RequestID: "dsh-request", Success: true},
		{Kind: "provider", Provider: "synthetic", Client: "dsh-mcp-client", RequestID: "dsh-request", Success: true},
		{Kind: "tool", Tool: "probe", Client: "dsh-probe", RequestID: "diagnostic-request", Success: true},
	} {
		if err := h.store.Record(e); err != nil {
			t.Fatal(err)
		}
	}
	for _, endpoint := range []string{"calls", "events", "request-events?request_id=codex-request"} {
		var rows []struct{ Client, Application, Project string }
		if err := json.Unmarshal(getJSON(t, mux, "/__admin/api/"+endpoint), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			t.Fatalf("%s: no rows", endpoint)
		}
		for _, r := range rows {
			wantApp, wantProject := "unknown", ""
			switch r.Client {
			case "codex-mcp-client":
				wantApp, wantProject = "codex", "Mapped"
			case "workbuddy":
				wantApp, wantProject = "workbuddy", "Explicit"
			case "dsh-mcp-client", "dsh-probe":
				wantApp = "dsh"
			}
			if r.Application != wantApp || r.Project != wantProject {
				t.Fatalf("%s: %+v want %s/%s", endpoint, r, wantApp, wantProject)
			}
		}
	}
	var usage struct{ Usage []telemetry.AttributionUsage }
	if err := json.Unmarshal(getJSON(t, mux, "/__admin/api/clients"), &usage); err != nil {
		t.Fatal(err)
	}
	var foundDSH bool
	for _, row := range usage.Usage {
		if row.Client == "dsh-mcp-client" {
			foundDSH = row.Application == "dsh" && row.Requests == 1
		}
	}
	if !foundDSH {
		t.Fatal("historical DSH tool call missing from application usage")
	}
	rows, err := h.store.Recent(20)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Client == "codex-mcp-client" && r.Project != "" {
			t.Fatal("display mapping changed history")
		}
	}
}
