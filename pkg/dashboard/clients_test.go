package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"websearch/pkg/telemetry"
)

func TestClientsWindowsAndCompatibility(t *testing.T) {
	h, mux := newTestHandler(t)
	if err := h.store.Record(telemetry.Event{Kind: "tool", Tool: "probe", Client: "codex-mcp-client", Project: "PZXM", Success: true}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"", "?days=1", "?days=7", "?days=30"} {
		var out struct {
			WindowDays int                          `json:"window_days"`
			Clients    []telemetry.ClientUsage      `json:"clients"`
			Usage      []telemetry.AttributionUsage `json:"usage"`
		}
		if err := json.Unmarshal(getJSON(t, mux, "/__admin/api/clients"+query), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Clients) != 1 || len(out.Usage) != 1 || out.Usage[0].Application != "codex" || out.Usage[0].Project != "PZXM" {
			t.Fatalf("response: %+v", out)
		}
		if query == "" && out.WindowDays != 7 {
			t.Fatalf("default: %d", out.WindowDays)
		}
	}
	for _, query := range []string{"?days=0", "?days=-1", "?days=2", "?days=31", "?days=all"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/__admin/api/clients"+query, nil))
		if rec.Code != 400 {
			t.Fatalf("%s: %d", query, rec.Code)
		}
	}
}

func TestExplicitClientProjects(t *testing.T) {
	h, mux := newTestHandler(t)
	h.conf.Dashboard.ClientProjects = map[string]string{
		"example-check": "Example", "header-client": "Fallback", "invalid-client": "C:/private",
	}
	for _, event := range []telemetry.Event{
		{Kind: "tool", Tool: "probe", Client: "example-check", Success: true},
		{Kind: "tool", Tool: "probe", Client: "header-client", Project: "Explicit", Success: true},
		{Kind: "tool", Tool: "probe", Client: "invalid-client", Success: true},
		{Kind: "tool", Tool: "probe", Client: "example-check-other", Success: true},
	} {
		if err := h.store.Record(event); err != nil {
			t.Fatal(err)
		}
	}
	var out struct {
		Usage []telemetry.AttributionUsage `json:"usage"`
	}
	if err := json.Unmarshal(getJSON(t, mux, "/__admin/api/clients"), &out); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"example-check": "Example", "header-client": "Explicit", "invalid-client": "", "example-check-other": ""}
	if len(out.Usage) != len(want) {
		t.Fatalf("usage: %+v", out.Usage)
	}
	for _, row := range out.Usage {
		if row.Project != want[row.Client] || row.Requests != 1 {
			t.Fatalf("mapping: %+v", row)
		}
	}
	events, err := h.store.Recent(20)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Client == "example-check" && event.Project != "" {
			t.Fatal("mapping rewrote raw history")
		}
	}
}
