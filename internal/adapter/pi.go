package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/xmly/agentsync/internal/registry"
)

// Pi 适配器：Pi coding agent。
// 特殊：Pi 原生读取 ~/.agents/skills/（正是 canonical），skill 分发零成本（无需软链）。
type Pi struct{}

func (Pi) Name() string { return "pi" }

func (Pi) Detect(ctx context.Context) (bool, error) {
	if dirExists(join(homeDir(), ".pi")) {
		return true, nil
	}
	if _, err := os.Stat(join(homeDir(), ".pi", "agent", "skills")); err == nil {
		return true, nil
	}
	return false, nil
}

func (Pi) KindSupported(k Kind) bool {
	return k == registry.KindSkill // Pi 无 MCP（官方明确不支持），rules 暂缓
}

func (Pi) SupportsSymlink() bool { return true }

// WatchSpecs Pi 监听自身用户级 skills 目录（~/.pi/agent/skills/）与兼容目录。
// 注意：Pi 同时读 ~/.agents/skills/（= canonical），那里是收敛目标，不监听。
func (Pi) WatchSpecs() []WatchSpec {
	return []WatchSpec{
		{
			Path:    join(homeDir(), ".pi", "agent", "skills"),
			Kind:    registry.KindSkill,
			Tool:    "pi",
			Recurse: true,
		},
	}
}

func (Pi) SkillsDir() string { return join(homeDir(), ".pi", "agent", "skills") }

// RulesDir Pi 无 rules 目录
func (Pi) RulesDir() string { return "" }

func (Pi) HasSKILL(dir string) bool {
	_, err := os.Stat(join(dir, "SKILL.md"))
	return err == nil
}

func (Pi) ParseSkill(_ context.Context, dir string) (*Skill, error) {
	return &Skill{Name: filepath.Base(dir), Path: dir}, nil
}

// ProjectSkill Pi 特例：canonical 本身就是 ~/.agents/skills/，Pi 原生读取，
// 无需在 ~/.pi/agent/skills/ 建软链（无操作）。
func (Pi) ProjectSkill(_ context.Context, canonicalPath, name string) error {
	return nil // zero-cost：Pi 直接读 canonical
}

// ProjectRule Pi 无 rules 目录，空操作
func (Pi) ProjectRule(_ context.Context, canonicalPath, name string) error { return nil }

func (Pi) RemoveProjection(_ context.Context, kind Kind, name string) error {
	// Pi 无独立投影，无需清理
	return nil
}

func (Pi) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	// Pi 目录里的实体 skill 不会是我方软链；canonical 由 sync 层管理
	return false, nil
}
