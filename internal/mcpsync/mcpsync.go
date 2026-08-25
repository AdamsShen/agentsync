// Package mcpsync MCP 跨工具同步：读取各工具配置 → 合并 registry → 写回。
//
// 多工具多格式：claude-code/cursor/qoder 为 JSON(mcpServers)，
// codex 为 TOML(mcp_servers)，opencode 为 JSON(mcp)，hermes 为 YAML(mcp_servers，待实测)。
package mcpsync

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/xmly/agentsync/internal/mcpread"
	"github.com/xmly/agentsync/internal/registry"
)

// McpAdapter 一个工具的 MCP 配置读写
type McpAdapter interface {
	Name() string
	McpFile() string        // 配置文件路径（不存在则视为空）
	Format() mcpread.Format // json / toml / yaml
	ServersKey() string     // servers 所在顶层键名
}

// mcpCfg 通用实现
type mcpCfg struct {
	name, path string
	format     mcpread.Format
	key        string
}

func (m mcpCfg) Name() string           { return m.name }
func (m mcpCfg) McpFile() string        { return m.path }
func (m mcpCfg) Format() mcpread.Format { return m.format }
func (m mcpCfg) ServersKey() string     { return m.key }

// Adapters 已支持 MCP 的全部工具（按格式+键名区分）
func Adapters() []McpAdapter {
	h, _ := os.UserHomeDir()
	return []McpAdapter{
		mcpCfg{"claude-code", filepath.Join(h, ".claude.json"), mcpread.FormatJSON, "mcpServers"},
		mcpCfg{"cursor", filepath.Join(h, ".cursor", "mcp.json"), mcpread.FormatJSON, "mcpServers"},
		mcpCfg{"qoder", filepath.Join(h, ".qoder", "mcp.json"), mcpread.FormatJSON, "mcpServers"},
		mcpCfg{"codex", filepath.Join(h, ".codex", "config.toml"), mcpread.FormatTOML, "mcp_servers"},
		mcpCfg{"opencode", filepath.Join(h, ".config", "opencode", "opencode.json"), mcpread.FormatJSON, "mcp"},
		// hermes config.yaml 的 mcp 键名待实测；先用 mcp_servers 占位
		mcpCfg{"hermes", filepath.Join(h, ".hermes", "config.yaml"), mcpread.FormatYAML, "mcp_servers"},
	}
}

// AdapterByName 按名字取 MCP 适配器
func AdapterByName(name string) (McpAdapter, bool) {
	for _, a := range Adapters() {
		if a.Name() == name {
			return a, true
		}
	}
	return nil, false
}

// readFile 读取某工具 MCP 配置（不存在返回空配置）
func readFile(a McpAdapter) (*mcpread.File, error) {
	return mcpread.Read(a.McpFile(), a.Format(), a.ServersKey())
}

// Diff 检测某工具配置相对 registry 的新增/变更 server
type Diff struct {
	Added   []string // 新增 server 名
	Changed []string // 内容变化 server 名
	Removed []string // registry 有、工具没了（外部删除）
}

// DetectDiff 对比工具配置与 registry，返回差异
func DetectDiff(f *mcpread.File, reg *registry.Registry, origin string) Diff {
	d := Diff{}
	for name := range f.Servers {
		it := reg.GetItem("mcp:" + name)
		if it == nil {
			d.Added = append(d.Added, name)
			continue
		}
		if it.Origin != origin {
			continue // 别处收敛的，不重复收敛
		}
		cur := hashServer(f.Servers[name])
		if it.LastHash != "" && it.LastHash != cur {
			d.Changed = append(d.Changed, name)
		}
	}
	for _, it := range reg.Items {
		if it.Kind != registry.KindMCP || it.Origin != origin {
			continue
		}
		name := it.ID[len("mcp:"):]
		if _, ok := f.Servers[name]; !ok {
			d.Removed = append(d.Removed, name)
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Changed)
	sort.Strings(d.Removed)
	return d
}

// SyncFromTool 某工具配置变化时：收敛新/变 server 进 registry。
// origin 是变化来源工具；返回本次收敛的 server 名。
func SyncFromTool(ctx context.Context, reg *registry.Registry, origin string, f *mcpread.File) ([]string, error) {
	diff := DetectDiff(f, reg, origin)
	var synced []string
	for _, name := range diff.Added {
		it := &registry.Item{
			ID:        "mcp:" + name,
			Kind:      registry.KindMCP,
			Origin:    origin,
			Config:    f.Servers[name],
			LastHash:  hashServer(f.Servers[name]),
			CreatedAt: time.Now(),
		}
		reg.UpsertItem(it)
		synced = append(synced, name)
	}
	for _, name := range diff.Changed {
		if it := reg.GetItem("mcp:" + name); it != nil {
			it.Config = f.Servers[name]
			it.LastHash = hashServer(f.Servers[name])
			reg.UpsertItem(it)
			synced = append(synced, name)
		}
	}
	if len(synced) > 0 {
		_ = reg.Save()
	}
	return synced, nil
}

// ProjectTo 把 registry 里的 mcp server 写回某工具配置。
func ProjectTo(reg *registry.Registry, a McpAdapter, names []string) error {
	f, err := readFile(a)
	if err != nil {
		return err
	}
	for _, name := range names {
		it := reg.GetItem("mcp:" + name)
		if it == nil || it.Kind != registry.KindMCP {
			continue
		}
		f.Servers[name] = it.Config
	}
	return f.Write()
}

// RemoveFromTool 从工具配置移除某 server
func RemoveFromTool(reg *registry.Registry, a McpAdapter, name string) error {
	f, err := readFile(a)
	if err != nil {
		return err
	}
	delete(f.Servers, name)
	return f.Write()
}

// hashServer 计算 server 定义的稳定哈希（json 序列化原始 map）
func hashServer(m map[string]any) string {
	b, _ := json.Marshal(m)
	s := sha256.Sum256(b)
	return fmt.Sprintf("%x", s[:])
}
