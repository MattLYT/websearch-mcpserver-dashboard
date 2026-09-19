// Package quota reads only documented, official provider usage endpoints.
// It never estimates quota from local request counts and never scrapes a web UI.
//
// Tavily 是唯一能用搜索 Key 查询官方用量的供应商，因此本包只查询并只返回
// Tavily；其他供应商既不探测也不出现在结果里。额度读取是只读操作：不写遥测、
// 不影响健康判定，也不产生计费调用。
package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"websearch/pkg/config"
)

const (
	// tavilyUsageEndpoint 是 Tavily 官方文档公开的用量查询端点。
	tavilyUsageEndpoint = "https://api.tavily.com/usage"
	// cacheTTL 保证控制中心不会高频访问官方端点。
	cacheTTL = 5 * time.Minute
	// requestTimeout 是单次额度查询的整体超时。
	requestTimeout = 12 * time.Second
	// providerTavily 是唯一被查询的供应商名。
	providerTavily = "tavily"
)

type Item struct {
	Provider   string         `json:"provider"`
	Configured bool           `json:"configured"`
	Status     string         `json:"status"`
	Unit       string         `json:"unit,omitempty"`
	Used       *float64       `json:"used,omitempty"`
	Limit      *float64       `json:"limit,omitempty"`
	Remaining  *float64       `json:"remaining,omitempty"`
	Plan       string         `json:"plan,omitempty"`
	UpdatedAt  string         `json:"updated_at,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	Error      string         `json:"error,omitempty"`
	Source     string         `json:"source,omitempty"`
}

type tavilyUsageResponse struct {
	Key struct {
		Usage        float64 `json:"usage"`
		Limit        float64 `json:"limit"`
		SearchUsage  float64 `json:"search_usage"`
		ExtractUsage float64 `json:"extract_usage"`
	} `json:"key"`
	Account struct {
		CurrentPlan string  `json:"current_plan"`
		PlanUsage   float64 `json:"plan_usage"`
		PlanLimit   float64 `json:"plan_limit"`
		PaygoUsage  float64 `json:"paygo_usage"`
		PaygoLimit  float64 `json:"paygo_limit"`
	} `json:"account"`
}

// Service 读取官方额度端点并缓存结果；Get 的返回值只包含 Tavily。
type Service struct {
	conf     config.Config
	client   *http.Client
	endpoint string
	now      func() time.Time

	mu       sync.Mutex
	cached   []Item
	cachedAt time.Time
}

// New 使用 Tavily 官方端点与默认超时构造 Service。
func New(conf config.Config) *Service {
	return newService(conf, tavilyUsageEndpoint, &http.Client{Timeout: requestTimeout})
}

// newService 允许测试注入端点与客户端，使测试无需访问真实网络。
func newService(conf config.Config, endpoint string, client *http.Client) *Service {
	if endpoint == "" {
		endpoint = tavilyUsageEndpoint
	}
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	return &Service{conf: conf, client: client, endpoint: endpoint, now: time.Now}
}

// Get 返回 Tavily 的额度条目（有且仅有一条），5 分钟内复用缓存结果。
func (s *Service) Get(ctx context.Context) []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil && s.clock().Sub(s.cachedAt) < cacheTTL {
		return append([]Item(nil), s.cached...)
	}
	items := []Item{s.tavily(ctx)}
	s.cached = items
	s.cachedAt = s.clock()
	return append([]Item(nil), items...)
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Service) tavily(ctx context.Context) Item {
	keys := s.conf.Tavily.EffectiveSKList()
	if len(keys) == 0 {
		return Item{Provider: providerTavily, Status: "not_configured"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if err != nil {
		return s.tavilyFailure(err, "")
	}
	req.Header.Set("Authorization", "Bearer "+keys[0])
	res, err := s.client.Do(req)
	if err != nil {
		return s.tavilyFailure(err, "")
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return s.tavilyFailure(nil, fmt.Sprintf("官方 API 拒绝该 Key（HTTP %d）", res.StatusCode))
	case http.StatusTooManyRequests:
		return s.tavilyFailure(nil, fmt.Sprintf("官方 API 限流（HTTP %d）", res.StatusCode))
	default:
		return s.tavilyFailure(nil, fmt.Sprintf("官方 API 返回 HTTP %d", res.StatusCode))
	}
	var body tavilyUsageResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return s.tavilyFailure(nil, "官方 API 响应无法解析")
	}
	item := tavilyItem(body, s.clock())
	item.Source = s.endpoint
	return item
}

// tavilyFailure 生成统一的失败条目：只给出状态与通用说明，不含 Key 或响应体。
func (s *Service) tavilyFailure(err error, msg string) Item {
	if msg == "" {
		msg = safeError(err)
	}
	return Item{Provider: providerTavily, Configured: true, Status: "query_failed", Error: msg, Source: s.endpoint}
}

func tavilyItem(body tavilyUsageResponse, updatedAt time.Time) Item {
	// The official endpoint returns both a per-key allowance and the account
	// plan allowance. The control center answers the account-level question, so
	// prefer the plan totals when present and fall back to key totals otherwise.
	used, limit := body.Account.PlanUsage, body.Account.PlanLimit
	if limit <= 0 {
		used, limit = body.Key.Usage, body.Key.Limit
	}
	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}
	return Item{
		Provider: "tavily", Configured: true, Status: "available", Unit: "credits",
		Used: &used, Limit: &limit, Remaining: &remaining, Plan: body.Account.CurrentPlan,
		UpdatedAt: updatedAt.Format(time.RFC3339), Source: "https://api.tavily.com/usage",
		Details: map[string]any{"key_usage": body.Key.Usage, "key_limit": body.Key.Limit,
			"search_usage": body.Key.SearchUsage, "extract_usage": body.Key.ExtractUsage,
			"account_plan_usage": body.Account.PlanUsage, "account_plan_limit": body.Account.PlanLimit,
			"paygo_usage": body.Account.PaygoUsage, "paygo_limit": body.Account.PaygoLimit},
	}
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "官方额度接口请求已取消"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "官方额度接口请求超时"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "官方额度接口请求超时"
	}
	return "官方额度接口连接失败"
}
