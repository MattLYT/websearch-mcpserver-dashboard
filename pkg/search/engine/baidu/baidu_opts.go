package baidu

import "websearch/pkg/antirobot"

// BaiduOpts 百度网页搜索配置（无需 API Key，直接抓取搜索页）。
type BaiduOpts struct {
	Enabled bool
	Blocked []string // 屏蔽域名
	PerSec  int      // 每秒限流，默认 1
	PerMin  int      // 每分钟限流，默认 20
}

// NewBaiduWeb 创建百度网页搜索引擎（tn=json JSON API）。
func NewBaiduWeb(opts BaiduOpts) antirobot.Engine {
	perSec, perMin := opts.PerSec, opts.PerMin
	if perSec <= 0 {
		perSec = 3
	}
	if perMin <= 0 {
		perMin = 60
	}
	e := &baiduEngine{
		opts:    opts,
		limiter: antirobot.NewRateLimiter(perSec, perMin),
	}
	e.rotateSession()
	return e
}
