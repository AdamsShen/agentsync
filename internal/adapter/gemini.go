package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/xmly/agentsync/internal/registry"
)

// Gemini 适配器：Gemini CLI（skills 目录 ~/.gemini/skills/，MCP 为 ~/.gemini/settings.json）
type Gemini struct{}

func (Gemini) Name() string { return "gemini" }

func (Gemini) Detect(ctx context.Context) (bool, error) {
	if dirExists(join(homeDir(), ".gemini")) {
		return true, nil
	}
	if _, err := os.Stat(join(homeDir(), ".gemini", "settings.json")); err == nil {
		return true, nil
	}
	return false, nil
}

func (Gemini) KindSupported(k Kind) bool {
	return k == registry.KindSkill || k == registry.KindMCP || k == registry.KindRules
}

func (Gemini) SupportsSymlink() bool { return true } // 本机未装 gemini，待实测

func (Gemini) WatchSpecs() []WatchSpec {
	return []WatchSpec{
		{
			Path:    join(homeDir(), ".gemini", "skills"),
			Kind:    registry.KindSkill,
			Tool:    "gemini",
			Recurse: true,
		},
	}
}

func (Gemini) SkillsDir() string { return join(homeDir(), ".gemini", "skills") }

// RulesDir gemini 本机未装，rules 目录约定待实测，暂不启用
func (Gemini) RulesDir() string { return "" }

func (Gemini) HasSKILL(dir string) bool {
	_, err := os.Stat(join(dir, "SKILL.md"))
	return err == nil
}

func (Gemini) ParseSkill(_ context.Context, dir string) (*Skill, error) {
	return &Skill{Name: filepath.Base(dir), Path: dir}, nil
}

func (g Gemini) ProjectSkill(_ context.Context, canonicalPath, name string) error {
	return symlinkDir(canonicalPath, join(g.SkillsDir(), name))
}

// ProjectRule gemini 无 rules 目录，空操作
func (g Gemini) ProjectRule(_ context.Context, canonicalPath, name string) error {
	return projectRule(g.RulesDir(), canonicalPath, name)
}

func (g Gemini) RemoveProjection(_ context.Context, kind Kind, name string) error {
	return removeProjection(kind, name, g.SkillsDir(), g.RulesDir())
}

func (g Gemini) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	return isSymlinkToCanonical(path), nil
}
