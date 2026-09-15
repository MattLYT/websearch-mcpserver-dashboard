$script:WebSearchServiceName = 'websearch'
$script:WebSearchContainerName = 'websearch-mcpserver'
$script:WebSearchImageName = 'websearch-mcpserver:v3.5.1-upstream-local'
$script:WebSearchPort = 8338

function Write-WebSearchGuardLog {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Message,
        [string]$LogPath = (Join-Path $PSScriptRoot 'ensure-websearch.log')
    )

    try {
        $directory = Split-Path -Parent $LogPath
        if (-not (Test-Path -LiteralPath $directory)) {
            New-Item -ItemType Directory -Path $directory -Force | Out-Null
        }

        if (Test-Path -LiteralPath $LogPath) {
            $length = (Get-Item -LiteralPath $LogPath).Length
            if ($length -gt 1048576) {
                $tail = Get-Content -LiteralPath $LogPath -Tail 200
                Set-Content -LiteralPath $LogPath -Value $tail -Encoding UTF8
            }
        }

        $line = '{0} {1}' -f (Get-Date).ToUniversalTime().ToString('o'), $Message
        Add-Content -LiteralPath $LogPath -Value $line -Encoding UTF8
    }
    catch {
        # A logging failure must not hide the service result or cause a retry storm.
    }
}

function Protect-WebSearchLogText {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Text
    )

    return ($Text -replace '(?i)([A-Z0-9_.-]*(?:KEY|TOKEN|SECRET|PASSWORD|SK)[A-Z0-9_.-]*\s*[:=]\s*)\S+', '$1<redacted>')
}

function Invoke-DockerProcess {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string[]]$Arguments)
    # Quote argv using the Windows C runtime rules, including trailing backslashes.
    $quoted = foreach ($argument in $Arguments) {
        '"' + ([regex]::Replace(([regex]::Replace($argument, '(\\*)"', '$1$1\"')), '(\\+)$', '$1$1')) + '"'
    }
    $info = New-Object System.Diagnostics.ProcessStartInfo
    $info.FileName = (Get-Command docker -CommandType Application -ErrorAction Stop | Select-Object -First 1).Source
    $info.Arguments = $quoted -join ' '
    $info.UseShellExecute = $false
    $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $info
    try {
        [void]$process.Start()
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        $process.WaitForExit()
        [pscustomobject]@{ ExitCode = $process.ExitCode; Output = $stdout.Result.Trim(); Error = $stderr.Result.Trim() }
    }
    finally { $process.Dispose() }
}

function Invoke-DockerCapture {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string[]]$Arguments)
    $result = Invoke-DockerProcess -Arguments $Arguments
    [pscustomobject]@{ ExitCode = $result.ExitCode; Output = $result.Output }
}

function Invoke-DockerResult {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string[]]$Arguments)
    $result = Invoke-DockerProcess -Arguments $Arguments
    $summary = (($result.Output + [Environment]::NewLine + $result.Error) -split '\r?\n' |
        Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -Last 1)
    [pscustomobject]@{ ExitCode = $result.ExitCode; Summary = Protect-WebSearchLogText -Text ([string]$summary) }
}

function Test-DockerReady {
    $result = Invoke-DockerCapture -Arguments @('info', '--format', '{{.ServerVersion}}')
    return ($result.ExitCode -eq 0 -and -not [string]::IsNullOrWhiteSpace($result.Output))
}

function Wait-DockerReady {
    param(
        [int]$TimeoutSeconds,
        [int]$PollSeconds
    )

    $deadline = (Get-Date).AddSeconds([Math]::Max(0, $TimeoutSeconds))
    do {
        if (Test-DockerReady) {
            return $true
        }

        if ((Get-Date) -ge $deadline) {
            break
        }

        Start-Sleep -Seconds ([Math]::Max(1, $PollSeconds))
    } while ((Get-Date) -lt $deadline)

    return $false
}

function Get-WebSearchContainerSnapshot {
    $result = Invoke-DockerCapture -Arguments @(
        'inspect',
        '--format',
        '{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}',
        $script:WebSearchContainerName
    )

    if ($result.ExitCode -ne 0 -or [string]::IsNullOrWhiteSpace($result.Output)) {
        return [pscustomobject]@{
            ContainerState = 'missing'
            HealthState    = 'none'
        }
    }

    $parts = $result.Output -split '\|', 2
    $containerState = $parts[0]
    $healthState = if ($parts.Count -gt 1) { $parts[1] } else { 'none' }
    return [pscustomobject]@{
        ContainerState = $containerState
        HealthState    = $healthState
    }
}

function Test-WebSearchImagePresent {
    $result = Invoke-DockerCapture -Arguments @('image', 'inspect', $script:WebSearchImageName)
    return ($result.ExitCode -eq 0 -and -not [string]::IsNullOrWhiteSpace($result.Output))
}

function Get-WebSearchReconcilePlan {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ContainerState,
        [Parameter(Mandatory = $true)]
        [string]$HealthState,
        [string]$ImageState = 'present'
    )

    if ($ImageState -eq 'missing') {
        return [pscustomobject]@{ Action = 'build'; Reason = 'image-missing' }
    }

    if ($ContainerState -eq 'missing') {
        return [pscustomobject]@{ Action = 'ensure'; Reason = 'container-missing' }
    }

    if ($ContainerState -eq 'running' -and $HealthState -eq 'healthy') {
        return [pscustomobject]@{ Action = 'healthy'; Reason = 'healthcheck-passed' }
    }

    if ($ContainerState -eq 'running' -and $HealthState -eq 'unhealthy') {
        return [pscustomobject]@{ Action = 'restart'; Reason = 'healthcheck-failed' }
    }

    if ($ContainerState -eq 'running' -and $HealthState -eq 'starting') {
        return [pscustomobject]@{ Action = 'wait'; Reason = 'healthcheck-starting' }
    }

    return [pscustomobject]@{ Action = 'ensure'; Reason = "container-$ContainerState" }
}

function Test-WebSearchHealth {
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri ("http://127.0.0.1:{0}/__admin/health" -f $script:WebSearchPort) -TimeoutSec 5
        if ($response.StatusCode -ne 200) {
            return $false
        }

        $payload = $response.Content | ConvertFrom-Json
        return ($payload.message -eq 'running')
    }
    catch {
        return $false
    }
}

function Wait-WebSearchHealth {
    param(
        [int]$TimeoutSeconds,
        [int]$PollSeconds
    )

    $deadline = (Get-Date).AddSeconds([Math]::Max(0, $TimeoutSeconds))
    do {
        if (Test-WebSearchHealth) {
            return $true
        }

        if ((Get-Date) -ge $deadline) {
            break
        }

        Start-Sleep -Seconds ([Math]::Max(1, $PollSeconds))
    } while ((Get-Date) -lt $deadline)

    return $false
}

function Invoke-WebSearchComposeUp {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ComposeFile,
        [int]$WaitSeconds
    )

    return Invoke-DockerResult -Arguments @(
        'compose',
        '-f',
        $ComposeFile,
        'up',
        '-d',
        '--wait',
        '--wait-timeout',
        ([string][Math]::Max(1, $WaitSeconds))
    )
}

function Invoke-WebSearchComposeBuild {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ComposeFile
    )

    return Invoke-DockerResult -Arguments @(
        'compose',
        '-f',
        $ComposeFile,
        'build',
        $script:WebSearchServiceName
    )
}

function Invoke-WebSearchReconcile {
    param(
        [string]$ComposeRoot = (Split-Path -Parent $PSScriptRoot),
        [int]$DockerWaitSeconds = 300,
        [int]$HealthWaitSeconds = 90,
        [int]$RetrySeconds = 5,
        [switch]$CheckOnly
    )

    $composeFile = Join-Path $ComposeRoot 'docker-compose.yml'
    $disabledMarker = Join-Path $ComposeRoot 'ops\websearch.disabled'
    $logPath = Join-Path $ComposeRoot 'ops\ensure-websearch.log'

    if (-not (Test-Path -LiteralPath $composeFile)) {
        Write-WebSearchGuardLog -Message 'compose-file-missing' -LogPath $logPath
        return 20
    }

    if (Test-Path -LiteralPath $disabledMarker) {
        Write-WebSearchGuardLog -Message 'disabled-marker-present' -LogPath $logPath
        if ($CheckOnly) {
            return [pscustomobject]@{
                Action = 'disabled'
                Reason = 'marker-present'
                ExitCode = 0
            }
        }
        return 0
    }

    if (-not (Wait-DockerReady -TimeoutSeconds $DockerWaitSeconds -PollSeconds $RetrySeconds)) {
        Write-WebSearchGuardLog -Message 'docker-not-ready' -LogPath $logPath
        return 21
    }

    $snapshot = Get-WebSearchContainerSnapshot
    $imagePresent = Test-WebSearchImagePresent
    $imageState = if ($imagePresent) { 'present' } else { 'missing' }
    $plan = Get-WebSearchReconcilePlan -ContainerState $snapshot.ContainerState -HealthState $snapshot.HealthState -ImageState $imageState

    if ($CheckOnly) {
        return [pscustomobject]@{
            Action         = $plan.Action
            Reason         = $plan.Reason
            ContainerState = $snapshot.ContainerState
            HealthState    = $snapshot.HealthState
            ImageState     = $imageState
            ExitCode       = $(if ($plan.Action -eq 'healthy') { 0 } else { 10 })
        }
    }

    if ($plan.Action -eq 'healthy') {
        Write-WebSearchGuardLog -Message 'healthy' -LogPath $logPath
        return 0
    }

    if ($plan.Action -eq 'build') {
        $buildResult = Invoke-WebSearchComposeBuild -ComposeFile $composeFile
        if ($buildResult.ExitCode -ne 0) {
            Write-WebSearchGuardLog -Message ("compose-build-failed exit={0} summary={1}" -f $buildResult.ExitCode, $buildResult.Summary) -LogPath $logPath
            return 29
        }
    }

    if ($plan.Action -eq 'restart') {
        $restartResult = Invoke-DockerResult -Arguments @(
            'compose',
            '-f',
            $composeFile,
            'restart',
            '--no-deps',
            $script:WebSearchServiceName
        )
        if ($restartResult.ExitCode -ne 0) {
            Write-WebSearchGuardLog -Message ("compose-restart-failed exit={0} summary={1}" -f $restartResult.ExitCode, $restartResult.Summary) -LogPath $logPath
            return 30
        }
    }

    $upResult = Invoke-WebSearchComposeUp -ComposeFile $composeFile -WaitSeconds $HealthWaitSeconds
    if ($upResult.ExitCode -ne 0) {
        Write-WebSearchGuardLog -Message ("compose-up-failed action={0} exit={1} summary={2}" -f $plan.Action, $upResult.ExitCode, $upResult.Summary) -LogPath $logPath
        return 31
    }

    if (-not (Wait-WebSearchHealth -TimeoutSeconds $HealthWaitSeconds -PollSeconds $RetrySeconds)) {
        Write-WebSearchGuardLog -Message ("healthcheck-timeout action={0}" -f $plan.Action) -LogPath $logPath
        return 32
    }

    Write-WebSearchGuardLog -Message ("reconciled action={0} reason={1}" -f $plan.Action, $plan.Reason) -LogPath $logPath
    return 0
}
