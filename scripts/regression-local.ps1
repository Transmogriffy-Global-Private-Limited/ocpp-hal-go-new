param(
    [switch] $SkipBuild
)

$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $repoRoot

if (-not $SkipBuild) {
    & ".\scripts\build-all.ps1"
}

if ([string]::IsNullOrWhiteSpace($env:TEST_DATABASE_URL)) {
    $env:TEST_DATABASE_URL = Read-Host "Disposable HAL PostgreSQL TEST_DATABASE_URL"
}

if ([string]::IsNullOrWhiteSpace($env:TEST_DATABASE_URL)) {
    throw "TEST_DATABASE_URL is required for the v1 regression suite."
}

# Older integration tests still read DATABASE_URL. They receive only the
# explicitly supplied disposable test target; the script never prompts for or
# falls back to the application's runtime database setting.
$env:DATABASE_URL = $env:TEST_DATABASE_URL

Write-Host ""
Write-Host "===== v1 HTTP, OCPP, PostgreSQL, and fact-receiver regression ====="

$env:HAL_RUN_CONTRACT_LIFECYCLE = "true"
go test ./internal/integration -count=1

Write-Host ""
Write-Host "===== v1 PostgreSQL lifecycle and fact-outbox regression ====="

go test ./internal/store -run 'TestV1(PostgresStoreDurabilityAndRuntime|FactOutbox|StopLifecycleConcurrentDeliveryAndCompletion)' -count=1

Write-Host ""
Write-Host "===== local v1 regression passed ====="
