package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPDFReadBlocksWithoutParserAndSavedUnicodeTable(t *testing.T) {
	initTestLogger()
	dir := t.TempDir()
	const md = "full.md"
	const sidecar = "document_content_list.json"
	if err := os.WriteFile(filepath.Join(dir, md), []byte("Original markdown"), 0600); err != nil {
		t.Fatal(err)
	}
	table := "<table><tr><td>" + strings.Repeat("速度🙂 2 m/s ", 1100) + "</td></tr></table>"
	content, _ := json.Marshal([]map[string]any{{"type": "table", "page_idx": 0, "table_body": table, "table_caption": []string{"Speed"}}})
	if err := os.WriteFile(filepath.Join(dir, sidecar), content, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"format": "websearch-mineru-v1", "source": filepath.Join(dir, "missing.pdf"), "total_pages": 10, "requested_pages": []int{8}, "batches": []map[string]any{{"pages": []int{8}, "markdown": md, "files": []string{md, sidecar}}}}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "work.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	previous := webfetchInst
	webfetchInst = nil
	t.Cleanup(func() { webfetchInst = previous })
	result, _, err := PDFParserHandler(context.Background(), nil, &PDFParserParams{Path: dir, Pages: "8", ReadBlocks: true})
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	ref := regexp.MustCompile(`result:[0-9a-f]{32}`).FindString(text)
	if ref == "" {
		t.Fatal("long blocks missing saved reference")
	}
	var full strings.Builder
	for offset := int64(0); ; {
		_, out, err := readSavedResult(context.Background(), nil, &ReadResultParams{Ref: ref, Offset: offset, Limit: 997})
		if err != nil {
			t.Fatal(err)
		}
		full.WriteString(out.Content)
		if out.EOF {
			break
		}
		offset = out.NextOffset
	}
	var decoded struct {
		Batches []struct {
			Blocks []struct {
				Page    int `json:"original_page_1based"`
				Content struct {
					Table string `json:"table_body"`
				} `json:"content"`
			} `json:"blocks"`
		} `json:"batches"`
	}
	if err := json.Unmarshal([]byte(full.String()), &decoded); err != nil {
		t.Fatal(err)
	}
	block := decoded.Batches[0].Blocks[0]
	if block.Page != 8 || block.Content.Table != table {
		t.Fatal("table or original page lost across Unicode reads")
	}
}
