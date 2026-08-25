package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xmly/agentsync/internal/adapter"
	"github.com/xmly/agentsync/internal/registry"
)

// mockHandler 记录 OnRuleFile 回调的 Handler 实现
type mockHandler struct {
	ruleFileCalls []string
	skillCalls    []string
}

func (m *mockHandler) OnSkill(_ context.Context, _ adapter.Adapter, dir string) error {
	m.skillCalls = append(m.skillCalls, dir)
	return nil
}
func (m *mockHandler) OnRules(context.Context, adapter.Adapter, string) error { return nil }
func (m *mockHandler) OnRuleFile(_ context.Context, _ adapter.Adapter, file string) error {
	m.ruleFileCalls = append(m.ruleFileCalls, file)
	return nil
}
func (m *mockHandler) OnMcpChange(context.Context, adapter.Adapter) error { return nil }
func (m *mockHandler) OnRescan(context.Context) error                     { return nil }

func TestScanRuleFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
	h := &mockHandler{}
	w := &Watcher{reg: reg, handler: h}
	ctx := context.Background()

	// 未收敛 → 回调一次
	w.scanRuleFile(ctx, adapter.Codex{}, file)
	if len(h.ruleFileCalls) != 1 {
		t.Fatalf("未收敛应回调 1 次，实际 %d", len(h.ruleFileCalls))
	}

	// 已收敛（registry 有 codex-AGENTS.md）→ 不再回调
	reg.UpsertItem(&registry.Item{ID: "rules:codex-AGENTS.md", Kind: registry.KindRules})
	w.scanRuleFile(ctx, adapter.Codex{}, file)
	if len(h.ruleFileCalls) != 1 {
		t.Fatalf("已收敛不应再回调，实际 %d", len(h.ruleFileCalls))
	}
}

func TestScanRuleFile_Missing(t *testing.T) {
	reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
	h := &mockHandler{}
	w := &Watcher{reg: reg, handler: h}
	w.scanRuleFile(context.Background(), adapter.Codex{}, filepath.Join(t.TempDir(), "AGENTS.md"))
	if len(h.ruleFileCalls) != 0 {
		t.Fatalf("文件不存在不应回调，实际 %d", len(h.ruleFileCalls))
	}
}

// TestDispatchInitialScanSkill 验证启动初始扫描：dispatch 对已有实体 skill 触发收敛。
func TestDispatchInitialScanSkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "foo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: foo\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
	h := &mockHandler{}
	w := &Watcher{
		reg:     reg,
		handler: h,
		entries: map[string]entry{
			dir: {a: adapter.ClaudeCode{}, spec: adapter.WatchSpec{Path: dir, Kind: registry.KindSkill, Tool: "claude-code", Recurse: true}},
		},
	}

	w.dispatch(context.Background(), nil, dir)
	if len(h.skillCalls) != 1 {
		t.Fatalf("初始扫描应收敛 1 个 skill，实际 %d", len(h.skillCalls))
	}
	if h.skillCalls[0] != skillDir {
		t.Fatalf("收敛路径错误: %s", h.skillCalls[0])
	}
}
