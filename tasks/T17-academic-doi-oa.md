# T17 学术 query 识别 DOI/arXiv + 合并后补 OA PDF

- 来源：[issues/academicsearch.md](../issues/academicsearch.md) P1-A / P1-B（收窄：不加工具参数，不改九源 `Search()`）
- 优先级：P0
- 状态：完成（2026-09-13，pkg/academic lookup/paperid/unpaywall）
- 依赖：工具描述里「把 `pdf_url` 交给 `pdf_parser`」建议 [T15](T15-pdf-parser-remote-url.md) 先合；代码无硬依赖

## 为什么做（agent loop）

1. Agent 手里已有 DOI / arXiv id 时仍走九源关键词并行，浪费配额，还可能搜不回同一篇。
2. Crossref 等命中经常没有 `PDFURL`，下一跳 `pdf_parser` 接不上，Agent 只能去抓出版商 HTML（付费墙 / 空转）。

都不需要新 MCP 工具或新参数：识别现有 `query`，补现有 `PDFURL` 字段。

## 要改什么

### A. query 识别后走单篇（adapter 内分流）

在 `AcademicAdapter.SearchAcademicRaw` **进入** `StrategyParallel` 关键词搜索之前：

- DOI：`antirobot.ExtractDOI` 已有。仅当 query 经 trim 后实质就是 DOI / `doi:...` / `https://doi.org/...` 时拦截（长句里顺带出现 DOI 仍走关键词，避免误伤）。
- arXiv id：`1706.03762`、`arXiv:1706.03762`、`arxiv.org/abs/...` 同类形式。

拦截后只打单篇 lookup（DOI：OpenAlex + Crossref 并发，DOI 去重最多 1 条；arXiv id：只打 arXiv），**不要**九路 `Engine.Search()`。

实现约束：

- **不要改** `antirobot.Engine.Search(query, page, timeRange)` 签名，也不要给九个引擎各塞一套 DOI 分支。
- Lookup 用 `pkg/academic` 里相对引擎的 helper，或可选接口（例如只有 Crossref/OpenAlex/arXiv 实现）；关键词 `Search()` 保持原语义。
- **不要改** `search.AcademicSearcher` / `SearchInf` / `TimeRange`。
- 缓存：`doAcademicSearch` 的 key 已含 query 字符串，DOI 当 query 时自然分开；不要和普通关键词结果互踩即可。

工具描述（`buildAcademicToolDescription`）：写清「已有 DOI 或 arXiv id 时直接作为 query，不要再用标题搜」。有 `pdf_url` 时交给 `pdf_parser` 的 `path`（远程 URL，见 T15）。

### B. 合并后补 OA（不是第十个引擎）

`SearchAcademicRaw` 合并去重之后、返回 / 写缓存之前：

1. 有 `DOI` 且 `PDFURL` 空：先修 OpenAlex **映射**——当前只读 `primary_location.pdf_url`（`pkg/academic/openalex.go`），漏了 `open_access.oa_url` / best OA。这是 parse 补字段，不是新 API。
2. 仍空：Unpaywall `GET /v2/{doi}?email=`。配置 `academic.unpaywall_email`，环境变量 `UNPAYWALL_EMAIL`。无邮箱则零 HTTP、不报错。
3. 只接受合法 OA（`is_oa` + `best_oa_location.url_for_pdf` 或等价）。失败静默，列表照出。
4. 并发钳制，不要对每条结果打爆 Unpaywall。
5. Unpaywall **不是** `Engine`，不要进 factory / `AcademicEngines()`。

配置同步：`pkg/config/config.go`、两份 `config.example.yaml`、`docs/configuration.md` + `docs/configuration.en.md`。`docs/search.md` 学术工具说明加 DOI-as-query 与 `pdf_url` → `pdf_parser`。

## 不要改

- 不新增 `doi` / `arxiv_id` / `graph` / `seed_doi` / `year_min` / `oa_only` 等 schema 字段
- 不改九源 `Search()`、不改 `TimeRange` 枚举语义
- 不做引用图、Sci-Hub、搜索阶段下 PDF、`fetch_pdf` 自动解析
- 不把 Unpaywall 接到 `smartsearch` / hybrid

## 验收

- `query=10.xxxx`（或 doi.org URL）：mock Crossref+OpenAlex 合并为 1 条；九源关键词 `Search()` 调用次数为 0
- `query=transformer attention` 含偶然 DOI 子串时仍走并行关键词（按「实质就是 DOI」规则写单测）
- 有 PDF 的条目不打 Unpaywall
- mock Unpaywall 200 + `url_for_pdf` → 结果带 `PDFURL`
- 无 `unpaywall_email` → 零 Unpaywall HTTP
- `go test ./pkg/academic/... ./pkg/search/... ./mcp/...`
