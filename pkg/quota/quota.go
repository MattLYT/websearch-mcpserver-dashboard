// Package quota reads only documented, official provider usage endpoints.
// It never estimates quota from local request counts and never scrapes a web UI.
package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"websearch/pkg/config"
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

type Service struct {
	conf     config.Config
	client   *http.Client
	mu       sync.Mutex
	cached   []Item
	cachedAt time.Time
}

func New(conf config.Config) *Service {
	return &Service{conf: conf, client: &http.Client{Timeout: 12 * time.Second}}
}

func (s *Service) Get(ctx context.Context) []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.cachedAt) < 5*time.Minute && s.cached != nil {
		return append([]Item(nil), s.cached...)
	}
	items := []Item{
		s.tavily(ctx),
		unsupported("anysearch", len(s.conf.Anysearch.EffectiveSKList()) > 0, "官方文档未提供可主动查询余额的端点；配额耗尽时会在官方错误响应中返回"),
		unsupported("doubao", len(s.conf.Doubao.EffectiveSKList()) > 0, "当前搜索 Key 不具备费用中心查询权限"),
		unsupported("exa", len(s.conf.Exa.EffectiveSKList()) > 0, "官方用量端点需要额外的 Team Management service key 与 key ID"),
		unsupported("jina", s.conf.Jina.APIKey != "", "官方文档未提供可用 Reader Key 查询余额的端点"),
		unsupported("mineru", s.conf.PDFParser.MinerUToken != "", "官方文档未提供可用解析 Token 查询余额的端点"),
		unsupported("baidu", len(s.conf.Baidu.EffectiveSKList()) > 0, "当前搜索 Key 没有已确认的余额查询接口"),
	}
	s.cached = items
	s.cachedAt = time.Now()
	return append([]Item(nil), items...)
}

func unsupported(provider string, configured bool, reason string) Item {
	status := "not_configured"
	if configured {
		status = "official_api_unavailable"
	}
	return Item{Provider: provider, Configured: configured, Status: status, Error: reason}
}

func (s *Service) tavily(ctx context.Context) Item {
	keys := s.conf.Tavily.EffectiveSKList()
	if len(keys) == 0 {
		return Item{Provider: "tavily", Status: "not_configured"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.tavily.com/usage", nil)
	if err != nil {
		return Item{Provider: "tavily", Configured: true, Status: "query_failed", Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+keys[0])
	res, err := s.client.Do(req)
	if err != nil {
		return Item{Provider: "tavily", Configured: true, Status: "query_failed", Error: safeError(err), Source: "https://api.tavily.com/usage"}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Item{Provider: "tavily", Configured: true, Status: "query_failed", Error: fmt.Sprintf("官方 API 返回 HTTP %d", res.StatusCode), Source: "https://api.tavily.com/usage"}
	}
	var body tavilyUsageResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return Item{Provider: "tavily", Configured: true, Status: "query_failed", Error: "官方 API 响应无法解析", Source: "https://api.tavily.com/usage"}
	}
	return tavilyItem(body, time.Now())
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
	return "官方额度接口连接失败"
}
