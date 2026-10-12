package mineru

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadMarkdownOCRReadableText(t *testing.T) {
	for _, tt := range []struct {
		name      string
		markdown  string
		ocr       bool
		wantError bool
	}{
		{"empty", "", true, true},
		{"whitespace", " \n\t", true, true},
		{"real image placeholders", "<!-- image-->\n\n<!-- image-->\n\n<!-- image-->\n\n<!-- image-->", true, true},
		{"multiline comment", "<!-- image\nplaceholder -->", true, true},
		{"images only", "![figure](images/one.jpg)\n![](images/two.jpg)", true, true},
		{"comments and images", "<!-- image-->\n![caption](images/one.jpg)", true, true},
		{"readable text", "SCAN PAGE 1\nORCHID DELTA 137", true, false},
		{"text and image", "<!-- image-->\nSCAN PAGE 1\n![figure](images/one.jpg)", true, false},
		{"non OCR images", "<!-- image-->\n![figure](images/one.jpg)", false, false},
		{"non OCR empty", "", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.markdown))
			}))
			defer server.Close()
			client := NewFromConfig("", "pipeline", "en", tt.ocr, true, true, "", 0)
			got, err := client.downloadMarkdown(context.Background(), server.URL)
			if tt.wantError {
				if err == nil || !strings.Contains(err.Error(), "OCR未提取到可读文本") || got != "" {
					t.Fatalf("markdown=%q error=%v; want unreadable OCR error", got, err)
				}
			} else if err != nil || got != tt.markdown {
				t.Fatalf("markdown=%q error=%v; want original markdown", got, err)
			}
		})
	}
}
