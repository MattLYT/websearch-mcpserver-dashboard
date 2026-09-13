package google

import (
	"net/http"
	"time"

	"websearch/pkg/antirobot"
	"websearch/pkg/proxy"
)

// GoogleOpts Google 搜索配置。
type GoogleOpts struct {
	Enabled      bool
	Blocked      []string            // 屏蔽域名
	PerSec       int                 // 每秒限流，默认 3
	PerMin       int                 // 每分钟限流，默认 30
	ProxyResolve proxy.ProxyResolver // 代理端点动态解析函数（每次请求实时获取）
}

// NewGoogle 创建 Google 搜索引擎。
func NewGoogle(opts GoogleOpts) antirobot.Engine {
	perSec, perMin := opts.PerSec, opts.PerMin
	if perSec <= 0 {
		perSec = 3
	}
	if perMin <= 0 {
		perMin = 30
	}
	e := &googleEngine{
		opts:    opts,
		limiter: antirobot.NewRateLimiter(perSec, perMin),
	}
	e.rotateSession()
	return e
}

func (o GoogleOpts) newHTTPClient() *http.Client {
	return proxy.NewDynamicHTTPClient(o.ProxyResolve, 15*time.Second)
}
