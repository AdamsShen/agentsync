// Package mcpread MCP 配置读取/写入：多工具格式适配（JSON/TOML/YAML）。
package mcpread

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Server 一个 MCP server 定义（统一中间结构）
type Server struct {
	Type    string            `json:"type,omitempty"`    // http/sse/ws/stdio（空=stdio）
	URL     string            `json:"url,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// File 一个 MCP 配置文件（读写模型）
type File struct {
	Path      string
	Tool      string
	Servers   map[string]Server
	Extra     map[string]any // 配置文件中其他键（写回时保留）
}

// ReadJSON 读取 JSON 格式 MCP 配置（.mcp.json / ~/.cursor/mcp.json / ~/.qoder/mcp.json）
func ReadJSON(path string) (*File, error) {
	f := &File{Path: path, Servers: map[string]Server{}, Extra: map[string]any{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil // 文件不存在 → 空配置
		}
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析 %s: %w", path, err)
	}
	if ms, ok := raw["mcpServers"]; ok {
		var srv map[string]json.RawMessage
		if err := json.Unmarshal(ms, &srv); err != nil {
			return nil, fmt.Errorf("解析 mcpServers in %s: %w", path, err)
		}
		for name, rawSrv := range srv {
			var s Server
			if err := json.Unmarshal(rawSrv, &s); err != nil {
				return nil, fmt.Errorf("解析 server %s: %w", name, err)
			}
			f.Servers[name] = s
		}
		delete(raw, "mcpServers")
	}
	for k, v := range raw {
		var any any
		if json.Unmarshal(v, &any) == nil {
			f.Extra[k] = any
		}
	}
	return f, nil
}

// WriteJSON 写回 JSON 配置（合并 mcpServers，保留 Extra 键）
func (f *File) WriteJSON() error {
	out := map[string]any{}
	for k, v := range f.Extra {
		out[k] = v
	}
	ms := map[string]any{}
	for name, s := range f.Servers {
		ms[name] = s
	}
	out["mcpServers"] = ms
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f.Path, data, 0o600)
}

// MergeFrom 把其他文件里的 server 合并进来（保留本文件已有、未被覆盖的）
func (f *File) MergeFrom(src *File, names []string) {
	for _, n := range names {
		if s, ok := src.Servers[n]; ok {
			f.Servers[n] = s
		}
	}
}
