package webfetch

import (
	"os"
	"strings"
	"testing"
)

func TestRemotePDFTextOutputLimits(t *testing.T) {
	for _, test := range []struct {
		text         string
		lines, chars int
		saved        bool
	}{
		{"short", 2, 0, false},
		{"one\ntwo\nthree", 2, 0, true},
		{"exact", 2, 5, false},
		{"exceeds", 2, 5, true},
		{strings.Repeat("x", 100001), 2, 0, true},
	} {
		fetcher := &Fetcher{outputDir: t.TempDir(), maxInlineLines: test.lines, maxInlineChars: test.chars}
		result := &Result{Mode: "inline", Markdown: test.text, Title: "Public PDF"}
		if err := fetcher.StoreLargePDFResult(result, "https://example.com/paper.pdf"); err != nil {
			t.Fatal(err)
		}
		if (result.Mode == "saved_to_file") != test.saved {
			t.Fatalf("unexpected output mode %s", result.Mode)
		}
		if test.saved {
			data, err := os.ReadFile(result.FilePath)
			if err != nil || string(data) != test.text || result.Markdown != "" || result.TotalChars != len(test.text) {
				t.Fatalf("saved PDF content changed: %+v %v", result, err)
			}
		} else if result.Markdown != test.text {
			t.Fatal("inline PDF content changed")
		}
	}
}
