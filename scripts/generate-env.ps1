$ErrorActionPreference = 'Stop'

$repoRoot   = Split-Path -Parent $PSScriptRoot
$envExample = Join-Path $repoRoot '.env.example'
$envFile    = Join-Path $repoRoot '.env'

if (-not (Test-Path $envExample)) {
    throw ".env.example not found in $PSScriptRoot"
}

if (Test-Path $envFile) {
    $answer = Read-Host ".env already exists. Replace it? [y/N]"
    if ($answer -notmatch '^(y|yes)$') {
        Write-Host "Cancelled."
        exit 0
    }
}

function New-RandomSecret {
    param(
        [int]$Bytes = 48
    )

    $buffer = [byte[]]::new($Bytes)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($buffer)

    # URL/env-friendly base64url
    return [Convert]::ToBase64String($buffer).
        TrimEnd('=').
        Replace('+', '-').
        Replace('/', '_')
}

Write-Host ""
Write-Host "Paste the disposable HAL PostgreSQL DATABASE_URL."
Write-Host "It will NOT be echoed back after this."
Write-Host ""

$databaseUrl = Read-Host "DATABASE_URL"

if ([string]::IsNullOrWhiteSpace($databaseUrl)) {
    throw "DATABASE_URL cannot be empty."
}

if ($databaseUrl -notmatch '^postgres(ql)?://') {
    throw "DATABASE_URL does not look like a PostgreSQL connection URL."
}

# Generate independent secrets
$apiKey          = New-RandomSecret
$apiAuthKey      = New-RandomSecret
$v1CmsBearer     = New-RandomSecret 64
$v1FactsBearer   = New-RandomSecret 64

# Unique simulator identifiers, useful when multiple test environments exist
$suffix = ([Guid]::NewGuid().ToString('N')).Substring(0, 8).ToUpperInvariant()

$content = @"
# ============================================================
# LOCAL DEVELOPMENT ONLY
# Generated: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss K')
# DO NOT COMMIT
# ============================================================

# REST API
HAL_ENVIRONMENT=development
HAL_V1_ENABLED=true
F_SERVER_HOST=127.0.0.1
F_SERVER_PORT=18080

# OCPP central system
OCPP_LISTEN_PORT=18081
OCPP_LISTEN_PATH=/{ws}

# Legacy API auth
API_KEY=$apiKey
APIAUTHKEY=$apiAuthKey

# New CMS -> HAL v1 service auth
HAL_V1_CMS_BEARER_TOKEN=$v1CmsBearer

# HAL -> CMS fact delivery remains opt-in. Use a local receiver for testing;
# never point this generated file at the inherited CMS.
HAL_V1_FACT_DELIVERY_ENABLED=false
HAL_V1_CMS_FACTS_URL=
HAL_V1_CMS_FACT_BEARER_TOKEN=$v1FactsBearer

# Logging
LOG_LEVEL=debug

# PostgreSQL
# Full DATABASE_URL takes precedence over the split DB_* settings.
DATABASE_URL=$databaseUrl

# Split PostgreSQL settings intentionally unused when DATABASE_URL is set.
DB_HOST=127.0.0.1
DB_PORT=5432
DB_NAME=ocppgo
DB_USER=ocppgodbadmin
DB_PASSWORD=$(New-RandomSecret)
DB_SSLMODE=require

# ============================================================
# LEGACY CMS INTEGRATION
#
# IMPORTANT:
# Do NOT point the new HAL at the old production CMS.
#
# These localhost dead-end URLs intentionally override the old
# production defaults currently present in .env.example.
# ============================================================

MAIN_CMS_START_TXN_HOOK_URL=http://127.0.0.1:1/legacy-disabled/start
MAIN_CMS_COMPLETED_TXN_URL=http://127.0.0.1:1/legacy-disabled/completed

SINGLE_SESSION_START_TXN_HOOK_URL=http://127.0.0.1:1/legacy-disabled/start
SINGLE_SESSION_COMPLETED_TXN_URL=http://127.0.0.1:1/legacy-disabled/completed

# Legacy charger directory disabled
APICHARGERDATA=
CHARGER_DATA_CACHE_TTL_SECONDS=7200

# Local mock-hook tooling
MOCK_HOOK_ADDR=127.0.0.1:19090
MOCK_START_MAX_KWH=7.5

MOCK_CHARGER_IDS=CP-REG-CORE-$suffix,CP-REG-SINGLE-$suffix,CP-REG-LIMIT-$suffix,CP-REG-OFFLINE-$suffix

# Virtual OCPP charger
CP_SIM_URL=ws://127.0.0.1:18081
CP_SIM_ID=CP-SIM-$suffix
CP_SIM_MODEL=TransEV-Simulator
CP_SIM_VENDOR=TransEV
CP_SIM_CONNECTOR=1
CP_SIM_METER_START_WH=100000
CP_SIM_VOLTAGE=230
CP_SIM_SOC=35
"@

# UTF-8 without BOM on PowerShell 7+
$content | Set-Content -Path $envFile -Encoding utf8NoBOM

Write-Host ""
Write-Host "Created:"
Write-Host "  $envFile"
Write-Host ""
Write-Host "Generated:"
Write-Host "  API_KEY"
Write-Host "  APIAUTHKEY"
Write-Host "  HAL_V1_CMS_BEARER_TOKEN"
Write-Host "  HAL_V1_CMS_FACT_BEARER_TOKEN"
Write-Host "  local simulator/test identities"
Write-Host ""
Write-Host "DATABASE_URL was written but not printed."
Write-Host ""

# Confirm Git ignores it
git check-ignore -q -- '.env'

if ($LASTEXITCODE -eq 0) {
    Write-Host "Git check: .env is ignored. Good."
}
else {
    Write-Warning ".env is NOT ignored by Git. Do not proceed until this is fixed."
}

Write-Host ""
Write-Host "Done."
