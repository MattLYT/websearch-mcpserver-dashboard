package mcpserver

// mock LLM 基准：量化流式摘要消费路径的热点分布。
// Drain 基准 = 通道 select/排干循环本身；Progress 基准 = 同一路径上
// 每 token 附加一次真实 MCP progress notification（经 InMemoryTransport
// 走完整 JSON-RPC 编码）。两者差值即 notifyProgress 的开销。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"websearch/pkg/llm"
	"websearch/pkg/search"
)

// newBenchLLMServer 启动 mock LLM SSE 服务，每次请求流式下发 n 个 token。
func newBenchLLMServer(n int) *httptest.Server {
	token, _ := json.Marshal("这是压测token，包含中英文mixed内容与符号~!@#用于贴近真实摘要负载。")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, canFlush := w.(http.Flusher)
		for i := 0; i < n; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s}}]}\n\n", token)
			if canFlush {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if canFlush {
			flusher.Flush()
		}
	}))
}

func benchStreamSummarize(b *testing.B, tokens int, withProgress bool, interval time.Duration) {
	ctx := context.Background()
	srv := newBenchLLMServer(tokens)
	defer srv.Close()

	old := summarizerInst
	oldInterval := summaryProgressInterval
	summarizerInst = llm.NewSummarizer(srv.URL, "bench-key", "bench-model")
	summaryProgressInterval = interval
	defer func() {
		summarizerInst = old
		summaryProgressInterval = oldInterval
	}()
	results := []search.SearchResult{
		{Title: "Bench Doc", Url: "https://example.com/doc", Content: "正文内容，用于构造提示词。"},
	}

	var req *mcp.CallToolRequest
	if withProgress {
		ct, st := mcp.NewInMemoryTransports()
		server := mcp.NewServer(&mcp.Implementation{Name: "bench-server", Version: "v0"}, nil)
		ss, err := server.Connect(ctx, st, nil)
		if err != nil {
			b.Fatal(err)
		}
		defer ss.Close()
		client := mcp.NewClient(&mcp.Implementation{Name: "bench-client", Version: "v0"}, nil)
		cs, err := client.Connect(ctx, ct, nil)
		if err != nil {
			b.Fatal(err)
		}
		defer cs.Close()
		req = &mcp.CallToolRequest{
			Session: ss,
			Params: &mcp.CallToolParamsRaw{
				Meta: mcp.Meta{"progressToken": int64(1)},
			},
		}
	} else {
		req = &mcp.CallToolRequest{}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := streamSummarize(ctx, req, "bench query", "summarize", results)
		if err != nil {
			b.Fatal(err)
		}
		if !strings.Contains(out, "压测token") {
			b.Fatalf("summary lost streamed tokens: %d chars", len(out))
		}
	}
}

func BenchmarkStreamSummarizeDrain(b *testing.B) {
	for _, n := range []int{64, 512, 4096} {
		b.Run(fmt.Sprintf("tokens_%d", n), func(b *testing.B) {
			benchStreamSummarize(b, n, false, 0)
		})
	}
}

func BenchmarkStreamSummarizeProgress(b *testing.B) {
	for _, n := range []int{64, 512, 4096} {
		b.Run(fmt.Sprintf("tokens_%d", n), func(b *testing.B) {
			benchStreamSummarize(b, n, true, 0)
		})
	}
}

// BenchmarkStreamSummarizeProgressBatched 按默认 1s 间隔攒批推送的生产行为。
func BenchmarkStreamSummarizeProgressBatched(b *testing.B) {
	for _, n := range []int{64, 512, 4096} {
		b.Run(fmt.Sprintf("tokens_%d", n), func(b *testing.B) {
			benchStreamSummarize(b, n, true, time.Second)
		})
	}
}
