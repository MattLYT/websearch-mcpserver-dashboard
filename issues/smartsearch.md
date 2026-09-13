# smartsearch — 结果深度与可操纵参数

> 对照：[overview.md](overview.md)
> 入口：`mcp/tool.go` `SearchParamsWithIntent` / `SearchParamsNoIntent` → `doWebSearch`
> 编排：`pkg/search/hybrid.go`；Tavily：`pkg/search/tavily.go`（`SearchDepth` 写死 `"basic"`）

大厂 grounding（Bing `count`/`market`/`set_lang`/`freshness`、Brave news、Exa advanced、Tavily `include_raw_content`）和自托管 MCP（`search_and_extract`）都让模型**当场**收窄或加深。本工具只有 `query` / `intent` / `time_range`，域名黑名单和 `max_size` 锁在 yaml 里。

---

## P0-1 搜后浅抽取 `fetch_top_n`

落地任务：[T16](../tasks/T16-smartsearch-fetch-top-n.md)

### 现状

`doWebSearch` 搜索完成后直接 `formatRawResults` / LLM 摘要。`Content` 是引擎 snippet，不是正文。模型要读原文必须再调 N 次 `cleanfetch`。

Tavily / Exa / Firecrawl 的默认产品形态是「搜索结果里就有可用正文」。本项目已有 webfetch + SSRF 预检，不必接托管抽取 API。

### 方案

`smartsearch` 增加可选参数：

```text
fetch_top_n  int  默认 0（保持现行为）。评分/截断之后，对前 N 条并发走现有 webfetch。
                 建议上限 5。单条失败跳过，保留 snippet，不让整次搜索失败。
```

约束：

- 走 `validateURLSecurity` + 现有 `Fetcher.Fetch`，与 `cleanfetch` 同一条路径。
- 命中缓存的 raw results 若当时没抽正文，按本次 `fetch_top_n` 再抽（或缓存 key 带上 n，避免把无正文结果当成完整命中）。
- LLM 摘要若开启：摘要输入用抽取后的正文，而不是 snippet。
- 超时吃搜索剩余预算，不要另开无限等待。

### 不做

- 不默认抽取（`0` = 现状）。
- 不在这一项做整站 crawl / map。
- 不把 Firecrawl/Tavily extract 当必依赖；有 Tavily Key 的深度抽取放到 P1。

### 测试

- `fetch_top_n=0` 与现输出一致。
- mock webfetch：N=2 时只有前 2 条 `Content` 被替换；第 2 条失败时第 1 条仍有正文、第 2 条保留 snippet。
- 安全：内网 URL 不得因抽取被打到。

### 落地缺口

见 [absorb-followup.md](absorb-followup.md) F1（webfetch 跟工具开关绑死，默认部署 `fetch_top_n>0` 静默 no-op）和 F4（未走 cleanfetch 的 HEAD 体积预检）。

---

## P0-2 工具层收窄参数

### 现状

可收窄能力都在配置：`smartsearch.max_size`、`black_list_host`、Tavily/Exa 构造时的 `includeDomains`/`excludeDomains`。工具 schema 没有 `max_results`、没有域名、没有 news 分类、没有语言/市场。

Bing Grounding 把 `count` / `market` / `set_lang` / `freshness` 暴露给 agent；Brave 有独立 `brave_news_search`；SearXNG MCP 有 `category`。本项目应用**参数**而不是新工具。

### 方案

给两套 `SearchParams*` 增加（均为可选，缺省 = 现配置）：

| 参数 | 类型 | 行为 |
|------|------|------|
| `max_results` | int | 覆盖本次 `smartsearch.max_size`；0 或省略用配置 |
| `include_domains` | []string | 白名单。Tavily/Exa/豆包能传上游的传上游，hybrid 其余引擎本地滤 |
| `exclude_domains` | []string | 与配置 `black_list_host` **并集**，不要覆盖掉管理员黑名单 |
| `category` | string | `web`（默认）\| `news`。news 时收紧时效（可把默认 `time_range` 从 3 个月收到 1）、引擎侧能映射的映射（Brave/Bing freshness、SearXNG 分类若以后接） |
| `lang` | string | 可选，透传给支持的引擎（Bing/DDG）；不支持的忽略 |

`time_range` 已有，对应 Bing `freshness`，不改语义。

实现落点：`doWebSearch` 收参数 → `SearchRaw` 前后过滤；需要改 `SearchInf` 的，优先在 hybrid/tool 层滤，避免每个适配器加签名。域名过滤提成 `pkg/search` 共用函数（豆包/AnySearch 已有本地黑名单，别再复制一份）。

### 不做

- 不新增 `news_search` / `image_search` / `video_search` 工具。
- 不接 Brave（免费档要信用卡，见 P1-2）。
- `intent` 继续只在 LLM 开启时出现。

### 测试

- 无新参数 = 旧 snapshot。
- `include_domains=["gov.cn"]` 丢掉其它主机。
- `exclude_domains` 与 yaml 黑名单同时生效。
- `max_results=3` 截断在评分/MMR 之后。

---

## P1-1 Tavily `search_depth` 可升级

### 现状

```go
SearchDepth: "basic",  // pkg/search/tavily.go SearchRaw
```

有 Key 的用户永远走 basic，advanced / `include_raw_content` 都没用上。

### 方案

- 配置 `tavily.search_depth`：`basic`（默认）\| `advanced`。
- 工具可选 `search_depth` 覆盖本次调用。
- `advanced` 成本更高，schema 写清「仅 Tavily 模式/hybrid 里的 Tavily 腿生效」。
- 不把 `include_raw_content` 默认打开（和 P0-1 本地 webfetch 重复烧配额）；若 `fetch_top_n>0` 且当前引擎是 Tavily，可优先用 API 正文、失败再 webfetch。

### 测试

- 默认请求 body 仍是 `basic`。
- 工具传 `advanced` 时 mock 看到字段。
- 无 Tavily Key 的 engine/hybrid 路径忽略该参数。

---

## P1-2 Brave 作为可选引擎 — **不做**

免费档（约 500 次/月）仍要绑信用卡，对本项目的零 Key / 配置裁剪路线是鸡肋。海外缺口继续用已有 Bing / DDG / Tavily / Exa，不新增 `pkg/search/brave.go`。

---

## 明确不做（本册）

- Brave Search API（信用卡门槛）。
- Exa `find_similar`、deep-reasoning、Firecrawl Agent。
- 把摘要拆成 Brave 式 `summarizer` 第二工具（已有 LLM `intent` 流式摘要）。
