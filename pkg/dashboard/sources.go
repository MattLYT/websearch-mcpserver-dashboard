package dashboard

import (
	"slices"
	"strings"

	"websearch/pkg/config"
	"websearch/pkg/telemetry"
)

type SystemSummary struct {
	Status    string `json:"status"`
	Enabled   int    `json:"enabled"`
	Observed  int    `json:"observed"`
	Degraded  int    `json:"degraded"`
	Down      int    `json:"down"`
	Suspended int    `json:"suspended"`
}

type SourceView struct {
	ID                  string                     `json:"id"`
	Name                string                     `json:"name"`
	Group               string                     `json:"group"`
	Configured          bool                       `json:"configured"`
	Active              bool                       `json:"active"`
	Status              string                     `json:"status"`
	LastSuccessAt       string                     `json:"last_success_at,omitempty"`
	LastFailureAt       string                     `json:"last_failure_at,omitempty"`
	LastSeenAt          string                     `json:"last_seen_at,omitempty"`
	FailureRate         float64                    `json:"failure_rate"`
	SampleSize          int                        `json:"sample_size"`
	ConsecutiveFailures int                        `json:"consecutive_failures"`
	RecentOutcomes      []bool                     `json:"recent_outcomes"`
	Today               telemetry.DailyUsage       `json:"today"`
	LastError           string                     `json:"last_error,omitempty"`
	State               string                     `json:"state,omitempty"`
	Suspended           bool                       `json:"suspended"`
	SuspendedUntil      string                     `json:"suspended_until,omitempty"`
	SuspendReason       string                     `json:"suspend_reason,omitempty"`
	SuspendCountdown    int64                      `json:"suspend_countdown_sec,omitempty"`
	P95MS               int64                      `json:"p95_ms"`
	ErrorKinds          []telemetry.ErrorKindCount `json:"error_kinds,omitempty"`
	QuotaQueryable      bool                       `json:"quota_queryable"`
	aliases             []string
}

type dashboardOverview struct {
	telemetry.Overview
	System          SystemSummary `json:"system"`
	Sources         []SourceView  `json:"sources"`
	AcademicSources []SourceView  `json:"academic_sources"`
	// ConfiguredTools 是公开 MCP 工具的固定清单，不依赖是否已经产生调用。
	ConfiguredTools []MCPToolView `json:"configured_tools"`
}

func buildDashboardOverview(conf config.Config, observed telemetry.Overview) dashboardOverview {
	web := joinSources(webSourceCatalog(conf), observed.Providers)
	academic := joinSources(academicSourceCatalog(conf), observed.Providers)
	return dashboardOverview{
		Overview:        observed,
		System:          summarizeSystem(web),
		Sources:         web,
		AcademicSources: academic,
		ConfiguredTools: mcpToolCatalog(conf),
	}
}

func joinSources(defs []SourceView, observed []telemetry.Health) []SourceView {
	byName := make(map[string]telemetry.Health, len(observed))
	for _, h := range observed {
		byName[h.Name] = h
	}
	for i := range defs {
		s := &defs[i]
		if !s.Configured {
			s.Status = "not_configured"
			continue
		}
		if !s.Active {
			s.Status = "inactive"
			continue
		}
		var h telemetry.Health
		var ok bool
		var today telemetry.DailyUsage
		for _, alias := range s.aliases {
			candidate, found := byName[alias]
			if !found {
				continue
			}
			// One source can have different runtime names in Hybrid and API-pool
			// modes. Use the newest alias for passive health, while adding today's
			// attempts across aliases so they do not disappear from the source row.
			if !ok || candidate.LastSeenAt > h.LastSeenAt {
				h = candidate
				ok = true
			}
			if today.Day == "" {
				today.Day, today.Kind, today.Provider = candidate.Today.Day, "provider", s.ID
			}
			if candidate.P95MS > h.P95MS {
				h.P95MS = candidate.P95MS
			}
			today.Requests += candidate.Today.Requests
			today.Successes += candidate.Today.Successes
			today.Failures += candidate.Today.Failures
			today.CacheHits += candidate.Today.CacheHits
			today.DurationMS += candidate.Today.DurationMS
			today.ResultCount += candidate.Today.ResultCount
		}
		if !ok {
			s.Status = "unknown"
			s.State = "unknown"
			continue
		}
		s.Status = h.Status
		s.LastSuccessAt = h.LastSuccessAt
		s.LastFailureAt = h.LastFailureAt
		s.LastSeenAt = h.LastSeenAt
		s.FailureRate = h.FailureRate
		s.SampleSize = h.SampleSize
		s.ConsecutiveFailures = h.ConsecutiveFailures
		s.RecentOutcomes = h.RecentOutcomes
		s.Today = today
		s.LastError = h.LastError
		s.State = h.State
		if s.State == "" {
			s.State = h.Status
		}
		if h.SuspendedUntil != "" {
			s.Suspended = true
			s.SuspendedUntil = h.SuspendedUntil
			s.SuspendReason = h.SuspendReason
			s.SuspendCountdown = h.SuspendCountdown
		} else {
			s.Suspended = false
		}
		s.P95MS = h.P95MS
		s.ErrorKinds = h.ErrorKinds
	}
	return defs
}

func summarizeSystem(sources []SourceView) SystemSummary {
	out := SystemSummary{Status: "waiting"}
	for _, s := range sources {
		if !s.Active {
			continue
		}
		out.Enabled++
		if s.Suspended || s.State == "suspended" {
			out.Suspended++
		}
		switch s.Status {
		case "down":
			out.Down++
			out.Observed++
		case "degraded":
			out.Degraded++
			out.Observed++
		case "healthy":
			out.Observed++
		}
	}
	switch {
	case out.Down > 0:
		out.Status = "down"
	case out.Degraded > 0:
		out.Status = "degraded"
	case out.Observed == 0:
		out.Status = "waiting"
	default:
		out.Status = "healthy"
	}
	return out
}

func webSourceCatalog(conf config.Config) []SourceView {
	mode := conf.GetMode()
	apiConfigured := map[string]bool{
		"anysearch": len(conf.Anysearch.EffectiveSKList()) > 0,
		"baidu":     len(conf.Baidu.EffectiveSKList()) > 0,
		"tavily":    len(conf.Tavily.EffectiveSKList()) > 0,
		"exa":       len(conf.Exa.EffectiveSKList()) > 0,
		"doubao":    len(conf.Doubao.EffectiveSKList()) > 0,
	}
	engineConfigured := map[string]bool{
		"baidu_web":  conf.Baidu.WebEnabled,
		"bing":       conf.Bing.Enabled,
		"google":     conf.Google.Enabled,
		"duckduckgo": conf.DuckDuckGo.Enabled && conf.Proxy.NeedsProxy(),
	}
	active := map[string]bool{}
	activateAPI := func(name string) {
		if apiConfigured[name] {
			active[name] = true
		}
	}
	activateEngines := func() {
		for name, configured := range engineConfigured {
			if configured {
				active[name] = true
			}
		}
	}
	switch mode {
	case config.ModeHybrid:
		for name := range apiConfigured {
			activateAPI(name)
		}
		activateEngines()
	case config.ModeEngine:
		activateEngines()
	case config.ModeApipool:
		for _, name := range conf.Apipool.GetEngines() {
			activateAPI(strings.ToLower(name))
		}
		if engineConfigured["baidu_web"] {
			active["baidu_web"] = true
		}
	case config.ModeBaidu:
		if apiConfigured["baidu"] {
			active["baidu"] = true
			if engineConfigured["baidu_web"] {
				active["baidu_web"] = true
			}
		} else if engineConfigured["baidu_web"] {
			active["baidu_web"] = true
		} else if engineConfigured["bing"] {
			active["bing"] = true
		}
	default:
		selected := mode
		if apiConfigured[selected] {
			active[selected] = true
		} else if engineConfigured["bing"] {
			active["bing"] = true
		}
	}

	baiduAPIAliases := []string{"baidu_ai", "baidu_api"}
	baiduWebAliases := []string{"baidu"}
	if mode == config.ModeApipool {
		baiduAPIAliases = append(baiduAPIAliases, "baidu")
		baiduWebAliases = []string{"baidu_web"}
	}
	return []SourceView{
		{ID: "anysearch", Name: "AnySearch", Group: "web", Configured: apiConfigured["anysearch"], Active: active["anysearch"], aliases: []string{"anysearch"}},
		{ID: "baidu", Name: "百度千帆", Group: "web", Configured: apiConfigured["baidu"], Active: active["baidu"], aliases: baiduAPIAliases},
		{ID: "tavily", Name: "Tavily", Group: "web", Configured: apiConfigured["tavily"], Active: active["tavily"], QuotaQueryable: true, aliases: []string{"tavily", "tavily_api"}},
		{ID: "exa", Name: "Exa", Group: "web", Configured: apiConfigured["exa"], Active: active["exa"], aliases: []string{"exa"}},
		{ID: "doubao", Name: "豆包搜索", Group: "web", Configured: apiConfigured["doubao"], Active: active["doubao"], aliases: []string{"doubao"}},
		{ID: "baidu_web", Name: "百度网页", Group: "web", Configured: engineConfigured["baidu_web"], Active: active["baidu_web"], aliases: baiduWebAliases},
		{ID: "bing", Name: "Bing", Group: "web", Configured: engineConfigured["bing"], Active: active["bing"], aliases: []string{"bing"}},
		{ID: "google", Name: "Google", Group: "web", Configured: engineConfigured["google"], Active: active["google"], aliases: []string{"google"}},
		{ID: "duckduckgo", Name: "DuckDuckGo", Group: "web", Configured: engineConfigured["duckduckgo"], Active: active["duckduckgo"], aliases: []string{"duckduckgo"}},
	}
}

func academicSourceCatalog(conf config.Config) []SourceView {
	enabled := conf.Academic.Enabled
	defs := []struct {
		id       string
		name     string
		disabled bool
		intlOnly bool
	}{
		{"arxiv", "arXiv", conf.Academic.DisableArxiv, false},
		{"crossref", "Crossref", conf.Academic.DisableCrossref, false},
		{"openalex", "OpenAlex", conf.Academic.DisableOpenAlex, false},
		{"pubmed", "PubMed", conf.Academic.DisablePubMed, false},
		{"europepmc", "Europe PMC", conf.Academic.DisableEuropePMC, false},
		{"dblp", "DBLP", conf.Academic.DisableDBLP, false},
		{"doaj", "DOAJ", conf.Academic.DisableDOAJ, false},
		{"semantic_scholar", "Semantic Scholar", conf.Academic.DisableSemanticScholar, true},
		{"google_scholar", "Google Scholar", conf.Academic.DisableGoogleScholar, true},
	}
	out := make([]SourceView, 0, len(defs))
	for _, d := range defs {
		configured := enabled && !d.disabled
		active := configured && (!d.intlOnly || (conf.IsInternational() && conf.Proxy.NeedsProxy()))
		out = append(out, SourceView{ID: d.id, Name: d.name, Group: "academic", Configured: configured, Active: active, aliases: []string{d.id}})
	}
	return out
}

func activeSourceIDs(sources []SourceView) []string {
	var out []string
	for _, source := range sources {
		if source.Active {
			out = append(out, source.ID)
		}
	}
	slices.Sort(out)
	return out
}
