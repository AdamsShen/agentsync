package mcpread

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeJSON(t *testing.T, path string, content string) {
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
	// 不存在 → 空配置
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
	writeJSON(t, p, `{
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
	if f.Servers["github"].Command != "npx" {
		t.Fatal("github 解析错误")
	}
	if f.Servers["xima"].URL == "" || f.Servers["xima"].Type == "stdio" {
		t.Fatal("xima url server 解析错误")
	}
}

func TestWriteJSON_PreservesExtraAndMerges(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	writeJSON(t, p, `{"mcpServers": {"a": {"command": "ca", "args": ["a1"]}}, "other": {"k": 1}}`)

	f, err := ReadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	// 添加 server b
	f.Servers["b"] = Server{Command: "cb", Args: []string{"b1"}}
	if err := f.WriteJSON(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, `"other"`) || !strings.Contains(s, `"b"`) {
		t.Fatalf("写回后应保留 other 且包含 b:\n%s", s)
	}
	if !strings.Contains(s, `"a"`) {
		t.Fatalf("原 server a 丢失:\n%s", s)
	}
	// 重新解析验证合法
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

func TestMergeFrom(t *testing.T) {
	src := &File{Servers: map[string]Server{"gh": {Command: "npx"}}}
	dst := &File{Servers: map[string]Server{}}
	dst.MergeFrom(src, []string{"gh"})
	if _, ok := dst.Servers["gh"]; !ok {
		t.Fatal("merge 失败")
	}
}

var _ = json.Marshal // 占位
