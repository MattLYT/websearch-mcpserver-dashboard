package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"websearch/pkg/fetch/mineru"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPDFTaskParametersRejectConflictingAndUnboundedInput(t *testing.T) {
	for _, tc := range []struct {
		params PDFParserParams
		want   string
	}{
		{PDFParserParams{TaskRef: "standard:job", Path: "https://example.com/file.pdf"}, "省略 path"},
		{PDFParserParams{TaskRef: "standard:job", Pages: "1-2"}, "省略 path"},
		{PDFParserParams{Path: "x.pdf", WaitSeconds: new(-1)}, "0-1800"},
		{PDFParserParams{TaskRef: "standard:job", WaitSeconds: new(1801)}, "0-1800"},
	} {
		_, _, err := PDFParserHandler(context.Background(), nil, &tc.params)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%+v: %v", tc.params, err)
		}
	}
}

func TestPDFTaskResultKeepsStatusAndAvailableContent(t *testing.T) {
	pending := &mineru.TaskPendingError{TaskRef: "batch:job", Status: "解析中，2/8 页"}
	for _, content := range []string{"", "# Already completed batch"} {
		var result *mcp.CallToolResult
		if content != "" {
			result = textResult(content)
		}
		got, err := pdfTaskResult(result, nil, errors.Join(pending, errors.New("fallback unavailable")))
		if content != "" {
			if got != result || got.Content[0].(*mcp.TextContent).Text != content {
				t.Fatal("discarded completed content")
			}
		} else if err != nil || !strings.Contains(got.Content[0].(*mcp.TextContent).Text, "尚无正文") ||
			!strings.Contains(got.Content[0].(*mcp.TextContent).Text, "fallback unavailable") {
			t.Fatalf("pending state=%+v error=%v", got, err)
		}
		meta := got.Meta["websearch/mineru_task"].(map[string]any)
		if meta["task_ref"] != pending.TaskRef || meta["status"] != pending.Status || meta["completed"] != false {
			t.Fatalf("metadata=%v", meta)
		}
	}
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, errors.New("upstream failure")} {
		got, err := pdfTaskResult(nil, nil, failure)
		if got != nil || !errors.Is(err, failure) {
			t.Fatalf("real failure became status: %v", err)
		}
	}
}
