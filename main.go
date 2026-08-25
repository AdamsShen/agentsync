// Command agentsync —— 跨 Agent 工具配置自动同步工具。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/xmly/agentsync/internal/adapter"
	"github.com/xmly/agentsync/internal/daemon"
	"github.com/xmly/agentsync/internal/registry"
	"github.com/xmly/agentsync/internal/svc"
	"github.com/xmly/agentsync/internal/tui"
)

const usage = `agentsync —— 跨 Agent 工具配置自动同步工具

用法：
  agentsync daemon            启动守护进程（监听工具目录 → 收敛 → 分发）
  agentsync install           注册开机自启服务（launchd/systemd/Windows Service）
  agentsync uninstall         移除开机自启服务
  agentsync status            检测本机 agent + 显示 registry 状态
  agentsync list              列出已收敛的配置
  agentsync tui               交互式状态面板
  agentsync help              显示帮助
`

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) < 2 {
		fmt.Print(usage)
		return
	}

	dir := configDir()
	reg, err := registry.Load(dir)
	if err != nil {
		log.Fatalf("加载 registry 失败: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "daemon":
		if err := daemon.Run(ctx, reg); err != nil {
			log.Fatalf("守护进程退出: %v", err)
		}
	case "install":
		if err := svc.Install(); err != nil {
			log.Fatalf("注册服务失败: %v", err)
		}
		fmt.Println("已注册开机自启服务。")
		_ = svc.Start()
	case "uninstall":
		if err := svc.Uninstall(); err != nil {
			log.Fatalf("移除服务失败: %v", err)
		}
		fmt.Println("已移除开机自启服务。")
	case "status":
		// 主动检测本机 agent（而非只读 registry 缓存的检测状态），再展示
		for _, a := range adapter.NewRegistry().DetectAll(ctx) {
			reg.Tools[a.Name()] = registry.ToolState{Detected: true, Enabled: true}
		}
		printStatus(reg)
	case "list":
		printList(reg)
	case "tui":
		if err := tui.Run(dir, reg); err != nil {
			log.Fatalf("TUI 退出异常: %v", err)
		}
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

// configDir 返回 registry 存放目录（~/.agentsync）
func configDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(h, ".agentsync")
}

func printStatus(reg *registry.Registry) {
	fmt.Println("=== agentsync 状态 ===")
	fmt.Printf("registry: %s\n", reg.Path())
	fmt.Printf("工具检测: %d\n", len(reg.Tools))
	for name, st := range reg.Tools {
		mark := "未检测"
		if st.Detected {
			mark = "已检测"
		}
		if st.Enabled {
			mark += " ✓启用"
		}
		fmt.Printf("  %-12s %s\n", name, mark)
	}
}

func printList(reg *registry.Registry) {
	fmt.Println("=== 已收敛配置 ===")
	if len(reg.Items) == 0 {
		fmt.Println("  （空）")
		return
	}
	for _, it := range reg.Items {
		fmt.Printf("  %-20s 来源=%-10s 分发到=%v\n", it.ID, it.Origin, it.ProjectedTo)
	}
}
