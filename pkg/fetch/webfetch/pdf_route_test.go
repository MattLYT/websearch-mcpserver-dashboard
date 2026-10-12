package webfetch

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"websearch/pkg/config"
	"websearch/pkg/fetch/mineru"

	webfetch "github.com/daidaiJ/go-webfetch"
)

func TestNewFromConfigUsesUpstreamPDFEngine(t *testing.T) {
	fetcher, err := NewFromConfig(config.CleanFetchConfig{FileOutputDir: t.TempDir()}, config.PDFParserConfig{}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer fetcher.Close()
	if _, ok := fetcher.engine.(*webfetch.Engine); !ok {
		t.Fatalf("configured engine type=%T; want upstream go-webfetch engine", fetcher.engine)
	}
}

func TestArxivPDFRoutes(t *testing.T) {
	for _, test := range []struct {
		url  string
		want bool
	}{
		{"https://arxiv.org/pdf/1706.03762", true},
		{"https://arxiv.org/pdf/2309.15217v2?download=1#page=2", true},
		{"https://arxiv.org/pdf/hep-th/9901001v2", true},
		{"https://arxiv.org/abs/1706.03762", false},
		{"https://evil.example/pdf/1706.03762", false},
		{"https://arxiv.org.evil.example/pdf/1706.03762", false},
		{"https://arxiv.org@evil.example/pdf/1706.03762", false},
		{"https://arxiv.org/pdf/1706.03762/extra", false},
		{"https://arxiv.org/pdf/1706.03762v0", false},
		{"https://arxiv.org/pdf/1706.03762x", false},
		{"file://arxiv.org/pdf/1706.03762", false},
	} {
		if got := isPDFURL(test.url); got != test.want {
			t.Errorf("isPDFURL(%q)=%v", test.url, got)
		}
	}
	spy := &spyMineru{hasToken: true}
	fetcher := &Fetcher{engine: &stubEngine{}, mineru: spy, mineruRemotePDF: true}
	result, err := fetcher.ParseMinerUPDFWithPages(context.Background(), "https://arxiv.org/pdf/1706.03762", []int{1, 2}, 20)
	if err != nil || result.ParseEngine != "mineru-remote" || spy.parseURLCalls != 1 || !reflect.DeepEqual(spy.pages, []int{1, 2}) {
		t.Fatalf("precise route result=%+v err=%v spy=%+v", result, err, spy)
	}
	spy.parseURLErr = mineru.ErrRemoteURLRejected
	if _, err := fetcher.ParseMinerUPDFWithPages(context.Background(), "https://arxiv.org/pdf/1706.03762", nil, 20); !errors.Is(err, mineru.ErrRemoteURLRejected) {
		t.Fatalf("precise error=%v", err)
	}
	if _, err := fetcher.ParseMinerUPDFWithPages(context.Background(), "https://example.com/page", nil, 20); err == nil {
		t.Fatal("unsupported URL must not take precise PDF route")
	}
}
