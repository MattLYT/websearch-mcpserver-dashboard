function Get-WebSearchTaskPlan {
    param(
        [string]$TaskName = 'WebSearchMCPServer-Reconcile'
    )

    $runnerPath = Join-Path $PSScriptRoot 'run-ensure-websearch.ps1'
    $taskHost = Join-Path $PSScriptRoot 'WebSearchTaskHost.exe'
    $userId = if ([string]::IsNullOrWhiteSpace($env:USERDOMAIN)) {
        $env:USERNAME
    }
    else {
        '{0}\{1}' -f $env:USERDOMAIN, $env:USERNAME
    }

    [pscustomobject]@{
        TaskName          = $TaskName
        RunnerPath        = $runnerPath
        Execute           = $taskHost
        UserId            = $userId
        RepetitionMinutes = 30
        RunLevel          = 'Limited'
        Description       = 'Reconcile the local websearch-mcpserver Compose service after logon and periodically.'
    }
}

function Register-WebSearchReconcilerTask {
    param(
        [string]$TaskName = 'WebSearchMCPServer-Reconcile',
        [switch]$RunNow
    )

    $compiler = Join-Path $env:SystemRoot 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
    $hostSource = Join-Path $PSScriptRoot 'WebSearchTaskHost.cs'
    $hostBinary = Join-Path $PSScriptRoot 'WebSearchTaskHost.exe'
    & $compiler /nologo /target:winexe /optimize+ "/out:$hostBinary" $hostSource
    if ($LASTEXITCODE -ne 0) { throw 'Unable to build windowless task host' }
    $plan = Get-WebSearchTaskPlan -TaskName $TaskName
    if (-not (Test-Path -LiteralPath $plan.RunnerPath)) {
        throw "reconciler runner not found: $($plan.RunnerPath)"
    }
    if (-not (Test-Path -LiteralPath $plan.Execute)) {
        throw "Windowless task host not found: $($plan.Execute)"
    }

    $action = New-ScheduledTaskAction -Execute $plan.Execute -WorkingDirectory (Split-Path -Parent $PSScriptRoot)
    $atLogon = New-ScheduledTaskTrigger -AtLogOn -User $plan.UserId
    $recurring = New-ScheduledTaskTrigger `
        -Once `
        -At (Get-Date).AddMinutes(1) `
        -RepetitionInterval (New-TimeSpan -Minutes $plan.RepetitionMinutes) `
        -RepetitionDuration (New-TimeSpan -Days 3650)
    $settings = New-ScheduledTaskSettingsSet `
        -StartWhenAvailable `
        -AllowStartIfOnBatteries `
        -DontStopIfGoingOnBatteries `
        -ExecutionTimeLimit (New-TimeSpan -Minutes 8) `
        -MultipleInstances IgnoreNew `
        -RestartCount 3 `
        -RestartInterval (New-TimeSpan -Minutes 1)
    $principal = New-ScheduledTaskPrincipal `
        -UserId $plan.UserId `
        -LogonType Interactive `
        -RunLevel Limited

    Register-ScheduledTask `
        -TaskName $plan.TaskName `
        -Action $action `
        -Trigger @($atLogon, $recurring) `
        -Settings $settings `
        -Principal $principal `
        -Description $plan.Description `
        -Force | Out-Null

    if ($RunNow) {
        Start-ScheduledTask -TaskName $plan.TaskName
    }

    return $plan
}

function Unregister-WebSearchReconcilerTask {
    param(
        [string]$TaskName = 'WebSearchMCPServer-Reconcile'
    )

    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
}
