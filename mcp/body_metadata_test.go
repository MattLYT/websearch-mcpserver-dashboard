package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
	"websearch/pkg/config"
	fetchcontent "websearch/pkg/fetch"
)

func TestRemoteBodyQualityFallsBack(t *testing.T) {
	for _, shell := range []string{"", "# Project", "# Project\n[Skip to content](#main)"} {
		remote := &fakeHubFetcher{text: shell}
		text, err := fetchPageWithFallback(context.Background(), "https://example.com", config.MCPHubFetchPrefer, []mcpHubFetcher{remote}, func(context.Context, string) (string, error) { return "Open", nil })
		if err != nil || text != "Open" {
			t.Fatalf("shell %q: %q %v", shell, text, err)
		}
	}
}

func TestSavedBodyMetadataDoesNotChangeUnicodePagination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "article.md")
	want := "# 标题\n温度为 5 ℃。😀\n条件：常压。"
	if err := os.WriteFile(path, []byte(want), 0600); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"body": fetchcontent.PageBodyStatus(want, "标题"), "source_url": "https://example.com/article", "paper": map[string]any{"status": "unverified"}}
	notice, err := registerResultFileWithMetadata(path, time.Hour, false, metadata)
	if err != nil {
		t.Fatal(err)
	}
	ref := resultRefFromNotice(t, notice)
	var got string
	var offset int64
	for {
		_, page, err := readSavedResult(context.Background(), nil, &ReadResultParams{Ref: ref, Offset: offset, Limit: 3})
		if err != nil {
			t.Fatal(err)
		}
		if page.Metadata["body"].(fetchcontent.BodyStatus).Status != "available" || page.Metadata["source_url"] != metadata["source_url"] || page.Metadata["paper"] == nil {
			t.Fatalf("lost metadata: %+v", page)
		}
		got += page.Content
		offset = page.NextOffset
		if page.EOF {
			break
		}
	}
	if got != want {
		t.Fatalf("changed original: %q", got)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readSavedResult(context.Background(), nil, &ReadResultParams{Ref: ref}); err == nil {
		t.Fatal("replacement retained old capability")
	}
	oldNotice, err := registerResultFile(path, time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	_, old, err := readSavedResult(context.Background(), nil, &ReadResultParams{Ref: resultRefFromNotice(t, oldNotice)})
	if err != nil || old.Metadata["body"].(fetchcontent.BodyStatus).Status != "unknown" {
		t.Fatalf("legacy metadata: %+v %v", old, err)
	}
}

func TestFetchErrorMetadataOverMCP(t *testing.T) {
	srv := newMCPHTTPTestServer(t, config.Config{MCPStateless: true, CleanFetch: config.CleanFetchConfig{Enabled: true}})
	_, _, message := mcpHTTPPost(t, &http.Client{Timeout: 10 * time.Second}, srv.URL+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"cleanfetch","arguments":{"url":"http://127.0.0.1/private"}}}`)
	raw, _ := json.Marshal(message["result"])
	var result struct {
		IsError  bool `json:"isError"`
		Metadata struct {
			Body fetchcontent.BodyStatus `json:"body"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Metadata.Body.Status != "unavailable" {
		t.Fatalf("lost fetch error metadata: %s", raw)
	}
}

func TestSavedResultRejectsNewExpectedIdentity(t *testing.T) {
	_, _, err := cleanFetch(context.Background(), nil, &CleanFetchParams{ResultRef: "result:old", ExpectedPaper: "10.1234/changed"}, false)
	if err == nil {
		t.Fatal("continuation silently ignored a different requested identity")
	}
}
