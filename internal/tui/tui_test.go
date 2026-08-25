package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/AdamsShen/agentsync/internal/registry"
)

func TestCounts(t *testing.T) {
	reg := &registry.Registry{
		Items: []*registry.Item{
			{ID: "skill:foo", Kind: registry.KindSkill},
			{ID: "skill:bar", Kind: registry.KindSkill},
			{ID: "rules:x", Kind: registry.KindRules},
			{ID: "mcp:y", Kind: registry.KindMCP},
		},
	}
	m := model{reg: reg}
	s, r, c := m.counts()
	if s != 2 || r != 1 || c != 1 {
		t.Fatalf("counts = (%d,%d,%d), 期望 (2,1,1)", s, r, c)
	}
}

func TestItemLines(t *testing.T) {
	reg := &registry.Registry{
		Items: []*registry.Item{
			{
				ID:          "skill:foo",
				Kind:        registry.KindSkill,
				Origin:      "cursor",
				ProjectedTo: []string{"codex", "pi"},
				CreatedAt:   time.Now(),
			},
		},
	}
	m := model{reg: reg}
	lines := m.itemLines()
	if len(lines) != 1 {
		t.Fatalf("itemLines 行数 = %d, 期望 1", len(lines))
	}
	if !strings.Contains(lines[0], "skill:foo") ||
		!strings.Contains(lines[0], "cursor") ||
		!strings.Contains(lines[0], "codex,pi") {
		t.Fatalf("itemLines 内容异常: %q", lines[0])
	}
}

func TestItemLinesEmpty(t *testing.T) {
	m := model{reg: &registry.Registry{}}
	lines := m.itemLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "暂无") {
		t.Fatalf("空 registry 应显示占位，得到 %v", lines)
	}
}

func TestAgentLinesEmpty(t *testing.T) {
	m := model{reg: &registry.Registry{}, agents: []agentInfo{}}
	lines := m.agentLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "无") {
		t.Fatalf("空 agents 应显示占位，得到 %v", lines)
	}
}
