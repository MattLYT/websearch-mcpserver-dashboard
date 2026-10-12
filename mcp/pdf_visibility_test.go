package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"websearch/pkg/config"
	"websearch/pkg/fetch/webfetch"
)

func TestPDFLocalPathFailureExplainsServiceVisibility(t *testing.T) {
	restoreGlobals(t)
	initTestLogger()
	fetcher, err := webfetch.NewFromConfig(config.CleanFetchConfig{FileOutputDir: t.TempDir()}, config.PDFParserConfig{}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer fetcher.Close()
	webfetchInst = fetcher
	_, _, err = PDFParserHandler(context.Background(), nil, &PDFParserParams{Path: filepath.Join(t.TempDir(), "missing.pdf")})
	if err == nil || !strings.Contains(err.Error(), "服务进程可见") || !strings.Contains(err.Error(), "宿主机路径") {
		t.Fatalf("expected actionable local path error, got %v", err)
	}
}
