[CmdletBinding()]
param(
    [ValidatePattern('^\d+\.\d+\.\d+$')]
    [string]$Version = '0.3.1',

    [ValidateRange(1, 2100000000)]
    [int]$AndroidVersionCode = 5,

    [string]$OutputDirectory,

    [switch]$SkipAndroid,

    [switch]$SkipWindows
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $projectRoot 'dist'
}
$OutputDirectory = [System.IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null

function Invoke-NativeCommand {
    param(
        [Parameter(Mandatory)] [string]$FilePath,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [Parameter(Mandatory)] [string]$WorkingDirectory
    )

    Push-Location $WorkingDirectory
    try {
        Write-Host "`n> $FilePath $($Arguments -join ' ')" -ForegroundColor Cyan
        & $FilePath @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "Command failed with exit code ${LASTEXITCODE}: $FilePath"
        }
    } finally {
        Pop-Location
    }
}

function Get-EnvironmentValue {
    param([Parameter(Mandatory)] [string]$Name)
    return [Environment]::GetEnvironmentVariable($Name, 'Process')
}

function Find-ApkSigner {
    $command = Get-Command 'apksigner.bat' -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        return $command.Source
    }

    $sdkRoots = @(
        @(
            (Get-EnvironmentValue 'ANDROID_SDK_ROOT'),
            (Get-EnvironmentValue 'ANDROID_HOME')
        ) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    )

    $localProperties = Join-Path $projectRoot 'local.properties'
    if (Test-Path -LiteralPath $localProperties) {
        $sdkLine = Get-Content -LiteralPath $localProperties |
            Where-Object { $_ -match '^sdk\.dir=' } |
            Select-Object -First 1
        if ($null -ne $sdkLine) {
            $sdkPath = $sdkLine.Substring($sdkLine.IndexOf('=') + 1)
            $sdkPath = $sdkPath -replace '\\:', ':' -replace '\\\\', '\'
            $sdkRoots += $sdkPath
        }
    }

    foreach ($sdkRoot in ($sdkRoots | Select-Object -Unique)) {
        $buildTools = Join-Path $sdkRoot 'build-tools'
        if (-not (Test-Path -LiteralPath $buildTools)) {
            continue
        }
        $candidate = Get-ChildItem -LiteralPath $buildTools -Recurse -Filter 'apksigner.bat' |
            Sort-Object FullName -Descending |
            Select-Object -First 1
        if ($null -ne $candidate) {
            return $candidate.FullName
        }
    }
    return $null
}

function Find-SignTool {
    $command = Get-Command 'signtool.exe' -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        return $command.Source
    }

    $kitRoots = @(
        (Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\bin'),
        (Join-Path $env:ProgramFiles 'Windows Kits\10\bin')
    ) | Where-Object { Test-Path -LiteralPath $_ }

    $candidates = foreach ($kitRoot in $kitRoots) {
        Get-ChildItem -LiteralPath $kitRoot -Recurse -Filter 'signtool.exe' -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -match '[\\/]x64[\\/]signtool\.exe$' }
    }
    $candidate = $candidates | Sort-Object FullName -Descending | Select-Object -First 1
    if ($null -eq $candidate) {
        return $null
    }
    return $candidate.FullName
}

$artifacts = [System.Collections.Generic.List[string]]::new()

if (-not $SkipAndroid) {
    $androidSigningNames = @(
        'USBRIDGE_ANDROID_KEYSTORE',
        'USBRIDGE_ANDROID_STORE_PASSWORD',
        'USBRIDGE_ANDROID_KEY_ALIAS',
        'USBRIDGE_ANDROID_KEY_PASSWORD'
    )
    $androidSigningValues = $androidSigningNames | ForEach-Object { Get-EnvironmentValue $_ }
    $androidSigningCount = @(
        $androidSigningValues | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    ).Count
    if ($androidSigningCount -ne 0 -and $androidSigningCount -ne $androidSigningNames.Count) {
        throw 'Android signing is partially configured. Set all four USBRIDGE_ANDROID_* variables.'
    }
    $androidSigned = $androidSigningCount -eq $androidSigningNames.Count

    Invoke-NativeCommand `
        -FilePath (Join-Path $projectRoot 'gradlew.bat') `
        -Arguments @(
            ':app:assembleRelease',
            "-PusbridgeVersionName=$Version",
            "-PusbridgeVersionCode=$AndroidVersionCode"
        ) `
        -WorkingDirectory $projectRoot

    $apkDirectory = Join-Path $projectRoot 'app\build\outputs\apk\release'
    $apk = if ($androidSigned) {
        Get-ChildItem -LiteralPath $apkDirectory -Filter '*.apk' |
            Where-Object { $_.Name -notmatch 'unsigned' } |
            Select-Object -First 1
    } else {
        Get-ChildItem -LiteralPath $apkDirectory -Filter '*unsigned*.apk' |
            Select-Object -First 1
    }
    if ($null -eq $apk) {
        throw "Android release APK was not found in $apkDirectory"
    }

    $androidLabel = if ($androidSigned) { 'signed' } else { 'unsigned' }
    $androidOutput = Join-Path $OutputDirectory "USBridge-Android-$Version-$androidLabel.apk"
    Copy-Item -LiteralPath $apk.FullName -Destination $androidOutput -Force

    if ($androidSigned) {
        $apkSigner = Find-ApkSigner
        if ([string]::IsNullOrWhiteSpace($apkSigner)) {
            throw 'apksigner was not found; refusing to publish an unverified signed APK.'
        }
        Invoke-NativeCommand `
            -FilePath $apkSigner `
            -Arguments @('verify', '--verbose', '--print-certs', $androidOutput) `
            -WorkingDirectory $projectRoot
    } else {
        Write-Warning 'Android signing variables are absent. The APK is clearly marked unsigned.'
    }
    $artifacts.Add($androidOutput)
}

if (-not $SkipWindows) {
    $wailsConfig = Get-Content -LiteralPath (Join-Path $projectRoot 'desktop\wails.json') -Raw |
        ConvertFrom-Json
    if ($wailsConfig.info.productVersion -ne $Version) {
        throw "wails.json productVersion is $($wailsConfig.info.productVersion), expected $Version"
    }

    $commit = 'unknown'
    if ((Test-Path -LiteralPath (Join-Path $projectRoot '.git')) -and
        ($null -ne (Get-Command 'git.exe' -ErrorAction SilentlyContinue))) {
        Push-Location $projectRoot
        try {
            $candidateCommit = (& git.exe rev-parse --short=12 HEAD 2>$null).Trim()
            if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($candidateCommit)) {
                $commit = $candidateCommit
            }
        } finally {
            Pop-Location
        }
    }

    $ldFlags = "-s -w -X github.com/usbridge/usbridge/desktop/internal/version.Version=$Version " +
        "-X github.com/usbridge/usbridge/desktop/internal/version.Commit=$commit"
    $wailsArguments = @(
        'build',
        '-trimpath',
        '-platform', 'windows/amd64',
        '-o', 'USBridge-release.exe',
        '-ldflags', $ldFlags
    )
    $wails = Get-Command 'wails.exe' -ErrorAction SilentlyContinue
    if ($null -ne $wails) {
        Invoke-NativeCommand -FilePath $wails.Source -Arguments $wailsArguments `
            -WorkingDirectory (Join-Path $projectRoot 'desktop')
    } else {
        $go = Get-Command 'go.exe' -ErrorAction Stop
        $localWails = Join-Path $projectRoot 'desktop\.tools\wails.exe'
        if (-not (Test-Path -LiteralPath $localWails)) {
            $goModuleCache = (& $go.Source env GOMODCACHE).Trim()
            if ($LASTEXITCODE -ne 0) {
                throw 'Unable to locate the Go module cache.'
            }
            $cachedWailsSource = Join-Path $goModuleCache `
                'github.com\wailsapp\wails\v2@v2.13.0'
            if (Test-Path -LiteralPath (Join-Path $cachedWailsSource 'cmd\wails\main.go')) {
                New-Item -ItemType Directory -Force -Path (Split-Path -Parent $localWails) |
                    Out-Null
                $previousGoProxy = Get-EnvironmentValue 'GOPROXY'
                try {
                    $env:GOPROXY = 'off'
                    Invoke-NativeCommand -FilePath $go.Source `
                        -Arguments @('build', '-trimpath', '-o', $localWails, '.\cmd\wails') `
                        -WorkingDirectory $cachedWailsSource
                } finally {
                    if ($null -eq $previousGoProxy) {
                        Remove-Item Env:GOPROXY -ErrorAction SilentlyContinue
                    } else {
                        $env:GOPROXY = $previousGoProxy
                    }
                }
            }
        }

        if (Test-Path -LiteralPath $localWails) {
            Invoke-NativeCommand -FilePath $localWails -Arguments $wailsArguments `
                -WorkingDirectory (Join-Path $projectRoot 'desktop')
        } else {
            Invoke-NativeCommand -FilePath $go.Source `
                -Arguments (@('run', 'github.com/wailsapp/wails/v2/cmd/wails@v2.13.0') + $wailsArguments) `
                -WorkingDirectory (Join-Path $projectRoot 'desktop')
        }
    }

    $windowsExe = Join-Path $projectRoot 'desktop\build\bin\USBridge-release.exe'
    if (-not (Test-Path -LiteralPath $windowsExe)) {
        throw "Windows executable was not found: $windowsExe"
    }
    $windowsVersionInfo = [Diagnostics.FileVersionInfo]::GetVersionInfo($windowsExe)
    if ($windowsVersionInfo.FileVersion -ne $Version -or
        $windowsVersionInfo.ProductVersion -ne $Version -or
        $windowsVersionInfo.ProductName -ne 'USBridge' -or
        [string]::IsNullOrWhiteSpace($windowsVersionInfo.CompanyName) -or
        [string]::IsNullOrWhiteSpace($windowsVersionInfo.FileDescription)) {
        throw 'Windows version resources are missing or do not match the requested release version.'
    }

    $pfxPath = Get-EnvironmentValue 'USBRIDGE_WINDOWS_CERTIFICATE'
    $pfxPassword = Get-EnvironmentValue 'USBRIDGE_WINDOWS_CERT_PASSWORD'
    $thumbprint = Get-EnvironmentValue 'USBRIDGE_WINDOWS_CERT_THUMBPRINT'
    if (-not [string]::IsNullOrWhiteSpace($pfxPath) -and
        -not [string]::IsNullOrWhiteSpace($thumbprint)) {
        throw 'Choose either USBRIDGE_WINDOWS_CERTIFICATE or USBRIDGE_WINDOWS_CERT_THUMBPRINT, not both.'
    }
    if (-not [string]::IsNullOrWhiteSpace($pfxPath) -and
        [string]::IsNullOrWhiteSpace($pfxPassword)) {
        throw 'USBRIDGE_WINDOWS_CERT_PASSWORD is required when a PFX certificate is used.'
    }
    $windowsSigned = -not [string]::IsNullOrWhiteSpace($pfxPath) -or
        -not [string]::IsNullOrWhiteSpace($thumbprint)
    $windowsLabel = if ($windowsSigned) { 'signed' } else { 'unsigned' }
    $windowsOutput = Join-Path $OutputDirectory "USBridge-Windows-x64-$Version-$windowsLabel.exe"
    Copy-Item -LiteralPath $windowsExe -Destination $windowsOutput -Force

    if ($windowsSigned) {
        $signTool = Find-SignTool
        if ([string]::IsNullOrWhiteSpace($signTool)) {
            throw 'signtool.exe was not found. Install the Windows SDK Signing Tools component.'
        }
        $timestampUrl = Get-EnvironmentValue 'USBRIDGE_WINDOWS_TIMESTAMP_URL'
        if ([string]::IsNullOrWhiteSpace($timestampUrl)) {
            $timestampUrl = 'http://timestamp.digicert.com'
        }
        $signArguments = @('sign', '/fd', 'SHA256', '/tr', $timestampUrl, '/td', 'SHA256')
        if (-not [string]::IsNullOrWhiteSpace($pfxPath)) {
            if (-not (Test-Path -LiteralPath $pfxPath)) {
                throw "Windows PFX certificate does not exist: $pfxPath"
            }
            $signArguments += @('/f', $pfxPath, '/p', $pfxPassword)
        } else {
            $signArguments += @('/sha1', ($thumbprint -replace '\s', ''))
        }
        $signArguments += $windowsOutput
        Invoke-NativeCommand -FilePath $signTool -Arguments $signArguments -WorkingDirectory $projectRoot
        Invoke-NativeCommand -FilePath $signTool `
            -Arguments @('verify', '/pa', '/all', '/v', $windowsOutput) `
            -WorkingDirectory $projectRoot
    } else {
        Write-Warning 'Windows signing certificate is absent. The EXE is clearly marked unsigned.'
    }
    $artifacts.Add($windowsOutput)
}

if ($artifacts.Count -eq 0) {
    throw 'Nothing was built because both platforms were skipped.'
}

$checksumLines = foreach ($artifact in $artifacts) {
    $hash = Get-FileHash -Algorithm SHA256 -LiteralPath $artifact
    "$($hash.Hash.ToLowerInvariant())  $([System.IO.Path]::GetFileName($artifact))"
}
$checksumFile = Join-Path $OutputDirectory 'SHA256SUMS.txt'
[System.IO.File]::WriteAllLines($checksumFile, $checksumLines, [System.Text.UTF8Encoding]::new($false))

Write-Host "`nRelease artifacts:" -ForegroundColor Green
Get-ChildItem -LiteralPath $OutputDirectory | Select-Object Name, Length, LastWriteTime
