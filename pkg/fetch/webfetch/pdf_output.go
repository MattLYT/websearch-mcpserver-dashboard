package webfetch

import (
	"strings"

	"github.com/daidaiJ/go-webfetch/store"
)

// StoreLargePDFResult preserves remote text fallback's existing output limits.
func (f *Fetcher) StoreLargePDFResult(result *Result, sourceURL string) error {
	lines, size := strings.Count(result.Markdown, "\n")+1, len(result.Markdown)
	maxLines := f.maxInlineLines
	if maxLines <= 0 {
		maxLines = 100
	}
	if lines <= maxLines && size <= 100000 && (f.maxInlineChars <= 0 || size <= f.maxInlineChars) {
		return nil
	}
	files, err := store.NewFileStore(f.outputDir, f.fileTTL)
	if err != nil {
		return err
	}
	path, totalLines, totalChars, _, err := files.WriteMarkdown(result.Markdown, result.Title, sourceURL)
	if err != nil {
		return err
	}
	result.Mode, result.FilePath = "saved_to_file", path
	result.TotalLines, result.TotalChars = totalLines, totalChars
	result.Markdown = ""
	result.AgentHint = "在服务运行环境中按行读取该文件；Docker 路径属于容器，不是宿主机路径。"
	return nil
}
