package adapter

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"websearch/pkg/antirobot"
	"websearch/pkg/search/core"
)

type sourceControlTransport func(*http.Request) (*http.Response, error)

func (f sourceControlTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAcademicExactLookupUsesSelectedRegisteredSources(t *testing.T) {
	for _, tc := range []struct {
		name, query          string
		registered, selected []string
		wantHits             map[string]int
		wantError            bool
	}{
		{"doi-all", "10.1234/test", []string{"crossref", "openalex"}, nil, map[string]int{"crossref": 1, "openalex": 1}, false},
		{"doi-openalex-only", "10.1234/test", []string{"crossref", "openalex"}, []string{" OPENALEX "}, map[string]int{"openalex": 1}, false},
		{"doi-crossref-only", "10.1234/test", []string{"crossref", "openalex"}, []string{"crossref"}, map[string]int{"crossref": 1}, false},
		{"doi-unregistered-openalex", "10.1234/test", []string{"crossref"}, nil, map[string]int{"crossref": 1}, false},
		{"doi-crossref-and-arxiv", "10.1234/test", []string{"crossref", "arxiv"}, nil, map[string]int{"crossref": 1}, false},
		{"doi-other-source", "10.1234/test", []string{"crossref", "openalex", "pubmed"}, []string{"pubmed"}, map[string]int{}, false},
		{"arxiv-selected", "arXiv:1706.03762", []string{"arxiv", "pubmed"}, []string{"arxiv"}, map[string]int{"arxiv": 1}, false},
		{"arxiv-crossref-and-arxiv", "arXiv:1706.03762", []string{"crossref", "arxiv"}, nil, map[string]int{"arxiv": 1}, false},
		{"arxiv-other-source", "arXiv:1706.03762", []string{"arxiv", "pubmed"}, []string{"pubmed"}, map[string]int{}, false},
		{"arxiv-unregistered", "arXiv:1706.03762", []string{"pubmed"}, nil, map[string]int{}, false},
		{"doi-unavailable-selection", "10.1234/test", []string{"crossref"}, []string{"openalex"}, map[string]int{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			hits := map[string]int{}
			oldTransport := http.DefaultTransport
			http.DefaultTransport = sourceControlTransport(func(r *http.Request) (*http.Response, error) {
				var source, body string
				switch r.URL.Host {
				case "api.crossref.org":
					source, body = "crossref", `{"message":{"DOI":"10.1234/test","title":["Exact paper"]}}`
				case "api.openalex.org":
					source, body = "openalex", `{"title":"Exact paper","doi":"https://doi.org/10.1234/test"}`
				case "export.arxiv.org":
					source, body = "arxiv", `<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>https://arxiv.org/abs/1706.03762</id><title>Exact paper</title><published>2017-06-12T00:00:00Z</published><summary>Paper abstract</summary></entry></feed>`
				default:
					t.Errorf("unexpected exact lookup host: %s", r.URL.Host)
				}
				mu.Lock()
				hits[source]++
				mu.Unlock()
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = oldTransport })
			var engines []antirobot.Engine
			for _, name := range tc.registered {
				engines = append(engines, &fakeEngine{name: name, results: []antirobot.Result{paperResult(name, "https://example.org/paper", "Ordinary paper")}})
			}
			a := newTestAdapter(engines...)
			a.enhance = false
			result, err := a.SearchAcademicRaw(tc.query, core.AcademicSearchOptions{Engines: tc.selected})
			if (err != nil) != tc.wantError || !tc.wantError && len(result.Results) == 0 {
				t.Fatalf("unexpected result: %#v error=%v", result, err)
			}
			if !reflect.DeepEqual(hits, tc.wantHits) {
				t.Fatalf("exact lookup sources=%v, want %v", hits, tc.wantHits)
			}
			attempted := map[string]int{}
			for _, name := range result.AttemptedEngines {
				attempted[name]++
			}
			if len(tc.wantHits) > 0 && !reflect.DeepEqual(attempted, tc.wantHits) {
				t.Fatalf("reported attempts=%v differ from actual HTTP calls=%v", attempted, hits)
			}
			if len(tc.wantHits) == 0 && result.AttemptedEngines != nil {
				t.Fatalf("ordinary/invalid-selection path should retain unknown attempt contract: %v", result.AttemptedEngines)
			}
		})
	}
}

func TestAcademicExactLookupFailureRetainsActualSources(t *testing.T) {
	oldTransport := http.DefaultTransport
	http.DefaultTransport = sourceControlTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable")), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	a := newTestAdapter(&fakeEngine{name: "crossref"}, &fakeEngine{name: "arxiv"})
	for _, tc := range []struct{ query, attempted string }{
		{"10.1234/test", "crossref"}, {"arXiv:1706.03762", "arxiv"},
	} {
		res, err := a.SearchAcademicRaw(tc.query)
		if err == nil || !reflect.DeepEqual(res.AttemptedEngines, []string{tc.attempted}) {
			t.Fatalf("failed exact path lost its actual attempt set: %+v, %v", res, err)
		}
	}
	// A directly empty exact selection is known to have attempted no sources.
	res, err := a.lookupPaper(context.Background(), "doi", "10.1234/test", nil)
	if err == nil || res.AttemptedEngines == nil || len(res.AttemptedEngines) != 0 {
		t.Fatalf("known empty set must remain distinct from legacy unknown: %+v, %v", res, err)
	}
}
