package mcpserver

// cleanfetch 续读分页基准：readSavedResult 每次调用从文件头逐 rune 扫描到
// offset，再读 limit 个字符；顺序读回整个文件的累计成本是 O(n²/limit)。
// 本基准量化一次完整顺序读回（所有分页调用串联）的真实耗时。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func BenchmarkReadSavedResultPaging(b *testing.B) {
	dir := b.TempDir()
	const totalChars = 131072 // 128k 字符（约 256KB+ UTF-8），典型落盘大正文量级
	content := strings.Repeat("摘要正文mixed English 123，用于压测续读扫描。", totalChars/22)
	path := filepath.Join(dir, "bench.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	ref := "result:benchpaging"
	resultFiles.Lock()
	resultFiles.files[ref] = resultFile{path: path, info: info, expires: time.Now().Add(time.Hour)}
	resultFiles.Unlock()
	defer func() {
		resultFiles.Lock()
		delete(resultFiles.files, ref)
		resultFiles.Unlock()
	}()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		offset := int64(0)
		pages := 0
		for {
			_, out, err := readSavedResult(ctx, nil, &ReadResultParams{Ref: ref, Offset: offset, Limit: 4096})
			if err != nil {
				b.Fatal(err)
			}
			offset = out.NextOffset
			pages++
			if out.EOF {
				break
			}
		}
		if pages < 2 {
			b.Fatalf("unexpected page count %d", pages)
		}
	}
}
