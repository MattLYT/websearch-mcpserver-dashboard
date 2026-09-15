$ErrorActionPreference = 'Stop'
 $ops = $PSScriptRoot
. (Join-Path $ops 'ensure-websearch.ps1')
$fixture = Join-Path $env:TEMP ('websearch-host-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force $fixture | Out-Null
$compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
@'
using System;
using System.Text;
class Fixture {
 static int Main(string[] args) {
  foreach(string arg in args) Console.WriteLine(Convert.ToBase64String(Encoding.UTF8.GetBytes(arg)));
  Console.Error.WriteLine("diagnostic");
  return 23;
 }
}
'@ | Set-Content (Join-Path $fixture 'docker.cs')
& $compiler /nologo /target:exe "/out:$fixture\docker.exe" "$fixture\docker.cs"
if ($LASTEXITCODE -ne 0) { throw 'Fixture compilation failed' }
$oldPath = $env:PATH
try {
    $env:PATH = "$fixture;$oldPath"
    $arguments = @('simple', 'two words', 'quote"inside', 'C:\path with space\', '你好', '', 'slash\"quote')
    $result = Invoke-DockerProcess -Arguments $arguments
    if ($result.ExitCode -ne 23 -or $result.Error -ne 'diagnostic') { throw 'Exit code or stderr lost' }
    $expected = ($arguments | ForEach-Object { [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($_)) }) -join "`r`n"
    if ($result.Output -cne $expected) { throw 'Argument round trip failed' }
    $result = Invoke-DockerResult -Arguments @('hello')
    if ($result.ExitCode -ne 23 -or $result.Summary -ne 'diagnostic') { throw 'Diagnostic summary lost' }
} finally { $env:PATH = $oldPath }
Copy-Item (Join-Path $ops 'WebSearchTaskHost.exe') $fixture -Force
'exit 23' | Set-Content (Join-Path $fixture 'run-ensure-websearch.ps1')
$info = New-Object Diagnostics.ProcessStartInfo
$info.FileName = Join-Path $fixture 'WebSearchTaskHost.exe'
$info.UseShellExecute = $false
$info.CreateNoWindow = $true
$process = [Diagnostics.Process]::Start($info)
$process.WaitForExit()
if ($process.ExitCode -ne 23) { throw 'Task host failed to propagate script exit code' }
$process.Dispose()
$bytes = [IO.File]::ReadAllBytes($info.FileName)
$pe = [BitConverter]::ToInt32($bytes, 0x3c)
if ([BitConverter]::ToUInt16($bytes, $pe+24+68) -ne 2) { throw 'Task host is not a Windows subsystem executable' }
Write-Output 'PASS: argument fidelity, stderr capture, nonzero exits, task-host exit propagation, Windows subsystem'
$result = Invoke-Pester -Path "$ops\ensure-websearch.Tests.ps1","$ops\websearch-task.Tests.ps1" -PassThru
if ($result.FailedCount -gt 0 -or $result.PassedCount -lt 10) { throw 'Pester regression suite failed' }
