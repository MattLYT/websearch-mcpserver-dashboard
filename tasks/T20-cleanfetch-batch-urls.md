# T20 cleanfetch 批量 URL

- 来源：[issues/cleanfetch.md](../issues/cleanfetch.md) P1-2
- 优先级：P1
- 状态：完成（2026-09-13，fetchCleanPage helper + urls 批量上限 5）
- 依赖：建议 [T18](T18-cleanfetch-reuse-summarizer.md) 先合或同一 PR 抽出 `fetchOne`，避免单 URL / 批量两套抓取

## 为什么做（agent loop）

Agent 已拿到 Top-K URL 时，串行 K 次 `cleanfetch` 全是协议往返。客户端经常不能真正并行工具调用。一次最多 5 个 URL、单条失败不影响其它，能少 K-1 轮。复用现有单 URL 预检 + webfetch + Jina，**不改** `Fetcher.Fetch` 签名。

与 [T16](T16-smartsearch-fetch-top-n.md) 分工：T16 是「还没选定 URL、跟在搜索后面」；本项是「手里已经有 URL」。

## 要改什么

兼容入参（不要 breaking）：

```text
url   string
urls  []string   与 url 至少一者非空；合计上限 5
```

- 两者都空 → 参数错误；超过 5 → 参数错误。
- `url` + `urls` 合并去重后计数。
- 并发抓取；每条独立 `validateURLSecurity` + `headCheck`；单条失败记入该条错误，其它成功仍返回。
- 输出按 URL 分节 Markdown。
- 有 `intent`（T18）时：**每条独立**走同一 summarizer helper，不要把多页拼成一次 prompt（避免串台、也避免改 summarizer 语义）。

实现落点：`mcp/tool.go` `CleanFetch`。循环调现有 Fetch，不要给 webfetch 加 Batch 接口（除非内部 private helper）。

## 不要改

- 不改 `Fetcher.Fetch(ctx, url)` 对外签名
- 不上 JS 回退、`mode=links`
- 不把批量塞进 `smartsearch`（那是 T16）

## 验收

- 3 个 URL、中间一个 SSRF 拒绝，另外两个有正文
- 超过 5 个 → 参数错误
- 只传 `url` 的旧调用输出不变
- `go test ./mcp/...`
