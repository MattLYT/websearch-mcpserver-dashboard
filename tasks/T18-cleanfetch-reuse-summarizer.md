# T18 cleanfetch 复用 LLM 摘要（解耦 SearchResult）

- 来源：[issues/cleanfetch.md](../issues/cleanfetch.md) P1-1（纠正：不要 Jaccard 切块；复用已有 summarizer）
- 优先级：P1
- 状态：不做（2026-09-13 决策）
- 依赖：无。与 [T16](T16-smartsearch-fetch-top-n.md) 互补（搜索侧补正文后走同一 Source）；[T20](T20-cleanfetch-batch-urls.md) 应复用本项抽出的 fetch/摘要 helper

> **决策（2026-09-13）**：不做。用户明确「不要把工具耦合太狠，尽量让 agent 自己选择组合」：
> cleanfetch 保持纯抓取，不加 intent 内嵌 LLM 摘要；summarizer 解耦 `SearchResult` 因失去消费者一并搁置。
> 若后续仍需压缩大页，优先考虑独立 summarize 工具或复用 smartsearch(intent, fetch_top_n)。

## 为什么做（agent loop + 抽象）

大页要么灌爆上下文，要么 `saved_to_file` 丢给 Agent 一条它经常读不到的本地路径，等于白抓一轮。`smartsearch` 在 LLM 开启时已经用 `intent` 把多条 snippet 收成摘要。`cleanfetch` 不应再做一套 `enhance_text` 摘录。

现状：`pkg/summarizer` 的 `Summarize` / `BuildUserPrompt` / `FormatCitation` 直接吃 `[]search.SearchResult`，工具层无法对「单页正文」复用。先解开这个耦合，两个工具共用同一个 `summarizerInst` 和现有流式 progress。

## 要改什么

### 1. summarizer 与搜索结果类型解耦

`pkg/summarizer` **不要再 import** `pkg/search`。

```go
type Source struct {
    Title   string
    URL     string
    Content string
}

func (s *Summarizer) Summarize(query, intent string, sources []Source) (string, error)
func (s *Summarizer) SummarizeStream(ctx, query, intent string, sources []Source, ch, errCh)
```

- Prompt 把「搜索结果」改成中性的「来源材料」（N=1 与 N>1 同一套）。
- `mcp/tool.go` 的 `doWebSearch` / `streamSummarize`：把 `[]search.SearchResult` 映成 `[]summarizer.Source`（只在 MCP 层转，不要把转换塞回引擎包）。
- 搜索路径的对外行为保持不变（intent 空不摘要；有 intent 则摘要 + 引用格式）。

### 2. cleanfetch 按现有 LLM 裁剪 schema

对齐 `smartsearch`：`LLMEnabled()` 才出现可选 `intent`。

- 无 LLM：params 仍只有 `url`（现行为）。
- 有 LLM：`intent` 可选。空 = 仍返回全文 / `saved_to_file`。非空 = 抓取成功后走**同一** `summarizerInst`（优先现有 `streamSummarize` 进度通道）。
- 入参：`query` 可用 URL 或页面 title；`intent` 即 Agent 本轮问题；`sources` 一条（Title/URL/Markdown）。
- `saved_to_file` 时：摘要输入用已经抽出的正文（`Result.Markdown`；若空则读本次写出的 `FilePath`），不要另接抓取管道。进 LLM 的材料设字符上限，避免把整本小说塞进 prompt（可复用或对齐现有 user prompt 体量；超限截断并在 preamble 说明已截）。
- 摘要失败：打日志，**回退原文**（inline 或文件路径），不让整次 fetch 失败。

`mcp/server.go`：LLM 开启时工具描述加一句「可用 intent 按问题压缩页面」。注册方式可仿 `WebSearchWithIntent` / `NoIntent` 两套 params，避免无 LLM 时 schema 多一个死参数。

文档：`docs/search.md` / `docs/search.en.md` 的 cleanfetch 参数表与 `smartsearch` 的 intent 说明对齐。

## 不要改

- 不上 Jaccard / `enhance_text` 切块，不新增 `query` 摘录参数（intent 已表达「按问题收」）
- 不新增 `urls`、`mode=links`、JS 回退（批量见 T20）
- 不改 `llm.Client` 的 Chat 签名，不改 webfetch `Fetch` 签名
- 不把 summarizer 接到九源 `Engine.Search` / `SearchInf`
- 本任务不改 `pdf_parser`（同一套 Source 以后可接，本波不做）
- 无 intent 时输出必须与现在一致

## 验收

- `go test ./pkg/summarizer/... ./mcp/...`（补 summarizer 单测：Source 与 SearchResult 映射后 prompt 含标题/URL/正文）
- 无 LLM：cleanfetch schema 无 `intent`；有 LLM：有可选 `intent`
- mock LLM：`intent` 非空时只把页面 Markdown 送进 summarizer，返回摘要；LLM 错误时仍有原文
- `intent` 空：不调 LLM，snapshot 与现输出一致
