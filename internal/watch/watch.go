// Package watch 守护进程的文件监听：多目录/多文件监听 + debounce + 按 kind 分发。
package watch

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/AdamsShen/agentsync/internal/adapter"
	"github.com/AdamsShen/agentsync/internal/registry"
)

// Handler 处理一个收敛/分发循环
type Handler interface {
	// OnSkill 工具 skills 目录出现新 skill 目录（未收敛）
	OnSkill(ctx context.Context, a adapter.Adapter, dir string) error
	// OnRules 工具 rules 目录出现新 rule 文件（目录式，未收敛）
	OnRules(ctx context.Context, a adapter.Adapter, file string) error
	// OnRuleFile 工具的单文件规则（AGENTS.md/SOUL.md 等）出现或变化（未收敛）
	OnRuleFile(ctx context.Context, a adapter.Adapter, file string) error
	// OnMcpChange 工具的 MCP 配置文件发生变化
	OnMcpChange(ctx context.Context, a adapter.Adapter) error
	// OnRescan 周期性重扫检测新 agent
	OnRescan(ctx context.Context) error
}

type entry struct {
	a    adapter.Adapter
	spec adapter.WatchSpec
}

// Watcher 监听器：持有已监听路径 → 归属映射
type Watcher struct {
	reg      *registry.Registry
	adapters []adapter.Adapter // 已检测适配器
	handler  Handler
	debounce time.Duration
	rescan   time.Duration
	fw       *fsnotify.Watcher
	entries  map[string]entry // spec.Path → 归属
	baseline map[string]bool  // 「无 adopt」基线：启动时既有的配置 ID（skill:/rules: 前缀）
}

// New 创建监听器。rescan 间隔取自 registry（缺省 5 分钟）。
func New(reg *registry.Registry, adapters []adapter.Adapter, h Handler, debounce time.Duration) *Watcher {
	rescan := time.Duration(reg.Defaults.RescanIntervalSec) * time.Second
	if rescan <= 0 {
		rescan = 5 * time.Minute
	}
	return &Watcher{
		reg:      reg,
		adapters: adapters,
		handler:  h,
		debounce: debounce,
		rescan:   rescan,
		entries:  map[string]entry{},
	}
}

// Run 启动监听，阻塞直到 ctx 取消
func (w *Watcher) Run(ctx context.Context) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fw.Close()
	w.fw = fw

	// 注册所有已检测工具的监听（skills/rules 目录 + MCP 文件）
	for _, a := range w.adapters {
		for _, spec := range a.WatchSpecs() {
			if err := w.addSpec(fw, a, spec); err != nil {
				log.Printf("[watch] 监听 %s 失败: %v", spec.Path, err)
			}
		}
	}

	// 建立「无 adopt」基线：记录启动时已存在的实体配置，之后只收敛基线之外的新增。
	w.buildBaseline()

	log.Printf("[watch] 已注册 %d 个监听，基线 %d 项，进入事件循环", len(w.entries), len(w.baseline))

	rescan := time.NewTicker(w.rescan)
	defer rescan.Stop()

	timer := time.NewTimer(w.debounce)
	timer.Stop()
	pending := map[string]bool{}

	flush := func() {
		for root := range pending {
			delete(pending, root)
			w.dispatch(ctx, fw, root)
		}
	}

	for {
		select {
		case ev, ok := <-fw.Events:
			if !ok {
				return nil
			}
			root, ok := w.route(ev.Name)
			if !ok {
				continue
			}
			pending[root] = true
			// 排空旧定时，避免残留触发导致过早 flush
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.debounce)

		case err, ok := <-fw.Errors:
			if !ok {
				return nil
			}
			log.Printf("[watch] 监听错误: %v", err)

		case <-timer.C:
			flush()

		case <-rescan.C:
			if w.handler != nil {
				_ = w.handler.OnRescan(ctx)
			}

		case <-ctx.Done():
			return nil
		}
	}
}

// addSpec 注册一个监听。以 Recurse 区分目录/文件：Recurse=false 为单文件（MCP 文件、单文件规则）。
func (w *Watcher) addSpec(fw *fsnotify.Watcher, a adapter.Adapter, spec adapter.WatchSpec) error {
	if !spec.Recurse {
		// 文件类监听
		switch spec.Kind {
		case registry.KindMCP:
			// MCP 文件不存在则写空 {}，便于监听
			if err := ensureMCPFile(spec.Path); err != nil {
				return err
			}
		case registry.KindRules:
			// 单文件规则是用户手写活文件：存在才监听，不存在不自动创建
			if _, err := os.Stat(spec.Path); os.IsNotExist(err) {
				return nil
			}
		}
		if err := fw.Add(spec.Path); err != nil {
			return err
		}
		w.entries[spec.Path] = entry{a, spec}
		return nil
	}
	// 目录类监听：MkdirAll + Add
	if err := os.MkdirAll(spec.Path, 0o755); err != nil {
		return err
	}
	if err := fw.Add(spec.Path); err != nil {
		return err
	}
	w.entries[spec.Path] = entry{a, spec}
	return nil
}

// buildBaseline 建立「无 adopt」基线：记录启动时已存在的实体 skill/rules，
// 之后 scanSkillDir/scanRulesDir/scanRuleFile 跳过基线，只收敛启动后新增的。
func (w *Watcher) buildBaseline() {
	w.baseline = map[string]bool{}
	ctx := context.Background()
	for _, a := range w.adapters {
		for _, spec := range a.WatchSpecs() {
			switch spec.Kind {
			case registry.KindSkill:
				entries, err := os.ReadDir(spec.Path)
				if err != nil {
					continue
				}
				for _, e := range entries {
					if !e.IsDir() {
						continue
					}
					p := filepath.Join(spec.Path, e.Name())
					if owned, _ := a.IsOwnedProjection(ctx, p); owned {
						continue // 我方软链（此前已收敛），非既有实体
					}
					if !a.HasSKILL(p) {
						continue
					}
					w.baseline["skill:"+e.Name()] = true
				}
			case registry.KindRules:
				if spec.Recurse {
					entries, err := os.ReadDir(spec.Path)
					if err != nil {
						continue
					}
					for _, e := range entries {
						if !e.IsDir() && isRuleFile(e.Name()) {
							w.baseline["rules:"+e.Name()] = true
						}
					}
				} else {
					// 单文件规则：名字 = tool 前缀 + basename（与 scanRuleFile 一致）
					if _, err := os.Stat(spec.Path); err == nil {
						w.baseline["rules:"+a.Name()+"-"+filepath.Base(spec.Path)] = true
					}
				}
			}
		}
	}
}

// route 由事件路径反查归属的 spec.Path（文件类精确匹配，目录类前缀匹配）
func (w *Watcher) route(p string) (string, bool) {
	if _, ok := w.entries[p]; ok {
		return p, true
	}
	sep := string(filepath.Separator)
	for root, e := range w.entries {
		if !e.spec.Recurse {
			continue // 文件类 spec（MCP/单文件规则）只精确匹配
		}
		if p == root || strings.HasPrefix(p, root+sep) {
			return root, true
		}
	}
	return "", false
}

// dispatch 按 kind 分发到 handler
func (w *Watcher) dispatch(ctx context.Context, fw *fsnotify.Watcher, root string) {
	e, ok := w.entries[root]
	if !ok {
		return
	}
	switch e.spec.Kind {
	case registry.KindSkill:
		w.scanSkillDir(ctx, fw, e.a, root)
	case registry.KindRules:
		if e.spec.Recurse {
			w.scanRulesDir(ctx, fw, e.a, root)
		} else {
			w.scanRuleFile(ctx, e.a, root)
		}
	case registry.KindMCP:
		if w.handler != nil {
			_ = w.handler.OnMcpChange(ctx, e.a)
		}
	}
}

// scanSkillDir 扫描 skills 目录，发现未收敛的新 skill
func (w *Watcher) scanSkillDir(ctx context.Context, fw *fsnotify.Watcher, a adapter.Adapter, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			_ = ensureAndWatch(fw, dir) // 目录被删，重建监听
		}
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if w.reg.GetItem("skill:"+e.Name()) != nil {
			continue // 已收敛
		}
		if w.baseline["skill:"+e.Name()] {
			continue // 启动前既有配置（无 adopt）
		}
		if owned, _ := a.IsOwnedProjection(ctx, p); owned {
			continue // 我方投影软链
		}
		if !a.HasSKILL(p) {
			continue
		}
		if w.handler != nil {
			if err := w.handler.OnSkill(ctx, a, p); err != nil {
				log.Printf("[watch] 收敛 skill %s 失败: %v", p, err)
			}
		}
	}
}

// scanRulesDir 扫描 rules 目录，发现未收敛的新 rule 文件
func (w *Watcher) scanRulesDir(ctx context.Context, fw *fsnotify.Watcher, a adapter.Adapter, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			_ = ensureAndWatch(fw, dir)
		}
		return
	}
	for _, e := range entries {
		if e.IsDir() || !isRuleFile(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if w.reg.GetItem("rules:"+e.Name()) != nil {
			continue
		}
		if w.baseline["rules:"+e.Name()] {
			continue // 启动前既有配置（无 adopt）
		}
		if owned, _ := a.IsOwnedProjection(ctx, p); owned {
			continue
		}
		if w.handler != nil {
			if err := w.handler.OnRules(ctx, a, p); err != nil {
				log.Printf("[watch] 收敛 rule %s 失败: %v", p, err)
			}
		}
	}
}

// scanRuleFile 处理单文件规则（AGENTS.md/SOUL.md 等）：存在且未收敛则回调。
// 命名规则：canonical 名 = tool 前缀 + "-" + basename（与 sync.IngestRuleFile 保持一致）。
func (w *Watcher) scanRuleFile(ctx context.Context, a adapter.Adapter, file string) {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return // 文件被删，跳过（不自动重建）
	}
	name := a.Name() + "-" + filepath.Base(file)
	if w.reg.GetItem("rules:"+name) != nil {
		return // 已收敛
	}
	if w.baseline["rules:"+name] {
		return // 启动前既有配置（无 adopt）
	}
	// 单文件规则源是用户手写活文件，不会被软链，无需 IsOwnedProjection 判断
	if w.handler != nil {
		if err := w.handler.OnRuleFile(ctx, a, file); err != nil {
			log.Printf("[watch] 收敛单文件 rule %s 失败: %v", file, err)
		}
	}
}

// isRuleFile 判断文件名是否为 rule 文件（.md / .mdc）
func isRuleFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".md" || ext == ".mdc"
}

// ensureMCPFile 确保 MCP 文件存在（不存在则写空 {}，便于监听）
func ensureMCPFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte("{}\n"), 0o600)
	}
	return nil
}

// ensureAndWatch 目录不存在则创建，然后加入监听
func ensureAndWatch(fw *fsnotify.Watcher, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return fw.Add(dir)
}

// AddAdapter 动态加入一个已检测适配器的监听（新 agent 接入）
func (w *Watcher) AddAdapter(a adapter.Adapter) {
	for _, e := range w.adapters {
		if e.Name() == a.Name() {
			return
		}
	}
	w.adapters = append(w.adapters, a)
	for _, spec := range a.WatchSpecs() {
		if err := w.addSpec(w.fw, a, spec); err != nil {
			log.Printf("[watch] 新监听 %s 失败: %v", spec.Path, err)
		} else {
			log.Printf("[watch] 新监听: %s", spec.Path)
		}
	}
}
