# Aether Installer for Windows PowerShell
$ErrorActionPreference = "Stop"

Write-Host "=====================================================" -ForegroundColor Blue -BackgroundColor Black
Write-Host "            🌀 Aether Installer Utility 🌀           " -ForegroundColor Blue -BackgroundColor Black
Write-Host "=====================================================" -ForegroundColor Blue -BackgroundColor Black

# 1. Check/Build binary
$binaryName = "aether.exe"
if (-not (Test-Path $binaryName)) {
    Write-Host "ℹ️  'aether.exe' binary not found. Compiling with Go..." -ForegroundColor Cyan
    if (Get-Command "go" -ErrorAction SilentlyContinue) {
        & go build -o aether.exe
        Write-Host "✅ Compilation successful!" -ForegroundColor Green
    } else {
        Write-Host "❌ 'go' compiler not found. Please compile the 'aether.exe' binary first." -ForegroundColor Red
        Exit
    }
}

# 2. Setup folders
$appDataDir = "$env:APPDATA\aether"
$binDir = "$appDataDir\bin"

Write-Host "📁 Creating installation folders in AppData Roaming..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path $binDir | Out-Null

Write-Host "🚀 Installing 'aether' executable..." -ForegroundColor Cyan
Copy-Item -Path $binaryName -Destination "$binDir\aether.exe" -Force
Write-Host "✅ Executable copied to $binDir\aether.exe" -ForegroundColor Green

# 3. Setup configuration
if (Test-Path "config.json") {
    Write-Host "⚙️  Copying configuration file..." -ForegroundColor Cyan
    if (Test-Path "$appDataDir\config.json") {
        Write-Host "⚠️  Configuration already exists. Backing up..." -ForegroundColor Yellow
        Copy-Item -Path "$appDataDir\config.json" -Destination "$appDataDir\config.json.bak" -Force
    }
    Copy-Item -Path "config.json" -Destination "$appDataDir\config.json" -Force
    Write-Host "✅ Config file copied successfully!" -ForegroundColor Green
} else {
    Write-Host "⚙️  Creating default config.json..." -ForegroundColor Cyan
    $defaultConfig = @{
        "surreal_url" = "ws://localhost:8000"
        "surreal_user" = "root"
        "surreal_pass" = "root"
        "surreal_ns" = "sts"
        "surreal_db" = "test"
        "embedding_provider" = "gemini"
        "embedding_model" = "gemini-embedding-2"
        "embedding_dim" = 768
    } | ConvertTo-Json -Depth 5
    Set-Content -Path "$appDataDir\config.json" -Value $defaultConfig
    Write-Host "✅ Default config file created at $appDataDir\config.json" -ForegroundColor Green
}

# 4. Update Windows User PATH Env Variable
Write-Host "⚙️  Checking PATH environment variable..." -ForegroundColor Cyan
$userPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
if ($userPath -split ";" -notcontains $binDir) {
    Write-Host "⚙️  Adding $binDir to User PATH..." -ForegroundColor Cyan
    $newUserPath = $userPath + ";" + $binDir
    # Remove any duplicate semicolons
    $newUserPath = $newUserPath -replace ";+", ";"
    [Environment]::SetEnvironmentVariable("Path", $newUserPath, [EnvironmentVariableTarget]::User)
    
    # Update current session PATH as well
    $env:Path += ";" + $binDir
    Write-Host "✅ Added successfully! PATH updated." -ForegroundColor Green
} else {
    Write-Host "✨ PATH is already configured." -ForegroundColor Green
}

Write-Host "=====================================================" -ForegroundColor Blue
Write-Host "🎉 Aether has been successfully installed!" -ForegroundColor Green
Write-Host "=====================================================" -ForegroundColor Blue
Write-Host "Binary Path:      $binDir\aether.exe" -ForegroundColor White
Write-Host "Config Path:      $appDataDir\config.json" -ForegroundColor White
Write-Host "=====================================================" -ForegroundColor Blue
Write-Host "💡 Note: You may need to restart your terminal/powershell window for PATH changes to take effect." -ForegroundColor Yellow
Write-Host "=====================================================" -ForegroundColor Blue
