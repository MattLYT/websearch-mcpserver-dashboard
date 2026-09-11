# websearch-mcpserver 项目记忆

## 发布约定（2026-08-29 确立，参考 2native-ssh-mcp 已验证工作流）

发布矩阵拆分（三套独立，不要合并成 6 平台）：

| 渠道 | 平台 | 工作流 |
|------|------|--------|
| GitHub Release 二进制 | linux/amd64, windows/amd64, darwin/amd64, darwin/arm64 | `release.yml` 普通 tag |
| GHCR 镜像 | linux/amd64, linux/arm64 | `docker.yml` 普通 tag |
| MCP Registry mcpb | 与 Release 相同的 4 平台 | `release.yml` 的 `-registry` tag（`--expect-packages 4`） |

linux-arm64 容器走 GHCR；不要为了「对齐」把 linux-arm64 / windows-arm64 加回 GitHub Release。

- **普通 tag**（`git tag -a v3.4.0 -m "..."`）→ Release 四平台 server+cli 二进制与 `.sha256`；Docker 另推 GHCR
- **特殊后缀 tag**（`git tag -a v3.4.0-registry -m "..."`）→ 官方 MCP Registry（从已存在的 `v3.4.0` release 拉二进制封 `.mcpb`，OIDC 免密钥）
- **v3.x 与 v3.x-registry 钉同一 commit**（registry 只多跑封包/发布 job）
- **发布到 registry 是显式动作**：只有打 `-registry` 后缀 tag 才会发布，普通 tag 不会漏发
- 发布流程：`v3.4.0` → 验证 Release 与 GHCR → `v3.4.0-registry` → 检查 publish-registry job → `curl https://registry.modelcontextprotocol.io/v0.1/servers?search=websearch-mcpserver` 验证
- release notes 来自 annotated tag message（gh api 读取，本地 checkout 的 lightweight tag 不可信）
- 发布失败重试：删旧 tag 重打（`git tag -d` + `git push origin :refs/tags/...`），同名 tag 重新推送不触发 workflow

## MCP Registry 信息

- 服务器名：`io.github.daidaiJ/websearch-mcpserver`（GitHub 账号 daidaiJ）
- 包类型：`mcpb`（MCP Bundle = zip + manifest.json，`server.type: "binary"`），**4** 平台各一个包条目（跟 GitHub Release，不是跟 GHCR）
- 打包与 server.json 生成由独立工具 `mcpb-tool-cli` 完成（`github.com/daidaiJ/mcpb-tool-cli`，`go install` 安装，`mcpb pack` / `mcpb serverjson`），替代 shell/jq 避免转义脆弱性；workflow 中 `go install github.com/daidaiJ/mcpb-tool-cli@latest` 后调用
- CLI 稳定性强化：`pack` 自带 zip 完整性自检；`serverjson` 支持 `--expect-packages 4` 防平台缺失、description >100 字符本地报错
- publish-registry job 闭环：publish 前 `mcp-publisher validate`（失败即停）+ publish 后 curl 自验证注册（jq 断言 name+version）
- description 硬限制 ≤100 字符（registry API 422 拒绝）
- 本地手动发布（备用）：安装 `mcp-publisher`（GitHub releases 下载）→ `mcp-publisher login github`（设备码）→ `mcp-publisher publish`

## 国际化状态（2026-08-29）

- 代码层 MCP 工具描述仍为中文（`mcp/server.go`、`mcp/tool.go`），国际曝光建议后续英文化（MCP 生态标准语言）
- 文档：`README.EN.md` 已存在（英文），`README.MD` 为中文，CHANGELOG 双语（`CHANGELOG.md` / `CHANGELOG.en.md`）
