$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$dataRoot = Join-Path $repoRoot '.data\device-real'
$pidPath = Join-Path $dataRoot 'process.json'
if (-not (Test-Path -LiteralPath $pidPath)) {
    Write-Output 'Real-device preset is not running.'
    exit 0
}
$state = Get-Content -Raw -LiteralPath $pidPath | ConvertFrom-Json
if ($state.executable -ne (Join-Path $dataRoot 'bin\measix-device-demo.exe')) {
    throw 'Refusing to stop a process not created by the real-device preset.'
}
try {
    $process = Get-CimInstance Win32_Process -Filter "ProcessId=$($state.pid)" -ErrorAction SilentlyContinue
    if ($null -ne $process -and $process.ExecutablePath -eq $state.executable) {
        # The process can exit after the ownership check and before the signal.
        # Treat that race as an already-completed stop, not as a launcher failure.
        Stop-Process -Id $state.pid -ErrorAction SilentlyContinue
        # Bounded wait: Wait-Process without -Timeout can block forever, and a
        # process that ignores the request must still be escalated.
        Wait-Process -Id $state.pid -Timeout 10 -ErrorAction SilentlyContinue
        $stillRunning = Get-Process -Id $state.pid -ErrorAction SilentlyContinue
        if ($null -ne $stillRunning) {
            Stop-Process -Id $state.pid -Force -ErrorAction SilentlyContinue
            Wait-Process -Id $state.pid -Timeout 10 -ErrorAction SilentlyContinue
            Write-Output "Force-stopped real-device preset PID $($state.pid)."
        } else {
            Write-Output "Stopped real-device preset PID $($state.pid)."
        }
    } else {
        Write-Output 'Recorded real-device preset process is not running; removing stale state.'
    }
} finally {
    # Always clear the PID file, otherwise the next start misreads stale state.
    Remove-Item -LiteralPath $pidPath -Force -ErrorAction SilentlyContinue
}
