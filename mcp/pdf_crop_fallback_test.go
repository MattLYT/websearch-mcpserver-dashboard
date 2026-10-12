package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"websearch/pkg/config"
	"websearch/pkg/fetch/webfetch"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPDFCropFailureFallsBackToBoundedText(t *testing.T) {
	initTestLogger()
	data := syntheticProxyPDFPages(3)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(data)
	}))
	defer proxy.Close()
	usePDFTestProxy(t, proxy.URL)
	fetcher, err := webfetch.NewFromConfig(config.CleanFetchConfig{FileOutputDir: t.TempDir()},
		config.PDFParserConfig{MinerUToken: "synthetic-test-token", MinerURemotePDF: true, MinerUPageLimit: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer fetcher.Close()
	fixture := filepath.Join(t.TempDir(), "reference.pdf")
	if err := os.WriteFile(fixture, data, 0600); err != nil {
		t.Fatal(err)
	}
	path, _ := resolvePDFPath(fixture)
	reference, err := fetcher.FetchPDFWithPages(context.Background(), path, []int{1, 2}, 2)
	if err != nil || reference.PageCount != 3 || reference.ParsedPages != 2 {
		t.Fatalf("upstream reference=%+v error=%v", reference, err)
	}
	webfetchInst = fetcher
	pdfMaxPages = 2
	t.Setenv("PATH", t.TempDir()) // Force a crop failure without contacting MinerU.
	result, _, err := PDFParserHandler(context.Background(), nil, &PDFParserParams{
		Path: "http://93.184.216.34/probe.pdf", Pages: "1,2",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	start := strings.Index(text, "# "+reference.Title+"\n\n")
	if !strings.Contains(text, "pdf-lib") || !strings.Contains(text, "MinerU") ||
		!strings.Contains(text, "文本提取") || start < 0 || text[start:] != formatWebFetchResult(reference) {
		t.Fatalf("crop fallback result=%s", text)
	}
}
