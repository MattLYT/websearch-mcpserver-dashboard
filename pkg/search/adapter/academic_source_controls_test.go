package adapter

import (
	"context"
	"errors"
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
					if r.Header.Get("Authorization") != "Bearer test-openalex-key" {
						t.Error("exact lookup lost the OpenAlex key")
					}
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
			a.openalexOpts = antirobot.OpenAlexOpts{Enabled: true, APIKey: "test-openalex-key"}
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

func TestAcademicExactLookupPreservesFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantFailure string
		status                  int
		transportError          error
	}{
		{name: "unavailable", status: 503, wantFailure: "503"},
		{name: "rate-limit", status: 429, wantFailure: "429"},
		{name: "missing", status: 404},
		{name: "invalid-json", status: 200, body: "{", wantFailure: "decode"},
		{name: "cancelled", transportError: context.Canceled, wantFailure: context.Canceled.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldTransport := http.DefaultTransport
			http.DefaultTransport = sourceControlTransport(func(r *http.Request) (*http.Response, error) {
				status, body := 200, `{"message":{"DOI":"10.1234/test","title":["Exact paper"]}}`
				if r.URL.Host == "api.openalex.org" {
					if tc.transportError != nil {
						return nil, tc.transportError
					}
					status, body = tc.status, tc.body
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = oldTransport })
			a := newTestAdapter(&fakeEngine{name: "crossref"}, &fakeEngine{name: "openalex"})
			res, err := a.SearchAcademicRaw("10.1234/test")
			if err != nil || len(res.Results) != 1 {
				t.Fatalf("successful source lost: %+v, %v", res, err)
			}
			if tc.wantFailure == "" {
				if len(res.EngineErrors) != 0 {
					t.Fatalf("404 is not a provider failure: %v", res.EngineErrors)
				}
			} else if len(res.EngineErrors) != 1 || !strings.Contains(res.EngineErrors["openalex"], tc.wantFailure) {
				t.Fatalf("provider failure lost: %v; want %q", res.EngineErrors, tc.wantFailure)
			}
		})
	}
}

func TestAcademicExactLookupAllFailuresRetainCause(t *testing.T) {
	for _, tc := range []struct {
		name, query, source string
		status              int
		transportError      error
	}{
		{name: "doi", query: "10.1234/test", source: "openalex", status: 503},
		{name: "arxiv", query: "arXiv:1706.03762", source: "arxiv", status: 429},
		{name: "cancelled-arxiv", query: "arXiv:1706.03762", source: "arxiv", transportError: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldTransport := http.DefaultTransport
			http.DefaultTransport = sourceControlTransport(func(r *http.Request) (*http.Response, error) {
				if tc.transportError != nil {
					return nil, tc.transportError
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable")), Request: r}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = oldTransport })
			res, err := newTestAdapter(&fakeEngine{name: tc.source}).SearchAcademicRaw(tc.query)
			if err == nil || len(res.EngineErrors) != 1 || res.EngineErrors[tc.source] == "" {
				t.Fatalf("exact failure lost its cause: %+v, %v", res, err)
			}
			if tc.transportError != nil && !errors.Is(err, tc.transportError) {
				t.Fatalf("cancellation sentinel lost: %v", err)
			}
		})
	}
}
