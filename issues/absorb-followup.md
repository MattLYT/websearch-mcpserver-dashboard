# 工具链吸收落地后评审（T15–T20）

> 评估日期：2026-09-13
> 对照：`docs/issues-toolchain-absorb` @ `86ca0c3` vs `master` @ `6850845`（v3.4.0）
> 范围：分支已合入的 T15–T20 行为缺口。`pkg/` 重组与 factory 别名不在本册重开。
> 测试背景：落地会话已跑 `go test ./...` 全绿；下列项是测试夹具盖不住的路径。
> **2026-09-13 更新：F1–F7 已全部落地（F1 按方案 1 惰性初始化；F4 随 F1 同修；见 CHANGELOG Unreleased）。**

T18 已明确不做（见 [T18](../tasks/T18-cleanfetch-reuse-summarizer.md)）。其余任务卡标「完成」，但默认部署和恶意输入上仍有断裂。一次会话只修一波：先 F1+F2，再 F3，文档类可同 PR。

| ID | 严重度 | 项 | 落点 |
|----|--------|----|------|
| F1 | bug | `fetch_top_n` 默认部署静默 no-op | [smartsearch.md](smartsearch.md) P0-1 |
| F2 | bug | `pages` 范围先分配再截断，可 OOM | [pdf-parser.md](pdf-parser.md) P0-2 |
| F3 | 建议 | `headCheck` 跟随 302 不复查内网/metadata | [cleanfetch.md](cleanfetch.md) / 远程 PDF |
| F4 | 建议 | `fetch_top_n` 不做 HEAD 体积预检 | [smartsearch.md](smartsearch.md) P0-1 |
| F5 | 建议 | DOI/arXiv-as-query 未写进工具描述与 search 文档 | [academicsearch.md](academicsearch.md) P1-B |
| F6 | 建议 | `academic.unpaywall_email` 未进 configuration 文档 | [academicsearch.md](academicsearch.md) P1-A |
| F7 | 建议 | 省略 `pages` 默认前 20 页，升级静默截断 | [pdf-parser.md](pdf-parser.md) P0-2 |

---

## F1 `fetch_top_n` 在默认部署上是空操作

### 现状

`smartsearch` schema 与工具描述始终挂着 `fetch_top_n`（默认 0、上限 5）。`applyWebFetch`（`mcp/options.go`）只在 `cleanfetch.enabled` 或 `pdf_parser.enabled` 时构造 `webfetchInst`，两者默认都是关。零 Key / engine 模式里 `fetch_top_n=3` 走到 `fetchPageContent` 的 `"webfetch 未初始化"`，`enrichTopN` 把错误吞掉，返回值与 `0` 相同：无 log、无 tool error。

T16 验收写的是「与 cleanfetch 同一条 webfetch」；测试注入 `webfetchInst`，因此全绿盖不住这条。

### 方案

二选一，优先前者：

1. webfetch 与 cleanfetch/pdf_parser 工具开关解耦：首次 `fetch_top_n>0` 时 lazy 初始化即可（仍走现有 `webfetch.NewFromConfig` + `validateURLSecurity`）。
2. 若坚持跟工具开关绑定：`fetch_top_n>0` 且 `webfetchInst==nil` 时 **fail closed**，返回明确 tool error，不要当成功搜索。

`enrichTopN` 对 nil-engine / 安全拒绝至少打 warning；单条页面失败仍可跳过。

### 不做

- 不默认抽取（`0` 仍为现状）。
- 不把 webfetch 初始化绑到「必须开 cleanfetch 工具」。
- 不改 `SearchInf`。

### 测试

- 默认配置（cleanfetch/pdf_parser 关）+ `fetch_top_n=3`：要么真抓到正文，要么 tool error；禁止静默等于 `0`。
- 注入 webfetch 的旧单测保持。
- `fetch_top_n=0` 仍不碰网络。

---

## F2 `pages` 范围解析无界分配

### 现状

`parsePagesSpec`（`mcp/tool_pdf.go`）把 `"1-N"` 先展开成切片，`PDFParserHandler` 再比较 `len(pages) > max_pages`。`pages="1-2147483647"`（或任意巨大递增区间）在报错前分配并循环。MCP 通常绑 loopback、无鉴权，一次调用可 OOM。单测只覆盖 `1-3`、`1-6`。

### 方案

解析时带上 `max_pages`（或硬顶，例如 1000）：区间宽度超上限立刻返回参数错误，禁止 `for p := from; p <= to` 无界 fill。`from`/`to` 用 `Atoi` 后先算 `to-from+1`，再分配。

### 不做

- 不把 `max_pages` 默认改成无限来「绕过」这个问题。
- 不上流式解析器；字符串页码表仍然可以。

### 测试

- `pages="1-2147483647"` → 参数错误，内存不明显上涨。
- `pages="1-20"` 在默认 `max_pages=20` 下合法。
- `pages="1-21"` → 超过上限错误（现有行为保留）。

---

## F3 `headCheck` 跟随重定向不复查私网

### 现状

远程 `pdf_parser`（及现有 cleanfetch）的 `headCheck` 用默认 `http.Client`，跟随 redirect，无私网 / metadata 再检。`http://evil.example/x.pdf` → 302 `http://169.254.169.254/`：`validateURLSecurity` 只看原始公网 host，HEAD 打到链路本地。go-webfetch 的 `CheckPrivateIP` 同样只看原始 URL，客户端最多跟 5 次 redirect。T15 让 `path` 接受任意 http(s) 后这条更暴露。

### 方案

给 `headCheck` 设 `CheckRedirect`：每一跳对 Location 再跑与 `validateURLSecurity` 相同的私网/metadata 检查，并封顶跳数。理想情况 webfetch dial 也拦私网；至少不要把 HEAD 当成对任意 redirect 目标的未鉴权探针。

### 不做

- 不因此关掉远程 PDF。
- 不引入独立 SSRF 中间件包。

### 测试

- 公网 URL 302 到 `169.254.169.254` / RFC1918 → HEAD 拒绝。
- 公网到公网的 302 仍允许。

---

## F4 `fetch_top_n` 跳过 HEAD 体积预检

### 现状

`fetchPageContent` 做 `validateURLSecurity`，不做 `headCheck`。cleanfetch 仍按 `max_fetch_size_mb`（默认 10MB）HEAD。Top-N URL 来自搜索引擎，通常是 HTML，但命中大二进制/PDF 时可拉远超 cap，最多 5 路并发，只有 15s / 剩余 ctx 当刹车。失败同样被 `enrichTopN` 吞掉，看起来像「保留 snippet」。

T16 写明与 cleanfetch 同一路径。

### 方案

与 `fetchCleanPage` 共用 helper（SSRF + HEAD + fetch）。若搜索命中上 HEAD 过严，至少在 webfetch 侧给这条路径加字节上限。

依赖 F1：webfetch 真正会跑之后这条才有意义。

### 不做

- 不把搜索结果里的 PDF 自动转去 `pdf_parser`。
- 不把上限改成「无限制」。

### 测试

- mock HEAD `Content-Length` 超过 cap → 该条保留 snippet，其它条仍可抽。
- 内网 URL 仍拒绝（T16 已有，保持）。

---

## F5 DOI / arXiv-as-query 未写进契约

### 现状

T17 要求：用户已有 DOI 或 arXiv id 时把它当 `query`，不要标题搜索；有 `pdf_url` 则交给 `pdf_parser` 的 `path`。`buildAcademicToolDescription` 仍只列引擎。`docs/search.md` 学术参数表把 `query` 写成「搜索关键词」，没有 DOI/arXiv 短路。适配器（`pkg/search/adapter/academic_adapter.go`）识别到 DOI/arXiv 后忽略 `engines`/`time_range`/`page`，合理但未文档化。Agent 会继续关键词搜 + 对 `https://…` 调本地 `pdf_parser`。

### 方案

把 T17 那两句补进 `buildAcademicToolDescription`、`docs/search.md` / `docs/search.en.md` 学术节。可选：走 lookup 分支时打一条 info 日志。

### 不做

- 不新增 `doi` / `arxiv_id` schema 字段（落地已是 query 识别，与任务卡后期决策一致）。
- 不在 academicsearch 内自动下 PDF。

### 测试

- 文档与 schema description 含 DOI/arXiv 用法（人工或字符串断言）。
- 现有 lookup 单测保持。

---

## F6 `unpaywall_email` 配置文档缺口

### 现状

`academic.unpaywall_email` / `UNPAYWALL_EMAIL` 已在 `pkg/config/config.go` 与 `config.example.yaml` 接线。`docs/configuration.md` / `.en.md` 本波只补了 `pdf_parser.max_pages`。无邮箱则跳过 Unpaywall、不报错；只读 configuration 指南的人发现不了 OA 补链。环境变量表也未列 Unpaywall。

### 方案

在其它 academic 键旁写清：空 = 跳过、不报错；列出 `UNPAYWALL_EMAIL`。同步中英。

### 不做

- 不改成无邮箱就报错。
- 不强行改成 `WEBCRAWLER_` 前缀（overview 已约定跟供应商自己的名字）。

---

## F7 省略 `pages` 时默认前 20 页

### 现状

省略 `pages` 一律走 `GetMaxPages()` 默认 20（`mcp/tool_pdf.go` → `FetchPDFWithPages`）。已开启 `pdf_parser.enabled`、解析长本地 PDF 的用户会只拿到前 20 页；仅当 `PageCount` 有值才有截断 preamble。符合 T19，但是相对 v3.4.0 的静默默认变更，yaml 不必出现该键就会生效。MinerU 远程/OCR 仍可能回全文并加 note，本地库与 MinerU 输出长度会分叉。

`pdf-parser.md` 方案仍写「省略 = 全部」，与落地不一致。

### 方案

保留 cap。发布说明写明行为变化。文档（本册 + `pdf-parser.md` + search/configuration）改成「省略 = 从首页起、受 max_pages 约束」。yaml `0` 表示不限制，便于升级。不必改成「超过阈值才 cap」——那会让默认成本再次失控。

### 不做

- 不把默认改回「全文」来迁就旧调用。
- 不上 Docling。

### 测试

- 文档不再写「省略 = 全部」。
- 现有截断 preamble 单测保持。

---

## 明确不做（本册）

- 不重开 `pkg/` 重组、factory 别名、网络测试门控。
- 不复活 T18（summarizer 与 cleanfetch 解耦的决策有效）。
- 不把 overview「不做」表里的 Brave / 引用图 / JS / Docling 借本轮塞进来。
- 工作区未提交的 `.qwen/settings.json` 与本册无关。

## 建议落地顺序

```
P0  ──►  F1 fetch_top_n 真能抽  +  F2 pages 解析先限宽
P1  ──►  F3 HEAD redirect 再检私网（建议与远程 PDF 同修）
P2  ──►  F4 体积预检（依赖 F1）; F5–F7 文档与默认说明
```

F1 与 F2 无相互依赖，可同 PR。F5–F7 可并进文档 PR。
