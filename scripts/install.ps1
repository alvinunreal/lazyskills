param(
  [string]$Version = "latest",
  [string]$BinDir = "$env:LOCALAPPDATA\Programs\lazyskills\bin",
  [switch]$NoPathUpdate
)

$ErrorActionPreference = "Stop"

$Owner = "alvinunreal"
$Repo = "lazyskills"
$Binary = "lazyskills.exe"

function Prompt-StarRepo {
  if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { return }
  if (-not [Environment]::UserInteractive -or [Console]::IsInputRedirected) { return }

  $answer = Read-Host "Would you like to star ${Owner}/${Repo} on GitHub? [Y/n]"
  if ($answer -eq "" -or $answer -match "^(?i:y|yes)$") {
    & gh repo star "${Owner}/${Repo}" | Out-Null
    if ($LASTEXITCODE -eq 0) {
      Write-Host "Starred ${Owner}/${Repo}."
    } else {
      Write-Warning "Could not star ${Owner}/${Repo} with GitHub CLI; continuing."
    }
  }
}

$osArchitecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
$processorArchitecture = $env:PROCESSOR_ARCHITECTURE

$arch = switch ($osArchitecture) {
  "X64" { "x86_64"; break }
  "Arm64" { "arm64"; break }
  default {
    switch ($processorArchitecture) {
      "AMD64" { "x86_64"; break }
      "ARM64" { "arm64"; break }
      default { throw "Unsupported architecture: OSArchitecture=${osArchitecture}; PROCESSOR_ARCHITECTURE=${processorArchitecture}" }
    }
  }
}

$archive = "${Repo}_Windows_${arch}.zip"
$baseUrl = "https://github.com/${Owner}/${Repo}/releases"
if ($Version -eq "latest") {
  $downloadUrl = "${baseUrl}/latest/download/${archive}"
  $checksumUrl = "${baseUrl}/latest/download/checksums.txt"
} else {
  $downloadUrl = "${baseUrl}/download/${Version}/${archive}"
  $checksumUrl = "${baseUrl}/download/${Version}/checksums.txt"
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
  $archivePath = Join-Path $tmp $archive
  $checksumsPath = Join-Path $tmp "checksums.txt"

  Write-Host "Downloading ${archive}..."
  Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath
  Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumsPath

  Write-Host "Verifying checksum..."
  $expectedLine = Get-Content $checksumsPath | Where-Object { $_ -match [regex]::Escape($archive) } | Select-Object -First 1
  if (-not $expectedLine) { throw "Checksum for ${archive} not found" }
  $expected = ($expectedLine -split "\s+")[0].ToLowerInvariant()
  $actual = (Get-FileHash -Algorithm SHA256 $archivePath).Hash.ToLowerInvariant()
  if ($expected -ne $actual) { throw "Checksum mismatch for ${archive}" }

  Expand-Archive -Path $archivePath -DestinationPath $tmp -Force

  New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
  Copy-Item -Path (Join-Path $tmp $Binary) -Destination (Join-Path $BinDir $Binary) -Force

  if (-not $NoPathUpdate) {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $paths = @($userPath -split ";" | Where-Object { $_ })
    if ($paths -notcontains $BinDir) {
      [Environment]::SetEnvironmentVariable("Path", (($paths + $BinDir) -join ";"), "User")
      Write-Host "Added ${BinDir} to your user PATH. Restart your terminal to use lazyskills everywhere."
    }
  }

  & (Join-Path $BinDir $Binary) version

  Prompt-StarRepo
} finally {
  Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
