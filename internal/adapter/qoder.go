package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/xmly/agentsync/internal/registry"
)

// Qoder 适配器：Qoder IDE（实测 skills 全软链、mcp.json 与 Claude 同构）
type Qoder struct{}

func (Qoder) Name() string { return "qoder" }

func (Qoder) Detect(ctx context.Context) (bool, error) {
	if dirExists(join(homeDir(), ".qoder")) {
		return true, nil
	}
	if dirExists(join(homeDir(), "Library", "Application Support", "Qoder")) {
		return true, nil
	}
	return false, nil
}

func (Qoder) KindSupported(k Kind) bool {
	return k == registry.KindSkill || k == registry.KindMCP || k == registry.KindRules
}

func (Qoder) SupportsSymlink() bool { return true } // 实测 43/43 全软链

func (Qoder) WatchSpecs() []WatchSpec {
	return []WatchSpec{
		{
			Path:    join(homeDir(), ".qoder", "skills"),
			Kind:    registry.KindSkill,
			Tool:    "qoder",
			Recurse: true,
		},
	}
}

func (Qoder) SkillsDir() string { return join(homeDir(), ".qoder", "skills") }

func (Qoder) HasSKILL(dir string) bool {
	_, err := os.Stat(join(dir, "SKILL.md"))
	return err == nil
}

func (Qoder) ParseSkill(_ context.Context, dir string) (*Skill, error) {
	return &Skill{Name: filepath.Base(dir), Path: dir}, nil
}

func (q Qoder) ProjectSkill(_ context.Context, canonicalPath, name string) error {
	return symlinkDir(canonicalPath, join(q.SkillsDir(), name))
}

func (Qoder) RemoveProjection(_ context.Context, kind Kind, name string) error {
	if kind != registry.KindSkill {
		return nil
	}
	return removeSymlinkOnly(join(homeDir(), ".qoder", "skills", name))
}

func (q Qoder) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	return isSymlinkToCanonical(path), nil
}
