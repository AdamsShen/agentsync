package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/AdamsShen/agentsync/internal/registry"
)

// Grok 适配器：Grok Build（xAI 的 grok-code-fast，CLI 名 grok）。
// 配置目录 ~/.grok/（GROK_HOME 可覆盖），主配置 config.toml。
// 已确认（官方文档）：skills 目录 ~/.grok/skills/，MCP 为 config.toml 的 [mcp_servers.<name>] 段（TOML，与 codex 同构）。
// rules 无独立目录：grok 兼容扫描 ~/.claude/rules/ 与 ~/.cursor/rules/，自动获得 agentsync 已分发的 rules。
// 待实测：本机未安装（x.ai 官方安装源被墙，无法下载二进制；配置格式经官方文档确认）。
type Grok struct{}

func (Grok) Name() string { return "grok" }

func (Grok) Detect(ctx context.Context) (bool, error) {
	if dirExists(join(homeDir(), ".grok")) {
		return true, nil
	}
	if _, err := os.Stat(join(homeDir(), ".grok", "config.toml")); err == nil {
		return true, nil
	}
	return false, nil
}

func (Grok) KindSupported(k Kind) bool {
	return k == registry.KindSkill || k == registry.KindMCP || k == registry.KindRules
}

func (Grok) SupportsSymlink() bool { return true } // 待实测（官方文档未明示 skills 软链语义）

func (Grok) WatchSpecs() []WatchSpec {
	return []WatchSpec{
		{
			Path:    join(homeDir(), ".grok", "skills"),
			Kind:    registry.KindSkill,
			Tool:    "grok",
			Recurse: true,
		},
		mcpWatch("grok", join(homeDir(), ".grok", "config.toml")),
	}
}

func (Grok) SkillsDir() string { return join(homeDir(), ".grok", "skills") }

// RulesDir grok 无独立 rules 目录：grok 兼容扫描 ~/.claude/rules/ 与 ~/.cursor/rules/，
// agentsync 已向这两处分发 rules，grok 自动获得，无需单独分发。
func (Grok) RulesDir() string { return "" }

func (Grok) HasSKILL(dir string) bool {
	_, err := os.Stat(join(dir, "SKILL.md"))
	return err == nil
}

func (Grok) ParseSkill(_ context.Context, dir string) (*Skill, error) {
	return &Skill{Name: filepath.Base(dir), Path: dir}, nil
}

func (g Grok) ProjectSkill(_ context.Context, canonicalPath, name string) error {
	return symlinkDir(canonicalPath, join(g.SkillsDir(), name))
}

// ProjectRule grok 无独立 rules 目录，空操作（规则经兼容扫描 Claude/Cursor 获得）
func (g Grok) ProjectRule(_ context.Context, canonicalPath, name string) error {
	return projectRule(g.RulesDir(), canonicalPath, name)
}

func (g Grok) RemoveProjection(_ context.Context, kind Kind, name string) error {
	return removeProjection(kind, name, g.SkillsDir(), g.RulesDir())
}

func (g Grok) IsOwnedProjection(_ context.Context, path string) (bool, error) {
	return isSymlinkToCanonical(path), nil
}
