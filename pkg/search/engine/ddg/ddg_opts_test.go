package ddg

import "testing"

// TestDuckDuckGoRateLimitClamp 限流配置钳制：宽松配置钳到内置上限（实测 202 阈值），
// 更严格配置保留，未配置用内置默认。
func TestDuckDuckGoRateLimitClamp(t *testing.T) {
	cases := []struct {
		name         string
		perSec, mPer int
		wantSec      int
		wantMin      int
	}{
		{"宽松配置被钳制", 10, 100, ddgMaxPerSec, ddgMaxPerMin},
		{"全局 rate_limit 默认 3/60 被钳制", 3, 60, ddgMaxPerSec, ddgMaxPerMin},
		{"未配置用内置默认", 0, 0, ddgMaxPerSec, ddgMaxPerMin},
		{"更严格配置保留", 1, 3, 1, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := NewDuckDuckGo(DuckDuckGoOpts{Enabled: true, PerSec: c.perSec, PerMin: c.mPer}).(*ddgEngine)
			gotSec, gotMin := e.limiter.Limits()
			if gotSec != c.wantSec {
				t.Errorf("perSec = %d, want %d", gotSec, c.wantSec)
			}
			if gotMin != c.wantMin {
				t.Errorf("perMin = %d, want %d", gotMin, c.wantMin)
			}
		})
	}
}
