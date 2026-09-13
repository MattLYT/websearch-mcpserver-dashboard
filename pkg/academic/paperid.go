package academic

import (
	"net/url"
	"regexp"
	"strings"

	"websearch/pkg/antirobot"
)

var (
	arxivAbsRe = regexp.MustCompile(`(?i)(?:arxiv:|arxiv\.org/(?:abs|pdf)/)(\d{4}\.\d{4,5}(?:v\d+)?)`)
	arxivBare  = regexp.MustCompile(`^\d{4}\.\d{4,5}(?:v\d+)?$`)
)

// ParsePaperQuery 判断 query 是否实质就是一篇论文的 DOI / arXiv id。
// 长句里顺带出现 DOI 时返回空，避免误伤关键词搜索。
func ParsePaperQuery(query string) (kind, id string) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", ""
	}
	if id := standaloneArxivID(q); id != "" {
		return "arxiv", id
	}
	doi := antirobot.ExtractDOI(q)
	if doi != "" && standaloneDOI(q, doi) {
		return "doi", doi
	}
	return "", ""
}

func standaloneDOI(q, doi string) bool {
	s := strings.TrimSpace(q)
	s = strings.TrimPrefix(strings.ToLower(s), "doi:")
	s = strings.TrimSpace(s)
	for _, p := range []string{
		"https://doi.org/", "http://doi.org/",
		"https://dx.doi.org/", "http://dx.doi.org/",
	} {
		s = strings.TrimPrefix(strings.ToLower(s), p)
	}
	// ExtractDOI 可能从原串取出；前缀剥完后应只剩 DOI
	s = strings.TrimSpace(s)
	return strings.EqualFold(s, doi) || strings.EqualFold(s, strings.ToLower("https://doi.org/"+doi))
}

func standaloneArxivID(q string) string {
	s := strings.TrimSpace(q)
	if arxivBare.MatchString(s) {
		return s
	}
	if m := arxivAbsRe.FindStringSubmatch(s); len(m) == 2 {
		rest := arxivAbsRe.ReplaceAllString(s, "")
		if strings.TrimSpace(rest) == "" {
			return m[1]
		}
		u, err := url.Parse(s)
		if err == nil && (strings.EqualFold(u.Host, "arxiv.org") || strings.EqualFold(u.Host, "www.arxiv.org")) {
			return m[1]
		}
	}
	return ""
}
