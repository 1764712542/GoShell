# GoShell Windows 构建脚本
# 需要安装 mingw-w64: scoop install mingw 或 choco install mingw

$ErrorActionPreference = "Stop"
$APP_NAME = "goshell"
$MAIN = "./cmd/meatshell"

Write-Host "Building ${APP_NAME} for Windows (amd64)..."

# 设置环境变量
$env:CGO_ENABLED = "1"
$env:GOOS = "windows"
$env:GOARCH = "amd64"

# 构建
go build -o "${APP_NAME}.exe" $MAIN

if ($LASTEXITCODE -eq 0) {
    Write-Host "Build successful: ${APP_NAME}.exe" -ForegroundColor Green

    # 创建 zip 包
    if (Test-Path "lang") {
        Compress-Archive -Path "${APP_NAME}.exe", "lang", "assets" -DestinationPath "${APP_NAME}-windows-x86_64.zip" -Force
        Write-Host "Created ${APP_NAME}-windows-x86_64.zip" -ForegroundColor Green
    }
} else {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}
