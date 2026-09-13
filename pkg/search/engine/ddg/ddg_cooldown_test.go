package ddg

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"websearch/pkg/antirobot"
)

// withShortCooldown 缩短冷却时长，避免单测等待真实窗口。
func withShortCooldown(t *testing.T, base time.Duration) {
	t.Helper()
	oldBase, oldMax := ddgRateCooldown, ddgRateCooldownMax
	ddgRateCooldown, ddgRateCooldownMax = base, 10*base
	t.Cleanup(func() { ddgRateCooldown, ddgRateCooldownMax = oldBase, oldMax })
}

func withDDGTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	oldEndpoint := ddgEndpoint
	ddgEndpoint = ts.URL
	t.Cleanup(func() { ddgEndpoint = oldEndpoint })
}

func cooldownTestEngine() *ddgEngine {
	return &ddgEngine{
		opts:    DuckDuckGoOpts{Enabled: true},
		limiter: antirobot.NewRateLimiter(100, 10000), // 本地限流不干扰
		client:  http.DefaultClient,
		ua:      ddgUAs[0],
	}
}

const okHTML = `<html><body><div class="result"><a class="result__a" href="https://example.com/x">T</a></div></body></html>`

// TestDDGRetryWithinBudget 短 Retry-After 在超时预算内：等待窗口后重试一次，
// 本次调用直接成功返回（不进冷却）。
func TestDDGRetryWithinBudget(t *testing.T) {
	withShortCooldown(t, 20*time.Second)
	var hits int32
	withDDGTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "1") // 窗口 1s，预算 10s 内
			w.WriteHeader(202)
			return
		}
		fmt.Fprint(w, okHTML)
	})

	e := cooldownTestEngine()
	resp, err := e.Search("q", 1, antirobot.TimeRangeNone)
	if err != nil {
		t.Fatalf("预算内应等待重试成功: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Errorf("应解析出 1 条结果, got %d", len(resp.Results))
	}
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("应恰好 2 次请求（202 + 重试）, got %d", n)
	}
	if e.cooldownRemaining() != 0 || e.cooldown != 0 {
		t.Errorf("重试成功不应留下冷却状态, remaining=%v cooldown=%v", e.cooldownRemaining(), e.cooldown)
	}
}

// TestDDGCooldownWhenWaitExceedsBudget 等待窗口超出剩余预算（注定超时）：
// 直接放弃进冷却快速失败，不做无意义的等待重试。
func TestDDGCooldownWhenWaitExceedsBudget(t *testing.T) {
	withShortCooldown(t, 20*time.Second)
	var hits int32
	withDDGTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(202)
			return
		}
		fmt.Fprint(w, okHTML)
	})

	e := cooldownTestEngine()
	e.opts.Timeout = 2 * time.Second // 预算 2s：preDelay(~1s)+窗口(1s)+重试(~2s) 注定超时

	// 第一次：放弃并进入冷却
	if _, err := e.Search("q", 1, antirobot.TimeRangeNone); err == nil || !strings.Contains(err.Error(), "cooling down") {
		t.Fatalf("预算不足应报冷却错误, got %v", err)
	}
	if e.cooldownRemaining() <= 0 {
		t.Fatal("应进入冷却期")
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("放弃路径不应有第二次请求, hits = %d", n)
	}

	// 冷却期内：快速失败不打上游
	if _, err := e.Search("q2", 1, antirobot.TimeRangeNone); err == nil || !strings.Contains(err.Error(), "cooling down") {
		t.Fatalf("冷却期内应快速失败, got %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("冷却期内不应请求上游, hits = %d", n)
	}

	// 窗口过后：自动恢复并成功，冷却清零
	time.Sleep(1100 * time.Millisecond)
	resp, err := e.Search("q3", 1, antirobot.TimeRangeNone)
	if err != nil {
		t.Fatalf("冷却结束后应恢复: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Errorf("应解析出 1 条结果, got %d", len(resp.Results))
	}
	if e.cooldownRemaining() != 0 || e.cooldown != 0 {
		t.Errorf("成功后冷却应清零, remaining=%v cooldown=%v", e.cooldownRemaining(), e.cooldown)
	}
}

// TestDDGCooldownDoubling 连续 202（无 Retry-After）冷却翻倍：
// 每次调用内等待重试仍 202 → 进冷却；跨调用连续限流时长翻倍。
func TestDDGCooldownDoubling(t *testing.T) {
	withShortCooldown(t, 20*time.Millisecond)
	withDDGTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(202)
	})
	e := cooldownTestEngine()

	if _, err := e.Search("q", 1, antirobot.TimeRangeNone); err == nil {
		t.Fatal("expect error")
	}
	if first := e.cooldown; first != 20*time.Millisecond {
		t.Fatalf("首次冷却应取默认值, got %v", first)
	}

	time.Sleep(30 * time.Millisecond) // 等首次冷却过期
	if _, err := e.Search("q", 1, antirobot.TimeRangeNone); err == nil {
		t.Fatal("expect error")
	}
	if e.cooldown != 2*20*time.Millisecond {
		t.Fatalf("连续 202 冷却应翻倍, got %v", e.cooldown)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter(""); d != 0 {
		t.Errorf("空串应返回 0, got %v", d)
	}
	if d := parseRetryAfter("3"); d != 3*time.Second {
		t.Errorf("秒数解析失败: %v", d)
	}
	if d := parseRetryAfter("garbage"); d != 0 {
		t.Errorf("垃圾值应返回 0, got %v", d)
	}
	future := time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(future); d <= 0 || d > 6*time.Second {
		t.Errorf("HTTP-Date 解析异常: %v", d)
	}
}
