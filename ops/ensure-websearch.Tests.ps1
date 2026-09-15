Describe 'websearch deployment reconciler' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'ensure-websearch.ps1')
    }

    It 'plans to recreate a missing container' {
        $plan = Get-WebSearchReconcilePlan -ContainerState 'missing' -HealthState 'none'
        $plan.Action | Should Be 'ensure'
        $plan.Reason | Should Be 'container-missing'
    }

    It 'leaves a healthy container untouched' {
        $plan = Get-WebSearchReconcilePlan -ContainerState 'running' -HealthState 'healthy'
        $plan.Action | Should Be 'healthy'
    }

    It 'restarts a running container whose healthcheck failed' {
        $plan = Get-WebSearchReconcilePlan -ContainerState 'running' -HealthState 'unhealthy'
        $plan.Action | Should Be 'restart'
    }

    It 'builds a missing local image before creating the service' {
        $plan = Get-WebSearchReconcilePlan -ContainerState 'missing' -HealthState 'none' -ImageState 'missing'
        $plan.Action | Should Be 'build'
        $plan.Reason | Should Be 'image-missing'
    }

    It 'waits for a container that is still starting' {
        $plan = Get-WebSearchReconcilePlan -ContainerState 'running' -HealthState 'starting'
        $plan.Action | Should Be 'wait'
    }

    It 'keeps wrapper parameters after loading the library' {
        $root = Join-Path $TestDrive 'deployment'
        New-Item -ItemType Directory -Path (Join-Path $root 'ops') -Force | Out-Null
        Set-Content -LiteralPath (Join-Path $root 'docker-compose.yml') -Value 'services: {}'
        New-Item -ItemType File -Path (Join-Path $root 'ops\websearch.disabled') -Force | Out-Null

        $output = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'run-ensure-websearch.ps1') `
            -CheckOnly -ComposeRoot $root -DockerWaitSeconds 0
        $exitCode = $LASTEXITCODE
        $payload = ($output -join '') | ConvertFrom-Json

        $exitCode | Should Be 0
        $payload.Action | Should Be 'disabled'
        $payload.Reason | Should Be 'marker-present'
    }

    It 'redacts credential-shaped values from diagnostic text' {
        $safe = Protect-WebSearchLogText -Text 'LLM_API_KEY=secret-value; TAVILY_SK:another-secret'
        $safe | Should Not Match 'secret-value'
        $safe | Should Not Match 'another-secret'
        $safe | Should Match '<redacted>'
    }

    It 'keeps the Compose recovery policy declarative' {
        $compose = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot '..\docker-compose.yml')
        $script:WebSearchImageName | Should Be 'websearch-mcpserver:v3.5.1-upstream-local'
        $compose | Should Match 'websearch-mcpserver:v3\.5\.1-upstream-local'
        $compose | Should Match 'pull_policy: never'
        $compose | Should Match 'restart: unless-stopped'
        $compose | Should Match 'healthcheck:'
        $compose | Should Match '127\.0\.0\.1:8338:8338'
    }
}
