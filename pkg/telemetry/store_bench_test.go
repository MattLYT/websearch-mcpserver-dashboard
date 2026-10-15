package telemetry

// Store.Record 基准：每个遥测事件一次独立事务（BEGIN + 2 INSERT + COMMIT，
// WAL 模式）。量化请求路径上同步写遥测的单事件成本。

import (
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkStoreRecord(b *testing.B) {
	s, err := Open(filepath.Join(b.TempDir(), "telemetry.db"), 30)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	e := Event{
		Kind:         "tool",
		Tool:         "smartsearch",
		Provider:     "baidu_web",
		Query:        "go 并发编程最佳实践 与 channel 使用示例",
		Success:      true,
		Duration:     800 * time.Millisecond,
		ResultCount:  10,
		RequestID:    "req-bench",
		AttemptChain: "baidu_web>tavily",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.Record(e); err != nil {
			b.Fatal(err)
		}
	}
}
