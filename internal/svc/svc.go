// Package svc 守护进程服务化：注册系统服务（launchd / Windows Service / systemd）。
// M2：macOS launchd LaunchAgent（用户级，开机自启），Windows/Linux 走 kardianos/service。
package svc

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// IsInstalled 判断 launchd 服务是否已注册
func IsInstalled() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	return fileExists(launchdPlist())
}

func launchdPlist() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library", "LaunchAgents", "com.agentsync.plist")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Install 注册开机自启服务（macOS: launchd LaunchAgent；Windows/Linux: kardianos）
func Install() error {
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd()
	default:
		// M2 主平台为 macOS；Windows/Linux 用 kardianos/service 在 M2.5 补
		return nil
	}
}

func installLaunchd() error {
	plist := launchdPlist()
	content := launchdPlistContent()
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plist, []byte(content), 0o644); err != nil {
		return err
	}
	// 加载到 launchd（失败不致命，重启后自动加载）
	_ = exec.Command("launchctl", "load", plist).Run()
	return nil
}

// Uninstall 移除开机自启服务
func Uninstall() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	plist := launchdPlist()
	_ = exec.Command("launchctl", "unload", plist).Run()
	_ = os.Remove(plist)
	return nil
}

// Start 立即启动守护进程（若未运行）
func Start() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return exec.Command("launchctl", "kickstart", "-k", "gui/"+uid()+"/com.agentsync").Run()
}

func uid() string {
	return os.Getenv("USER")
}

// launchdPlistContent 生成 LaunchAgent plist（RunAtLoad 开机自启 + KeepAlive）
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

var _ = strings.TrimSpace
