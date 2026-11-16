package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"websearch/pkg/academic"
)

func TestPaperMetadataObservedTypeAndContinuation(t *testing.T) {
	text := "# Other paper\nDOI:10.1234/other\nAbstract\nReferences include 10.1234/target."
	metadata := metadataForPaper(text, "10.1234/target", "text/html; charset=utf-8")
	paper := metadata["paper"].(academic.PaperIdentity)
	if paper.BodyStatus != "mismatch" || paper.MetadataStatus != "unverified" || paper.ContentType != "html" {
		t.Fatalf("conflated identity/type: %+v", paper)
	}
	if p := metadataForPaper(text, "10.1234/target", "application/pdf")["paper"].(academic.PaperIdentity); p.ContentType != "pdf" {
		t.Fatal(p)
	}
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	notice, err := registerResultFileWithMetadata(path, time.Hour, false, metadata)
	if err != nil {
		t.Fatal(err)
	}
	limit := 4
	result, _, err := CleanFetch(context.Background(), nil, &CleanFetchParams{ResultRef: resultRefFromNotice(t, notice), Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	if result.StructuredContent != nil {
		t.Fatal("continuation must not duplicate the body in structuredContent")
	}
	var page struct {
		Content    string `json:"content"`
		NextOffset int64  `json:"next_offset"`
		Metadata   struct {
			Paper academic.PaperIdentity `json:"paper"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &page); err != nil {
		t.Fatal(err)
	}
	if page.Metadata.Paper != paper || page.Content != "# Ot" || page.NextOffset != 4 {
		t.Fatalf("continuation lost identity or changed body offsets: %+v", page)
	}
}
