package core

import "fmt"

func FormatMD(id int, title, url, context string) string {
	return fmt.Sprintf("## 结果 %d \n**标题**: %s  \n**url**: %s  \n**内容**: %s  \n", id, title, url, context)
}

// FormatMDScore 格式化带引擎来源和相关性分数的搜索结果。
func FormatMDScore(id int, title, url, engine, scoreStr, context string) string {
	s := fmt.Sprintf("## 结果 %d \n**标题**: %s  \n**url**: %s  \n", id, title, url)
	if engine != "" {
		s += fmt.Sprintf("**来源**: %s  \n", engine)
	}
	if scoreStr != "" {
		s += fmt.Sprintf("**相关性**: %s  \n", scoreStr)
	}
	s += fmt.Sprintf("**内容**: %s  \n", context)
	return s
}

func FormatPaperMD(id int, title, url, authors, doi, journal, pubDate, pdfURL, citedBy, content string) string {
	s := fmt.Sprintf("## 结果 %d \n**标题**: %s  \n**url**: %s  \n", id, title, url)
	if authors != "" {
		s += fmt.Sprintf("**作者**: %s  \n", authors)
	}
	if journal != "" {
		s += fmt.Sprintf("**期刊**: %s  \n", journal)
	}
	if pubDate != "" {
		s += fmt.Sprintf("**发表日期**: %s  \n", pubDate)
	}
	if doi != "" {
		s += fmt.Sprintf("**DOI**: %s  \n", doi)
	}
	if citedBy != "" {
		s += fmt.Sprintf("**引用次数**: %s  \n", citedBy)
	}
	if pdfURL != "" {
		s += fmt.Sprintf("**PDF**: %s  \n", pdfURL)
	}
	s += fmt.Sprintf("**内容**: %s  \n", content)
	return s
}

func MDSearchHeader(query string, count int) string {
	return fmt.Sprintf("#搜索结果  \n查询: %s  \n 结果数: %d  \n", query, count)
}
