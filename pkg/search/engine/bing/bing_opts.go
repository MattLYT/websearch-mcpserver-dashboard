package bing

import "websearch/pkg/antirobot"

// BingOpts Bing 通用搜索配置。
type BingOpts struct {
	Enabled    bool
	Blocked    []string // 屏蔽域名
	PerSec     int      // 每秒限流，默认 1
	PerMin     int      // 每分钟限流，默认 20
	SafeSearch int      // 0=关, 1=中, 2=严
}

// NewBing 创建 Bing 引擎。
func NewBing(opts BingOpts) antirobot.Engine {
	perSec, perMin := opts.PerSec, opts.PerMin
	if perSec <= 0 {
		perSec = 3
	}
	if perMin <= 0 {
		perMin = 60
	}
	e := &bingEngine{
		opts:    opts,
		limiter: antirobot.NewRateLimiter(perSec, perMin),
	}
	e.rotateSession()
	return e
}
