package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/xmly/agentsync/internal/registry"
)

// ClaudeCode 适配器：Anthropic Claude Code
type ClaudeCode struct{}

func (ClaudeCode) Name() string { return "claude-code" }

func (ClaudeCode) Detect(ctx context.Context) (bool, error) {
	// 配置目录或可执行文件任一存在即视为已安装
	if dirExists(join(homeDir(), ".claude")) {
		return true, nil
	}
	if _, err := os.Stat(join(homeDir(), ".claude.json")); err == nil {
		return true, nil
	}
	return false, nil
}

func (ClaudeCode) KindSupported(k Kind) bool {
	return k == registry.KindSkill || k == registry.KindMCP || k == registry.KindRules
}

func (ClaudeCode) SupportsSymlink() bool { return true }

func (ClaudeCode) WatchSpecs() []WatchSpec {
	return []WatchSpec{
		{
			Path:    join(homeDir(), ".claude", "skills"),
			Kind:    registry.KindSkill,
			Tool:    "claude-code",
			Recurse: true,
		},
	}
}

// SkillsDir 本工具 skill 目录
func (ClaudeCode) SkillsDir() string { return join(homeDir(), ".claude", "skills") }

func (ClaudeCode) HasSKILL(dir string) bool {
	_, err := os.Stat(join(dir, "SKILL.md"))
	return err == nil
}

func (ClaudeCode) ParseSkill(_ context.Context, dir string) (*Skill, error) {
	return &Skill{Name: filepath.Base(dir), Path: dir}, nil
}

// ProjectSkill 在 ~/.claude/skills/ 建软链指向 canonical
func (c ClaudeCode) ProjectSkill(_ context.Context, canonicalPath, name string) error {
	return symlinkDir(canonicalPath, join(c.SkillsDir(), name))
}

func (ClaudeCode) RemoveProjection(_ context.Context, kind Kind, name string) error {
	if kind != registry.KindSkill {
		return nil
	}
	return removeSymlinkOnly(join(homeDir(), ".claude", "skills", name))
}

func (c ClaudeCode) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	return isSymlinkToCanonical(path), nil
}

// --- 辅助函数（本包共享） ---

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// symlinkDir 建软链 target <- link（跨平台，Windows 回退 junction/复制由调用方处理）
func symlinkDir(target, link string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	// 已存在则先删（仅当它是软链）
	if _, err := os.Lstat(link); err == nil {
		if fi, e := os.Lstat(link); e == nil && fi.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(link)
		}
	}
	return os.Symlink(target, link)
}

// removeSymlinkOnly 只删软链，不递归删实体
func removeSymlinkOnly(p string) error {
	fi, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return os.Remove(p)
	}
	return nil // 实体目录不动（可能是用户手动建的）
}
