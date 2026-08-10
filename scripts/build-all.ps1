param()

$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $repoRoot

Write-Host ""
Write-Host "===== gofmt ====="

$goFiles = (Get-ChildItem -Recurse -Filter "*.go" |
    Where-Object {
        $_.FullName -notmatch "\\_parity\\" -and
        $_.FullName -notmatch "\\.git\\"
    } |
    ForEach-Object { $_.FullName }) |
    Sort-Object -Unique

gofmt -w $goFiles

Write-Host ""
Write-Host "===== go test ./... ====="

go test ./...

Write-Host ""
Write-Host "===== build binaries ====="

New-Item -ItemType Directory -Force -Path ".\builds" | Out-Null

$targets = @(
    @{ Name = "ocpphal";         Path = ".\cmd\ocpphal" },
    @{ Name = "cpconsole";       Path = ".\cmd\cpconsole" }
)

foreach ($target in $targets) {
    $out = ".\builds\$($target.Name).exe"
    Write-Host "Building $out"
    go build -o $out $target.Path
}

Write-Host ""
Write-Host "===== build complete ====="

Get-ChildItem ".\builds" -Filter "*.exe" |
    Select-Object FullName, Length, LastWriteTime |
    Format-Table -AutoSize
