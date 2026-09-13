# pdf_parser — 远程 URL、分页、结构

> 对照：[overview.md](overview.md)
> 入口：`mcp/tool.go` `PDFParserHandler`；`PDFParserParams` 只有 `path`
> 解析：`pkg/webfetch/webfetch.go` `parseLocalPDF`（ledongthuc/pdf → 可选 MinerU OCR）；远程 `.pdf` 可走 MinerU 精准 API

2026 评测里 MinerU 仍是公式/表格/中文扫描件第一档；Marker 吞吐更高但 GPL；Docling MIT、CPU 友好、结构弱一些。主引擎不要换。本册只补**接口与分页**，让学术 `pdf_url` 接得上。

---

## P0-1 schema 接受远程 URL

落地任务：[T15](../tasks/T15-pdf-parser-remote-url.md)（**不加**独立 `url` 参数，现有 `path` 接受 http(s)）

### 现状

- `docs/search.md` 写 `path` 为「本地 PDF 文件路径或远程 URL」。
- `PDFParserParams` 只有 `Path`，描述是本地 / `file://`。
- `Fetcher.Fetch` 已能处理 `https://.../x.pdf`（`mineruRemotePDF` + webfetch）。
- `CleanFetch` 对网页 URL 做 SSRF；`PDFParserHandler` 只拼 `file://`，**远程 URL 若塞进 path 会被加成 `file:///https://...`**。

文档与实现不一致，学术 PDF 链接无法直接喂入。

### 方案

参数改为（向后兼容）：

```text
path  string  本地路径或 file://（现行为）
url   string  http(s) PDF；与 path 互斥，必须有一者
```

- `url`：先 `validateURLSecurity` + HEAD（复用 `headCheck`，限制 content-type / 大小），再 `webfetchInst.Fetch(ctx, url)`。
- 非 `.pdf` 后缀但 `Content-Type: application/pdf` 也允许（HEAD 已有类型信息的话）。
- 工具描述与 `docs/search.md` 对齐：学术结果的 `pdf_url` 用本参数。

`cleanfetch` 遇到 PDF URL 仍可走现有 MinerU 分支，不必强制用户换工具；`pdf_parser` 是明确入口。

### 测试

- `url=https://example.com/a.pdf` 不经过 `file://` 拼接。
- 内网 URL 拒绝。
- 只传 path 的旧调用不变。
- path 与 url 同时传 → 参数错误。

---

## P0-2 页范围 `pages`

落地任务：[T19](../tasks/T19-pdf-parser-pages.md)

### 现状

整本抽取。长论文一次灌进上下文会爆；大文档已经 `saved_to_file`，但模型仍可能把几百页当一次任务。

### 方案

```text
pages  string  可选。例：`1-10`、`1,3,5-7`。省略 = 前 max_pages（默认 20）页
               并提示截断（落地行为，v3.4.0 起生效；本句已按 F7 更正）。
               上限可配，默认最多 20 页/次（MinerU Agent 本就有 20 页档）。
```

- 本地库路径：能按页抽的按页抽；ledongthuc/pdf 若只能全文，先全文再按页切（差但可用），或文档写明「本地库忽略 pages、仅 MinerU 生效」——优先真正按页，避免静默忽略。
- MinerU：查现有 API 是否支持页范围；不支持则下载后本地切页再 OCR（注意体积）。
- 超上限报错，让模型拆多次调用。

### 测试

- `pages=1-2` 的输出明显短于全文（用仓库里现有测试 PDF 或最小夹具）。
- 非法 `pages=abc` → 参数错误。
- 省略 `pages` = 前 20 页（max_pages 默认值），见下方落地缺口 F7。

### 落地缺口

见 [absorb-followup.md](absorb-followup.md) F2（`parsePagesSpec` 先展开 `"1-N"` 再比 `max_pages`，可 OOM）和 F7（省略 `pages` 实际是前 20 页，不是「全部」；上表方案句已过时）。远程 URL 的 HEAD 重定向缺口见 F3。

---

## P1 表格 / 公式结构保留

### 现状

MinerU 能出 HTML 表 + LaTeX 公式；当前多半揉成一块 Markdown（`ParseFile`/`ParseURL` 返回 `string`）。学术 PDF 下游更吃结构。

### 方案

有 MinerU 结果时：

- 表格尽量保持 Markdown 表或 HTML `<table>`（二选一，文档写死），不要拍扁成纯文本行。
- 公式保留 `$...$` / `$$...$$`。
- 无 MinerU、仅本地文本层：不强求，维持现状。

不新增 `format=` 参数，除非 Markdown 过大需要 `json` 开关（默认仍 Markdown）。

### 不做

- 不把 Marker/LlamaParse 设为默认。
- 不在无 Token 时强制上传云端。

---

## P2 Docling 仅作无 Token CPU 备选（可选，默关）

仅当用户明确要「完全离线 OCR、无 MinerU」时考虑。Docling 是 Python，**不能链进当前纯 Go 二进制**。可行形态：

- 外部命令：`pdf_parser.docling_cmd` 指向本机 `docling`，扫描件且 `mineru_ocr=false` 时 spawn。
- 默认空 = 永不调用。

GPL 的 Marker 不进默认文档推荐。LlamaParse / Adobe 是云上传，只允许用户自己配 Key 的可选后端（本波甚至不必写代码）。

### 不做

- 不把 Python 解释器打进 Docker 默认镜像。
- 不换掉 ledongthuc + MinerU 主路径。

---

## 与学术工具的衔接

依赖本册 P0-1。`academicsearch` 描述里写：有 `pdf_url` 时调用 `pdf_parser` 的 `url`。不要在学术搜索里内嵌下载。
