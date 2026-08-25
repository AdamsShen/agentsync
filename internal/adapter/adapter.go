// Package adapter 定义各 agent 工具适配器接口与注册表。
package adapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/AdamsShen/agentsync/internal/registry"
)

// Kind 复用 registry.Kind（skill / mcp / rules）
type Kind = registry.Kind

// Skill 解析出的 skill 元信息
type Skill struct {
	Name string // 目录名
	Path string // 绝对路径
}

// WatchSpec 描述一个要监听的路径
type WatchSpec struct {
	Path    string // 绝对路径
	Kind    Kind   // skill / mcp / rules
	Tool    string // 归属工具
	Glob    string // 可选过滤
	Recurse bool   // 目录类递归监听
}

// Adapter 每个 agent 工具一个实现。
type Adapter interface {
	Name() string
	Detect(ctx context.Context) (bool, error)
	KindSupported(kind Kind) bool
	SupportsSymlink() bool

	// WatchSpecs 返回需监听的路径
	WatchSpecs() []WatchSpec

	// Ingest 入站：解析工具目录里的内容
	HasSKILL(dir string) bool
	ParseSkill(ctx context.Context, dir string) (*Skill, error)

	// Project 出站：把 canonical 分发到本工具
	ProjectSkill(ctx context.Context, canonicalPath, name string) error
	ProjectRule(ctx context.Context, canonicalPath, name string) error
	RemoveProjection(ctx context.Context, kind Kind, name string) error
	IsOwnedProjection(ctx context.Context, path string) (bool, error)

	// RulesDir 返回本工具 rules 目录（不支持 rules 返回 ""）
	RulesDir() string
}

// homeDir 统一取用户主目录（跨平台）
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// canonicalRoot 统一副本根目录（~/.agents/），与 sync.CanonicalRoot 保持一致
func canonicalRoot() string { return filepath.Join(homeDir(), ".agents") }

// join 拼接绝对路径
func join(elem ...string) string { return filepath.Join(elem...) }

// isSymlinkToCanonical 判断 path 是否是我方投影：软链且指向 canonical 根下
func isSymlinkToCanonical(path string) bool {
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	tgt := filepath.Clean(target)
	root := filepath.Clean(canonicalRoot())
	return tgt == root || len(tgt) > len(root) && tgt[:len(root)+1] == root+string(filepath.Separator)
}
