Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepoSlug = if ($env:PLANMARK_REPO) { $env:PLANMARK_REPO } else { "Vekram1/PlanMark" }
$InstallDir = if ($env:PLANMARK_INSTALL_DIR) { $env:PLANMARK_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\PlanMark\bin" }
$Channel = if ($env:PLANMARK_CHANNEL) { $env:PLANMARK_CHANNEL } else { "stable" }
$Ref = if ($env:PLANMARK_REF) { $env:PLANMARK_REF } else { "" }
$BinName = if ($env:PLANMARK_BIN_NAME) { $env:PLANMARK_BIN_NAME } else { "planmark.exe" }
$LegacyAlias = if ($env:PLANMARK_LEGACY_ALIAS) { $env:PLANMARK_LEGACY_ALIAS } else { "1" }
$GithubBaseUrl = if ($env:PLANMARK_GITHUB_BASE_URL) { $env:PLANMARK_GITHUB_BASE_URL } else { "https://github.com/$RepoSlug" }
$TempDir = $null

function Write-Log {
    param([string]$Message)
    Write-Host "[planmark-install] $Message"
}

function Resolve-TargetRef {
    if ($Ref) {
        return $Ref
    }

    if ($Channel -eq "edge") {
        return "master"
    }
    if ($Channel -ne "stable") {
        throw "Invalid PLANMARK_CHANNEL=$Channel. Use stable or edge."
    }

    $latestApi = "https://api.github.com/repos/$RepoSlug/releases/latest"
    $latest = Invoke-RestMethod -Uri $latestApi
    if (-not $latest.tag_name) {
        throw "Could not resolve latest stable release tag from GitHub."
    }
    return [string]$latest.tag_name
}

function Resolve-Arch {
    switch ($env:PROCESSOR_ARCHITECTURE.ToUpperInvariant()) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}

try {
    $targetRef = Resolve-TargetRef
    $arch = Resolve-Arch
    $version = $targetRef.TrimStart("v")
    $archiveName = "planmark_${version}_windows_${arch}.zip"
    $downloadUrl = "$GithubBaseUrl/releases/download/$targetRef/$archiveName"

    $TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("planmark-install-" + [System.Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $TempDir | Out-Null

    $archivePath = Join-Path $TempDir $archiveName
    Write-Log "Downloading $downloadUrl"
    Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath
    Expand-Archive -Path $archivePath -DestinationPath $TempDir -Force

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item (Join-Path $TempDir "planmark.exe") (Join-Path $InstallDir $BinName) -Force
    if ($LegacyAlias -eq "1") {
        Copy-Item (Join-Path $TempDir "planmark.exe") (Join-Path $InstallDir "plan.exe") -Force
    }

    Write-Log "Installed: $(Join-Path $InstallDir $BinName)"
    & (Join-Path $InstallDir $BinName) version --format text *> $null
    Write-Log "Verification: OK"

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not $userPath) {
        $userPath = ""
    }
    $pathEntries = $userPath -split ";" | Where-Object { $_ }
    if ($pathEntries -notcontains $InstallDir) {
        Write-Log "Add this directory to your user PATH if needed:"
        Write-Log "  $InstallDir"
    }

    Write-Log "Next steps:"
    Write-Log "  1) cd <your-project>"
    Write-Log "  2) planmark --help"
    Write-Log "  3) planmark init --dir . --format text"
    Write-Log "  4) planmark compile --plan PLAN.md --out .planmark/tmp/plan.json"
    Write-Log "Installed release/ref: $targetRef"
}
finally {
    if ($TempDir -and (Test-Path $TempDir)) {
        Remove-Item -Path $TempDir -Recurse -Force
    }
}
