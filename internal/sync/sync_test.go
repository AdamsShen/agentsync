package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xmly/agentsync/internal/adapter"
	"github.com/xmly/agentsync/internal/registry"
)

// 用临时 HOME 做隔离测试（适配器走 os.UserHomeDir）
func withTempHome(t *testing.T, fn func(home string)) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// 建各工具 skills 目录
	for _, d := range []string{".claude/skills", ".cursor/skills", ".qoder/skills"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fn(home)
}

// makeSkill 在工具目录建一个 skill
func makeSkill(t *testing.T, home, tool, name string) string {
	t.Helper()
	dir := filepath.Join(home, "."+tool, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: test\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestIngestSkill_ConvergesAndRecords(t *testing.T) {
	withTempHome(t, func(home string) {
		reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
		ctx := context.Background()

		dir := makeSkill(t, home, "cursor", "foo")
		it, err := IngestSkill(ctx, reg, adapter.Cursor{}, dir)
		if err != nil {
			t.Fatalf("IngestSkill 失败: %v", err)
		}

		// canonical 存在
		if _, err := os.Stat(it.Canonical); err != nil {
			t.Fatalf("canonical 不存在: %v", err)
		}
		if _, err := os.Stat(filepath.Join(it.Canonical, "SKILL.md")); err != nil {
			t.Fatalf("canonical 缺 SKILL.md")
		}
		// origin 记录正确
		if it.Origin != "cursor" || it.Kind != registry.KindSkill {
			t.Fatalf("item 元信息错误: %+v", it)
		}
		// 原目录仍是实体（ReplaceWithSymlink 后才换软链）
		fi, _ := os.Lstat(dir)
		if fi.Mode()&os.ModeSymlink != 0 {
			t.Fatal("原目录不应是软链")
		}
	})
}

func TestReplaceWithSymlink_AndProject(t *testing.T) {
	withTempHome(t, func(home string) {
		reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
		ctx := context.Background()

		dir := makeSkill(t, home, "cursor", "bar")
		it, _ := IngestSkill(ctx, reg, adapter.Cursor{}, dir)

		// 原副本换软链
		if err := ReplaceWithSymlink(ctx, adapter.Cursor{}, dir, it); err != nil {
			t.Fatalf("ReplaceWithSymlink: %v", err)
		}
		if owned, _ := (adapter.Cursor{}).IsOwnedProjection(ctx, dir); !owned {
			t.Fatal("原目录应已替换为我方软链")
		}

		// 分发到 claude-code
		if err := ProjectSkill(ctx, adapter.ClaudeCode{}, it); err != nil {
			t.Fatalf("ProjectSkill: %v", err)
		}
		cl := filepath.Join(home, ".claude", "skills", "bar")
		if owned, _ := (adapter.ClaudeCode{}).IsOwnedProjection(ctx, cl); !owned {
			t.Fatal("claude 目录应有我方软链")
		}
	})
}

func TestIngestSkill_DedupNoOverwrite(t *testing.T) {
	withTempHome(t, func(home string) {
		reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
		ctx := context.Background()

		// 先在 cursor 装 foo
		dir1 := makeSkill(t, home, "cursor", "dup")
		if _, err := IngestSkill(ctx, reg, adapter.Cursor{}, dir1); err != nil {
			t.Fatal(err)
		}
		// 再在 qoder 出现同名（未收敛 → 视为新？registry 有记录则跳过收敛）
		dir2 := makeSkill(t, home, "qoder", "dup")
		if reg.GetItem("skill:dup") != nil {
			// registry 已有 → 上游(daemon)会跳过；这里验证 IngestSkill 幂等替换为同一 canonical
			it2, err := IngestSkill(ctx, reg, adapter.Qoder{}, dir2)
			if err != nil {
				t.Fatal(err)
			}
			if it2.ID != "skill:dup" {
				t.Fatal("ID 应复用")
			}
			_ = dir2
		}
	})
}
