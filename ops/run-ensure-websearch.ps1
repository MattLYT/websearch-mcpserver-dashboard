[CmdletBinding()]
param(
    [switch]$CheckOnly,
    [int]$DockerWaitSeconds = 300,
    [int]$HealthWaitSeconds = 90,
    [int]$RetrySeconds = 5,
    [string]$ComposeRoot = ''
)

if ([string]::IsNullOrWhiteSpace($ComposeRoot)) {
    $ComposeRoot = Split-Path -Parent $PSScriptRoot
}

$requestedCheckOnly = $CheckOnly
$requestedDockerWaitSeconds = $DockerWaitSeconds
$requestedHealthWaitSeconds = $HealthWaitSeconds
$requestedRetrySeconds = $RetrySeconds

. (Join-Path $PSScriptRoot 'ensure-websearch.ps1')

$result = Invoke-WebSearchReconcile `
    -CheckOnly:$requestedCheckOnly `
    -ComposeRoot $ComposeRoot `
    -DockerWaitSeconds $requestedDockerWaitSeconds `
    -HealthWaitSeconds $requestedHealthWaitSeconds `
    -RetrySeconds $requestedRetrySeconds

if ($requestedCheckOnly) {
    $result | ConvertTo-Json -Compress
    exit [int]$result.ExitCode
}

exit [int]$result
