package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/xmly/agentsync/internal/registry"
)

// Cursor 适配器：Anysphere Cursor IDE
type Cursor struct{}

func (Cursor) Name() string { return "cursor" }

func (Cursor) Detect(ctx context.Context) (bool, error) {
	return dirExists(join(homeDir(), ".cursor")) ||
		dirExists(join(homeDir(), "Library", "Application Support", "Cursor")), nil
}

func (Cursor) KindSupported(k Kind) bool {
	return k == registry.KindSkill || k == registry.KindMCP || k == registry.KindRules
}

func (Cursor) SupportsSymlink() bool { return true }

func (Cursor) WatchSpecs() []WatchSpec {
	return []WatchSpec{
		{
			Path:    join(homeDir(), ".cursor", "skills"),
			Kind:    registry.KindSkill,
			Tool:    "cursor",
			Recurse: true,
		},
		ruleWatch("cursor", join(homeDir(), ".cursor", "rules")),
		mcpWatch("cursor", join(homeDir(), ".cursor", "mcp.json")),
	}
}

func (Cursor) SkillsDir() string { return join(homeDir(), ".cursor", "skills") }

// RulesDir 本工具 rules 目录
func (Cursor) RulesDir() string { return join(homeDir(), ".cursor", "rules") }

func (Cursor) HasSKILL(dir string) bool {
	_, err := os.Stat(join(dir, "SKILL.md"))
	return err == nil
}

func (Cursor) ParseSkill(_ context.Context, dir string) (*Skill, error) {
	return &Skill{Name: filepath.Base(dir), Path: dir}, nil
}

func (c Cursor) ProjectSkill(_ context.Context, canonicalPath, name string) error {
	return symlinkDir(canonicalPath, join(c.SkillsDir(), name))
}

// ProjectRule 在 ~/.cursor/rules/ 建软链指向 canonical
func (c Cursor) ProjectRule(_ context.Context, canonicalPath, name string) error {
	return symlinkDir(canonicalPath, join(c.RulesDir(), name))
}

func (c Cursor) RemoveProjection(_ context.Context, kind Kind, name string) error {
	return removeProjection(kind, name, c.SkillsDir(), c.RulesDir())
}

func (c Cursor) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	return isSymlinkToCanonical(path), nil
}
