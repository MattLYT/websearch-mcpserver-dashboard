# Local deployment knowledge

## Current release

- The deployment baseline is upstream `v3.5.1` (`da877c1`), with image `websearch-mcpserver:v3.5.1-upstream-local`. Search, fetch and academic Go code is unmodified upstream code.
- Upstream PR #7 absorbed the academic time filters and Doubao adapter. One Doubao instance uses `global` or `custom`; upstream configuration does not support simultaneous `both`. The deployment keeps its existing `global` selection.
- Supported upstream Doubao key variables are `DOUBAO_SEARCH_API_KEY` and `ASK_ECHO_SEARCH_INFINITY_API_KEY`. These are Search API credentials; availability depends on the account's Search API entitlement.
- The local overlay retains loopback-only Docker publishing, the existing `/data` volume, and Windows recovery tooling. Compose maps `JINA_API_KEY` to `APP_JINA.API_KEY`; the empty YAML `jina.api_key` declaration makes upstream Viper load this environment value. Fetch outputs use `/data/fetchdata`.
- Before production replacement, the candidate passed upstream `go test -short ./...`, ten Pester recovery tests, MCP initialization/tools listing, hybrid search, independent Doubao Global and Custom searches (ten results each), cleanfetch and remote PDF parsing on port 8339. Crossref and DOAJ returned year-filtered results. arXiv timed out; the old production instance also encountered arXiv HTTP 429/503, so arXiv live availability remains unresolved.
- Rollback baseline: branch `codex/websearch-doubao-search` (`58ddaea`) and image `websearch-mcpserver:v3.3.0-doubao-both-local`. Never switch the production checkout during PR preparation; use a separate worktree and retain all active provider credentials during container replacement.

## Scheduled search-service recovery

- `WebSearchMCPServer-Reconcile` starts `ops/WebSearchTaskHost.exe` at user logon and every 30 minutes, with limited interactive-user privileges. Docker Desktop must already be available; this task does not start it.
- The task reconstructs missing Docker objects and recovers unhealthy services. `ops/websearch.disabled` is the maintenance opt-out. Compose and its restart policy remain unchanged.
- Root cause verified 2026-09-08: invoking the original task directly launched PowerShell with `-WindowStyle Hidden`, yet Windows Terminal emitted visible show/hide events at 14:00:43/14:00:48. Hiding the shell after startup did not prevent terminal activation.
- The Windows-subsystem task host and explicit no-console Docker subprocess creation remove that launch path. An actual post-change task trigger produced zero Terminal/console show/hide events in the 18-second capture window.
- Ten Pester recovery tests and host argument/exit/stderr/subsystem checks passed. Historical successful tests did not verify desktop window behavior. A real reboot/logon has not been performed during this repair.
- `ops/README.md` documents verification and the exact rollback snapshots. Credentials and persistent volumes were outside this change.
