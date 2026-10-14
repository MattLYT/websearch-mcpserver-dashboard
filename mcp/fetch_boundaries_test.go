package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCleanFetchCombinedURLLimit(t *testing.T) {
	// Loopback URLs stop at the security precheck; no fetch or fallback is needed.
	const first = "https://127.0.0.1/page1"
	urls := []string{
		first, "", " https://127.0.0.1/page2 ", first,
		"https://127.0.0.1/page3", "https://127.0.0.1/page4", "https://127.0.0.1/page5",
	}
	t.Run("raw over five deduplicates to five", func(t *testing.T) {
		result, _, err := CleanFetch(context.Background(), nil, &CleanFetchParams{URL: " " + first + " ", URLs: urls})
		if err != nil {
			t.Fatalf("five unique URLs must be accepted: %v", err)
		}
		if result == nil || len(result.Content) != 1 {
			t.Fatalf("expected one sectioned response, got %v", result)
		}
		text := result.Content[0].(*mcp.TextContent).Text
		if got := strings.Count(text, "## ["); got != 5 {
			t.Fatalf("response sections = %d, want 5", got)
		}
		for i := 1; i <= 5; i++ {
			heading := fmt.Sprintf("## [%d] https://127.0.0.1/page%d\n", i, i)
			if strings.Count(text, heading) != 1 {
				t.Fatalf("expected exactly one ordered section %q", heading)
			}
		}
	})
	t.Run("six unique URLs rejected after merge", func(t *testing.T) {
		tooMany := append(append([]string(nil), urls...), "https://127.0.0.1/page6")
		result, _, err := CleanFetch(context.Background(), nil, &CleanFetchParams{URL: first, URLs: tooMany})
		if err == nil || result != nil {
			t.Fatalf("six unique URLs must be rejected wholesale, result=%v error=%v", result, err)
		}
	})
}
