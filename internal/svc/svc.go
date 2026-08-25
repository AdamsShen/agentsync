// Package svc 守护进程服务化：注册系统服务（launchd / systemd / Windows 服务）。
// macOS 用 launchd LaunchAgent（用户级），Linux 用 systemd 用户级 unit，Windows 用系统服务（sc）。
package svc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// RunDaemon 以守护模式运行（由系统服务拉起时入口调用）
func RunDaemon() error {
	// 复用 daemon.Run；此处通过环境变量标记，main 据此进入守护逻辑
	return nil
}

// ExePath 当前可执行文件路径
func ExePath() string {
	p, err := os.Executable()
	if err != nil {
		return "agentsync"
	}
	return p
}

// IsInstalled 判断服务是否已注册
func IsInstalled() bool {
	switch runtime.GOOS {
	case "darwin":
		return fileExists(launchdPlist())
	case "linux":
		return fileExists(systemdUnitPath())
	case "windows":
		out, _ := exec.Command("sc", "query", "agentsync").CombinedOutput()
		return strings.Contains(string(out), "SERVICE_NAME")
	}
	return false
}

// Install 注册开机自启服务（macOS: launchd；Linux: systemd 用户级；Windows: 系统服务）
func Install() error {
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd()
	case "linux":
		return installSystemd()
	case "windows":
		return installWindows()
	default:
		return fmt.Errorf("不支持的系统: %s", runtime.GOOS)
	}
}

// Uninstall 移除开机自启服务
func Uninstall() error {
	switch runtime.GOOS {
	case "darwin":
		return uninstallLaunchd()
	case "linux":
		return uninstallSystemd()
	case "windows":
		return uninstallWindows()
	default:
		return nil
	}
}

// Start 立即启动守护进程（若未运行）
func Start() error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("launchctl", "kickstart", "-k", "gui/"+uid()+"/com.agentsync").Run()
	case "linux":
		return exec.Command("systemctl", "--user", "start", "agentsync").Run()
	case "windows":
		return exec.Command("sc", "start", "agentsync").Run()
	default:
		return nil
	}
}

// --- macOS launchd ---

func installLaunchd() error {
	plist := launchdPlist()
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plist, []byte(launchdPlistContent()), 0o644); err != nil {
		return err
	}
	// 加载到 launchd（RunAtLoad 立即启动；失败不致命，重启后自动加载）
	_ = exec.Command("launchctl", "load", plist).Run()
	return nil
}

func uninstallLaunchd() error {
	plist := launchdPlist()
	_ = exec.Command("launchctl", "unload", plist).Run()
	_ = os.Remove(plist)
	return nil
}

func launchdPlist() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library", "LaunchAgents", "com.agentsync.plist")
}

func uid() string {
	return os.Getenv("USER")
}

func launchdPlistContent() string {
	exe := ExePath()
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.agentsync</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + exe + `</string>
    <string>daemon</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/tmp/agentsync.log</string>
  <key>StandardErrorPath</key><string>/tmp/agentsync.log</string>
</dict>
</plist>
`
}

// --- Linux systemd（用户级，无需 sudo）---

func installSystemd() error {
	unit := systemdUnitPath()
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unit, []byte(systemdUnitContent()), 0o644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	if out, err := exec.Command("systemctl", "--user", "enable", "--now", "agentsync").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable 失败: %w: %s", err, out)
	}
	return nil
}

func uninstallSystemd() error {
	_ = exec.Command("systemctl", "--user", "disable", "--now", "agentsync").Run()
	_ = os.Remove(systemdUnitPath())
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

func systemdUnitPath() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "systemd", "user", "agentsync.service")
}

func systemdUnitContent() string {
	exe := strconv.Quote(ExePath()) // 路径含空格时加引号
	return fmt.Sprintf(`[Unit]
Description=agentsync daemon（跨 Agent 工具配置自动同步）
After=network.target

[Service]
ExecStart=%s daemon
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, exe)
}

// --- Windows 系统服务（sc，需管理员权限）---

func installWindows() error {
	exe := ExePath()
	binPath := fmt.Sprintf(`"%s" daemon`, exe)
	if out, err := exec.Command("sc", "create", "agentsync", "binPath=", binPath, "start=", "auto").CombinedOutput(); err != nil {
		return fmt.Errorf("sc create 失败: %w: %s", err, out)
	}
	if out, err := exec.Command("sc", "start", "agentsync").CombinedOutput(); err != nil {
		return fmt.Errorf("sc start 失败: %w: %s", err, out)
	}
	return nil
}

func uninstallWindows() error {
	_ = exec.Command("sc", "stop", "agentsync").Run()
	if out, err := exec.Command("sc", "delete", "agentsync").CombinedOutput(); err != nil {
		return fmt.Errorf("sc delete 失败: %w: %s", err, out)
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
