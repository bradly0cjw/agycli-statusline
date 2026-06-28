# install.ps1 - PowerShell installer for Windows

Write-Host "====================================================" -ForegroundColor Blue
Write-Host "  Installing Antigravity CLI Statusline (Windows)  " -ForegroundColor Green
Write-Host "====================================================" -ForegroundColor Blue

$installDir = Join-Path $HOME ".antigravity"
if (-not (Test-Path $installDir)) {
    Write-Host "Creating installation directory: $installDir"
    New-Item -ItemType Directory -Path $installDir | Out-Null
}

$targetScript = Join-Path $installDir "statusline.exe"
$targetUninstall = Join-Path $installDir "uninstall.ps1"

# Check if we have local files
$isLocal = $false
if ($PSScriptRoot) {
    $sourceGo = Join-Path $PSScriptRoot "main.go"
    if (Test-Path $sourceGo) {
        $isLocal = $true
    }
}

# Verify go compiler is installed
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "Error: 'go' (Go compiler) is not found. Please install Go 1.21+ first." -ForegroundColor Red
    exit 1
}

if ($isLocal) {
    Write-Host "Installing from local files..."
    Write-Host "Compiling statusline.exe..."
    Push-Location $PSScriptRoot
    $commitHash = ""
    try {
        $commitHash = (git rev-parse --short HEAD 2>$null).Trim()
    } catch {}
    try {
        & go build -ldflags="-s -w -X main.commitHash=$commitHash" -o $targetScript
    } finally {
        Pop-Location
    }
    if (LastExitCode -ne 0) {
        Write-Host "Error: Failed to compile statusline binary." -ForegroundColor Red
        exit 1
    }
    
    $sourceUninstall = Join-Path $PSScriptRoot "uninstall.ps1"
    if (Test-Path $sourceUninstall) {
        Write-Host "Copying uninstall.ps1 to $targetUninstall..."
        Copy-Item -Path $sourceUninstall -Destination $targetUninstall -Force
    }
} else {
    Write-Host "Installing from remote repository..."
    $tempDir = Join-Path $env:TEMP ([Guid]::NewGuid().ToString())
    Write-Host "Cloning repository..."
    & git clone --depth 1 https://github.com/bradly0cjw/agycli-statusline.git $tempDir
    if (LastExitCode -ne 0) {
        Write-Host "Error: Failed to clone repository." -ForegroundColor Red
        exit 1
    }

    Write-Host "Compiling statusline.exe..."
    Push-Location $tempDir
    $commitHash = ""
    try {
        $commitHash = (git rev-parse --short HEAD 2>$null).Trim()
    } catch {}
    try {
        & go build -ldflags="-s -w -X main.commitHash=$commitHash" -o $targetScript
    } finally {
        Pop-Location
    }
    if (LastExitCode -ne 0) {
        Write-Host "Error: Failed to compile statusline binary." -ForegroundColor Red
        Remove-Item -Path $tempDir -Recurse -Force
        exit 1
    }
    
    Copy-Item -Path (Join-Path $tempDir "uninstall.ps1") -Destination $targetUninstall -Force
    Remove-Item -Path $tempDir -Recurse -Force
}

# Configuration file
$settingsFile = "$HOME\.gemini\antigravity-cli\settings.json"
$settingsDir = Split-Path $settingsFile

if (-not (Test-Path $settingsDir)) {
    New-Item -ItemType Directory -Path $settingsDir | Out-Null
}

# Format script path for settings.json compatibility (using forward slashes)
$escapedScriptPath = $targetScript.Replace('\', '/')
$commandString = $escapedScriptPath

Write-Host "Configuring statusline in settings.json..."
if (Test-Path $settingsFile) {
    # Backup existing settings
    Copy-Item -Path $settingsFile -Destination "${settingsFile}.bak" -Force
    Write-Host "Backup of settings.json saved to ${settingsFile}.bak"

    $json = Get-Content -Raw -Path $settingsFile | ConvertFrom-Json
    if ($null -eq $json) {
        $json = @{}
    }
    
    # Update statusLine
    $json.statusLine = @{
        type = ""
        command = $commandString
        enabled = $true
    }
    
    $json | ConvertTo-Json -Depth 100 | Out-File -FilePath $settingsFile -Encoding utf8
} else {
    $config = @{
        statusLine = @{
            type = ""
            command = $commandString
            enabled = $true
        }
    }
    $config | ConvertTo-Json -Depth 100 | Out-File -FilePath $settingsFile -Encoding utf8
}

Write-Host "====================================================" -ForegroundColor Blue
Write-Host "🎉 Installation completed successfully!" -ForegroundColor Green
Write-Host "Restart your Antigravity CLI session to see your new statusline."
Write-Host "Uninstaller copied to: $targetUninstall"
Write-Host "====================================================" -ForegroundColor Blue
