[CmdletBinding()]
param(
    [string]$TaskName = 'WebSearchMCPServer-Reconcile',
    [switch]$RunNow,
    [switch]$Unregister
)

. (Join-Path $PSScriptRoot 'websearch-task.ps1')

if ($Unregister) {
    Unregister-WebSearchReconcilerTask -TaskName $TaskName
    exit 0
}

Register-WebSearchReconcilerTask -TaskName $TaskName -RunNow:$RunNow | Out-Null
Write-Output ("registered task: {0}" -f $TaskName)
if ($RunNow) {
    Write-Output 'started task for an immediate reconciliation'
}
