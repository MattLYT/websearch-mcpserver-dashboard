package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"websearch/pkg/log"
	"websearch/pkg/search"
	"websearch/pkg/llm"
)

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchParamsWithIntent LLM 摘要启用时使用的参数（含 intent）。

// LLM 摘要：流式生成与 MCP progress 推送。
// 客户端断开（ctx 取消）时自动中止。
func streamSummarize(ctx context.Context, req *mcp.CallToolRequest, query, intent string, results []search.SearchResult) (string, error) {
	notifyProgress(ctx, req, 1, 2, fmt.Sprintf("搜索完成，共 %d 条结果，正在生成摘要...", len(results)))

	ch := make(chan string, 64)
	errCh := make(chan error, 1)
	go summarizerInst.SummarizeStream(ctx, query, intent, results, ch, errCh)

	var sb strings.Builder
	for {
		select {
		case token, ok := <-ch:
			if !ok {
				// 流结束；errCh 可能已发送结果（缓冲 1，非阻塞读取）
				select {
				case err := <-errCh:
					if err != nil {
						return "", err
					}
				default:
				}
				return llm.FormatCitation(query, sb.String(), results), nil
			}
			sb.WriteString(token)
			notifyProgress(ctx, req, 2, 2, token)
		case err := <-errCh:
			return "", err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// notifyProgress 向 MCP 客户端推送进度通知（StreamableHTTP 传输下实时到达）。
// 推送失败仅记录日志，不影响主流程。
func notifyProgress(ctx context.Context, req *mcp.CallToolRequest, progress, total float64, message string) {
	if req == nil || req.Session == nil {
		return
	}
	params := &mcp.ProgressNotificationParams{
		ProgressToken: req.Params.GetProgressToken(),
		Progress:      progress,
		Total:         total,
		Message:       message,
	}
	if err := req.Session.NotifyProgress(ctx, params); err != nil {
		log.Debugf("progress notification 推送失败: %v", err)
	}
}
