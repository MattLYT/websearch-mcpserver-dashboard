package academic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"websearch/pkg/antirobot"
)

var (
	openalexAPI   = "https://api.openalex.org"
	unpaywallAPI  = "https://api.unpaywall.org/v2"
	lookupClient  = defaultHTTPClient
	unpaywallGate = make(chan struct{}, 4)
)

// LookupDOI queries only the allowed exact-lookup sources for a single DOI.
func LookupDOI(ctx context.Context, doi string, crossref antirobot.CrossrefOpts, openalex antirobot.OpenAlexOpts) ([]antirobot.Result, map[string]error) {
	doi = stringsTrimDOI(doi)
	if doi == "" {
		return nil, nil
	}
	var (
		mu       sync.Mutex
		all      []antirobot.Result
		wg       sync.WaitGroup
		failures = make(map[string]error)
	)
	if openalex.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rs, err := lookupOpenAlexDOI(ctx, doi, openalex.APIKey)
			mu.Lock()
			all = append(all, rs...)
			if err != nil {
				failures["openalex"] = err
			}
			mu.Unlock()
		}()
	}
	if crossref.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rs, err := lookupCrossrefDOI(ctx, doi)
			mu.Lock()
			all = append(all, rs...)
			if err != nil {
				failures["crossref"] = err
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return antirobot.DeduplicateResults(all), failures
}

// LookupArxivID 用 id_list 取单篇，不走关键词 Search()。
func LookupArxivID(ctx context.Context, id string) ([]antirobot.Result, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	u := fmt.Sprintf("%s?id_list=%s", arxivEndpoint, url.QueryEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("arxiv request: %w", err)
	}
	resp, err := lookupClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("arxiv request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arxiv HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("arxiv body: %w", err)
	}
	parsed, err := parseArxivFeed(body)
	if err != nil {
		return nil, fmt.Errorf("arxiv decode: %w", err)
	}
	if parsed == nil {
		return nil, fmt.Errorf("arxiv decode: empty response")
	}
	return parsed.Results, nil
}

func lookupOpenAlexDOI(ctx context.Context, doi, apiKey string) ([]antirobot.Result, error) {
	u := openalexAPI + "/works/" + url.PathEscape("https://doi.org/"+doi)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("openalex request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := lookupClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openalex request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openalex HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openalex body: %w", err)
	}
	var w openalexWork
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("openalex decode: %w", err)
	}
	r, ok := openalexWorkToResult(w)
	if !ok {
		return nil, nil
	}
	return []antirobot.Result{r}, nil
}

func lookupCrossrefDOI(ctx context.Context, doi string) ([]antirobot.Result, error) {
	u := stringsTrimRightSlash(crossrefPath) + "/" + url.PathEscape(doi)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("crossref request: %w", err)
	}
	req.Header.Set("User-Agent", "websearch/1.0 (mailto:search@example.com)")
	resp, err := lookupClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crossref request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crossref HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("crossref body: %w", err)
	}
	var wrap struct {
		Message json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("crossref decode: %w", err)
	}
	var item crossrefItem
	if err := json.Unmarshal(wrap.Message, &item); err != nil {
		return nil, fmt.Errorf("crossref decode: %w", err)
	}
	fake, err := json.Marshal(crossrefResp{Message: crossrefMessage{Items: []crossrefItem{item}}})
	if err != nil {
		return nil, fmt.Errorf("crossref decode: %w", err)
	}
	parsed, err := (&crossrefEngine{}).parse(fake)
	if err != nil {
		return nil, fmt.Errorf("crossref decode: %w", err)
	}
	if parsed == nil {
		return nil, fmt.Errorf("crossref decode: empty response")
	}
	return parsed.Results, nil
}

func stringsTrimDOI(doi string) string {
	s := strings.TrimSpace(doi)
	s = strings.TrimPrefix(s, "https://doi.org/")
	s = strings.TrimPrefix(s, "http://doi.org/")
	return strings.TrimSpace(s)
}

func stringsTrimRightSlash(s string) string {
	return strings.TrimRight(s, "/")
}
