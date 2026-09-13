# academicsearch — 从「搜论文」到「研论文」

> 对照：[overview.md](overview.md)
> 入口：`mcp/tool.go` `AcademicSearchParams` → `doAcademicSearch`
> 适配：`pkg/search/academic_adapter.go`；结果字段：`pkg/search/inf.go`（已有 `DOI` / `PDFURL` / `CitedBy`）
> 引擎：`pkg/academic/*` 九源并行，DOI+URL 双键去重（v3.2）

Consensus / aletheia-mcp / paper-search-mcp 的差距不在「再堆一个引擎」，而在**单篇详情、合法 OA 全文、引用游走**。本工具停在列表页。

---

## P1-A Unpaywall / OpenAlex 补 OA PDF

落地任务：[T17](../tasks/T17-academic-doi-oa.md)（与 P1-B 合并，不新增 schema 字段）

### 现状

`SearchResult.PDFURL` 来自各引擎自带字段；`FormatPaperMD` 会打出来。很多 Crossref / OpenAlex 命中没有 PDF。模型看到论文但无法 `pdf_parser`。

paper-search-mcp、research-mcp 用 Unpaywall（邮箱即可）和 OpenAlex `open_access.oa_url` 做合法补链。

### 方案

搜索合并去重之后、写缓存之前：

1. 有 `DOI` 且 `PDFURL` 空 → 查 OpenAlex 已返回的 OA 字段（若本条来自 OpenAlex 且漏映射，先修映射，别多打一次）。
2. 仍空 → Unpaywall `GET /v2/{doi}?email=`（配置 `academic.unpaywall_email`，环境变量 `UNPAYWALL_EMAIL`）。无邮箱则跳过，不报错。
3. 只接受 `is_oa` + `best_oa_location.url_for_pdf`（或等价 OA URL），写入 `PDFURL`。
4. 并发限流（Unpaywall 礼貌：约 10万/天，本机仍应钳制，别打爆）。
5. 补链失败静默，列表照出。

### 不做

- Sci-Hub / 任何绕过出版商的镜像。
- 搜索阶段为每条结果下载 PDF（那是 `pdf_parser` / 可选衔接）。

### 测试

- 有 PDF 的条目不打 Unpaywall。
- mock Unpaywall 200 + `url_for_pdf` → 结果带链接。
- 无 email 配置 → 零 HTTP。

---

## P1-B 按 DOI / arXiv id 取单篇

落地任务：[T17](../tasks/T17-academic-doi-oa.md)（query 识别，不加 `doi` 参数）

### 现状

只有关键词搜索 + `page`。Consensus 有 `fetch`；aletheia 有 `get_paper_details`。Agent 已有 DOI 时还要再搜一遍，浪费配额且可能搜不回同一篇。

### 方案

**不新增 MCP 工具。** 给 `AcademicSearchParams` 增加可选：

```text
doi       string  非空则走单篇路径，忽略 query 的「搜索」语义（query 可空）
arxiv_id  string  同上，打 arXiv id 查询
```

`query` 与 `doi`/`arxiv_id` 都空 → 400。单篇路径：

- DOI：Crossref + OpenAlex（已有客户端）并发，DOI 去重后最多 1 条。
- arXiv id：只打 arXiv。
- 再走 P1-A 补 PDF。
- 缓存 key 用 `doi|...` / `arxiv|...`，不要和关键词搜索混。

工具描述写清：「已有 DOI 时用 doi，不要再用标题搜索。」落地后改为 query 识别、不加独立字段，但描述与 `docs/search.md` 仍未写清，见 [absorb-followup.md](absorb-followup.md) F5；Unpaywall 配置文档见 F6。

### 不做

- 不单独注册 `get_paper`（四工具短链）。
- 不在这一项做引用图。

### 测试

- `doi=10.xxxx` mock 两源合并为 1 条。
- 只有 doi、空 query 合法。
- 两者都空返回错误。

---

## P1-C 引用一跳（OpenAlex / Semantic Scholar）

### 现状

aletheia `get_citation_graph(direction=citations|references)`。OpenAlex / S2 已在引擎列表里，但适配器只做 keyword search，没有 works/{id}/referenced_works。

### 方案

同一工具再加可选：

```text
graph      string  空 = 普通搜索；references | citations
seed_doi   string  graph 非空时必填
limit      int     默认 10，上限 20
```

实现：用 OpenAlex 主键（DOI → OpenAlex id，已有 OpenAlex 引擎可抽一个 `Lookup`）。S2 作国际区补源（默认禁用逻辑不变）。结果仍是 `[]SearchResult` paper，走现有 Markdown 格式。

国内 `network: china` 且无代理时：只用 OpenAlex，S2 跳过。

### 不做

- 不做多跳 BFS、Connected Papers 式可视化。
- 不引入新 MCP 工具名。

### 测试

- `graph=references` + mock OpenAlex 返回 N 篇，格式与普通学术结果一致。
- 无 `seed_doi` 报参数错误。

---

## P1-D 过滤：年份区间 / OA / 预印本

### 现状

`time_range` 只有 `year|month|week|day`，v3.4.0 已把 Crossref/DOAJ/arXiv 语法修好。Consensus 还有 `year_min`/`year_max`、`open_access_only`、`exclude_preprints`、`study_types`。

文献检索更常用「2019–2024」而不是「近一个月」。

### 方案

| 参数 | 行为 |
|------|------|
| `year_min` / `year_max` | 有则覆盖粗粒度 `time_range` 的「从某日至今」；能推上游的推（Crossref `from-pub-date`/`until-pub-date`，arXiv submittedDate 闭区间，DOAJ year 闭区间），不能的本地滤 |
| `oa_only` | 只要 `PDFURL != ""`（P1-A 之后）。不要只信 DOAJ 引擎 |
| `exclude_preprints` | 丢掉 arXiv / 明确 preprint 类型；Crossref 无类型时保守保留 |

`study_types`（RCT 等）**本波不做**，需要 PubMed publication type 映射，单开。

### 测试

- `year_min=2020,year_max=2021` 时 Crossref/arXiv/DOAJ 查询串含区间（可复用 `timefilter_test.go` 风格）。
- `oa_only=true` 过滤掉无 PDF 条目。
- `exclude_preprints=true` 不含 arXiv 引擎结果。

---

## 衔接（依赖 pdf-parser P0）

列表 Markdown 里已有 PDF 链接。P1-A 补全后，工具描述加一句：

> 需要全文时把 `pdf_url` 交给 `pdf_parser`（远程 URL）或 `cleanfetch`（HTML 落地页）。

不要在 `academicsearch` 内自动下载解析（token / 超时 / 隐私）。若以后做 `fetch_pdf=true`，必须默认 false，且复用 `pdf_parser` 而不是抄一套。

---

## 明确不做（本册）

- 再堆 Zenodo/HAL/SSRN/CORE/BASE 等长尾库（先把 9 源 + OA 补链用满）。
- Elicit 式自动综述（上层 agent / web-researcher）。
- 医学 `study_types`、SJR 分区过滤（Consensus 付费能力，本地没有稳定免费源）。
