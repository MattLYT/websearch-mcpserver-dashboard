package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNotifyProgressRequiresRequestedToken(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta mcp.Meta
		want any
	}{
		{"missing", nil, nil},
		{"null", mcp.Meta{"progressToken": nil}, nil},
		{"string", mcp.Meta{"progressToken": "pdf-batch"}, "pdf-batch"},
		{"zero", mcp.Meta{"progressToken": 0}, float64(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server := mcp.NewServer(&mcp.Implementation{Name: "progress-test", Version: "1"}, nil)
			mcp.AddTool(server, &mcp.Tool{Name: "probe"}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
				notifyProgress(ctx, req, 1, 2, "batch")
				return &mcp.CallToolResult{}, nil, nil
			})
			received := make(chan *mcp.ProgressNotificationParams, 1)
			client := mcp.NewClient(&mcp.Implementation{Name: "progress-client", Version: "1"}, &mcp.ClientOptions{
				ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
					received <- req.Params
				},
			})
			t1, t2 := mcp.NewInMemoryTransports()
			ss, err := server.Connect(ctx, t1, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ss.Close()
			cs, err := client.Connect(ctx, t2, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "probe", Meta: tc.meta}); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-received:
				if tc.want == nil {
					t.Fatal("unsolicited progress notification")
				}
				if got.ProgressToken != tc.want || got.Progress != 1 || got.Total != 2 || got.Message != "batch" {
					t.Fatalf("progress = %+v, want token %v", got, tc.want)
				}
			case <-time.After(100 * time.Millisecond):
				if tc.want != nil {
					t.Fatal("requested progress not received")
				}
			}
		})
	}
}
