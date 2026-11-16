package academic

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"websearch/pkg/antirobot"
)

// PaperIdentity keeps metadata identity separate from the identity of fetched content.
type PaperIdentity = antirobot.PaperIdentity

var arxivVersion = regexp.MustCompile(`v\d+$`)
var bodySection = regexp.MustCompile(`(?i)^(?:\d+[.\s]*)?(abstract|introduction|references|bibliography|摘要|引言|参考文献)(?:[\s:：.]|$)`)

// ComparePaperID compares full identifiers; a requested arXiv version must be observed.
func ComparePaperID(kind, expected, actual string) string {
	if actual == "" || expected == "" {
		return "unverified"
	}
	if kind == "doi" {
		expectedKind, expectedID := ParsePaperQuery(expected)
		actualKind, actualID := ParsePaperQuery(actual)
		if expectedKind != kind || actualKind != kind {
			return "unverified"
		}
		if antirobot.NormalizeDOI(expectedID) == antirobot.NormalizeDOI(actualID) {
			return "matched"
		}
		return "mismatch"
	}
	if kind != "arxiv" {
		return "unverified"
	}
	expectedKind, expectedID := ParsePaperQuery(expected)
	actualKind, actualID := ParsePaperQuery(actual)
	if actualKind != kind || expectedKind != kind {
		return "unverified"
	}
	expected = expectedID
	if arxivVersion.ReplaceAllString(expected, "") != arxivVersion.ReplaceAllString(actualID, "") {
		return "mismatch"
	}
	if arxivVersion.MatchString(expected) {
		if !arxivVersion.MatchString(actualID) {
			return "unverified"
		}
		if expected != actualID {
			return "mismatch"
		}
	}
	return "matched"
}

func MetadataIdentity(kind, expected, actual string) PaperIdentity {
	return PaperIdentity{RequestedKind: kind, RequestedID: expected, MetadataID: actual,
		MetadataStatus: ComparePaperID(kind, expected, actual), BodyStatus: "unverified", ContentType: "unknown", Reason: "body_not_fetched"}
}

// InspectPaperBody accepts explicit front-matter identifiers, never DOI substrings in prose.
func InspectPaperBody(text, expected string) PaperIdentity {
	kind, id := ParsePaperQuery(expected)
	p := PaperIdentity{RequestedKind: kind, RequestedID: id, MetadataStatus: "unverified", BodyStatus: "unverified", ContentType: "unknown", Reason: "body_identity_not_observed"}
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "%PDF-") {
		p.ContentType = "pdf"
	}
	header := strings.ToLower(trimmed[:min(len(trimmed), 32)])
	if strings.HasPrefix(header, "<!doctype html") || strings.HasPrefix(header, "<html") {
		p.ContentType = "html"
		if doc, err := goquery.NewDocumentFromReader(strings.NewReader(text)); err == nil && kind != "" {
			doc.Find("meta").EachWithBreak(func(_ int, meta *goquery.Selection) bool {
				name := strings.ToLower(meta.AttrOr("name", meta.AttrOr("property", "")))
				if kind == "doi" && (name == "citation_doi" || name == "dc.identifier" || name == "prism.doi") || kind == "arxiv" && name == "citation_arxiv_id" {
					value := meta.AttrOr("content", "")
					if kind == "arxiv" {
						value = "arxiv:" + strings.TrimPrefix(value, "arxiv:")
					}
					observedKind, observed := ParsePaperQuery(value)
					if observedKind == kind {
						p.BodyID, p.BodyStatus, p.Reason = observed, ComparePaperID(kind, id, observed), "document_metadata_identifier"
						return false
					}
				}
				return true
			})
			return p
		}
	}
	if kind == "" {
		p.Reason = "no_expected_identity"
		return p
	}
	lines := strings.SplitN(text, "\n", 41)
	for _, line := range lines[:min(len(lines), 40)] {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
		lower := strings.ToLower(line)
		if bodySection.MatchString(lower) {
			break
		}
		if !strings.HasPrefix(lower, "doi:") && !strings.HasPrefix(lower, "arxiv:") {
			continue
		}
		observedKind, observed := ParsePaperQuery(line)
		if observedKind != kind {
			continue
		}
		p.BodyID = observed
		p.BodyStatus = ComparePaperID(kind, id, observed)
		p.Reason = "explicit_front_matter_identifier"
		return p
	}
	return p
}
