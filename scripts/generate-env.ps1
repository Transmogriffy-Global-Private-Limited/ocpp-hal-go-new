$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$envExample = Join-Path $repoRoot '.env.example'
$envFile = Join-Path $repoRoot '.env'

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
    param([int] $Bytes = 48)

    $buffer = [byte[]]::new($Bytes)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($buffer)
    return [Convert]::ToBase64String($buffer).TrimEnd('=').Replace('+', '-').Replace('/', '_')
}

Write-Host ""
Write-Host "Paste the disposable HAL PostgreSQL DATABASE_URL. It will not be echoed."
$databaseUrl = Read-Host "DATABASE_URL"

if ([string]::IsNullOrWhiteSpace($databaseUrl) -or $databaseUrl -notmatch '^postgres(ql)?://') {
    throw "DATABASE_URL must be a PostgreSQL connection URL."
}

$v1CmsBearer = New-RandomSecret 64
$v1FactsBearer = New-RandomSecret 64
$suffix = ([Guid]::NewGuid().ToString('N')).Substring(0, 8).ToUpperInvariant()

$content = @"
# LOCAL DEVELOPMENT ONLY. DO NOT COMMIT.
HAL_ENVIRONMENT=development
F_SERVER_HOST=127.0.0.1
F_SERVER_PORT=18080
OCPP_LISTEN_PORT=18081
OCPP_LISTEN_PATH=/{ws}

# CMS-to-HAL service identity
HAL_V1_CMS_BEARER_TOKEN=$v1CmsBearer

# Keep fact delivery disabled until a local contract receiver is intentionally started.
HAL_V1_FACT_DELIVERY_ENABLED=false
HAL_V1_CMS_FACTS_URL=
HAL_V1_CMS_FACT_BEARER_TOKEN=$v1FactsBearer
API_DOCS_ENABLED=false
LOG_LEVEL=debug

DATABASE_URL=$databaseUrl
DB_HOST=127.0.0.1
DB_PORT=5432
DB_NAME=ocppgo
DB_USER=ocppgodbadmin
DB_PASSWORD=$(New-RandomSecret)
DB_SSLMODE=require

CP_SIM_URL=ws://127.0.0.1:18081
CP_SIM_ID=CP-SIM-$suffix
CP_SIM_MODEL=TransEV-Simulator
CP_SIM_VENDOR=TransEV
CP_SIM_CONNECTOR=1
CP_SIM_METER_START_WH=100000
CP_SIM_VOLTAGE=230
CP_SIM_SOC=35
"@

$content | Set-Content -Path $envFile -Encoding utf8NoBOM

git check-ignore -q -- '.env'
if ($LASTEXITCODE -ne 0) {
    throw ".env is not ignored; do not use generated local secrets."
}

Write-Host "Created $envFile with independent v1 service secrets."
