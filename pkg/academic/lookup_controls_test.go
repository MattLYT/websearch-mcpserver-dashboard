package academic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"websearch/pkg/antirobot"
)

func TestLookupDOISourceControls(t *testing.T) {
	for _, tc := range []struct{ crossref, openalex bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		t.Run(fmt.Sprintf("crossref=%v/openalex=%v", tc.crossref, tc.openalex), func(t *testing.T) {
			var crossrefHits, openalexHits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/works/") {
					openalexHits.Add(1)
				} else {
					crossrefHits.Add(1)
				}
				fmt.Fprint(w, `{}`)
			}))
			defer server.Close()
			oldAPI, oldCrossref, oldClient := openalexAPI, crossrefPath, lookupClient
			openalexAPI, crossrefPath, lookupClient = server.URL, server.URL+"/crossref", server.Client()
			t.Cleanup(func() { openalexAPI, crossrefPath, lookupClient = oldAPI, oldCrossref, oldClient })
			LookupDOI(context.Background(), "10.1234/test", antirobot.CrossrefOpts{Enabled: tc.crossref}, antirobot.OpenAlexOpts{Enabled: tc.openalex})
			if (crossrefHits.Load() == 1) != tc.crossref || (openalexHits.Load() == 1) != tc.openalex {
				t.Fatalf("exact lookup violated controls: crossref=%d openalex=%d", crossrefHits.Load(), openalexHits.Load())
			}
		})
	}
}

func TestExactLookupHTTPAndCancellation(t *testing.T) {
	for _, source := range []struct {
		name   string
		lookup func(context.Context) ([]antirobot.Result, error)
	}{
		{"openalex", func(ctx context.Context) ([]antirobot.Result, error) {
			return lookupOpenAlexDOI(ctx, "10.1234/test", "test-openalex-key")
		}},
		{"crossref", func(ctx context.Context) ([]antirobot.Result, error) { return lookupCrossrefDOI(ctx, "10.1234/test") }},
		{"arxiv", func(ctx context.Context) ([]antirobot.Result, error) { return LookupArxivID(ctx, "1706.03762") }},
	} {
		for _, tc := range []struct {
			name, body, want string
			status           int
			cancelled        bool
		}{
			{name: "unavailable", status: 503, body: "test-openalex-key", want: "503"},
			{name: "rate-limit", status: 429, body: "test-openalex-key", want: "429"},
			{name: "not-found", status: 404},
			{name: "invalid-body", status: 200, body: "<", want: "decode"},
			{name: "cancelled", cancelled: true},
		} {
			t.Run(source.name+"/"+tc.name, func(t *testing.T) {
				var hits atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					hits.Add(1)
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				}))
				defer server.Close()
				oldOpenAlex, oldCrossref, oldArxiv, oldClient := openalexAPI, crossrefPath, arxivEndpoint, lookupClient
				openalexAPI, crossrefPath, arxivEndpoint, lookupClient = server.URL, server.URL+"/crossref", server.URL+"/arxiv", server.Client()
				t.Cleanup(func() {
					openalexAPI, crossrefPath, arxivEndpoint, lookupClient = oldOpenAlex, oldCrossref, oldArxiv, oldClient
				})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tc.cancelled {
					cancel()
				}
				results, err := source.lookup(ctx)
				if len(results) != 0 {
					t.Fatalf("failure returned results: %+v", results)
				}
				if tc.cancelled {
					if !errors.Is(err, context.Canceled) || hits.Load() != 0 {
						t.Fatalf("cancellation lost: err=%v hits=%d", err, hits.Load())
					}
				} else if tc.want == "" {
					if err != nil {
						t.Fatalf("404 should be an empty result: %v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "test-openalex-key") {
					t.Fatalf("cause lost or credential exposed: %v", err)
				}
			})
		}
	}
}
