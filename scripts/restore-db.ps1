param(
    [Parameter(Mandatory = $true)]
    [string]$BackupFile,
    [switch]$NoStop
)

$container = $env:GATEWAY_CONTAINER_NAME
if (-not $container) {
    $container = "docker-faas-gateway"
}

$dbPath = $env:STATE_DB_PATH
if (-not $dbPath) {
    $dbPath = "/data/docker-faas.db"
}

if (-not (Test-Path $BackupFile)) {
    Write-Error "Backup file not found: $BackupFile"
    exit 1
}

if (-not $NoStop) {
    docker stop $container | Out-Null
}

docker cp $BackupFile "$container:$dbPath" | Out-Null

if (-not $NoStop) {
    docker start $container | Out-Null
}

Write-Host "Restored $BackupFile to $container:$dbPath"
