package academic

import "testing"

func TestPaperIdentityBoundaries(t *testing.T) {
	for _, tc := range []struct{ kind, expected, actual, want string }{
		{"doi", "DOI:10.1234/ABC", "https://doi.org/10.1234/abc", "matched"},
		{"doi", "10.1234/a", "10.1234/b", "mismatch"},
		{"doi", "10.1234/a", "", "unverified"},
		{"doi", "10.1234/a", "1912.01214", "unverified"},
		{"arxiv", "1912.01214", "https://arxiv.org/abs/1912.01214v2", "matched"},
		{"arxiv", "1912.01214v1", "1912.01214v2", "mismatch"},
		{"arxiv", "1912.01214v1", "1912.01214", "unverified"},
		{"arxiv", "1912.01214", "1811.00383", "mismatch"},
	} {
		if got := ComparePaperID(tc.kind, tc.expected, tc.actual); got != tc.want {
			t.Errorf("%+v: %s", tc, got)
		}
	}
	for _, tc := range []struct{ body, want string }{
		{"# A paper\nDOI:10.1234/abc\nAbstract\nText", "matched"},
		{"# Other paper\nDOI:10.1234/other\nText cites 10.1234/abc", "mismatch"},
		{"# Other paper\nText cites 10.1234/abc", "unverified"},
		{"# Other paper\nReferences\nDOI:10.1234/abc", "unverified"},
		{"# Other paper\nReferences:\nDOI:10.1234/abc", "unverified"},
		{"# Other paper\n参考文献\nDOI:10.1234/abc", "unverified"},
		{"# Other paper\nAbstract\nDOI:10.1234/abc", "unverified"},
	} {
		p := InspectPaperBody(tc.body, "10.1234/abc")
		if p.BodyStatus != tc.want || p.MetadataStatus != "unverified" || p.ContentType != "unknown" {
			t.Errorf("body=%q: %+v", tc.body, p)
		}
	}
	if p := InspectPaperBody("DOI:10.1234/abc", ""); p.BodyStatus != "unverified" || p.Reason != "no_expected_identity" {
		t.Fatal(p)
	}
	for _, tc := range []struct{ raw, expected, status, contentType string }{
		{`<!doctype html><html><head><meta name="citation_doi" content="10.1234/abc"></head><body>Paper</body></html>`, "10.1234/abc", "matched", "html"},
		{`<html><head><meta name="citation_doi" content="10.1234/other"></head><body>References 10.1234/abc</body></html>`, "10.1234/abc", "mismatch", "html"},
		{`<html><body>References 10.1234/abc</body></html>`, "10.1234/abc", "unverified", "html"},
		{"%PDF-1.7\n...", "10.1234/abc", "unverified", "pdf"},
	} {
		p := InspectPaperBody(tc.raw, tc.expected)
		if p.BodyStatus != tc.status || p.ContentType != tc.contentType {
			t.Errorf("%+v: %+v", tc, p)
		}
	}
}

func TestOpenAlexLandingIsNotPDF(t *testing.T) {
	w := openalexWork{Title: "Paper", OpenAccess: &openalexOA{IsOA: true, OAURL: "https://example.org/landing"}}
	r, ok := openalexWorkToResult(w)
	if !ok || r.PDFURL != "" || r.URL != w.OpenAccess.OAURL {
		t.Fatalf("landing mislabeled: %+v", r)
	}
	w.BestOALocation = &openalexLocation{PDFUrl: "https://example.org/download"}
	if r, _ := openalexWorkToResult(w); r.PDFURL != w.BestOALocation.PDFUrl {
		t.Fatal("explicit PDF candidate lost")
	}
}
