# T19 pdf_parser 页范围与最大页

- 来源：[issues/pdf-parser.md](../issues/pdf-parser.md) P0-2
- 优先级：P1
- 状态：完成（2026-09-13，go-webfetch 页选择支持 + pages 参数 + max_pages 截断 preamble）
- 依赖：[T15](T15-pdf-parser-remote-url.md)（远程 URL 与本地同一 handler 上加页控制）

## 为什么做（agent loop）

长论文一次灌进上下文会爆；`saved_to_file` 的路径 Agent 常常读不到，等于白解析。学术闭环里第一跳通常只要摘要区/前若干页。配置层 `max_pages` 默认钳制成本；可选 `pages` 让 Agent 按需拆多次调用。

## 要改什么

1. 配置 `pdf_parser.max_pages`，默认 **20**（与 MinerU 档对齐）。省略 `pages` 时：总页数超过上限则只处理前 `max_pages`，并在 preamble 写明「已截断，用 pages 继续」。不要静默丢页还不提示。
2. 工具可选 `pages` 字符串：`1-10`、`1,3,5-7`。省略 = 受 max_pages 约束的「从首页起」。非法串 → 参数错误。显式 `pages` 的页数超过 max_pages → 参数错误，让模型拆多次。
3. **禁止只对 MinerU 生效、本地库静默忽略。** 能按页抽的按页抽；ledongthuc 若只能全文，先抽再按页切（差但可见）。MinerU：有页范围 API 则用；没有则本地下载切页再送，不要改 `SearchInf`。
4. `Fetcher.Fetch` 若需要页信息：用可选 `FetchOption` / 新方法，保持无 option 时旧行为。不要把 pages 泄漏到搜索引擎接口。

文档：`docs/search.md` + `.en.md` 参数表；`docs/configuration.md` 写 `max_pages`。

## 不要改

- 不新增第二个 `url` 参数（远程继续走 T15 的 `path`）
- 不上 Docling / Marker
- 不在 `academicsearch` 内自动下 PDF
- 不改表格/公式拍扁（仍 lazy）

## 验收

- `pages=1-2` 的输出明显短于全文（现有测试 PDF 或最小夹具）
- `pages=abc` → 参数错误
- 省略 `pages` 且页数少于 max_pages = 与 T15 之后的旧行为一致
- 省略 `pages` 且页数远大于 20 → 截断并有说明
- `go test ./mcp/... ./pkg/webfetch/...`
