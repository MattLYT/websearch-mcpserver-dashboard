package jina

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReaderRequiresArticleAndTargetSuccess(t *testing.T) {
	for _, tt := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"article", `{"code":200,"status":20000,"data":{"title":"Article","content":"A short body"}}`, false},
		{"target missing", `{"code":200,"status":20000,"data":{"title":"Missing","warning":"Target URL returned error 404: Not Found","content":"Missing page navigation"}}`, true},
		{"title only", `{"code":200,"status":20000,"data":{"title":"SPA","content":" \n "}}`, true},
		{"title and skip navigation", `{"code":200,"status":20000,"data":{"title":"Project","content":"# Project\n\n[Skip to content](#VPContent)"}}`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("X-Respond-With"); got != "content" {
					t.Errorf("article extraction header = %q, want content", got)
				}
				if got := r.Header.Get("X-Assert-Status-Code"); got != "200" {
					t.Errorf("target status assertion = %q, want 200", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			reader := NewReader("test-key", server.URL, server.Client())
			_, err := reader.Fetch("https://example.com/page")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}
