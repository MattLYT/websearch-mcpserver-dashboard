# T16 smartsearch 搜后浅抽取 `fetch_top_n`

- 来源：[issues/smartsearch.md](../issues/smartsearch.md) P0-1
- 优先级：P0
- 状态：完成（2026-09-13，enrichTopN + cache key 带 n）
- 依赖：无。与 [T18](T18-cleanfetch-reuse-summarizer.md) 互补：本项补正文，T18 按 intent 压缩；有 intent 时摘要输入用抽取后的 `Content`

## 为什么做（agent loop）

`Content` 现在是引擎 snippet。Agent 要读原文必须再调 N 次 `cleanfetch`，多数 MCP 客户端是串行工具调用，这是搜索链路最贵的往返。默认 0 保持现状；需要时一次搜索带回 Top-N 正文，少 N 轮工具往返。复用 `cleanfetch` 同一条 webfetch + SSRF，不接托管 extract。

## 要改什么

`smartsearch` 增加可选 `fetch_top_n`（int，默认 0，上限 5）：

- 在 hybrid **评分/截断之后**，对前 N 条并发走现有 `validateURLSecurity` + `webfetchInst.Fetch`（与 cleanfetch 同一路径）。
- 单条失败跳过，保留 snippet，整次搜索不失败。
- 内网 URL 不得因抽取被打到。
- 超时吃搜索剩余预算，不要另开无限等待。
- 缓存：当时没抽正文的 raw hit 不能当完整命中；cache key 带上 n，或命中后再按本次 n 补抽。
- LLM 摘要若开启：摘要输入用抽取后的正文，不是 snippet。

实现落点：`mcp/tool.go` `doWebSearch`。**不要**给 `SearchInf.SearchRaw` 加 fetch 参数，引擎适配器保持只搜。

## 不要改

- 默认抽取（0 = 现状）
- 不改 `SearchInf` / 九源 `Search()` / hybrid 签名
- 不接 Firecrawl/Tavily extract；不在本项做 crawl/map
- 不加 `include_domains` / `category` / `lang`

## 验收

- `fetch_top_n=0` 与现输出一致
- mock webfetch：N=2 时只有前 2 条 `Content` 被替换；第 2 条失败时第 1 条仍有正文、第 2 条保留 snippet
- 内网 URL 不被抽取
- `go test ./mcp/...`
