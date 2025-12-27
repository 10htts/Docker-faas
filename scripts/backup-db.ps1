param(
    [string]$BackupDir = "./backups",
    [int]$RetentionDays = 7
)

$container = $env:GATEWAY_CONTAINER_NAME
if (-not $container) {
    $container = "docker-faas-gateway"
}

$dbPath = $env:STATE_DB_PATH
if (-not $dbPath) {
    $dbPath = "/data/docker-faas.db"
}

$timestamp = Get-Date -Format "yyyyMMddHHmmss"
$backupName = "backup-$timestamp.db"
$containerBackup = "/data/$backupName"

Write-Host "Creating backup from ${container}:${dbPath}"
docker exec $container sqlite3 $dbPath ".backup '$containerBackup'"

New-Item -ItemType Directory -Force -Path $BackupDir | Out-Null
docker cp "${container}:$containerBackup" (Join-Path $BackupDir $backupName) | Out-Null
docker exec $container rm -f $containerBackup | Out-Null

Get-ChildItem -Path $BackupDir -Filter "backup-*.db" |
    Where-Object { $_.LastWriteTime -lt (Get-Date).AddDays(-$RetentionDays) } |
    Remove-Item -Force

Write-Host "Backup stored at $(Join-Path $BackupDir $backupName)"
