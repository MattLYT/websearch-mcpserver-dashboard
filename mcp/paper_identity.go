package mcpserver

import (
	"mime"
	"websearch/pkg/academic"
)

func metadataForPaper(text, expected, observedType string) map[string]any {
	paper := academic.InspectPaperBody(text, expected)
	if mediaType, _, err := mime.ParseMediaType(observedType); err == nil {
		switch mediaType {
		case "text/html", "application/xhtml+xml":
			observedType = "html"
		case "application/pdf":
			observedType = "pdf"
		}
	}
	if observedType == "html" || observedType == "pdf" {
		paper.ContentType = observedType
	}
	return map[string]any{"paper": paper}
}
