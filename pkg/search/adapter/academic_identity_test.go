package adapter

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"websearch/pkg/antirobot"
)

func TestExactLookupSelectsMatchingIdentity(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = sourceControlTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"message":{"DOI":"10.1234/wrong","title":["Wrong paper"]}}`
		if r.URL.Host == "api.openalex.org" {
			body = `{"title":"Right paper","doi":"https://doi.org/10.1234/right"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
	a := newTestAdapter(&fakeEngine{name: "crossref"}, &fakeEngine{name: "openalex"})
	a.openalexOpts = antirobot.OpenAlexOpts{Enabled: true}
	res, err := a.lookupPaper(context.Background(), "doi", "10.1234/right", []string{"crossref", "openalex"})
	if err != nil || len(res.Results) != 1 {
		t.Fatalf("%+v, %v", res, err)
	}
	p := res.Results[0].Paper
	if p == nil || p.MetadataStatus != "matched" || p.BodyStatus != "unverified" || res.Results[0].DOI != "10.1234/right" {
		t.Fatalf("wrong selection: %+v", res.Results[0])
	}
}

func TestOAFillOncePerDOIAndIgnoresInvalidCandidate(t *testing.T) {
	old := http.DefaultTransport
	hits := 0
	http.DefaultTransport = sourceControlTransport(func(r *http.Request) (*http.Response, error) {
		hits++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"is_oa":true,"best_oa_location":{"url_for_pdf":"https://example.org/full.pdf"}}`)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
	a := newTestAdapter(&fakeEngine{name: "crossref"})
	a.unpaywallEmail = "test@example.org"
	results := toSearchResults([]antirobot.Result{{DOI: "10.1234/test", PDFURL: ":invalid"}, {DOI: "10.1234/test"}})
	a.enrichOAPDF(context.Background(), results)
	if hits != 1 || results[0].PDFURL != "https://example.org/full.pdf" || results[0].OACompletion != "completed" || results[1].OACompletion != "shared_doi_lookup" {
		t.Fatalf("hits=%d, results=%+v", hits, results)
	}
}

func TestOAFillReportsConfigurationAndFailure(t *testing.T) {
	a := newTestAdapter(&fakeEngine{name: "crossref"})
	results := toSearchResults([]antirobot.Result{{DOI: "10.1234/test"}})
	a.enrichOAPDF(context.Background(), results)
	if results[0].OACompletion != "not_configured" {
		t.Fatal(results)
	}
	old := http.DefaultTransport
	http.DefaultTransport = sourceControlTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable")), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
	a.unpaywallEmail = "test@example.org"
	a.enrichOAPDF(context.Background(), results)
	if results[0].OACompletion != "failed" || results[0].PDFURL != "" {
		t.Fatal(results)
	}
}
