package mcpsync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xmly/agentsync/internal/mcpread"
	"github.com/xmly/agentsync/internal/registry"
)

// 用临时 HOME 隔离（各工具 MCP 路径走 os.UserHomeDir）
func withTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	// 预建 cursor/qoder 目录
	for _, d := range []string{".cursor", ".qoder"} {
		if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeClaudeMcp(t *testing.T, content string) {
	t.Helper()
	os.WriteFile(filepath.Join(os.Getenv("HOME"), ".claude.json"), []byte(content), 0o600)
}

func writeCursorMcp(t *testing.T, content string) {
	t.Helper()
	os.WriteFile(filepath.Join(os.Getenv("HOME"), ".cursor", "mcp.json"), []byte(content), 0o600)
}

func TestSyncFromTool_AddsToRegistry(t *testing.T) {
	withTempHome(t)
	reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
	ctx := context.Background()

	// cursor 配置出现新 server
	writeCursorMcp(t, `{"mcpServers": {"github": {"command": "npx", "args": ["-y", "x"]}}}`)
	f, err := mcpread.ReadJSON(filepath.Join(os.Getenv("HOME"), ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	synced, err := SyncFromTool(ctx, reg, "cursor", f)
	if err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 || synced[0] != "github" {
		t.Fatalf("期望收敛 github，得 %v", synced)
	}
	it := reg.GetItem("mcp:github")
	if it == nil || it.Origin != "cursor" || it.Kind != registry.KindMCP {
		t.Fatal("registry 无正确 mcp item")
	}
}

func TestProjectTo_WritesOtherTools(t *testing.T) {
	withTempHome(t)
	reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
	ctx := context.Background()

	// 先在 claude 配置加 server，收敛进 registry
	writeClaudeMcp(t, `{"mcpServers": {"db": {"command": "uv", "args": ["run", "db.py"], "env": {"DB": "x"}}}}`)
	f, err := mcpread.ReadJSON(filepath.Join(os.Getenv("HOME"), ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SyncFromTool(ctx, reg, "claude-code", f); err != nil {
		t.Fatal(err)
	}

	// 写回 cursor
	cursor := jsonMcp{name: "cursor", path: filepath.Join(os.Getenv("HOME"), ".cursor", "mcp.json")}
	if err := ProjectTo(reg, cursor, []string{"db"}); err != nil {
		t.Fatal(err)
	}
	got, err := mcpread.ReadJSON(cursor.McpFile())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Servers["db"]; !ok {
		t.Fatal("cursor 未收到 db server")
	}
	if got.Servers["db"].Env["DB"] != "x" {
		t.Fatal("env 未同步")
	}
}

func TestProjectTo_PreservesExistingServers(t *testing.T) {
	withTempHome(t)
	reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
	ctx := context.Background()

	// claude 有 server A
	writeClaudeMcp(t, `{"mcpServers": {"A": {"command": "ca"}}}`)
	f, _ := mcpread.ReadJSON(filepath.Join(os.Getenv("HOME"), ".claude.json"))
	SyncFromTool(ctx, reg, "claude-code", f)

	// cursor 已有 server B，写回 A 时 B 不能丢
	writeCursorMcp(t, `{"mcpServers": {"B": {"command": "cb"}}}`)
	cursor := jsonMcp{name: "cursor", path: filepath.Join(os.Getenv("HOME"), ".cursor", "mcp.json")}
	if err := ProjectTo(reg, cursor, []string{"A"}); err != nil {
		t.Fatal(err)
	}
	got, _ := mcpread.ReadJSON(cursor.McpFile())
	if _, ok := got.Servers["B"]; !ok {
		t.Fatal("写回 A 时丢失了 cursor 原有 server B")
	}
	if _, ok := got.Servers["A"]; !ok {
		t.Fatal("A 未写入")
	}
}

func TestRemoveFromTool(t *testing.T) {
	withTempHome(t)
	reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}

	cursor := jsonMcp{name: "cursor", path: filepath.Join(os.Getenv("HOME"), ".cursor", "mcp.json")}
	writeCursorMcp(t, `{"mcpServers": {"X": {"command": "cx"}, "Y": {"command": "cy"}}}`)
	if err := RemoveFromTool(reg, cursor, "X"); err != nil {
		t.Fatal(err)
	}
	got, _ := mcpread.ReadJSON(cursor.McpFile())
	if _, ok := got.Servers["X"]; ok {
		t.Fatal("X 未删除")
	}
	if _, ok := got.Servers["Y"]; !ok {
		t.Fatal("Y 被误删")
	}
}
