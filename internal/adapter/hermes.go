package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/AdamsShen/agentsync/internal/registry"
)

// Hermes 适配器：Hermes（本机实测，用户级 skills 目录为 ~/.hermes/skills/）
type Hermes struct{}

func (Hermes) Name() string { return "hermes" }

func (Hermes) Detect(ctx context.Context) (bool, error) {
	if dirExists(join(homeDir(), ".hermes")) {
		return true, nil
	}
	if _, err := os.Stat(join(homeDir(), ".hermes", "config.yaml")); err == nil {
		return true, nil
	}
	return false, nil
}

func (Hermes) KindSupported(k Kind) bool {
	return k == registry.KindSkill || k == registry.KindMCP || k == registry.KindRules
}

func (Hermes) SupportsSymlink() bool { return true } // 已实测：skills 目录内软链生效

func (Hermes) WatchSpecs() []WatchSpec {
	return []WatchSpec{
		{
			Path:    join(homeDir(), ".hermes", "skills"),
			Kind:    registry.KindSkill,
			Tool:    "hermes",
			Recurse: true,
		},
		mcpWatch("hermes", join(homeDir(), ".hermes", "config.yaml")),
		// 单文件规则：只收敛进 canonical，不分发回 hermes（SOUL.md 是 hermes 人格文件）
		ruleFileWatch("hermes", join(homeDir(), ".hermes", "SOUL.md")),
	}
}

func (Hermes) SkillsDir() string { return join(homeDir(), ".hermes", "skills") }

// RulesDir hermes 已实测无独立 rules 目录（仅根目录 SOUL.md 单文件）。
// SOUL.md 经 ruleFileWatch 收敛进 canonical（只收敛，不分发到 hermes）。
func (Hermes) RulesDir() string { return "" }

func (Hermes) HasSKILL(dir string) bool {
	_, err := os.Stat(join(dir, "SKILL.md"))
	return err == nil
}

func (Hermes) ParseSkill(_ context.Context, dir string) (*Skill, error) {
	return &Skill{Name: filepath.Base(dir), Path: dir}, nil
}

func (h Hermes) ProjectSkill(_ context.Context, canonicalPath, name string) error {
	return symlinkDir(canonicalPath, join(h.SkillsDir(), name))
}

// ProjectRule hermes 无 rules 目录，空操作
func (h Hermes) ProjectRule(_ context.Context, canonicalPath, name string) error {
	return projectRule(h.RulesDir(), canonicalPath, name)
}

func (h Hermes) RemoveProjection(_ context.Context, kind Kind, name string) error {
	return removeProjection(kind, name, h.SkillsDir(), h.RulesDir())
}

func (h Hermes) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	return isSymlinkToCanonical(path), nil
}
