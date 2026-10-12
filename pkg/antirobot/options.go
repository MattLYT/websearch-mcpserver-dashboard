package antirobot

// ── 学术引擎 Options ──

// ArxivOpts arXiv 预印本配置。
type ArxivOpts struct {
	Enabled bool
	PerSec  int // 每秒限流（默认/上限 1，宽松配置被钳制）
	PerMin  int // 每分钟限流（默认/上限 12，宽松配置被钳制）
}

// CrossrefOpts Crossref 学术元数据配置。
type CrossrefOpts struct {
	Enabled bool
}

// OpenAlexOpts OpenAlex 开放学术图谱配置。
type OpenAlexOpts struct {
	Enabled bool
	MailTo  string // Optional contact email.
	APIKey  string // Optional bearer token; empty uses the shared anonymous budget.
}

// SemanticScholarOpts Semantic Scholar 配置。
type SemanticScholarOpts struct {
	Enabled bool
	APIKey  string // 可选 API key（x-api-key 头）；匿名限流严格，带 key 连续 429 自动降级匿名
}

// PubMedOpts PubMed 生物医学文献配置。
type PubMedOpts struct {
	Enabled bool
}

// GoogleScholarOpts Google Scholar 学术搜索配置。
type GoogleScholarOpts struct {
	Enabled bool
	Domain  string // 可选自定义域名（默认 scholar.google.com）
}

// EuropePMCOpts Europe PMC 生物医学文献配置（PubMed 增补源）。
type EuropePMCOpts struct {
	Enabled bool
}

// DBLPOpts DBLP 计算机科学文献配置。
type DBLPOpts struct {
	Enabled bool
}

// DOAJOpts DOAJ 开放获取期刊文章配置。
type DOAJOpts struct {
	Enabled bool
}
