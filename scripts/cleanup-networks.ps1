param(
    [switch]$DryRun
)

$label = "com.docker-faas.network.type=function"
$gateway = $env:GATEWAY_CONTAINER_NAME
if (-not $gateway) {
    $gateway = "docker-faas-gateway"
}

$networks = docker network ls --filter "label=$label" --format "{{.Name}}"
if (-not $networks) {
    Write-Host "No managed function networks found."
    exit 0
}

foreach ($network in $networks) {
    $inspect = docker network inspect $network | ConvertFrom-Json
    $containers = $inspect[0].Containers
    if (-not $containers) {
        Write-Host "Removing unused network: $network"
        if (-not $DryRun) {
            docker network rm $network | Out-Null
        }
        continue
    }

    $names = @()
    foreach ($entry in $containers.GetEnumerator()) {
        $names += $entry.Value.Name
    }

    if ($names.Count -eq 1 -and $names[0] -eq $gateway) {
        Write-Host "Disconnecting gateway and removing network: $network"
        if (-not $DryRun) {
            docker network disconnect -f $network $gateway | Out-Null
            docker network rm $network | Out-Null
        }
        continue
    }

    Write-Host "Skipping network in use: $network (containers: $($names.Count))"
}
