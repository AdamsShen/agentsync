// Package daemon 守护进程核心循环：实现 watch.Handler，串起 收敛→询问→分发。
package daemon

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/xmly/agentsync/internal/adapter"
	"github.com/xmly/agentsync/internal/ask"
	"github.com/xmly/agentsync/internal/registry"
	"github.com/xmly/agentsync/internal/sync"
	"github.com/xmly/agentsync/internal/watch"
)

// debounce 事件合并窗口
const debounce = 2 * time.Second

// Handler 实现 watch.Handler
type Handler struct {
	Reg      *registry.Registry
	Adapters []adapter.Adapter // 已检测适配器（动态更新）
	W        *watch.Watcher    // 监听器（动态接入新 agent 用）
}

// SetWatcher 注入监听器（供动态接入）
func (h *Handler) SetWatcher(w *watch.Watcher) { h.W = w }

// OnNewSkill 工具目录出现新 skill：收敛 → 原副本换软链 → 询问 → 分发
func (h *Handler) OnNewSkill(ctx context.Context, a adapter.Adapter, dir string) error {
	log.Printf("[daemon] 检测到新 skill: %s (来源: %s)", dir, a.Name())

	// 1. 收敛到 canonical
	it, err := sync.IngestSkill(ctx, h.Reg, a, dir)
	if err != nil {
		return err
	}
	log.Printf("[daemon] 已收敛到 %s", it.Canonical)

	// 2. 原工具副本替换为软链（决策 9）
	if err := sync.ReplaceWithSymlink(ctx, a, dir, it); err != nil {
		log.Printf("[daemon] 替换源副本为软链失败: %v", err)
	}

	// 3. 询问分发目标（只列已检测且支持 skill 的工具；源工具已在步骤2处理）
	candidates := []string{}
	defaults := []string{}
	for _, ad := range h.Adapters {
		if ad.Name() == a.Name() {
			continue
		}
		if ad.KindSupported(registry.KindSkill) {
			candidates = append(candidates, ad.Name())
			defaults = append(defaults, ad.Name())
		}
	}
	targets, err := ask.MultiSelect(
		"检测到以下工具支持 skill，同步到哪些？（空回车=全部）",
		candidates, defaults)
	if err != nil {
		return err
	}

	// 4. 分发软链到选中的工具
	for _, ad := range h.Adapters {
		for _, t := range targets {
			if ad.Name() == t {
				if err := sync.ProjectSkill(ctx, ad, it); err != nil {
					log.Printf("[daemon] 分发到 %s 失败: %v", ad.Name(), err)
				} else {
					log.Printf("[daemon] 已分发到 %s", ad.Name())
				}
			}
		}
	}
	it.ProjectedTo = targets
	h.Reg.UpsertItem(it)
	_ = h.Reg.Save()
	return nil
}

// OnRescan 周期重扫：检测新 agent 并加入监听
func (h *Handler) OnRescan(ctx context.Context) error {
	adReg := adapter.NewRegistry()
	detected := adReg.DetectAll(ctx)
	// 找出新出现的工具
	have := map[string]bool{}
	for _, a := range h.Adapters {
		have[a.Name()] = true
	}
	for _, a := range detected {
		if !have[a.Name()] {
			log.Printf("[daemon] 检测到新 agent: %s，加入监听", a.Name())
			h.Adapters = append(h.Adapters, a)
			if h.W != nil {
				h.W.AddAdapter(a)
			}
		}
	}
	// 记录检测状态进 registry
	for _, a := range detected {
		h.Reg.Tools[a.Name()] = registry.ToolState{Detected: true, Enabled: true}
	}
	_ = h.Reg.Save()
	return nil
}

// EnsureCanonical 确保 canonical 目录存在
func EnsureCanonical() error {
	for _, d := range []string{sync.SkillsRoot(), sync.RulesRoot()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// Run 启动守护进程（阻塞）
func Run(ctx context.Context, reg *registry.Registry) error {
	if err := EnsureCanonical(); err != nil {
		return err
	}
	// 检测本机已装 agent
	adReg := adapter.NewRegistry()
	detected := adReg.DetectAll(ctx)
	log.Printf("[daemon] 检测到 %d 个 agent: %v", len(detected), adapterNames(detected))

	h := &Handler{Reg: reg, Adapters: detected}
	w := watch.New(reg, detected, h, debounce)
	h.SetWatcher(w)
	return w.Run(ctx)
}

// adapterNames 返回适配器名列表
func adapterNames(as []adapter.Adapter) []string {
	names := make([]string, 0, len(as))
	for _, a := range as {
		names = append(names, a.Name())
	}
	return names
}
