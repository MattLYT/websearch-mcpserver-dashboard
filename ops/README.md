# Local deployment operations

The deployment uses upstream v3.5.1 (da877c1), including the upstream
academic time filters and single-version Doubao adapter. The local overlay
only supplies Docker packaging, configuration and recovery. Doubao defaults
to Global; select `doubao.version: custom` for Custom. Upstream does not
provide simultaneous Global/Custom selection through configuration.

Compose maps the existing `JINA_API_KEY` to `APP_JINA.API_KEY`, which matches
upstream Viper's literal dotted key. The configuration declares `jina.api_key`
as empty so environment loading can populate it. Keys stay in the process
environment. Fetch output and the existing cache use the `/data` volume.

The checked-in `docker-compose.yml` is the source of truth for the local
deployment. It publishes the MCP endpoint only on `127.0.0.1:8338` and keeps
the optional provider credentials in the host environment.

## Reconciliation

`run-ensure-websearch.ps1` is a one-shot, idempotent reconciliation entry
point. It waits for Docker Desktop, checks the exact container and local image,
builds the image only when it is missing, starts missing/stopped services with
`docker compose up --wait`, and restarts a running service whose healthcheck is
unhealthy. `ops/websearch.disabled` is an explicit stop marker for maintenance;
the scheduled task leaves the deployment untouched while that file exists.

The script writes bounded, credential-redacted status lines to
`ops/ensure-websearch.log`, which is ignored by Git.

## Persistent startup guard

Register the user-level Task Scheduler guard once after checking out this
deployment:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\ops\register-websearch-reconciler.ps1 -RunNow
```

The task runs at the user's logon and every thirty minutes with limited
privileges. It does not start Docker Desktop, change Hindsight, or expose the
service to the LAN; it waits for the Docker engine and then reconciles this
Compose project. The Compose service retains `restart: unless-stopped` for
ordinary container crashes, while the task covers a removed container or local
image—the case a restart policy cannot repair.

To pause automatic reconciliation for maintenance, create the ignored marker
and stop the service. Remove the marker and run the registration command again
to resume:

```powershell
New-Item -ItemType File .\ops\websearch.disabled -Force
docker compose stop websearch
Remove-Item -LiteralPath .\ops\websearch.disabled
```

## Verification

```powershell
Invoke-Pester -Path .\ops\ensure-websearch.Tests.ps1,.\ops\websearch-task.Tests.ps1
docker compose config --quiet
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\ops\run-ensure-websearch.ps1 -CheckOnly
Invoke-WebRequest http://127.0.0.1:8338/__admin/health
```

The original failure was a deployment-state gap: `restart: unless-stopped`
only applies to an existing container. When both the local image and the
container are absent, Docker has nothing to restart and Codex's enabled MCP
entry only reports configuration, not service health. The reconciler and the
scheduled task close that gap without relying on a live container or a manual
restart.

## Windowless scheduled execution

The task launches `WebSearchTaskHost.exe`, built from `WebSearchTaskHost.cs`
with the installed .NET Framework compiler as a Windows-subsystem executable.
It starts PowerShell with `UseShellExecute=false` and `CreateNoWindow=true`,
waits for completion, and returns the script exit code. Docker subprocesses use
the same no-console creation mode and drain stdout/stderr concurrently.
Registration builds the host; no third-party runtime is downloaded.

The task runs at logon and every 30 minutes under the existing limited user.
Recovery of missing or unhealthy Docker objects can therefore wait up to one
interval after Docker becomes available. Ordinary container restart behavior
remains controlled by Compose. Use the maintenance marker for intentional stops.

Run `powershell -NoProfile -File ops/verify-windowless-host.ps1` for the ten
recovery tests plus argument, stderr, exit-code and executable-subsystem checks.
Run `python ops/trace-websearch-window.py` in the interactive user session to
capture Terminal/console show/hide events while triggering the actual task.
This check uses the current healthy service; it does not remove Docker objects.

Rollback evidence is stored in `backups/windowless-task-20260908-140434`
(task XML and previous scripts) and `backups/task-interval-20260908-140556`
(task XML and definitions before the interval change). Restore the matching
scripts and import the saved task XML to roll back; importing the first snapshot
also restores its original five-minute frequency and PowerShell launch action.
