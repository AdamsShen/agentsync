package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/xmly/agentsync/internal/registry"
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

func (Hermes) SupportsSymlink() bool { return true } // 待实测确认

func (Hermes) WatchSpecs() []WatchSpec {
	return []WatchSpec{
		{
			Path:    join(homeDir(), ".hermes", "skills"),
			Kind:    registry.KindSkill,
			Tool:    "hermes",
			Recurse: true,
		},
	}
}

func (Hermes) SkillsDir() string { return join(homeDir(), ".hermes", "skills") }

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

func (Hermes) RemoveProjection(_ context.Context, kind Kind, name string) error {
	if kind != registry.KindSkill {
		return nil
	}
	return removeSymlinkOnly(join(homeDir(), ".hermes", "skills", name))
}

func (h Hermes) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	return isSymlinkToCanonical(path), nil
}
