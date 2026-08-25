// Package mcpread MCP 配置读取/写入：多工具、多格式适配（JSON/TOML/YAML）。
//
// 统一中间模型 File：Servers 存「原始 map」（保留每个 server 的全部字段，
// 如 codex 的 startup_timeout_sec），Extra 存配置文件中 servers 键以外的其他顶层键。
// 写回时按 format 序列化，避免丢字段、避免破坏工具自有的其他配置。
package mcpread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Format 配置序列化格式
type Format string

const (
	FormatJSON Format = "json"
	FormatTOML Format = "toml"
	FormatYAML Format = "yaml"
)

// File 一个 MCP 配置文件（读写模型）
type File struct {
	Path    string
	Tool    string
	Servers map[string]map[string]any // server 名 -> 原始定义（保留全部字段）
	Extra   map[string]any            // 配置文件中其他顶层键（写回时保留）

	format Format // 写回格式
	key    string // servers 所在顶层键名（如 mcpServers / mcp_servers / mcp）
}

// Read 按格式读取 MCP 配置；文件不存在返回空配置。
// key 是 servers 所在的顶层键名。
func Read(path string, format Format, key string) (*File, error) {
	f := &File{
		Path:    path,
		Servers: map[string]map[string]any{},
		Extra:   map[string]any{},
		format:  format,
		key:     key,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil // 文件不存在 → 空配置
		}
		return nil, err
	}
	raw := map[string]any{}
	switch format {
	case FormatJSON:
		// 用 UseNumber 保留数字原始形态，避免 json.Unmarshal 默认把整数解析成 float64，
		// 导致跨格式写回（如 JSON 源分发到 TOML 工具）时 120 变成 120.0。
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("解析 %s: %w", path, err)
		}
		normalizeJSONNumbers(raw)
	case FormatTOML:
		if err := toml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("解析 %s: %w", path, err)
		}
	case FormatYAML:
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("解析 %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("未知格式 %q", format)
	}

	if v, ok := raw[key]; ok {
		if sm, ok := v.(map[string]any); ok {
			for name, srv := range sm {
				if s, ok := srv.(map[string]any); ok {
					f.Servers[name] = s
				}
			}
		}
		delete(raw, key)
	}
	f.Extra = raw
	return f, nil
}

// normalizeJSONNumbers 递归把 JSON 解析产生的 json.Number 归一化为 int64/float64，
// 避免整数被解析成 float64 后，跨格式写回（TOML/YAML）时 120 变成 120.0。
func normalizeJSONNumbers(v any) {
	switch vv := v.(type) {
	case map[string]any:
		for k, val := range vv {
			if n, ok := val.(json.Number); ok {
				vv[k] = numberToAny(n)
			} else {
				normalizeJSONNumbers(val)
			}
		}
	case []any:
		for i, val := range vv {
			if n, ok := val.(json.Number); ok {
				vv[i] = numberToAny(n)
			} else {
				normalizeJSONNumbers(val)
			}
		}
	}
}

// numberToAny json.Number → int64（整数）或 float64（小数）。
func numberToAny(n json.Number) any {
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return f
	}
	return n.String()
}

// ReadJSON JSON 格式快捷读取（servers 键 = mcpServers，Claude/Cursor/Qoder 同构）
func ReadJSON(path string) (*File, error) { return Read(path, FormatJSON, "mcpServers") }

// Write 按格式写回配置文件（合并 servers 键，保留 Extra）
func (f *File) Write() error {
	out := map[string]any{}
	for k, v := range f.Extra {
		out[k] = v
	}
	ms := map[string]any{}
	for name, s := range f.Servers {
		ms[name] = s
	}
	out[f.key] = ms

	var (
		data []byte
		err  error
	)
	switch f.format {
	case FormatJSON:
		data, err = json.MarshalIndent(out, "", "  ")
		if err == nil {
			data = append(data, '\n')
		}
	case FormatTOML:
		data, err = toml.Marshal(out)
	case FormatYAML:
		data, err = yaml.Marshal(out)
	default:
		return fmt.Errorf("未知格式 %q", f.format)
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f.Path, data, 0o600)
}

// WriteJSON JSON 格式写回快捷方法
func (f *File) WriteJSON() error { return f.Write() }

// MergeFrom 把其他文件里的 server 合并进来（保留本文件已有、未被覆盖的）
func (f *File) MergeFrom(src *File, names []string) {
	for _, n := range names {
		if s, ok := src.Servers[n]; ok {
			f.Servers[n] = s
		}
	}
}
