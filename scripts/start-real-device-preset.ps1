param(
    [switch]$Reset
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$backendRoot = Join-Path $repoRoot 'backend'
$portalRoot = Join-Path $repoRoot '..\measix-enterprise-portal'
$dataRoot = Join-Path $repoRoot '.data\device-real'
$secretRoot = Join-Path $repoRoot '.secrets'
$passwordPath = Join-Path $secretRoot 'device-real-admin-password.txt'
$masterKeyPath = Join-Path $secretRoot 'device-real-master.key'
$jwtKeyPath = Join-Path $secretRoot 'device-real-jwt-ed25519.seed'
$relayTokenPath = Join-Path $secretRoot 'device-real-relay-service.token'
$dbPath = Join-Path $dataRoot 'hub.db'
$spoolPath = Join-Path $dataRoot 'relay-spool.db'
$binaryPath = Join-Path $dataRoot 'bin\measix-device-demo.exe'
$pidPath = Join-Path $dataRoot 'process.json'
$logRoot = Join-Path $dataRoot 'logs'

function New-RandomFile([string]$Path, [int]$Length, [switch]$Text) {
    if (Test-Path -LiteralPath $Path) { return }
    $bytes = [byte[]]::new($Length)
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    if ($Text) {
        [System.IO.File]::WriteAllText($Path, ([Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_')) + [Environment]::NewLine)
    } else {
        [System.IO.File]::WriteAllBytes($Path, $bytes)
    }
}

function Get-DeviceIPv4 {
    $route = Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' -ErrorAction Stop |
        Where-Object { $_.NextHop -ne '0.0.0.0' } | Sort-Object RouteMetric | Select-Object -First 1
    if ($null -eq $route) { throw 'No active IPv4 default route. Set MEASIX_REAL_DEVICE_ORIGIN explicitly after connecting the LAN.' }
    $address = Get-NetIPAddress -AddressFamily IPv4 -InterfaceIndex $route.InterfaceIndex -ErrorAction Stop |
        Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254*' } | Select-Object -First 1
    if ($null -eq $address) { throw 'No usable IPv4 address on the default-route interface. Set MEASIX_REAL_DEVICE_ORIGIN explicitly.' }
    return $address.IPAddress
}

if ($Reset) {
    & (Join-Path $PSScriptRoot 'stop-real-device-preset.ps1')
    if (Test-Path -LiteralPath $dataRoot) {
        $resolved = (Resolve-Path $dataRoot).Path
        if (-not $resolved.StartsWith((Resolve-Path (Join-Path $repoRoot '.data')).Path)) { throw 'Unsafe device-real data path.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}
if (Test-Path -LiteralPath $pidPath) {
    & (Join-Path $PSScriptRoot 'stop-real-device-preset.ps1')
}

$origin = $env:MEASIX_REAL_DEVICE_ORIGIN
if ([string]::IsNullOrWhiteSpace($origin)) { $origin = "http://$(Get-DeviceIPv4):9100" }
if ($origin -notmatch '^https?://[^/]+$') { throw 'MEASIX_REAL_DEVICE_ORIGIN must be an origin such as http://192.168.100.138:9100.' }
$uri = [Uri]$origin
if ($uri.Scheme -ne 'http') { throw 'The local real-device preset serves HTTP only. Use an HTTP LAN origin.' }

New-Item -ItemType Directory -Force -Path $dataRoot, $secretRoot, $logRoot, (Split-Path -Parent $binaryPath) | Out-Null
New-RandomFile $masterKeyPath 32
New-RandomFile $jwtKeyPath 32
New-RandomFile $relayTokenPath 32 -Text
if (-not (Test-Path -LiteralPath $passwordPath)) {
    $bytes = [byte[]]::new(18)
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    [System.IO.File]::WriteAllText($passwordPath, "measix-device-$([BitConverter]::ToString($bytes).Replace('-', '').ToLowerInvariant())" + [Environment]::NewLine)
}
if (-not (Test-Path -LiteralPath (Join-Path $secretRoot 'supplier-keys.env'))) {
    throw "Missing ignored supplier key file: $(Join-Path $secretRoot 'supplier-keys.env')"
}

Push-Location $repoRoot
try {
    & pnpm -C console build
    if ($LASTEXITCODE -ne 0) { throw 'Admin Console build failed.' }
    & pnpm -C $portalRoot build:all
    if ($LASTEXITCODE -ne 0) { throw 'Enterprise Portal build failed.' }
    Push-Location $backendRoot
    try {
        & go run ./cmd/devmigrate --db $dbPath
        if ($LASTEXITCODE -ne 0) { throw "Current database initialization failed. If the error reports a non-current schema or checksum, this environment's database predates the current schema: run 'npm run device:real:reset' to delete and recreate only .data/device-real." }
        & go run ./cmd/control-hub bootstrap-admin --if-empty --db $dbPath --master-key-file $masterKeyPath --jwt-private-key-file $jwtKeyPath --deployment-name 'MEASIX Device Demo' --username admin --display-name 'Device Demo Admin' --password-file $passwordPath
        if ($LASTEXITCODE -ne 0) { throw 'Admin bootstrap failed.' }
        & go build -o $binaryPath ./cmd/device-demo
        if ($LASTEXITCODE -ne 0) { throw 'Device-demo build failed.' }
    } finally { Pop-Location }

    $args = @(
        '--listen', "$($uri.Host):$($uri.Port)", '--hub-internal-listen', '127.0.0.1:9101', '--relay-internal-listen', '127.0.0.1:9103',
        '--public-origin', $origin, '--db', $dbPath, '--master-key-file', $masterKeyPath, '--jwt-private-key-file', $jwtKeyPath,
        '--relay-service-token-file', $relayTokenPath, '--spool', $spoolPath,
        '--admin-assets-dir', (Join-Path $repoRoot 'console\dist\spa'), '--portal-assets-dir', (Join-Path $portalRoot 'dist')
    )
    $process = Start-Process -FilePath $binaryPath -ArgumentList $args -WorkingDirectory $repoRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $logRoot 'device-demo.out.log') -RedirectStandardError (Join-Path $logRoot 'device-demo.err.log')
    @{ pid = $process.Id; executable = $binaryPath; origin = $origin; startedAt = [DateTime]::UtcNow.ToString('o') } | ConvertTo-Json | Set-Content -LiteralPath $pidPath -NoNewline

    $ready = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing "$origin/ready" -TimeoutSec 2
            if ($response.StatusCode -eq 200) { $ready = $true; break }
        } catch { Start-Sleep -Milliseconds 500 }
    }
    if (-not $ready) { throw "Device-demo did not become ready. Inspect $logRoot" }
    $env:MEASIX_REAL_DEVICE_ORIGIN = $origin
    $env:MEASIX_REAL_DEVICE_ADMIN_PASSWORD_FILE = $passwordPath
    $env:MEASIX_REAL_DEVICE_STATE = (Join-Path $dataRoot 'preset-state.json')
    & node scripts/real-device-preset.mjs
    if ($LASTEXITCODE -ne 0) { throw 'Real-device configuration publish failed.' }
    $discovery = Invoke-RestMethod -UseBasicParsing "$origin/.well-known/measix"
    if ($discovery.clientApiBase -ne '/api/client/v1' -or $discovery.runtimeApiBase -ne '/runtime/v1') { throw 'Discovery did not expose the expected same-origin paths.' }
    Write-Output "Real-device preset is ready: $origin"
    Write-Output "Admin: $origin/admin/"
    Write-Output "Admin password file: $passwordPath"
    Write-Output 'Create a member and enrollment material in Admin > Users, then scan or paste it in Android.'
} catch {
    if (Test-Path -LiteralPath $pidPath) { & (Join-Path $PSScriptRoot 'stop-real-device-preset.ps1') }
    throw
} finally { Pop-Location }
