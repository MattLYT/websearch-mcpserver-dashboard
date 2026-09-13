# Issues — 工具链吸收方案

> 对照 v3.4.0（`master` @ `6850845`）评估同类开源 / 大厂 MCP 后，把值得吸收的特性写成可执行规划。
> 不是 GitHub Issue 原文，落地时再拆 PR。风格对齐 [`plans/`](../plans/README.md)（现状 / 方案 / 不做）。

编排层（RRF / Boost / MMR / Key 池 / 回退 / 9 源学术）已经够厚。缺口在**工具参数、结果深度、学术闭环、抓取抗 JS、输出安全**。保持四工具短链，不拆成 8–10 个厂商式工具。

| 文件 | 内容 |
|------|------|
| [overview.md](overview.md) | 对照图谱、原则、分期、明确不做 |
| [smartsearch.md](smartsearch.md) | 搜后浅抽取、工具层收窄参数、Tavily depth（Brave 已明确不做） |
| [academicsearch.md](academicsearch.md) | OA PDF、DOI 详情、引用一跳、年份/OA 过滤 |
| [cleanfetch.md](cleanfetch.md) | 复用 LLM 摘要（非 Jaccard）、批量 URL、不可信包裹、可选 JS 回退、轻量 map |
| [pdf-parser.md](pdf-parser.md) | 远程 URL、页范围、结构保留、Docling 仅作可选 CPU 备选 |

建议落地顺序见 [overview.md §分期](overview.md#分期)。一次会话只做一波，不要把 P0–P2 塞进同一个 PR。

已拆任务见 [`tasks/README.md`](../tasks/README.md)（T15–T20）。仍未拆：工具层域名/news/lang、Tavily depth、引用图、年份闭区间、不可信包裹、JS/map、Docling、Brave。
