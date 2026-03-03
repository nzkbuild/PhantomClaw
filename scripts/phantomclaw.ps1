param(
    [ValidateSet("menu", "help", "build", "run", "rebuild", "test", "token", "handshake")]
    [string]$Action = "menu",
    [string]$Config = "config.yaml",
    [string]$Token = "",
    [int]$TokenLength = 48,
    [switch]$NoPrompt,
    [switch]$AutoInstallConfig,
    [switch]$SetSessionEnv,
    [switch]$SetUserEnv,
    [switch]$Background
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$script:Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $script:Root

if (-not $env:GOCACHE) {
    $env:GOCACHE = Join-Path $script:Root ".gocache"
}
if (-not $env:GOMODCACHE) {
    $env:GOMODCACHE = Join-Path $script:Root ".gomodcache"
}

$exePath = Join-Path $script:Root "phantomclaw.exe"

function Invoke-NativeChecked {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter()][string[]]$ArgumentList = @()
    )

    & $FilePath @ArgumentList
    if ($LASTEXITCODE -ne 0) {
        throw ("Command failed (exit {0}): {1} {2}" -f $LASTEXITCODE, $FilePath, ($ArgumentList -join " "))
    }
}

function Show-Help {
    Write-Host "PhantomClaw helper"
    Write-Host ""
    Write-Host "Usage:"
    Write-Host "  .\scripts\phantomclaw.ps1 -Action build"
    Write-Host "  .\scripts\phantomclaw.ps1 -Action run"
    Write-Host "  .\scripts\phantomclaw.ps1 -Action rebuild"
    Write-Host "  .\scripts\phantomclaw.ps1 -Action token"
    Write-Host "  .\scripts\phantomclaw.ps1 -Action handshake -Token <bridge-token>"
    Write-Host ""
    Write-Host "Actions:"
    Write-Host "  menu      Interactive menu (default)"
    Write-Host "  help      Show this help"
    Write-Host "  build     Build phantomclaw.exe"
    Write-Host "  run       Run phantomclaw.exe with -config"
    Write-Host "  rebuild   Build then run"
    Write-Host "  test      Run go test ./..."
    Write-Host "  token     Launch token rotation helper"
    Write-Host "  handshake Check bridge health/account endpoints"
    Write-Host ""
    Write-Host "Token action options:"
    Write-Host "  -TokenLength <n>      Token length (default: 48)"
    Write-Host "  -AutoInstallConfig    Write token into bridge.auth_token in config.yaml"
    Write-Host "  -SetSessionEnv        Set PHANTOM_BRIDGE_AUTH_TOKEN for current terminal"
    Write-Host "  -SetUserEnv           Persist PHANTOM_BRIDGE_AUTH_TOKEN for current Windows user"
    Write-Host "  -NoPrompt             Non-interactive mode (no y/n questions)"
}

function Invoke-Menu {
    while ($true) {
        Write-Host ""
        Write-Host "PhantomClaw Menu"
        Write-Host "1) Build phantomclaw.exe"
        Write-Host "2) Run phantomclaw.exe"
        Write-Host "3) Rebuild (build + run)"
        Write-Host "4) Run tests"
        Write-Host "5) Rotate bridge token"
        Write-Host "6) Handshake check"
        Write-Host "7) Help"
        Write-Host "0) Exit"

        $choice = (Read-Host "Choose an option").Trim()
        switch ($choice) {
            "1" { Invoke-Build }
            "2" { Invoke-Run }
            "3" { Invoke-Build; Invoke-Run }
            "4" { Invoke-Tests }
            "5" { Invoke-Token }
            "6" {
                if ([string]::IsNullOrWhiteSpace($Token) -and [string]::IsNullOrWhiteSpace($env:PHANTOM_BRIDGE_AUTH_TOKEN)) {
                    $Token = Read-Host "Bridge token (leave empty to use PHANTOM_BRIDGE_AUTH_TOKEN)"
                }
                Invoke-Handshake
            }
            "7" { Show-Help }
            "0" { return }
            default { Write-Host "Invalid option. Please choose 0-7." }
        }
    }
}

function Invoke-Build {
    Write-Host "Building phantomclaw.exe ..."
    Invoke-NativeChecked -FilePath "go" -ArgumentList @("build", "-o", $exePath, "./cmd/phantomclaw/")
    Write-Host "Build complete: $exePath"
}

function Invoke-Run {
    if (-not (Test-Path -LiteralPath $exePath)) {
        throw "phantomclaw.exe not found. Run -Action build first."
    }

    if ($Background) {
        Write-Host "Starting PhantomClaw in background ..."
        Start-Process -FilePath $exePath -ArgumentList @("-config", $Config) -WorkingDirectory $script:Root | Out-Null
        Write-Host "Started in background."
        return
    }

    Write-Host "Running PhantomClaw ..."
    & $exePath -config $Config
}

function Invoke-Tests {
    Write-Host "Running tests ..."
    Invoke-NativeChecked -FilePath "go" -ArgumentList @("test", "./...")
}

function Invoke-Token {
    if ($TokenLength -lt 24) {
        throw "TokenLength must be >= 24."
    }

    function New-SecureToken {
        param([int]$Length)
        $alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789".ToCharArray()
        $bytes = New-Object byte[] $Length
        $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
        try {
            $rng.GetBytes($bytes)
        } finally {
            if ($null -ne $rng) {
                $rng.Dispose()
            }
        }
        $chars = for ($i = 0; $i -lt $Length; $i++) {
            $alphabet[$bytes[$i] % $alphabet.Length]
        }
        -join $chars
    }

    function Ask-YesNo {
        param([string]$Question)
        while ($true) {
            $answer = (Read-Host "$Question [y/N]").Trim().ToLowerInvariant()
            if ($answer -eq "" -or $answer -eq "n" -or $answer -eq "no") { return $false }
            if ($answer -eq "y" -or $answer -eq "yes") { return $true }
            Write-Host "Please answer y or n."
        }
    }

    function Set-BridgeTokenInConfig {
        param(
            [string]$Path,
            [string]$BridgeToken
        )

        if (-not (Test-Path -LiteralPath $Path)) {
            throw "Config file not found: $Path"
        }

        $lines = [System.Collections.Generic.List[string]]::new()
        foreach ($line in Get-Content -LiteralPath $Path) {
            $lines.Add($line)
        }

        $bridgeStart = -1
        for ($i = 0; $i -lt $lines.Count; $i++) {
            if ($lines[$i] -match '^\s*bridge:\s*$') {
                $bridgeStart = $i
                break
            }
        }
        if ($bridgeStart -lt 0) {
            throw "Could not find 'bridge:' section in $Path"
        }

        $bridgeEnd = $lines.Count
        for ($i = $bridgeStart + 1; $i -lt $lines.Count; $i++) {
            if ($lines[$i] -match '^\S') {
                $bridgeEnd = $i
                break
            }
        }

        $updated = $false
        for ($i = $bridgeStart + 1; $i -lt $bridgeEnd; $i++) {
            if ($lines[$i] -match '^\s*auth_token:\s*') {
                $lines[$i] = '  auth_token: "' + $BridgeToken + '"'
                $updated = $true
                break
            }
        }

        if (-not $updated) {
            $insertAt = $bridgeStart + 1
            if ($bridgeEnd -gt $bridgeStart + 1) {
                $insertAt = $bridgeEnd
            }
            $lines.Insert($insertAt, '  auth_token: "' + $BridgeToken + '"')
        }

        $backupPath = "$Path.bak.$(Get-Date -Format yyyyMMddHHmmss)"
        Copy-Item -LiteralPath $Path -Destination $backupPath -Force
        Set-Content -LiteralPath $Path -Value $lines -Encoding UTF8
        return $backupPath
    }

    $generated = New-SecureToken -Length $TokenLength
    Write-Host ""
    Write-Host "Generated bridge token:"
    Write-Host $generated

    $installConfig = $AutoInstallConfig.IsPresent
    $setSession = $SetSessionEnv.IsPresent
    $setUser = $SetUserEnv.IsPresent

    if (-not $NoPrompt) {
        if (-not $installConfig) {
            $installConfig = Ask-YesNo "Install this token into $Config (bridge.auth_token)?"
        }
        if (-not $setSession) {
            $setSession = Ask-YesNo "Set PHANTOM_BRIDGE_AUTH_TOKEN for this terminal session?"
        }
        if (-not $setUser) {
            $setUser = Ask-YesNo "Persist PHANTOM_BRIDGE_AUTH_TOKEN for this Windows user?"
        }
    }

    $summary = [System.Collections.Generic.List[string]]::new()
    $summary.Add("token generated")

    if ($installConfig) {
        $backup = Set-BridgeTokenInConfig -Path $Config -BridgeToken $generated
        $summary.Add("config updated: $Config")
        $summary.Add("config backup: $backup")
    }
    if ($setSession) {
        $env:PHANTOM_BRIDGE_AUTH_TOKEN = $generated
        $summary.Add("session env set: PHANTOM_BRIDGE_AUTH_TOKEN")
    }
    if ($setUser) {
        [Environment]::SetEnvironmentVariable("PHANTOM_BRIDGE_AUTH_TOKEN", $generated, "User")
        $summary.Add("user env set: PHANTOM_BRIDGE_AUTH_TOKEN")
    }

    Write-Host ""
    Write-Host "Done:"
    foreach ($line in $summary) {
        Write-Host ("- " + $line)
    }
    Write-Host ""
    Write-Host "Next:"
    Write-Host "- Set EA input BridgeAuthToken to this token."
    Write-Host "- Restart PhantomClaw and reload EA."
    Write-Host ""
    Write-Host "Token (copy): $generated"
    return
}

function Invoke-Handshake {
    $bridgeToken = $Token
    if ([string]::IsNullOrWhiteSpace($bridgeToken)) {
        $bridgeToken = $env:PHANTOM_BRIDGE_AUTH_TOKEN
    }
    if ([string]::IsNullOrWhiteSpace($bridgeToken)) {
        throw "Provide -Token or set PHANTOM_BRIDGE_AUTH_TOKEN."
    }

    Write-Host "Bridge /health"
    Invoke-NativeChecked -FilePath "curl.exe" -ArgumentList @("-s", "-i", "http://127.0.0.1:8765/health")
    Write-Host ""
    Write-Host "Bridge /account"
    Invoke-NativeChecked -FilePath "curl.exe" -ArgumentList @("-s", "-i", "-H", "X-Phantom-Bridge-Token: $bridgeToken", "-H", "X-Phantom-Bridge-Contract: v3", "http://127.0.0.1:8765/account")
}

switch ($Action) {
    "menu" { Invoke-Menu }
    "help" { Show-Help }
    "build" { Invoke-Build }
    "run" { Invoke-Run }
    "rebuild" { Invoke-Build; Invoke-Run }
    "test" { Invoke-Tests }
    "token" { Invoke-Token }
    "handshake" { Invoke-Handshake }
    default { Show-Help }
}
