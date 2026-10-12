package academic

import (
	"strings"
	"testing"

	"websearch/pkg/antirobot"
)

// ── Europe PMC ──────────────────────────────────────────────────────────────

func TestParseEuropePMC(t *testing.T) {
	body := `{"hitCount":2,"resultList":{"result":[
		{"id":"32153613","source":"MED","title":"CRISPR gene editing: advances and applications",
		 "authorString":"J A Doudna, E Charpentier.","doi":"10.1038/s41586-020-2649-2",
		 "journalInfo":{"journal":{"title":"Nature"}},
		 "citedByCount":150,"firstPublicationDate":"2020-07-01","isOpenAccess":"Y"},
		{"id":"PMC7348671","source":"PMC","title":"Open Access Study"}
	]}}`

	e := NewEuropePMC(antirobot.EuropePMCOpts{Enabled: true}, nil)
	resp, err := e.(*europePMCEngine).parse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Engine != "europepmc" || len(resp.Results) != 2 {
		t.Fatalf("engine=%s results=%d", resp.Engine, len(resp.Results))
	}

	r := resp.Results[0]
	if r.DOI != "10.1038/s41586-020-2649-2" || r.URL != "https://doi.org/10.1038/s41586-020-2649-2" {
		t.Errorf("DOI/URL = %s / %s", r.DOI, r.URL)
	}
	if r.Journal != "Nature" || r.CitedBy != 150 || r.PublishedAt != "2020-07-01" {
		t.Errorf("journal=%s citedBy=%d date=%s", r.Journal, r.CitedBy, r.PublishedAt)
	}
	if r.Authors != "J A Doudna, E Charpentier" {
		t.Errorf("authors = %q", r.Authors)
	}

	// 无 DOI 时回退 source/id 文章页（逐条唯一，保证去重键不撞车）
	r2 := resp.Results[1]
	if r2.URL != "https://europepmc.org/article/PMC/PMC7348671" {
		t.Errorf("无 DOI 回退 URL = %q", r2.URL)
	}
}

// ── DBLP ────────────────────────────────────────────────────────────────────

func TestParseDBLPChallengeAndMalformedResponse(t *testing.T) {
	engine := NewDBLP(antirobot.DBLPOpts{Enabled: true}, nil).(*dblpEngine)
	for _, tc := range []struct{ name, body, want string }{
		{"challenge", `<html><script id="anubis_challenge" type="application/json">{}</script></html>`, "Anubis challenge required"},
		{"ordinary HTML", `<html><title>Service unavailable</title></html>`, "dblp parse:"},
		{"malformed JSON", `{"result":`, "dblp parse:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := engine.parse([]byte(tc.body))
			if resp != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("response=%v error=%v, want %q", resp, err, tc.want)
			}
		})
	}
	resp, err := engine.parse([]byte(`{"result":{"hits":{"hit":[{"info":{"title":"anubis_challenge study","ee":"https://example.com/paper"}}]}}}`))
	if err != nil || len(resp.Results) != 1 {
		t.Fatalf("valid JSON mentioning the marker must remain usable: response=%v error=%v", resp, err)
	}
}

func TestParseDBLP_AuthorsArray(t *testing.T) {
	body := `{"result":{"hits":{"hit":[
		{"info":{"authors":{"author":[{"text":"Ashish Vaswani"},{"text":"Noam Shazeer"}]},
		 "title":"Attention is all you need.","venue":"NeurIPS","year":"2017",
		 "doi":"10.5555/3295222","ee":"https://dl.acm.org/doi/10.5555/3295222",
		 "url":"https://dblp.org/rec/conf/nips/VaswaniSPUJGKP17"}}
	]}}}`

	e := NewDBLP(antirobot.DBLPOpts{Enabled: true}, nil)
	resp, err := e.(*dblpEngine).parse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("results = %d", len(resp.Results))
	}
	r := resp.Results[0]
	// 标题尾部句号应被剥离
	if r.Title != "Attention is all you need" {
		t.Errorf("title = %q", r.Title)
	}
	if r.Authors != "Ashish Vaswani, Noam Shazeer" {
		t.Errorf("authors = %q", r.Authors)
	}
	if r.URL != "https://dl.acm.org/doi/10.5555/3295222" {
		t.Errorf("URL 应优先 ee = %q", r.URL)
	}
	if r.DOI != "10.5555/3295222" || r.Journal != "NeurIPS" || r.PublishedAt != "2017" {
		t.Errorf("doi=%s journal=%s year=%s", r.DOI, r.Journal, r.PublishedAt)
	}
}

func TestParseDBLP_SingleAuthorObjectAndMissingFields(t *testing.T) {
	body := `{"result":{"hits":{"hit":[
		{"info":{"authors":{"author":{"text":"Solo Author"}},
		 "title":"No Venue No Year No DOI","ee":"https://example.com/x"}},
		{"info":{"authors":{"author":[{"text":"A"},{"text":"B"}]},"title":"Missing EE",
		 "url":"https://dblp.org/rec/x"}}
	]}}}`

	e := NewDBLP(antirobot.DBLPOpts{Enabled: true}, nil)
	resp, err := e.(*dblpEngine).parse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d", len(resp.Results))
	}
	if resp.Results[0].Authors != "Solo Author" {
		t.Errorf("单作者应为对象形态: %q", resp.Results[0].Authors)
	}
	if resp.Results[0].DOI != "" || resp.Results[0].Journal != "" || resp.Results[0].PublishedAt != "" {
		t.Errorf("缺字段应为空: %+v", resp.Results[0])
	}
	// ee 缺失时回退 url
	if resp.Results[1].URL != "https://dblp.org/rec/x" {
		t.Errorf("URL 应回退 url = %q", resp.Results[1].URL)
	}
}

// ── DOAJ ────────────────────────────────────────────────────────────────────

func TestParseDOAJ(t *testing.T) {
	body := `{"total":1,"results":[
		{"bibjson":{"title":"An Open Access Study","abstract":"Full abstract text.",
		 "author":[{"name":"A Author"},{"name":"B Author"}],
		 "identifier":[{"type":"doi","id":"10.1234/doaj.001"},{"type":"issn","id":"1234-5678"}],
		 "link":[{"type":"fulltext","url":"https://journal.example.com/article1"}],
		 "journal":{"title":"Journal of Open Science"},"year":"2021"}},
		{"bibjson":{"title":"No URL Fallback to DOI",
		 "identifier":[{"type":"doi","id":"10.1234/doaj.002"}]}},
		{"bibjson":{"title":"No URL No DOI Skipped"}}
	]}`

	e := NewDOAJ(antirobot.DOAJOpts{Enabled: true}, nil)
	resp, err := e.(*doajEngine).parse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// 第三条无全文链接且无 DOI → 跳过
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(resp.Results))
	}
	r := resp.Results[0]
	if r.URL != "https://journal.example.com/article1" {
		t.Errorf("URL 应优先全文链接 = %q", r.URL)
	}
	if r.DOI != "10.1234/doaj.001" || r.Journal != "Journal of Open Science" || r.PublishedAt != "2021" {
		t.Errorf("doi=%s journal=%s year=%s", r.DOI, r.Journal, r.PublishedAt)
	}
	if r.Authors != "A Author, B Author" {
		t.Errorf("authors = %q", r.Authors)
	}
	if r.Content != "Full abstract text." {
		t.Errorf("content = %q", r.Content)
	}
	// 无全文链接时回退 DOI 落地页
	if resp.Results[1].URL != "https://doi.org/10.1234/doaj.002" {
		t.Errorf("URL 应回退 DOI 落地页 = %q", resp.Results[1].URL)
	}
}

// ── 网络冒烟（-short 跳过；每引擎单次请求，限流友好） ─────────────────────────

func TestEuropePMCSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test")
	}
	resp, err := NewEuropePMC(antirobot.EuropePMCOpts{Enabled: true}, nil).Search("CRISPR gene editing", 1, antirobot.TimeRangeNone)
	if err != nil {
		t.Skipf("Europe PMC 不可达: %v", err)
	}
	if resp.Engine != "europepmc" || len(resp.Results) == 0 {
		t.Fatalf("engine=%s results=%d", resp.Engine, len(resp.Results))
	}
	r := resp.Results[0]
	t.Logf("Title: %s | DOI: %s | CitedBy: %d | Date: %s", r.Title, r.DOI, r.CitedBy, r.PublishedAt)
	if r.Type != antirobot.ResultPaper || r.URL == "" {
		t.Errorf("结果不完整: %+v", r)
	}
}

func TestDBLPSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test")
	}
	resp, err := NewDBLP(antirobot.DBLPOpts{Enabled: true}, nil).Search("graph neural network", 1, antirobot.TimeRangeNone)
	if err != nil {
		t.Skipf("DBLP 不可达: %v", err)
	}
	if resp.Engine != "dblp" || len(resp.Results) == 0 {
		t.Fatalf("engine=%s results=%d", resp.Engine, len(resp.Results))
	}
	r := resp.Results[0]
	t.Logf("Title: %s | Venue: %s | Year: %s | Authors: %s", r.Title, r.Journal, r.PublishedAt, r.Authors)
	if r.Type != antirobot.ResultPaper || r.URL == "" {
		t.Errorf("结果不完整: %+v", r)
	}
}

func TestDOAJSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test")
	}
	resp, err := NewDOAJ(antirobot.DOAJOpts{Enabled: true}, nil).Search("machine learning", 1, antirobot.TimeRangeNone)
	if err != nil {
		t.Skipf("DOAJ 不可达: %v", err)
	}
	if resp.Engine != "doaj" || len(resp.Results) == 0 {
		t.Fatalf("engine=%s results=%d", resp.Engine, len(resp.Results))
	}
	r := resp.Results[0]
	t.Logf("Title: %s | Journal: %s | Year: %s", r.Title, r.Journal, r.PublishedAt)
	if r.Type != antirobot.ResultPaper || r.URL == "" {
		t.Errorf("结果不完整: %+v", r)
	}
}
