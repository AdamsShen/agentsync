package mcpread

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadJSON_EmptyFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".cursor", "mcp.json")
	f, err := ReadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Servers) != 0 {
		t.Fatal("应返回空 servers")
	}
}

func TestReadJSON_ParsesServers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	writeFile(t, p, `{
	  "mcpServers": {
	    "github": { "command": "npx", "args": ["-y", "@x/server-github"], "env": {"TOKEN": "x"} },
	    "xima": { "url": "http://sse.local/sse" }
	  }
	}`)
	f, err := ReadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Servers) != 2 {
		t.Fatalf("期望 2 个 server，得 %d", len(f.Servers))
	}
	if f.Servers["github"]["command"] != "npx" {
		t.Fatal("github 解析错误")
	}
	if f.Servers["xima"]["url"] == "" {
		t.Fatal("xima url server 解析错误")
	}
}

func TestWriteJSON_PreservesExtraAndMerges(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	writeFile(t, p, `{"mcpServers": {"a": {"command": "ca", "args": ["a1"]}}, "other": {"k": 1}}`)

	f, err := ReadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	f.Servers["b"] = map[string]any{"command": "cb", "args": []any{"b1"}}
	if err := f.WriteJSON(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{`"other"`, `"b"`, `"a"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("写回后应包含 %s:\n%s", want, s)
		}
	}
	f2, err := ReadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f2.Servers["b"]; !ok {
		t.Fatal("b 未写回")
	}
	if _, ok := f2.Extra["other"]; !ok {
		t.Fatal("extra 丢失")
	}
}

func TestReadToml_ParsesCodexServers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	writeFile(t, p, `[mcp_servers]
[mcp_servers.memory]
type = "stdio"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-memory"]
startup_timeout_sec = 120

[mcp_servers.memory.env]
FOO = "bar"

[plugins."x"]
enabled = true
`)
	f, err := Read(p, FormatTOML, "mcp_servers")
	if err != nil {
		t.Fatal(err)
	}
	srv := f.Servers["memory"]
	if srv == nil {
		t.Fatal("memory server 未解析")
	}
	if srv["command"] != "npx" || srv["startup_timeout_sec"] != int64(120) {
		t.Fatalf("codex server 字段解析错误: %#v", srv)
	}
	// 嵌套 env 表解析成 map
	env, ok := srv["env"].(map[string]any)
	if !ok || env["FOO"] != "bar" {
		t.Fatalf("env 嵌套表解析错误: %#v", srv["env"])
	}
	// 其他顶层键保留进 Extra
	if _, ok := f.Extra["plugins"]; !ok {
		t.Fatal("plugins 顶层键丢失")
	}
}

func TestWriteToml_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	writeFile(t, p, `[mcp_servers.memory]
command = "npx"
args = ["-y", "x"]

[other]
k = 1
`)
	f, err := Read(p, FormatTOML, "mcp_servers")
	if err != nil {
		t.Fatal(err)
	}
	f.Servers["new"] = map[string]any{"command": "uv", "args": []any{"run", "x.py"}}
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	f2, err := Read(p, FormatTOML, "mcp_servers")
	if err != nil {
		t.Fatal(err)
	}
	if f2.Servers["new"]["command"] != "uv" {
		t.Fatal("new server 未写回")
	}
	if f2.Servers["memory"] == nil {
		t.Fatal("memory server 丢失")
	}
	if _, ok := f2.Extra["other"]; !ok {
		t.Fatal("other 顶层键丢失")
	}
}

func TestReadYaml_ParsesServers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	writeFile(t, p, "mcp_servers:\n  db:\n    command: uv\n    args: [run, db.py]\nother:\n  x: 1\n")
	f, err := Read(p, FormatYAML, "mcp_servers")
	if err != nil {
		t.Fatal(err)
	}
	if f.Servers["db"]["command"] != "uv" {
		t.Fatalf("yaml server 解析错误: %#v", f.Servers)
	}
	if _, ok := f.Extra["other"]; !ok {
		t.Fatal("yaml other 顶层键丢失")
	}
}

func TestMergeFrom(t *testing.T) {
	src := &File{Servers: map[string]map[string]any{"gh": {"command": "npx"}}}
	dst := &File{Servers: map[string]map[string]any{}}
	dst.MergeFrom(src, []string{"gh"})
	if _, ok := dst.Servers["gh"]; !ok {
		t.Fatal("merge 失败")
	}
}

func TestReadJSON_PreservesIntType(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	writeFile(t, p, `{"mcpServers": {"x": {"command": "echo", "timeout": 120, "ratio": 1.5}}}`)
	f, err := Read(p, FormatJSON, "mcpServers")
	if err != nil {
		t.Fatal(err)
	}
	srv := f.Servers["x"]
	// 整数保持 int64，不是 float64（避免跨格式写回 120 → 120.0）
	if v, ok := srv["timeout"].(int64); !ok || v != 120 {
		t.Fatalf("timeout 应为 int64(120)，实际 %T %v", srv["timeout"], srv["timeout"])
	}
	// 小数保持 float64
	if v, ok := srv["ratio"].(float64); !ok || v != 1.5 {
		t.Fatalf("ratio 应为 float64(1.5)，实际 %T %v", srv["ratio"], srv["ratio"])
	}
}

func TestWriteToml_PreservesIntNotFloat(t *testing.T) {
	dir := t.TempDir()
	// 模拟 JSON 源分发到 TOML 工具：读 JSON 后写 TOML
	jsonP := filepath.Join(dir, "mcp.json")
	writeFile(t, jsonP, `{"mcpServers": {"x": {"command": "echo", "timeout": 120}}}`)
	f, err := Read(jsonP, FormatJSON, "mcpServers")
	if err != nil {
		t.Fatal(err)
	}
	// 改为 TOML 目标写回
	tomlP := filepath.Join(dir, "config.toml")
	f.Path = tomlP
	f.format = FormatTOML
	f.key = "mcp_servers"
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(tomlP)
	out := string(data)
	if strings.Contains(out, "120.0") {
		t.Fatalf("TOML 写回不应出现 120.0（整数应保持 int）:\n%s", out)
	}
	if !strings.Contains(out, "timeout = 120") {
		t.Fatalf("TOML 写回应含 timeout = 120:\n%s", out)
	}
}
