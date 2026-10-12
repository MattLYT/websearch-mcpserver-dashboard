package fetch

import (
	"regexp"
	"strings"
)

var skipContentLink = regexp.MustCompile(`(?i)^\[(?:skip to content|skip to main content)\]\(#[^\s)]+\)$`)

// HasPageBody rejects an empty shell without imposing a minimum article length.
func HasPageBody(markdown, title string) bool {
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "# "+strings.TrimSpace(title) || skipContentLink.MatchString(line) {
			continue
		}
		return true
	}
	return false
}
