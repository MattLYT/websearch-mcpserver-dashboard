package adapter

import (
	"errors"
	"strings"
	"testing"

	"websearch/pkg/antirobot"
)

// fakeEngine 单测用学术引擎桩。
type fakeEngine struct {
	name    string
	results []antirobot.Result
	err     error
}

func (f *fakeEngine) Name() string                    { return f.name }
func (f *fakeEngine) Region() antirobot.NetworkRegion { return antirobot.RegionChina }
func (f *fakeEngine) Search(_ string, _ int, _ antirobot.TimeRange) (*antirobot.SearchResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &antirobot.SearchResponse{Engine: f.name, Results: f.results}, nil
}

func paperResult(engine, url, title string) antirobot.Result {
	return antirobot.Result{Type: antirobot.ResultPaper, Engine: engine, URL: url, Title: title}
}

func newTestAdapter(engines ...antirobot.Engine) *AcademicAdapter {
	return &AcademicAdapter{
		searcher:  antirobot.NewSearcher(antirobot.StrategyParallel, engines),
		engines:   engines,
		enhance:   true,
		threshold: 0.0,
	}
}

// TestSearchAcademicRaw_PartialEngineError 部分引擎失败：返回成功结果 + EngineErrors 仅含失败引擎。
func TestSearchAcademicRaw_PartialEngineError(t *testing.T) {
	adapter := newTestAdapter(
		&fakeEngine{name: "arxiv", results: []antirobot.Result{paperResult("arxiv", "https://arxiv.org/abs/1", "Paper A")}},
		&fakeEngine{name: "crossref", results: []antirobot.Result{paperResult("crossref", "https://doi.org/10.1/b", "Paper B")}},
		&fakeEngine{name: "openalex", err: errors.New("openalex HTTP 429")},
	)

	res, err := adapter.SearchAcademicRaw("attention")
	if err != nil {
		t.Fatalf("部分引擎失败不应报错: %v", err)
	}
	if len(res.Results) != 2 {
		t.Errorf("应返回 2 条成功结果, got %d", len(res.Results))
	}
	if len(res.EngineErrors) != 1 {
		t.Fatalf("EngineErrors 应只含 1 个引擎, got %v", res.EngineErrors)
	}
	if res.EngineErrors["openalex"] != "openalex HTTP 429" {
		t.Errorf("EngineErrors[openalex] = %q", res.EngineErrors["openalex"])
	}

	// Markdown 呈现：末尾含警告行
	out, err := adapter.MergeContentWithErrors("attention", res.Results, res.EngineErrors)
	if err != nil {
		t.Fatalf("MergeContentWithErrors: %v", err)
	}
	if !strings.Contains(out, "Paper A") {
		t.Errorf("输出应含成功结果: %s", out)
	}
	if !strings.Contains(out, "⚠ 部分引擎本次失败") || !strings.Contains(out, "openalex (openalex HTTP 429)") {
		t.Errorf("输出末尾应含逐引擎警告: %s", out)
	}
}

// TestSearchAcademicRaw_AllSuccess 全部成功：无 EngineErrors，输出无警告行。
func TestSearchAcademicRaw_AllSuccess(t *testing.T) {
	adapter := newTestAdapter(
		&fakeEngine{name: "arxiv", results: []antirobot.Result{paperResult("arxiv", "https://arxiv.org/abs/1", "Paper A")}},
	)

	res, err := adapter.SearchAcademicRaw("attention")
	if err != nil {
		t.Fatalf("全部成功不应报错: %v", err)
	}
	if len(res.EngineErrors) != 0 {
		t.Errorf("EngineErrors 应为空, got %v", res.EngineErrors)
	}

	out, err := adapter.MergeContentWithErrors("attention", res.Results, res.EngineErrors)
	if err != nil {
		t.Fatalf("MergeContentWithErrors: %v", err)
	}
	if strings.Contains(out, "⚠") {
		t.Errorf("全部成功时不应有警告行: %s", out)
	}
}

// TestSearchAcademicRaw_AllFailed 全部失败：返回 error，message 含各引擎错误。
func TestSearchAcademicRaw_AllFailed(t *testing.T) {
	adapter := newTestAdapter(
		&fakeEngine{name: "arxiv", err: errors.New("timeout")},
		&fakeEngine{name: "crossref", err: errors.New("crossref HTTP 503")},
	)

	_, err := adapter.SearchAcademicRaw("attention")
	if err == nil {
		t.Fatal("全部失败应返回 error")
	}
	if !strings.Contains(err.Error(), "学术引擎搜索无结果") ||
		!strings.Contains(err.Error(), "arxiv: timeout") ||
		!strings.Contains(err.Error(), "crossref: crossref HTTP 503") {
		t.Errorf("error 应汇总各引擎错误, got %v", err)
	}
}

// TestSearchAcademicRaw_DisableEnhance 非增强路径同样透传引擎错误。
func TestSearchAcademicRaw_DisableEnhance(t *testing.T) {
	adapter := newTestAdapter(
		&fakeEngine{name: "arxiv", results: []antirobot.Result{paperResult("arxiv", "https://arxiv.org/abs/1", "Paper A")}},
		&fakeEngine{name: "crossref", err: errors.New("crossref HTTP 500")},
	)
	adapter.enhance = false

	res, err := adapter.SearchAcademicRaw("attention")
	if err != nil {
		t.Fatalf("部分引擎失败不应报错: %v", err)
	}
	if len(res.Results) != 1 {
		t.Errorf("应返回 1 条成功结果, got %d", len(res.Results))
	}
	if res.EngineErrors["crossref"] != "crossref HTTP 500" {
		t.Errorf("EngineErrors[crossref] = %q", res.EngineErrors["crossref"])
	}
}
