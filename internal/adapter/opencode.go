package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/AdamsShen/agentsync/internal/registry"
)

// OpenCode 适配器：opencode（skills 目录 ~/.config/opencode/skills/，MCP 为 opencode.json 的 mcp 键）
type OpenCode struct{}

func (OpenCode) Name() string { return "opencode" }

func (OpenCode) Detect(ctx context.Context) (bool, error) {
	if dirExists(join(homeDir(), ".config", "opencode")) {
		return true, nil
	}
	if dirExists(join(homeDir(), ".opencode")) {
		return true, nil
	}
	return false, nil
}

func (OpenCode) KindSupported(k Kind) bool {
	return k == registry.KindSkill || k == registry.KindMCP || k == registry.KindRules
}

func (OpenCode) SupportsSymlink() bool { return true } // 待实测

func (OpenCode) WatchSpecs() []WatchSpec {
	return []WatchSpec{
		{
			Path:    join(homeDir(), ".config", "opencode", "skills"),
			Kind:    registry.KindSkill,
			Tool:    "opencode",
			Recurse: true,
		},
		mcpWatch("opencode", join(homeDir(), ".config", "opencode", "opencode.json")),
	}
}

func (OpenCode) SkillsDir() string { return join(homeDir(), ".config", "opencode", "skills") }

// RulesDir opencode 走 AGENTS.md 单文件约定，无 rules 目录
func (OpenCode) RulesDir() string { return "" }

func (OpenCode) HasSKILL(dir string) bool {
	_, err := os.Stat(join(dir, "SKILL.md"))
	return err == nil
}

func (OpenCode) ParseSkill(_ context.Context, dir string) (*Skill, error) {
	return &Skill{Name: filepath.Base(dir), Path: dir}, nil
}

func (o OpenCode) ProjectSkill(_ context.Context, canonicalPath, name string) error {
	return symlinkDir(canonicalPath, join(o.SkillsDir(), name))
}

// ProjectRule opencode 无 rules 目录，空操作
func (o OpenCode) ProjectRule(_ context.Context, canonicalPath, name string) error {
	return projectRule(o.RulesDir(), canonicalPath, name)
}

func (o OpenCode) RemoveProjection(_ context.Context, kind Kind, name string) error {
	return removeProjection(kind, name, o.SkillsDir(), o.RulesDir())
}

func (o OpenCode) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	return isSymlinkToCanonical(path), nil
}
