Describe 'websearch scheduled reconciler' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'websearch-task.ps1')
    }

    It 'ships the task definition library' {
        Test-Path -LiteralPath (Join-Path $PSScriptRoot 'websearch-task.ps1') |
            Should Be $true
    }

    It 'runs at logon and repeats often enough to recreate a removed container' {
        $plan = Get-WebSearchTaskPlan
        $plan.RepetitionMinutes | Should Be 30
        $plan.RunLevel | Should Be 'Limited'
        $plan.Execute | Should Match 'WebSearchTaskHost\.exe$'
        $plan.RunnerPath | Should Match 'run-ensure-websearch\.ps1'
    }
}
