// Package mcpsync MCP 跨工具同步：读取各工具配置 → 合并 registry → 写回。
// M1：claude-code / cursor / qoder 为 JSON 同构；hermes 为 YAML（暂延 M1.5）。
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

// McpAdapter 一个工具的 MCP 配置读写（JSON 同构族）
type McpAdapter interface {
	Name() string
	McpFile() string // 配置文件路径（不存在则视为空）
}

// JSON 同构族实现
type jsonMcp struct{ name, path string }

func (j jsonMcp) Name() string    { return j.name }
func (j jsonMcp) McpFile() string { return j.path }

// 各工具 MCP 文件位置
func claudeMcpFile() string { h, _ := os.UserHomeDir(); return filepath.Join(h, ".claude.json") }
func cursorMcpFile() string { h, _ := os.UserHomeDir(); return filepath.Join(h, ".cursor", "mcp.json") }
func qoderMcpFile() string  { h, _ := os.UserHomeDir(); return filepath.Join(h, ".qoder", "mcp.json") }

// adapters 已支持 MCP 的工具（JSON 同构）
func adapters() []McpAdapter {
	return []McpAdapter{
		jsonMcp{name: "claude-code", path: claudeMcpFile()},
		jsonMcp{name: "cursor", path: cursorMcpFile()},
		jsonMcp{name: "qoder", path: qoderMcpFile()},
	}
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
		// 对比内容：config 哈希
		cur := hashServer(f.Servers[name])
		if it.LastHash != "" && it.LastHash != cur {
			d.Changed = append(d.Changed, name)
		}
	}
	// removed：registry 里有该 origin 的 mcp，但工具文件里没了
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

// SyncFromTool 某工具配置变化时：收敛新/变 server 进 registry → 写回其他工具。
// origin 是变化的来源工具；targets 是要写回的工具（不含 origin，除非显式）。
func SyncFromTool(ctx context.Context, reg *registry.Registry, origin string, f *mcpread.File) ([]string, error) {
	// 1. 收敛新增/变更 server 进 registry
	diff := DetectDiff(f, reg, origin)
	var synced []string
	for _, name := range diff.Added {
		it := &registry.Item{
			ID:        "mcp:" + name,
			Kind:      registry.KindMCP,
			Origin:    origin,
			Config:    serverToMap(f.Servers[name]),
			LastHash:  hashServer(f.Servers[name]),
			CreatedAt: time.Now(),
		}
		reg.UpsertItem(it)
		synced = append(synced, name)
	}
	for _, name := range diff.Changed {
		if it := reg.GetItem("mcp:" + name); it != nil {
			it.Config = serverToMap(f.Servers[name])
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
	f, err := mcpread.ReadJSON(a.McpFile())
	if err != nil {
		return err
	}
	for _, name := range names {
		it := reg.GetItem("mcp:" + name)
		if it == nil || it.Kind != registry.KindMCP {
			continue
		}
		srv, ok := mapToServer(it.Config)
		if !ok {
			continue
		}
		// 写回（覆盖该 server 定义，保留其他）
		f.Servers[name] = srv
	}
	return f.WriteJSON()
}

// RemoveFromTool 从工具配置移除某 server
func RemoveFromTool(reg *registry.Registry, a McpAdapter, name string) error {
	f, err := mcpread.ReadJSON(a.McpFile())
	if err != nil {
		return err
	}
	delete(f.Servers, name)
	return f.WriteJSON()
}

// hashServer 计算 server 定义的稳定哈希（json 序列化）
func hashServer(s mcpread.Server) string {
	b, _ := jsonMarshal(s)
	return fmt.Sprintf("%x", sha256Sum(b))
}

func serverToMap(s mcpread.Server) map[string]any {
	m := map[string]any{}
	if s.Type != "" {
		m["type"] = s.Type
	}
	if s.URL != "" {
		m["url"] = s.URL
	}
	if s.Command != "" {
		m["command"] = s.Command
	}
	if len(s.Args) > 0 {
		m["args"] = s.Args
	}
	if len(s.Env) > 0 {
		m["env"] = s.Env
	}
	if len(s.Headers) > 0 {
		m["headers"] = s.Headers
	}
	return m
}

func mapToServer(m map[string]any) (mcpread.Server, bool) {
	var s mcpread.Server
	b, err := jsonMarshal(m)
	if err != nil {
		return s, false
	}
	if err := jsonUnmarshal(b, &s); err != nil {
		return s, false
	}
	return s, true
}

// 隔离 json/sha 便于测试与替换
var (
	jsonMarshal   = func(v any) ([]byte, error) { return json.Marshal(v) }
	jsonUnmarshal = func(b []byte, v any) error { return json.Unmarshal(b, v) }
	sha256Sum     = func(b []byte) []byte { s := sha256.Sum256(b); return s[:] }
)
