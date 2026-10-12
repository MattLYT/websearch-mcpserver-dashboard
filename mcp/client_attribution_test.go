package mcpserver

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
	"websearch/pkg/telemetry"
)

func TestHTTPProjectAttribution(t *testing.T) {
	for _, stateless := range []bool{false, true} {
		t.Run(map[bool]string{false: "stateful", true: "stateless"}[stateless], func(t *testing.T) {
			store, err := telemetry.Open(filepath.Join(t.TempDir(), "usage.db"), 30)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			server := mcp.NewServer(&mcp.Implementation{Name: "attribution-test", Version: "1"}, nil)
			server.AddReceivingMiddleware(ClientAttributionMiddleware)
			mcp.AddTool(server, &mcp.Tool{Name: "probe", Description: "synthetic attribution test"}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
				store.RecordContext(ctx, telemetry.Event{Kind: "tool", Tool: "probe", Success: true})
				store.RecordContext(ctx, telemetry.Event{Kind: "provider", Provider: "synthetic", Success: true})
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
			})
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: stateless})
			srv := httptest.NewServer(withClientContext(stateless, handler))
			defer srv.Close()
			client := &http.Client{Timeout: 5 * time.Second}
			code, headers, _ := mcpHTTPPost(t, client, srv.URL, testInitializeReq, "X-WebSearch-Project", "Handshake-only")
			if code != 200 {
				t.Fatalf("initialize: %d", code)
			}
			sid := headers.Get("Mcp-Session-Id")
			for _, project := range []string{"PZXM", "FP", "", "C:/private"} {
				code, _, msg := mcpHTTPPost(t, client, srv.URL, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"probe","arguments":{}}}`,
					"Mcp-Session-Id", sid, "User-Agent", "WorkBuddy/1", "X-WebSearch-Project", project)
				if code != 200 || msg["error"] != nil {
					t.Fatalf("call: %d %+v", code, msg)
				}
			}
			events, err := store.RecentFiltered(telemetry.EventFilter{Kind: "tool", Limit: 20})
			if err != nil || len(events) != 4 {
				t.Fatalf("events: %+v %v", events, err)
			}
			for i, want := range []string{"", "", "FP", "PZXM"} {
				if events[i].Project != want {
					t.Fatalf("project %d: %q want %q", i, events[i].Project, want)
				}
				if stateless && events[i].Client != "workbuddy" {
					t.Fatalf("stateless client: %q", events[i].Client)
				}
				if !stateless && events[i].Client != "test-client" {
					t.Fatalf("handshake client: %q", events[i].Client)
				}
			}
			providers, err := store.RecentFiltered(telemetry.EventFilter{Kind: "provider", Limit: 20})
			if err != nil || len(providers) != 4 || providers[3].Project != "PZXM" {
				t.Fatalf("provider attribution: %+v %v", providers, err)
			}
		})
	}
}
