package webfetch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	engine "github.com/daidaiJ/go-webfetch"
)

type qualityEngine struct {
	stubEngine
	result *engine.FetchResult
	err    error
}

func (e *qualityEngine) Fetch(context.Context, string) (*engine.FetchResult, error) {
	return e.result, e.err
}

func TestFetchPreservesNotFound(t *testing.T) {
	f := &Fetcher{engine: &qualityEngine{err: &engine.NotFoundError{StatusCode: 404}}}
	_, err := f.Fetch(context.Background(), "https://example.com/missing")
	var missing *engine.NotFoundError
	if !errors.As(err, &missing) || missing.StatusCode != 404 {
		t.Fatalf("expected typed 404, got %v", err)
	}
}

func TestFetchRequiresBodyNotLength(t *testing.T) {
	path := filepath.Join(t.TempDir(), "article.md")
	if err := os.WriteFile(path, []byte("# Article\nShort valid body"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name    string
		result  *engine.FetchResult
		wantErr bool
	}{
		{"title only", &engine.FetchResult{Title: "SPA", Mode: "inline"}, true},
		{"whitespace", &engine.FetchResult{Title: "SPA", Mode: "inline", Markdown: " \n\t"}, true},
		{"title and skip navigation", &engine.FetchResult{Title: "Project", Mode: "inline", Markdown: "# Project\n\n[Skip to content](#VPContent)"}, true},
		{"short meaningful body", &engine.FetchResult{Title: "Status", Mode: "inline", Markdown: "Open"}, false},
		{"saved body", &engine.FetchResult{Title: "Article", Mode: "saved_to_file", FilePath: path, TotalChars: 1000}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fetcher{engine: &qualityEngine{result: tt.result}}
			_, err := f.Fetch(context.Background(), "https://example.com/page")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

func TestSavedFetchRejectsEmptyShell(t *testing.T) {
	for _, body := range []string{"", "# Article\n[Skip to content](#main)"} {
		path := filepath.Join(t.TempDir(), "article.md")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		f := &Fetcher{engine: &qualityEngine{result: &engine.FetchResult{Title: "Article", Mode: "saved_to_file", FilePath: path}}}
		if _, err := f.Fetch(context.Background(), "https://example.com"); err == nil {
			t.Fatal("saved shell reported available")
		}
	}
}

type qualityMineru struct {
	spyMineru
	text string
}

func (m *qualityMineru) ParseURL(context.Context, string) (string, error) { return m.text, nil }
func TestMinerURemoteSuccessRequiresBody(t *testing.T) {
	for _, body := range []string{"", "# Paper", "Open"} {
		parser := &qualityMineru{spyMineru: spyMineru{hasToken: true}, text: body}
		f := &Fetcher{mineru: parser, mineruRemotePDF: true}
		result, err := f.Fetch(context.Background(), "https://example.com/paper.pdf")
		if body == "Open" {
			if err != nil || result.Metadata["body"] == nil {
				t.Fatalf("short body rejected: %+v %v", result, err)
			}
		} else if err == nil {
			t.Fatalf("MinerU shell %q accepted", body)
		}
	}
}
