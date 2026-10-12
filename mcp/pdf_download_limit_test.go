package mcpserver

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestPDFDownloadEnforcesStreamingLimit(t *testing.T) {
	for _, data := range []string{"12345", "123456"} {
		path, cleanup, err := saveLimitedPDF(strings.NewReader(data), 5)
		if len(data) > 5 {
			if err == nil || path != "" || cleanup != nil {
				t.Fatalf("oversized response was retained: %q %v", path, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		cleanup()
		if err != nil || string(got) != data {
			t.Fatalf("download changed bytes: %q %v", got, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("temporary PDF not removed: %v", err)
		}
	}
	path, cleanup, err := saveLimitedPDF(io.MultiReader(strings.NewReader("123"), failingPDFReader{}), 5)
	if err == nil || path != "" || cleanup != nil {
		t.Fatal("partial failed download must not be retained")
	}
}

type failingPDFReader struct{}

func (failingPDFReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
