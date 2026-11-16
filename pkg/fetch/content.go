package fetch

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

var skipContentLink = regexp.MustCompile(`(?i)^\[(?:skip to content|skip to main content)\]\(#[^\s)]+\)$`)

// BodyStatus describes the extraction, not relevance or whether a reader has read it.
type BodyStatus struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func PageBodyStatus(markdown, title string) BodyStatus {
	status, _ := PageBodyStatusReader(strings.NewReader(markdown), title)
	return status
}

// PageBodyStatusReader applies the same rule to saved text without loading the whole file.
func PageBodyStatusReader(reader io.Reader, title string) (BodyStatus, error) {
	buffered := bufio.NewReader(reader)
	seen := false
	for {
		line, err := buffered.ReadString('\n')
		if err != nil && err != io.EOF {
			return BodyStatus{Status: "unavailable", Reason: "read_failed"}, err
		}
		line = strings.TrimSpace(line)
		if line != "" {
			seen = true
		}
		heading := strings.HasPrefix(line, "# ") && (title == "" || strings.TrimSpace(strings.TrimPrefix(line, "# ")) == strings.TrimSpace(title))
		if line != "" && !heading && !skipContentLink.MatchString(line) {
			return BodyStatus{Status: "available", Reason: "extracted_body"}, nil
		}
		if err == io.EOF {
			break
		}
	}
	if seen {
		return BodyStatus{Status: "empty", Reason: "title_or_navigation_only"}, nil
	}
	return BodyStatus{Status: "empty", Reason: "empty_content"}, nil
}

// HasPageBody rejects an empty shell without imposing a minimum article length.
func HasPageBody(markdown, title string) bool {
	return PageBodyStatus(markdown, title).Status == "available"
}
