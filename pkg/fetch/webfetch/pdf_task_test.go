package webfetch

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"websearch/pkg/fetch/mineru"
)

func TestFetchPDFWithPagesPreservesMinerUAndFallbackSentinels(t *testing.T) {
	parseErr := errors.New("mineru sentinel")
	fallbackErr := errors.New("fallback sentinel")
	engine := &stubEngine{fetchErr: fallbackErr}
	spy := &spyMineru{hasToken: true, parseURLErr: parseErr}
	fetcher := &Fetcher{engine: engine, mineru: spy, mineruRemotePDF: true}

	_, err := fetcher.FetchPDFWithPages(context.Background(), "https://example.com/doc.pdf", []int{1}, 0)
	if err == nil {
		t.Fatal("expected MinerU + fallback error")
	}
	if !errors.Is(err, parseErr) || !errors.Is(err, fallbackErr) {
		t.Fatalf("error chain lost sentinel: %v", err)
	}
	if engine.fetchWithOptsCalls != 1 {
		t.Fatalf("fallback calls=%d", engine.fetchWithOptsCalls)
	}
}

func TestFetchPDFWithPagesStopsFallbackWhenCallerCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	parseErr := errors.New("mineru query failed")
	engine := &stubEngine{}
	spy := &spyMineru{hasToken: true, parseURLErr: parseErr}
	fetcher := &Fetcher{engine: engine, mineru: spy, mineruRemotePDF: true}

	_, err := fetcher.FetchPDFWithPages(ctx, "https://example.com/doc.pdf", []int{1}, 0)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, parseErr) {
		t.Fatalf("caller cancellation not preserved: %v", err)
	}
	if engine.fetchCalls != 0 || engine.fetchWithOptsCalls != 0 {
		t.Fatalf("fallback ran after caller cancellation: fetch=%d fetchWithOpts=%d", engine.fetchCalls, engine.fetchWithOptsCalls)
	}
}

func TestLocalOCRFallbackPreservesMinerUErrorTypes(t *testing.T) {
	pending := &mineru.TaskPendingError{TaskRef: "agent:job-1", Status: "running", Wait: time.Second}
	tests := []struct {
		name      string
		mineruErr error
		cancel    bool
		check     func(error) bool
	}{
		{
			name:      "pending",
			mineruErr: pending,
			check: func(err error) bool {
				var got *mineru.TaskPendingError
				return errors.As(err, &got) && got.TaskRef == pending.TaskRef
			},
		},
		{
			name:      "canceled",
			mineruErr: context.Canceled,
			cancel:    true,
			check: func(err error) bool {
				return errors.Is(err, context.Canceled)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			localErr := errors.New("no text extracted")
			engine := &stubEngine{pdfErr: localErr}
			spy := &spyMineru{parseFileErr: tt.mineruErr}
			fetcher := &Fetcher{engine: engine, mineru: spy, mineruOCR: true}

			_, err := fetcher.FetchPDFWithPages(ctx, "file:///tmp/scan.pdf", nil, 0)
			if err == nil || !tt.check(err) || !errors.Is(err, localErr) {
				t.Fatalf("MinerU error type lost: %v", err)
			}
			if spy.parseFileCalls != 1 {
				t.Fatalf("MinerU OCR calls=%d", spy.parseFileCalls)
			}
		})
	}
}

func TestResumeMinerUTaskResultAndLargeStorage(t *testing.T) {
	const preamble = "> 已取回原 MinerU 任务结果；页码范围沿用原任务，上传裁切任务的页码从 1 重新编号。"
	tests := []struct {
		name         string
		markdown     string
		maxInline    int
		wantFileName bool
	}{
		{"inline", "# resumed\nbody", 100, false},
		{"large", "# resumed\n" + strings.Repeat("line\n", 3), 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &spyMineru{resumeResult: tt.markdown}
			fetcher := &Fetcher{
				mineru:         spy,
				outputDir:      t.TempDir(),
				fileTTL:        time.Hour,
				maxInlineLines: tt.maxInline,
			}

			result, err := fetcher.ResumeMinerUTask(context.Background(), "batch:job-1")
			if err != nil {
				t.Fatal(err)
			}
			if result.ParseEngine != "mineru-remote" || result.Preamble != preamble {
				t.Fatalf("unexpected resume result: %+v", result)
			}
			if spy.resumeCalls != 1 || spy.resumeRef != "batch:job-1" || spy.parseURLCalls != 0 || spy.parseFileCalls != 0 {
				t.Fatalf("resume unexpectedly submitted or queried wrong ref: spy=%+v", spy)
			}

			if tt.wantFileName {
				if result.Mode != "saved_to_file" || result.FilePath == "" || result.Markdown != "" {
					t.Fatalf("large result not stored: %+v", result)
				}
				data, err := os.ReadFile(result.FilePath)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != tt.markdown {
					t.Fatalf("stored result mismatch")
				}
				return
			}
			if result.Mode != "inline" || result.Markdown != tt.markdown || result.FilePath != "" {
				t.Fatalf("inline result mismatch: %+v", result)
			}
		})
	}
}
