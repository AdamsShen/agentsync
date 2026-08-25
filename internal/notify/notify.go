// Package notify 跨平台桌面通知：同步完成后告知用户。
// macOS 用 osascript，Linux 用 notify-send，Windows 用 PowerShell Toast。
package notify

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
)

// Send 发送桌面通知。失败返回错误（调用方决定是否忽略，通知不应阻塞主流程）。
func Send(title, msg string) error {
	switch runtime.GOOS {
	case "darwin":
		return sendDarwin(title, msg)
	case "linux":
		return sendLinux(title, msg)
	case "windows":
		return sendWindows(title, msg)
	default:
		return fmt.Errorf("不支持的系统: %s", runtime.GOOS)
	}
}

// sendDarwin macOS：osascript display notification。
func sendDarwin(title, msg string) error {
	return exec.Command("osascript", "-e", darwinScript(title, msg)).Run()
}

// darwinScript 生成 osascript 通知脚本（strconv.Quote 转义，防内容含引号导致注入/语法错）。
func darwinScript(title, msg string) string {
	return fmt.Sprintf("display notification %s with title %s",
		strconv.Quote(msg), strconv.Quote(title))
}

// sendLinux Linux：notify-send（依赖 libnotify，主流桌面环境自带）。
func sendLinux(title, msg string) error {
	return exec.Command("notify-send", title, msg).Run()
}

// sendWindows Windows：PowerShell 调 WinRT Toast（Win8+）。
func sendWindows(title, msg string) error {
	ps := fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$t = New-Object Windows.Data.Xml.Dom.XmlDocument
$t.LoadXml('<toast><visual><binding template="ToastText02"><text id="1">%s</text><text id="2">%s</text></binding></visual></toast>')
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('agentsync').Show([Windows.UI.Notifications.ToastNotification]::new($t))`,
		title, msg)
	return exec.Command("powershell", "-NoProfile", "-Command", ps).Run()
}
