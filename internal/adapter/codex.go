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
	}
}

func (Codex) SkillsDir() string { return join(homeDir(), ".codex", "skills") }

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

func (Codex) RemoveProjection(_ context.Context, kind Kind, name string) error {
	if kind != registry.KindSkill {
		return nil
	}
	return removeSymlinkOnly(join(homeDir(), ".codex", "skills", name))
}

func (c Codex) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	return isSymlinkToCanonical(path), nil
}
