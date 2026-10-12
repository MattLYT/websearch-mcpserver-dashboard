package mcpserver

import (
	"context"
	"fmt"
	"testing"

	engine "github.com/daidaiJ/go-webfetch"
)

func TestAllowFetchFallback(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{"404", &engine.NotFoundError{StatusCode: 404}, false},
		{"410", &engine.NotFoundError{StatusCode: 410}, false},
		{"SSRF", &engine.SSRFError{}, false},
		{"challenge", &engine.WAFError{}, false},
		{"canceled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
		{"empty body", &engine.EmptyContentError{}, true},
		{"transport failure", fmt.Errorf("connection reset"), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowFetchFallback(fmt.Errorf("friendly message: %w", tt.err)); got != tt.want {
				t.Fatalf("fallback = %v, want %v", got, tt.want)
			}
		})
	}
}
