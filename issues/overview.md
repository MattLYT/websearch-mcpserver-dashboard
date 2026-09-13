# 工具链吸收 — 总方案

> 评估日期：2026-09-13
> 对照提交：`master` @ `6850845`（v3.4.0）
> 原则：本地优先、零 Key 可跑、单二进制、四工具短链。只吸收贴现有架构的特性，不跟托管爬虫平台。

## 筛选标准（校正）

不要把「Agent 可以再调一次工具」当成更优默认。MCP 往返（尤其串行客户端）比多一个可选参数更贵。立项只看：

1. **往返 / 配额 / 上下文**：少一轮 tool call、少一次九源空转、或避免整页/整本灌爆后又被迫再调。
2. **复用现有模块**：webfetch、summarizer、SearchInf、SSRF；禁止再做一套 Jaccard/摘录算法。
3. **不拆统一封装**：不改 `SearchInf` / `Engine.Search()` / `TimeRange` 去迁就单个供应商；新引擎必须挂进现有接口；无 Key 时 factory 行为不变。
4. **冷门能力 lazy**：图搜、JS 运行时、引用图可视化、Docling 等等用户开口。

曾误判：把 `fetch_top_n` / 批量 URL / PDF 分页当成「破坏原生多轮」而放弃。校正后它们是**压缩多轮**，不是替代 Agent 决策。

## 当前工具面（已核实）

| 工具 | 参数 | 实现入口 |
|------|------|----------|
| `smartsearch` | `query` / `intent`（LLM 开时）/ `time_range`（月） | `mcp/tool.go` `SearchParams*` → `doWebSearch` |
| `academicsearch` | `query` / `engines` / `time_range`（year\|month\|week\|day）/ `page` | `AcademicSearchParams` → `doAcademicSearch` |
| `cleanfetch` | `url` | `CleanFetchParams` → `CleanFetch` |
| `pdf_parser` | `path` | `PDFParserParams` → `PDFParserHandler` |

配置层已有、工具层够不到的能力：`black_list_host`、Tavily/Exa `include/exclude_domains`、`smartsearch.max_size`、Tavily 固定 `search_depth=basic`（`pkg/search/tavily.go`）。学术结果已有 `PDFURL`（`pkg/search/inf.go`），但停在列表，没有 OA 补链 / 引用图 / 解析衔接。

## 对照图谱

| 角色 | 代表 | 强项 | 对本项目 |
|------|------|------|----------|
| 厂商 MCP | Tavily / Exa / Firecrawl | 搜+抽一体、crawl/map、JS 渲染、schema 抽取 | 学结果深度，不搬托管爬虫 |
| 大厂 grounding | Bing Grounding、Gemini Search、Perplexity | market/lang、freshness、域名白名单 | 学工具层可操纵参数 |
| 独立索引 | Brave MCP | news/freshness | **不做**：免费档仍要信用卡，鸡肋 |
| 自托管元搜 | SearXNG MCP、MetaSearchMCP、gawirable/websearch-mcp | 分类、Playwright、trust score | 学 news 分类与可选 JS 回退 |
| 学术专用 | Consensus、paper-search-mcp、aletheia-mcp | OA 解析、引用图、DOI 取文 | 学术从「搜」补到「研」 |
| 安全向 | charley-forey/web-search-mcp、Rover | `<untrusted_content>`、隐藏字符剥离、robots | 低成本高收益 |

已经领先、不必再立项：零 Key 国内引擎、hybrid RRF+MMR、9 源学术 + DOI 去重、SSRF/DNS rebinding、MinerU 扫描件回退、apipool 加权、系统代理。v2.14 评分管线不要重复做。

## 最小闭环（目标态）

```
smartsearch(可选 fetch_top_n 带正文)
  → academicsearch(query 识别 DOI/arXiv + 合并后补 OA)
  → cleanfetch(可选 intent 复用摘要、可选批量)
  → pdf_parser(path 吃远程 URL、可选 pages / max_pages)
```

默认行为不变：`fetch_top_n=0`、无浏览器运行时。

可执行任务：[tasks/README.md](../tasks/README.md) T15–T20。不要一个 PR 塞完全表。

## 分期（按往返收益，不再按「工具参数大礼包」）

```
P0  ──►  T15 远程 PDF → T16 fetch_top_n → T17 DOI/OA
P1  ──►  T18 复用 summarizer → T19 pages → T20 批量 URL
```

| 结论 | 项 | 往返账 |
|------|----|--------|
| 做 | 远程 `path`（T15） | 学术 `pdf_url` 现在会走错工具 |
| 做 | `fetch_top_n`（T16） | 1 次搜索 vs 搜索 + N 次 cleanfetch |
| 做 | DOI-as-query + OA 补链（T17） | 少九路空转；少付费墙空 fetch |
| 做 | cleanfetch 复用 summarizer（T18） | 少「页面太大再总结」一轮；解耦 SearchResult |
| 做 | PDF `pages` / `max_pages`（T19） | 避免整本灌爆后再调一次 |
| 做 | 批量 `urls`（T20） | 1 次 vs K 次 fetch；与 T16 分工（未选定 URL vs 已有 URL） |
| 不做 | Brave | 免费 500 次仍要信用卡，不接 |
| 不做 | `max_results` / domains / news / lang | 不省往返；`site:` 与 yaml 黑名单已有；news/lang 会按引擎分叉 |
| 不做 | Tavily `search_depth` 进 schema | 单腿参数，破坏统一工具面；最多以后做配置 |
| 不做 | 引用图 / year_min / oa_only | 新模式或改 TimeRange；不是搜后必经下一跳 |
| 不做 | Jaccard 摘录 | 用 T18 同一 summarizer |
| 不做 | 不可信包裹 / JS / map / Docling | 不省往返；安全与运行时等开口 |

## 配置变更约定

新增项必须同步：

- `pkg/config/config.go`
- `config.example.yaml`
- `pkg/config/config.example.yaml`
- `docs/configuration.md` + `docs/configuration.en.md`
- `docs/search.md`（工具参数表）+ `docs/search.en.md`

环境变量沿用现有风格（`TAVILY_SK` / `UNPAYWALL_EMAIL` 这类供应商自己的名字），不要强行 `WEBCRAWLER_`。

## 明确不做

- 整站 crawl、浏览器 interact（点击/填表）、截图、schema LLM 抽取
- 图搜 / 视搜 / 本地商户 / YouTube 转录 / Python sandbox
- Exa `find_similar`、Firecrawl Agent、把 4 工具拆成 8–10 个
- 再堆评分算法（RRF/域名/词汇/MMR 已在 v2.14）
- Google JS 挑战绕过、Sci-Hub、Marker 作默认 PDF 引擎（GPL + 非 Go）
- 默认引入 Playwright/chromedp 运行时（P2 仅可选开关，默认关）
- Brave Search API（免费档仍要信用卡，不接）

## 验收总表

| 波次 | 行为 |
|------|------|
| P0 | 不传新参数时与 v3.4.0 一致；`fetch_top_n=3` 时 Top-3 带正文且失败跳过；`path=https://.../x.pdf` 不拼 `file://`；query 为 DOI 时不打九路关键词；无邮箱则不打 Unpaywall |
| P1 | LLM 开且 cleanfetch `intent` 非空时走同一 summarizer；`urls` 上限 5、单条失败不影响其它；`pages`/`max_pages` 截断可见 |

各工具的现状、方案、测试见同目录分册。落地任务见 [`../tasks/README.md`](../tasks/README.md)。
