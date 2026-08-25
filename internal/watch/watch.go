// Package watch 守护进程的文件监听：多目录监听 + debounce + 收敛触发。
package watch

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/xmly/agentsync/internal/adapter"
	"github.com/xmly/agentsync/internal/registry"
	"github.com/xmly/agentsync/internal/sync"
)

// Handler 处理一个收敛/分发循环
type Handler interface {
	// OnNewSkill 工具目录出现新 skill（未收敛）
	OnNewSkill(ctx context.Context, a adapter.Adapter, dir string) error
	// OnRescan 周期性重扫检测新 agent
	OnRescan(ctx context.Context) error
}

// Watcher 多目录监听器
type Watcher struct {
	reg     *registry.Registry
	adapters []adapter.Adapter // 已检测适配器
	handler Handler
	debounce time.Duration
}

// New 创建监听器
func New(reg *registry.Registry, adapters []adapter.Adapter, h Handler, debounce time.Duration) *Watcher {
	return &Watcher{reg: reg, adapters: adapters, handler: h, debounce: debounce}
}

// Run 启动监听，阻塞直到 ctx 取消
func (w *Watcher) Run(ctx context.Context) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fw.Close()

	// 注册所有已检测工具的 skills 目录
	for _, a := range w.adapters {
		for _, spec := range a.WatchSpecs() {
			if spec.Kind != registry.KindSkill {
				continue // M0 只监听 skill
			}
			if err := ensureAndWatch(fw, spec.Path); err != nil {
				log.Printf("[watch] 监听 %s 失败: %v", spec.Path, err)
			}
		}
	}

	// 周期重扫检测新 agent（间隔默认 5 分钟，可由调用方覆盖）
	rescan := time.NewTicker(5 * time.Minute)
	defer rescan.Stop()

	// debounce 队列：目录路径 -> 最后事件时间
	var pending = map[string]time.Time{}
	var pendingMu = make(chan struct{}, 1)

	flush := func() {
		for path := range pending {
			delete(pending, path)
			w.handleSkillDir(ctx, fw, path)
		}
	}

	timer := time.NewTimer(w.debounce)
	timer.Stop()

	for {
		select {
		case ev, ok := <-fw.Events:
			if !ok {
				return nil
			}
			// 只关心目录事件（Create/Write/Rename/Remove），且指向 skill 目录下
			if !isSkillChild(ev.Name) {
				continue
			}
			dir := filepath.Dir(ev.Name)
			pendingMu <- struct{}{}
			pending[dir] = time.Now()
			<-pendingMu
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

// handleSkillDir 扫描目录，发现未收敛的新 skill
func (w *Watcher) handleSkillDir(ctx context.Context, fw *fsnotify.Watcher, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// 目录被删，重建监听（如工具重装）
			_ = ensureAndWatch(fw, dir)
		}
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		a := w.adapterForDir(dir)
		if a == nil {
			continue
		}
		// 已收敛且 owned → 跳过
		if w.reg.GetItem("skill:"+e.Name()) != nil {
			continue
		}
		// 是软链且指向 canonical → 我方投影，跳过
		if owned, _ := a.IsOwnedProjection(ctx, p); owned {
			continue
		}
		// 有 SKILL.md 才是 skill
		if !a.HasSKILL(p) {
			continue
		}
		// 新 skill → 交给 handler 收敛
		if w.handler != nil {
			if err := w.handler.OnNewSkill(ctx, a, p); err != nil {
				log.Printf("[watch] 收敛 %s 失败: %v", p, err)
			}
		}
	}
}

// adapterForDir 由目录路径反查所属适配器
func (w *Watcher) adapterForDir(dir string) adapter.Adapter {
	for _, a := range w.adapters {
		for _, spec := range a.WatchSpecs() {
			if spec.Path == dir {
				return a
			}
		}
	}
	return nil
}

func isSkillChild(p string) bool {
	// 路径含 <某工具>/skills/ 即视为 skill 子路径
	return strings.Contains(p, string(filepath.Separator)+"skills"+string(filepath.Separator)) ||
		filepath.Base(filepath.Dir(p)) == "skills"
}

// ensureAndWatch 目录不存在则创建，然后加入监听
func ensureAndWatch(fw *fsnotify.Watcher, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return fw.Add(dir)
}

// 占位引用 sync 包，避免未使用（Handler 实际实现方会用到）
var _ = sync.CanonicalRoot
