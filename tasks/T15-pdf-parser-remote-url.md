# T15 pdf_parser 的 path 接受远程 URL

- 来源：[issues/pdf-parser.md](../issues/pdf-parser.md) P0-1（按 agent loop 收益收窄：只修断裂闭环，不加参数）
- 优先级：P0
- 状态：完成（2026-09-13，resolvePDFPath + validateURLSecurity/headCheck）
- 依赖：无
- 后继：[T17](T17-academic-doi-oa.md) 的 `pdf_url` 才能直接喂进来；[T19](T19-pdf-parser-pages.md) 在同一 handler 上加页控制

## 为什么做（agent loop）

学术结果已经带 `pdf_url`，文档也写 `path` 可以是远程 URL，但 `PDFParserHandler` 无条件拼 `file://`，`https://.../x.pdf` 会变成 `file:///https://...`。Agent 只能改调 `cleanfetch` 或放弃全文，多一轮错误工具调用。

## 要改什么

1. `mcp/tool.go` `PDFParserHandler`：
   - `http://` / `https://`：走现有 `validateURLSecurity` + `headCheck`，再 `webfetchInst.Fetch(ctx, url)`，**禁止**加 `file://`
   - 本地路径 / `file://`：保持现在的三斜杠拼接
2. `PDFParserParams.Path` 的 jsonschema 与 `mcp/server.go` 工具描述改成「本地路径或 http(s) PDF URL」，与 `docs/search.md` 对齐
3. 非 `.pdf` 后缀但 HEAD 已是 `application/pdf` 的，交给现有 webfetch（不要新参数、不要新接口）

配置 / 文档：不新增 yaml 项。`docs/search.md` 已写远程 URL 的，核一下实现一致即可；`docs/search.en.md`、`docs/api.md` 若仍写「仅本地」则改一句。

## 不要改

- 不新增 `url` / `pages` 参数
- 不改 `pkg/webfetch` 的 `Fetch` 签名（远程 PDF + MinerU 分支已有）
- 不改 `SearchInf` / 学术引擎
- 不在 `academicsearch` 里自动下载 PDF
- 不上 Docling、不改表格/公式拍扁逻辑

## 验收

- `path=https://example.com/a.pdf` 不经过 `file://` 拼接（mock webfetch 或 httptest）
- 内网 URL 被 `validateURLSecurity` 拒绝
- 只传本地 path 的旧调用不变
- `go test ./mcp/... ./pkg/webfetch/...`
