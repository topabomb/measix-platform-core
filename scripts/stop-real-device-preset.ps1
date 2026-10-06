$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$dataRoot = Join-Path $repoRoot '.data\device-real'
$pidPath = Join-Path $dataRoot 'process.json'
if (-not (Test-Path -LiteralPath $pidPath)) {
    Write-Output 'Real-device preset is not running.'
    exit 0
}
$state = Get-Content -Raw -LiteralPath $pidPath | ConvertFrom-Json
$recordedPid = 0
if (-not [int]::TryParse([string]$state.pid, [ref]$recordedPid) -or $recordedPid -le 0) {
    throw 'Invalid real-device preset PID; process record preserved.'
}
$expectedExecutable = Join-Path $dataRoot 'bin\measix-device-demo.exe'
# A failed query is not evidence that the process has exited.
$process = Get-CimInstance Win32_Process -Filter "ProcessId=$recordedPid" -ErrorAction Stop
foreach ($forceStop in @($false, $true)) {
    if ($null -eq $process) { break }
    if ($state.executable -ne $expectedExecutable -or $process.ExecutablePath -ne $expectedExecutable) {
        throw "Real-device preset process identity mismatch for PID $recordedPid. Recorded: $($state.executable); Expected: $expectedExecutable; Actual: $($process.ExecutablePath). No stop requested for this mismatched process; process record and data preserved. Verify the instance before manual cleanup."
    }
    # An exit between inspection and signaling is harmless; verify afterward.
    Stop-Process -Id $recordedPid -Force:$forceStop -ErrorAction SilentlyContinue
    Wait-Process -Id $recordedPid -Timeout 10 -ErrorAction SilentlyContinue
    $process = Get-CimInstance Win32_Process -Filter "ProcessId=$recordedPid" -ErrorAction Stop
}
if ($null -ne $process) {
    throw "Real-device preset PID $recordedPid remains running after stop attempts; process record and data preserved. Check permissions and process status before retrying."
}
Remove-Item -LiteralPath $pidPath -Force
Write-Output "Real-device preset PID $recordedPid is no longer running; process record cleared."
