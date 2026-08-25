package mcpsync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AdamsShen/agentsync/internal/mcpread"
	"github.com/AdamsShen/agentsync/internal/registry"
)

// withTempHome 用临时 HOME 隔离（各工具 MCP 路径走 os.UserHomeDir）
func withTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, d := range []string{".cursor", ".qoder", ".codex"} {
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

func jsonMcp(name, path string) McpAdapter {
	return mcpCfg{name: name, path: path, format: mcpread.FormatJSON, key: "mcpServers"}
}

func TestSyncFromTool_AddsToRegistry(t *testing.T) {
	withTempHome(t)
	reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
	ctx := context.Background()

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

	writeClaudeMcp(t, `{"mcpServers": {"db": {"command": "uv", "args": ["run", "db.py"], "env": {"DB": "x"}}}}`)
	f, err := mcpread.ReadJSON(filepath.Join(os.Getenv("HOME"), ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SyncFromTool(ctx, reg, "claude-code", f); err != nil {
		t.Fatal(err)
	}

	cursor := jsonMcp("cursor", filepath.Join(os.Getenv("HOME"), ".cursor", "mcp.json"))
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
	if got.Servers["db"]["env"].(map[string]any)["DB"] != "x" {
		t.Fatal("env 未同步")
	}
}

func TestProjectTo_PreservesExistingServers(t *testing.T) {
	withTempHome(t)
	reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
	ctx := context.Background()

	writeClaudeMcp(t, `{"mcpServers": {"A": {"command": "ca"}}}`)
	f, _ := mcpread.ReadJSON(filepath.Join(os.Getenv("HOME"), ".claude.json"))
	SyncFromTool(ctx, reg, "claude-code", f)

	writeCursorMcp(t, `{"mcpServers": {"B": {"command": "cb"}}}`)
	cursor := jsonMcp("cursor", filepath.Join(os.Getenv("HOME"), ".cursor", "mcp.json"))
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

	cursor := jsonMcp("cursor", filepath.Join(os.Getenv("HOME"), ".cursor", "mcp.json"))
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

// TestProjectTo_CodexToml 验证 TOML 工具写回（codex config.toml 保留其他顶层键）
func TestProjectTo_CodexToml(t *testing.T) {
	withTempHome(t)
	reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
	ctx := context.Background()

	writeClaudeMcp(t, `{"mcpServers": {"db": {"command": "uv", "args": ["run", "db.py"]}}}`)
	f, _ := mcpread.ReadJSON(filepath.Join(os.Getenv("HOME"), ".claude.json"))
	SyncFromTool(ctx, reg, "claude-code", f)

	// codex 已有一个自己的 server + 其他顶层键
	codexFile := filepath.Join(os.Getenv("HOME"), ".codex", "config.toml")
	os.WriteFile(codexFile, []byte("[mcp_servers.memory]\ncommand = \"npx\"\n\n[model]\nprovider = \"openai\"\n"), 0o600)

	codex, _ := AdapterByName("codex")
	if err := ProjectTo(reg, codex, []string{"db"}); err != nil {
		t.Fatal(err)
	}
	got, err := mcpread.Read(codex.McpFile(), codex.Format(), codex.ServersKey())
	if err != nil {
		t.Fatal(err)
	}
	if got.Servers["db"] == nil {
		t.Fatal("db 未写入 codex")
	}
	if got.Servers["memory"] == nil {
		t.Fatal("codex 原有 memory 丢失")
	}
	if _, ok := got.Extra["model"]; !ok {
		t.Fatal("codex 其他顶层键 model 丢失")
	}
}
