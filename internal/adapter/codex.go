package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/xmly/agentsync/internal/registry"
)

// Codex 适配器：OpenAI Codex（skills 目录 ~/.codex/skills/，MCP 为 config.toml TOML）
type Codex struct{}

func (Codex) Name() string { return "codex" }

func (Codex) Detect(ctx context.Context) (bool, error) {
	if dirExists(join(homeDir(), ".codex")) {
		return true, nil
	}
	if _, err := os.Stat(join(homeDir(), ".codex", "config.toml")); err == nil {
		return true, nil
	}
	return false, nil
}

func (Codex) KindSupported(k Kind) bool {
	return k == registry.KindSkill || k == registry.KindMCP || k == registry.KindRules
}

func (Codex) SupportsSymlink() bool { return true } // 待实测（官方 skills 为实体目录）

func (Codex) WatchSpecs() []WatchSpec {
	return []WatchSpec{
		{
			Path:    join(homeDir(), ".codex", "skills"),
			Kind:    registry.KindSkill,
			Tool:    "codex",
			Recurse: true,
		},
		mcpWatch("codex", join(homeDir(), ".codex", "config.toml")),
	}
}

func (Codex) SkillsDir() string { return join(homeDir(), ".codex", "skills") }

// RulesDir codex 走 AGENTS.md 单文件约定，无 rules 目录（待 M4.5 支持单文件规则）
func (Codex) RulesDir() string { return "" }

func (Codex) HasSKILL(dir string) bool {
	_, err := os.Stat(join(dir, "SKILL.md"))
	return err == nil
}

func (Codex) ParseSkill(_ context.Context, dir string) (*Skill, error) {
	return &Skill{Name: filepath.Base(dir), Path: dir}, nil
}

func (c Codex) ProjectSkill(_ context.Context, canonicalPath, name string) error {
	return symlinkDir(canonicalPath, join(c.SkillsDir(), name))
}

// ProjectRule codex 无 rules 目录，空操作
func (c Codex) ProjectRule(_ context.Context, canonicalPath, name string) error {
	return projectRule(c.RulesDir(), canonicalPath, name)
}

func (c Codex) RemoveProjection(_ context.Context, kind Kind, name string) error {
	return removeProjection(kind, name, c.SkillsDir(), c.RulesDir())
}

func (c Codex) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	return isSymlinkToCanonical(path), nil
}
