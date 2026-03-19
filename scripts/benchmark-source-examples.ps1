param(
    [string]$GatewayUrl = "http://localhost:15012",
    [string]$Username = "admin",
    [string]$Password = "admin",
    [int]$ScaleFromZeroRuns = 3,
    [string[]]$ExampleId = @(),
    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Net.Http
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem

$repoRoot = Split-Path -Parent $PSScriptRoot

$examples = @(
    @{
        Id = "python-hello"
        Runtime = "python"
        BuildType = "manifest"
        Directory = Join-Path $repoRoot "examples\source-packaging\python-hello"
        FunctionName = "bench-python-hello"
        Payload = "bench-python"
        Expect = "Hello from docker-faas (python)."
    },
    @{
        Id = "python-polars"
        Runtime = "python"
        BuildType = "manifest"
        Directory = Join-Path $repoRoot "examples\source-packaging\python-polars"
        FunctionName = "bench-polars-stats"
        Payload = "name,value`nalpha,10`nbeta,20`n"
        Expect = "polars summary:"
    },
    @{
        Id = "python-opencv"
        Runtime = "python"
        BuildType = "manifest"
        Directory = Join-Path $repoRoot "examples\source-packaging\python-opencv"
        FunctionName = "bench-opencv-edge"
        Payload = "edges"
        Expect = "opencv edges:"
    },
    @{
        Id = "go-hello"
        Runtime = "go"
        BuildType = "manifest"
        Directory = Join-Path $repoRoot "examples\source-packaging\go-hello"
        FunctionName = "bench-hello-go"
        Payload = "bench-go"
        Expect = "Hello from docker-faas (go)."
    },
    @{
        Id = "node-hello"
        Runtime = "node"
        BuildType = "manifest"
        Directory = Join-Path $repoRoot "examples\source-packaging\node-hello"
        FunctionName = "bench-hello-node"
        Payload = "bench-node"
        Expect = "Hello from docker-faas (node)."
    },
    @{
        Id = "bash-uppercase"
        Runtime = "bash"
        BuildType = "manifest"
        Directory = Join-Path $repoRoot "examples\source-packaging\bash-uppercase"
        FunctionName = "bench-uppercase-bash"
        Payload = "bench-bash"
        Expect = "BENCH-BASH"
    },
    @{
        Id = "python-uv"
        Runtime = "python"
        BuildType = "dockerfile"
        Directory = Join-Path $repoRoot "examples\source-packaging\python-uv"
        FunctionName = "bench-python-uv"
        Payload = "bench-uv"
        Expect = "python-uv:"
    }
)

if ($ExampleId.Count -gt 0) {
    $requested = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($id in $ExampleId) {
        $requested.Add($id) | Out-Null
    }
    $examples = @($examples | Where-Object { $requested.Contains($_.Id) })
    if ($examples.Count -eq 0) {
        throw "No examples matched ExampleId filter: $($ExampleId -join ', ')"
    }
}

function New-HttpClient {
    param(
        [string]$User,
        [string]$Pass
    )

    $client = [System.Net.Http.HttpClient]::new()
    $client.Timeout = [TimeSpan]::FromMinutes(30)
    $bytes = [System.Text.Encoding]::ASCII.GetBytes("${User}:${Pass}")
    $token = [Convert]::ToBase64String($bytes)
    $client.DefaultRequestHeaders.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new("Basic", $token)
    return $client
}

function Invoke-Http {
    param(
        [System.Net.Http.HttpClient]$Client,
        [System.Net.Http.HttpMethod]$Method,
        [string]$Uri,
        [System.Net.Http.HttpContent]$Content = $null
    )

    $request = [System.Net.Http.HttpRequestMessage]::new($Method, $Uri)
    if ($null -ne $Content) {
        $request.Content = $Content
    }

    $response = $Client.SendAsync($request).GetAwaiter().GetResult()
    $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
    return [pscustomobject]@{
        StatusCode = [int]$response.StatusCode
        IsSuccessStatusCode = $response.IsSuccessStatusCode
        Body = $body
    }
}

function Get-Functions {
    param(
        [System.Net.Http.HttpClient]$Client,
        [string]$Gateway
    )

    $response = Invoke-Http -Client $Client -Method ([System.Net.Http.HttpMethod]::Get) -Uri "$Gateway/system/functions"
    if (-not $response.IsSuccessStatusCode) {
        throw "Failed to list functions ($($response.StatusCode)): $($response.Body)"
    }
    if ([string]::IsNullOrWhiteSpace($response.Body)) {
        return @()
    }
    return @($response.Body | ConvertFrom-Json)
}

function Remove-FunctionIfExists {
    param(
        [System.Net.Http.HttpClient]$Client,
        [string]$Gateway,
        [string]$FunctionName
    )

    $functions = Get-Functions -Client $Client -Gateway $Gateway
    if ($functions | Where-Object { $_.name -eq $FunctionName }) {
        $response = Invoke-Http -Client $Client -Method ([System.Net.Http.HttpMethod]::Delete) -Uri "$Gateway/system/functions?functionName=$FunctionName"
        if (-not $response.IsSuccessStatusCode) {
            throw "Failed to delete $FunctionName ($($response.StatusCode)): $($response.Body)"
        }

        $deadline = (Get-Date).AddMinutes(2)
        do {
            Start-Sleep -Milliseconds 500
            $functions = Get-Functions -Client $Client -Gateway $Gateway
        } while (($functions | Where-Object { $_.name -eq $FunctionName }) -and (Get-Date) -lt $deadline)

        if ($functions | Where-Object { $_.name -eq $FunctionName }) {
            throw "Timed out deleting function $FunctionName"
        }
    }
}

function Wait-ForReplicaCount {
    param(
        [System.Net.Http.HttpClient]$Client,
        [string]$Gateway,
        [string]$FunctionName,
        [int]$AvailableReplicas,
        [int]$TimeoutSeconds = 120
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        $functions = Get-Functions -Client $Client -Gateway $Gateway
        $function = $functions | Where-Object { $_.name -eq $FunctionName } | Select-Object -First 1
        if ($null -ne $function -and [int]$function.availableReplicas -eq $AvailableReplicas) {
            return $function
        }
        Start-Sleep -Milliseconds 500
    } while ((Get-Date) -lt $deadline)

    throw "Timed out waiting for $FunctionName to reach availableReplicas=$AvailableReplicas"
}

function Set-FunctionReplicas {
    param(
        [System.Net.Http.HttpClient]$Client,
        [string]$Gateway,
        [string]$FunctionName,
        [int]$Replicas
    )

    $body = @{ serviceName = $FunctionName; replicas = $Replicas } | ConvertTo-Json -Compress
    $content = [System.Net.Http.StringContent]::new($body, [System.Text.Encoding]::UTF8, "application/json")
    $response = Invoke-Http -Client $Client -Method ([System.Net.Http.HttpMethod]::Post) -Uri "$Gateway/system/scale-function/$FunctionName" -Content $content
    if (-not $response.IsSuccessStatusCode) {
        throw "Failed to scale $FunctionName to $Replicas ($($response.StatusCode)): $($response.Body)"
    }
}

function Invoke-FunctionUntilSuccess {
    param(
        [System.Net.Http.HttpClient]$Client,
        [string]$Gateway,
        [string]$FunctionName,
        [string]$Payload,
        [string]$Expect,
        [int]$TimeoutSeconds = 180
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $attempts = 0
    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    $lastStatus = 0
    $lastBody = ""

    do {
        $attempts++
        $content = [System.Net.Http.StringContent]::new($Payload, [System.Text.Encoding]::UTF8, "text/plain")
        $response = Invoke-Http -Client $Client -Method ([System.Net.Http.HttpMethod]::Post) -Uri "$Gateway/function/$FunctionName" -Content $content
        $lastStatus = $response.StatusCode
        $lastBody = $response.Body

        if ($response.IsSuccessStatusCode -and ($response.Body -like "*$Expect*")) {
            $stopwatch.Stop()
            return [pscustomobject]@{
                DurationMs = [math]::Round($stopwatch.Elapsed.TotalMilliseconds, 2)
                Attempts = $attempts
                StatusCode = $response.StatusCode
                Body = $response.Body
            }
        }

        Start-Sleep -Milliseconds 500
    } while ((Get-Date) -lt $deadline)

    throw "Invocation failed for $FunctionName after $attempts attempt(s). Last status=$lastStatus body=$lastBody"
}

function New-ZipFromDirectory {
    param(
        [string]$SourceDirectory,
        [string]$DestinationZip
    )

    if (Test-Path $DestinationZip) {
        Remove-Item $DestinationZip -Force
    }

    $sourceRoot = (Resolve-Path $SourceDirectory).Path
    $zipStream = [System.IO.File]::Open($DestinationZip, [System.IO.FileMode]::CreateNew)
    try {
        $archive = [System.IO.Compression.ZipArchive]::new($zipStream, [System.IO.Compression.ZipArchiveMode]::Create, $false)
        try {
            $files = Get-ChildItem -Path $sourceRoot -Recurse -Force -File
            foreach ($file in $files) {
                $relativePath = $file.FullName.Substring($sourceRoot.Length).TrimStart('\')
                $entryName = $relativePath -replace '\\', '/'
                $entry = $archive.CreateEntry($entryName, [System.IO.Compression.CompressionLevel]::Optimal)
                $entryStream = $entry.Open()
                try {
                    $fileStream = [System.IO.File]::OpenRead($file.FullName)
                    try {
                        $fileStream.CopyTo($entryStream)
                    } finally {
                        $fileStream.Dispose()
                    }
                } finally {
                    $entryStream.Dispose()
                }
            }
        } finally {
            $archive.Dispose()
        }
    } finally {
        $zipStream.Dispose()
    }
}

function Invoke-Build {
    param(
        [System.Net.Http.HttpClient]$Client,
        [string]$Gateway,
        [string]$Name,
        [string]$ZipPath
    )

    $multipart = [System.Net.Http.MultipartFormDataContent]::new()
    $multipart.Add([System.Net.Http.StringContent]::new($Name), "name")
    $multipart.Add([System.Net.Http.StringContent]::new("zip"), "sourceType")
    $multipart.Add([System.Net.Http.StringContent]::new("true"), "deploy")

    $stream = [System.IO.File]::OpenRead($ZipPath)
    try {
        $fileContent = [System.Net.Http.StreamContent]::new($stream)
        $fileContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse("application/zip")
        $multipart.Add($fileContent, "file", [System.IO.Path]::GetFileName($ZipPath))

        $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
        $response = Invoke-Http -Client $Client -Method ([System.Net.Http.HttpMethod]::Post) -Uri "$Gateway/system/builds" -Content $multipart
        $stopwatch.Stop()

        if (-not $response.IsSuccessStatusCode) {
            throw "Build failed for $Name ($($response.StatusCode)): $($response.Body)"
        }

        $parsed = $response.Body | ConvertFrom-Json
        return [pscustomobject]@{
            DurationMs = [math]::Round($stopwatch.Elapsed.TotalMilliseconds, 2)
            Response = $parsed
        }
    } finally {
        $stream.Dispose()
        $multipart.Dispose()
    }
}

$client = New-HttpClient -User $Username -Pass $Password
$results = [System.Collections.Generic.List[object]]::new()

foreach ($example in $examples) {
    Write-Host ("Benchmarking {0} ({1}, {2})" -f $example.Id, $example.Runtime, $example.BuildType) -ForegroundColor Cyan
    Remove-FunctionIfExists -Client $client -Gateway $GatewayUrl -FunctionName $example.FunctionName

    $zipPath = Join-Path ([System.IO.Path]::GetTempPath()) ("docker-faas-{0}.zip" -f $example.FunctionName)
    New-ZipFromDirectory -SourceDirectory $example.Directory -DestinationZip $zipPath

    try {
        $build = Invoke-Build -Client $client -Gateway $GatewayUrl -Name $example.FunctionName -ZipPath $zipPath
        $firstSuccess = Invoke-FunctionUntilSuccess -Client $client -Gateway $GatewayUrl -FunctionName $example.FunctionName -Payload $example.Payload -Expect $example.Expect
        $warmSuccess = Invoke-FunctionUntilSuccess -Client $client -Gateway $GatewayUrl -FunctionName $example.FunctionName -Payload $example.Payload -Expect $example.Expect -TimeoutSeconds 30

        $scaleRuns = @()
        for ($i = 0; $i -lt $ScaleFromZeroRuns; $i++) {
            Set-FunctionReplicas -Client $client -Gateway $GatewayUrl -FunctionName $example.FunctionName -Replicas 0
            $null = Wait-ForReplicaCount -Client $client -Gateway $GatewayUrl -FunctionName $example.FunctionName -AvailableReplicas 0
            Start-Sleep -Seconds 2

            $scaleResult = Invoke-FunctionUntilSuccess -Client $client -Gateway $GatewayUrl -FunctionName $example.FunctionName -Payload $example.Payload -Expect $example.Expect
            $scaleRuns += $scaleResult
        }

        $scaleMetrics = $scaleRuns | ForEach-Object { [double]$_.DurationMs }
        $result = [pscustomobject]@{
            example = $example.Id
            runtime = $example.Runtime
            buildType = $example.BuildType
            functionName = $example.FunctionName
            buildApiMs = $build.DurationMs
            buildToFirstSuccessMs = [math]::Round(($build.DurationMs + [double]$firstSuccess.DurationMs), 2)
            firstInvokeMs = $firstSuccess.DurationMs
            warmInvokeMs = $warmSuccess.DurationMs
            scaleFromZeroRuns = $ScaleFromZeroRuns
            scaleFromZeroAvgMs = [math]::Round((($scaleMetrics | Measure-Object -Average).Average), 2)
            scaleFromZeroMinMs = [math]::Round((($scaleMetrics | Measure-Object -Minimum).Minimum), 2)
            scaleFromZeroMaxMs = [math]::Round((($scaleMetrics | Measure-Object -Maximum).Maximum), 2)
            scaleFromZeroAttempts = @($scaleRuns | ForEach-Object { $_.Attempts })
            sampleResponse = $firstSuccess.Body.Trim()
        }
        $results.Add($result)

        Write-Host ("  build={0}ms first={1}ms warm={2}ms scale0(avg/min/max)={3}/{4}/{5}ms" -f `
            $result.buildApiMs, $result.firstInvokeMs, $result.warmInvokeMs, `
            $result.scaleFromZeroAvgMs, $result.scaleFromZeroMinMs, $result.scaleFromZeroMaxMs) -ForegroundColor Green
    } catch {
        $results.Add([pscustomobject]@{
            example = $example.Id
            runtime = $example.Runtime
            buildType = $example.BuildType
            functionName = $example.FunctionName
            error = $_.Exception.Message
        })
        Write-Host ("  failed: {0}" -f $_.Exception.Message) -ForegroundColor Red
    } finally {
        Remove-Item $zipPath -Force -ErrorAction SilentlyContinue
        Remove-FunctionIfExists -Client $client -Gateway $GatewayUrl -FunctionName $example.FunctionName
    }
}

$json = $results | ConvertTo-Json -Depth 6

if ($OutputPath) {
    $resolvedOutput = if ([System.IO.Path]::IsPathRooted($OutputPath)) {
        $OutputPath
    } else {
        Join-Path $repoRoot $OutputPath
    }
    $outputDir = Split-Path -Parent $resolvedOutput
    if ($outputDir -and -not (Test-Path $outputDir)) {
        New-Item -ItemType Directory -Path $outputDir | Out-Null
    }
    Set-Content -Path $resolvedOutput -Value $json
    Write-Host "Wrote benchmark results to $resolvedOutput"
}

$results | Format-Table -AutoSize
$json
