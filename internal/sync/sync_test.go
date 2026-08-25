package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AdamsShen/agentsync/internal/adapter"
	"github.com/AdamsShen/agentsync/internal/registry"
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

func TestIngestRuleFile_ToolPrefixName(t *testing.T) {
	withTempHome(t, func(home string) {
		reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
		ctx := context.Background()

		// 建 codex 的单文件规则 ~/.codex/AGENTS.md
		src := filepath.Join(home, ".codex", "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(src, []byte("# codex rules\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		it, err := IngestRuleFile(ctx, reg, adapter.Codex{}, src)
		if err != nil {
			t.Fatalf("IngestRuleFile 失败: %v", err)
		}

		// canonical 名带 tool 前缀，避免与 hermes SOUL.md 等冲突
		if filepath.Base(it.Canonical) != "codex-AGENTS.md" {
			t.Fatalf("canonical 名错误: %s", filepath.Base(it.Canonical))
		}
		if it.ID != "rules:codex-AGENTS.md" {
			t.Fatalf("ID 错误: %s", it.ID)
		}
		// canonical 是实体副本，非软链
		if fi, _ := os.Lstat(it.Canonical); fi.Mode()&os.ModeSymlink != 0 {
			t.Fatal("canonical 不应是软链")
		}
		// 源单文件规则仍是实体（不软链源文件）
		if fi, _ := os.Lstat(src); fi.Mode()&os.ModeSymlink != 0 {
			t.Fatal("源单文件规则不应被软链")
		}
	})
}

func TestIngestRuleFile_NoNameCollision(t *testing.T) {
	withTempHome(t, func(home string) {
		reg := &registry.Registry{Items: []*registry.Item{}, Tools: map[string]registry.ToolState{}}
		ctx := context.Background()

		// codex AGENTS.md 与 hermes SOUL.md 同为单文件规则，收敛后名字不冲突
		codexSrc := filepath.Join(home, ".codex", "AGENTS.md")
		os.MkdirAll(filepath.Dir(codexSrc), 0o755)
		os.WriteFile(codexSrc, []byte("codex rules"), 0o644)

		hermesSrc := filepath.Join(home, ".hermes", "SOUL.md")
		os.MkdirAll(filepath.Dir(hermesSrc), 0o755)
		os.WriteFile(hermesSrc, []byte("hermes soul"), 0o644)

		it1, err := IngestRuleFile(ctx, reg, adapter.Codex{}, codexSrc)
		if err != nil {
			t.Fatal(err)
		}
		it2, err := IngestRuleFile(ctx, reg, adapter.Hermes{}, hermesSrc)
		if err != nil {
			t.Fatal(err)
		}

		if it1.ID == it2.ID {
			t.Fatal("不同工具的单文件规则 ID 不应相同")
		}
		if it1.ID != "rules:codex-AGENTS.md" || it2.ID != "rules:hermes-SOUL.md" {
			t.Fatalf("ID 错误: %s / %s", it1.ID, it2.ID)
		}
	})
}
