// Package sync 同步引擎：收敛(ingest) + 分发(project) + 冲突/询问回调。
package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/xmly/agentsync/internal/adapter"
	"github.com/xmly/agentsync/internal/registry"
)

// CanonicalRoot 返回 canonical 根目录（绝对路径）
func CanonicalRoot() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".agents")
}

// SkillsRoot 统一 skill 副本目录
func SkillsRoot() string { return filepath.Join(CanonicalRoot(), "skills") }

// RulesRoot 统一 rules 副本目录
func RulesRoot() string { return filepath.Join(CanonicalRoot(), "rules") }

// Ask 询问回调：分发前让调用方决定目标工具集合
type Ask func(ctx context.Context, q AskQuestion) ([]string, error)

// AskQuestion 一次询问的上下文
type AskQuestion struct {
	Kind     registry.Kind
	Name     string
	Origin   string
	Default  []string // 默认勾选（已检测且支持该 kind 的工具）
	Candidates []string
}

// IngestSkill 把工具目录里的一个 skill 收敛到 canonical。
// 返回新建的 item。
func IngestSkill(ctx context.Context, reg *registry.Registry, a adapter.Adapter, dir string) (*registry.Item, error) {
	name := filepath.Base(dir)
	dest := filepath.Join(SkillsRoot(), name)

	if err := os.MkdirAll(SkillsRoot(), 0o755); err != nil {
		return nil, err
	}
	// 复制目录（跟随软链，拷贝实体）
	if err := copyDir(dir, dest); err != nil {
		return nil, fmt.Errorf("复制 skill 到 canonical 失败: %w", err)
	}

	it := &registry.Item{
		ID:        "skill:" + name,
		Kind:      registry.KindSkill,
		Canonical: dest,
		Origin:    a.Name(),
		CreatedAt: time.Now(),
	}
	reg.UpsertItem(it)
	return it, nil
}

// ProjectSkill 把 item 分发到指定工具（软链）。
func ProjectSkill(ctx context.Context, a adapter.Adapter, it *registry.Item) error {
	name := filepath.Base(it.Canonical)
	return a.ProjectSkill(ctx, it.Canonical, name)
}

// ReplaceWithSymlink 把工具目录里的实体副本替换为指向 canonical 的软链。
func ReplaceWithSymlink(ctx context.Context, a adapter.Adapter, toolDir string, it *registry.Item) error {
	if a.SupportsSymlink() {
		// 删除实体，建软链
		if err := os.RemoveAll(toolDir); err != nil {
			return err
		}
		return a.ProjectSkill(ctx, it.Canonical, filepath.Base(it.Canonical))
	}
	return nil // 不支持软链的工具保留实体副本
}

// RemoveProjections 清理 item 在指定工具的投影（软链）
func RemoveProjections(ctx context.Context, reg *registry.Registry, adapters []adapter.Adapter, it *registry.Item, tools []string) error {
	for _, a := range adapters {
		for _, t := range tools {
			if a.Name() == t {
				if err := a.RemoveProjection(ctx, it.Kind, filepath.Base(it.Canonical)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// --- 复制目录工具函数 ---

func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("源不是目录: %s", src)
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		fi, err := os.Lstat(s)
		if err != nil {
			return err
		}
		switch {
		case fi.IsDir():
			if err := copyDir(s, d); err != nil {
				return err
			}
		case fi.Mode()&os.ModeSymlink != 0:
			// 复制软链本身（保留相对/绝对目标）
			target, err := os.Readlink(s)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, d); err != nil {
				return err
			}
		default:
			data, err := os.ReadFile(s)
			if err != nil {
				return err
			}
			if err := os.WriteFile(d, data, fi.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}
