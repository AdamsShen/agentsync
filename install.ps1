# agentsync 一键安装脚本（Windows PowerShell）
# 用法：irm https://raw.githubusercontent.com/AdamsShen/agentsync/master/install.ps1 | iex
$ErrorActionPreference = "Stop"

$Repo       = "AdamsShen/agentsync"
$Bin        = "agentsync"
$Version    = if ($env:AGENTSYNC_VERSION) { $env:AGENTSYNC_VERSION } else { "latest" }
$InstallDir = if ($env:AGENTSYNC_INSTALL_DIR) { $env:AGENTSYNC_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "agentsync\bin" }

# 1. 检测架构（amd64 / arm64）
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }

# 2. 下载
$url = "https://github.com/$Repo/releases/$Version/download/${Bin}_windows_${arch}.exe"
Write-Host "下载 $url ..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$exe = Join-Path $InstallDir "$Bin.exe"
try {
    Invoke-WebRequest -Uri $url -OutFile $exe -UseBasicParsing
} catch {
    Write-Error "下载失败：可能尚未发布 ${Version} 的 windows/${arch} 二进制。"
    Write-Host "可先用 go install 安装：go install github.com/$Repo@latest"
    exit 1
}
Write-Host "✓ 已安装到 $exe"

# 3. 加入用户 PATH
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
    Write-Host "✓ 已将 $InstallDir 加入用户 PATH（新开终端生效）"
} else {
    Write-Host "✓ $InstallDir 已在 PATH 中"
}

# 4. 注册开机自启 + 启动（sc create 需管理员权限）
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if ($isAdmin) {
    & $exe install
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ 已注册开机自启服务（daemon 已启动）"
    } else {
        Write-Host "⚠ 注册开机自启失败，可稍后手动：$Bin install"
    }
} else {
    Write-Host "⚠ 当前非管理员，跳过开机自启注册（sc create 系统服务需管理员）。"
    Write-Host "  请以管理员身份运行本脚本，或稍后手动：$Bin install"
}

# 5. 使用提示
Write-Host ""
Write-Host "使用方式："
Write-Host "  agentsync status     # 查看检测到的 agent"
Write-Host "  agentsync list       # 查看已收敛的配置"
Write-Host "  agentsync tui        # 交互式状态面板"
Write-Host "  agentsync uninstall  # 移除开机自启服务"
