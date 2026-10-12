package academic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"websearch/pkg/antirobot"
)

func TestOpenAlexAuthentication(t *testing.T) {
	for _, key := range []string{"", "test-openalex-key"} {
		name := "anonymous"
		if key != "" {
			name = "authenticated"
		}
		t.Run(name, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if strings.HasPrefix(r.URL.Path, "/crossref/") {
					if r.Header.Get("Authorization") != "" {
						t.Error("OpenAlex credential leaked to Crossref")
					}
					fmt.Fprint(w, `{"message":{}}`)
					return
				}
				want := ""
				if key != "" {
					want = "Bearer " + key
				}
				if r.Header.Get("Authorization") != want {
					t.Error("incorrect OpenAlex authorization header")
				}
				if r.URL.Query().Has("api_key") {
					t.Error("credential must not appear in request URL")
				}
				work := `{"id":"https://openalex.org/W1","title":"Test paper","doi":"https://doi.org/10.1/test"}`
				if r.URL.Path == "/works" {
					if r.URL.Query().Get("search") != "query" {
						t.Error("search parameters changed")
					}
					fmt.Fprintf(w, `{"results":[%s]}`, work)
				} else {
					fmt.Fprint(w, work)
				}
			}))
			defer server.Close()
			oldAPI, oldCrossref, oldClient := openalexAPI, crossrefPath, lookupClient
			openalexAPI, crossrefPath, lookupClient = server.URL, server.URL+"/crossref", server.Client()
			t.Cleanup(func() { openalexAPI, crossrefPath, lookupClient = oldAPI, oldCrossref, oldClient })
			engine := NewOpenAlex(antirobot.OpenAlexOpts{Enabled: true, APIKey: key}, server.Client())
			resp, err := engine.Search("query", 1, antirobot.TimeRangeNone)
			if err != nil || len(resp.Results) != 1 {
				t.Fatalf("search failed: %v", err)
			}
			if results := LookupDOI(context.Background(), "10.1/test", antirobot.OpenAlexOpts{Enabled: true, APIKey: key}); len(results) != 1 {
				t.Fatal("exact DOI lookup failed")
			}
			if hits.Load() != 3 {
				t.Fatalf("request count=%d, want 3", hits.Load())
			}
		})
	}
}

func TestLookupDOIDisabledOpenAlex(t *testing.T) {
	var openalexHits, crossrefHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/works/") {
			openalexHits.Add(1)
		} else {
			crossrefHits.Add(1)
		}
		fmt.Fprint(w, `{"message":{}}`)
	}))
	defer server.Close()
	oldAPI, oldCrossref, oldClient := openalexAPI, crossrefPath, lookupClient
	openalexAPI, crossrefPath, lookupClient = server.URL, server.URL+"/crossref", server.Client()
	t.Cleanup(func() { openalexAPI, crossrefPath, lookupClient = oldAPI, oldCrossref, oldClient })
	LookupDOI(context.Background(), "10.1/test", antirobot.OpenAlexOpts{Enabled: false, APIKey: "test-openalex-key"})
	if openalexHits.Load() != 0 || crossrefHits.Load() != 1 {
		t.Fatalf("disabled provider called: openalex=%d crossref=%d", openalexHits.Load(), crossrefHits.Load())
	}
}

func TestOpenAlexFailureDoesNotExposeKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"message":"test-openalex-key"}`)
	}))
	defer server.Close()
	oldAPI := openalexAPI
	openalexAPI = server.URL
	t.Cleanup(func() { openalexAPI = oldAPI })
	engine := NewOpenAlex(antirobot.OpenAlexOpts{Enabled: true, APIKey: "test-openalex-key"}, server.Client())
	_, err := engine.Search("query", 1, antirobot.TimeRangeNone)
	if err == nil || !strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "test-openalex-key") {
		t.Fatalf("unexpected error: %v", err)
	}
}
