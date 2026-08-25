// Package daemon 守护进程核心循环：实现 watch.Handler，串起 收敛→询问→分发。
package daemon

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/xmly/agentsync/internal/adapter"
	"github.com/xmly/agentsync/internal/ask"
	"github.com/xmly/agentsync/internal/mcpread"
	"github.com/xmly/agentsync/internal/mcpsync"
	"github.com/xmly/agentsync/internal/registry"
	"github.com/xmly/agentsync/internal/sync"
	"github.com/xmly/agentsync/internal/watch"
)

// debounce 事件合并窗口
const debounce = 2 * time.Second

// Handler 实现 watch.Handler
type Handler struct {
	Reg      *registry.Registry
	Adapters []adapter.Adapter // 已检测适配器（动态更新）
	W        *watch.Watcher    // 监听器（动态接入新 agent 用）

	mcpBaseline map[string]bool // 「无 adopt」基线：启动时既有的 MCP server 名
}

// SetWatcher 注入监听器（供动态接入）
func (h *Handler) SetWatcher(w *watch.Watcher) { h.W = w }

// OnSkill 工具目录出现新 skill：收敛 → 原副本换软链 → 询问 → 分发
func (h *Handler) OnSkill(ctx context.Context, a adapter.Adapter, dir string) error {
	log.Printf("[daemon] 检测到新 skill: %s (来源: %s)", dir, a.Name())

	// 1. 收敛到 canonical
	it, err := sync.IngestSkill(ctx, h.Reg, a, dir)
	if err != nil {
		return err
	}
	log.Printf("[daemon] 已收敛到 %s", it.Canonical)

	// 2. 原工具副本替换为软链（决策 9）
	if err := sync.ReplaceWithSymlink(ctx, a, dir, it); err != nil {
		log.Printf("[daemon] 替换源副本为软链失败: %v", err)
	}

	// 3. 询问分发目标（只列已检测且支持 skill 的工具；源工具已在步骤2处理）
	candidates := []string{}
	for _, ad := range h.Adapters {
		if ad.Name() == a.Name() {
			continue
		}
		if ad.KindSupported(registry.KindSkill) {
			candidates = append(candidates, ad.Name())
		}
	}
	targets, err := ask.MultiSelect(
		"检测到以下工具支持 skill，同步到哪些？（空回车=全部）",
		candidates, candidates)
	if err != nil {
		return err
	}

	// 4. 分发软链到选中的工具
	for _, ad := range h.Adapters {
		for _, t := range targets {
			if ad.Name() == t {
				if err := sync.ProjectSkill(ctx, ad, it); err != nil {
					log.Printf("[daemon] 分发到 %s 失败: %v", ad.Name(), err)
				} else {
					log.Printf("[daemon] 已分发到 %s", ad.Name())
				}
			}
		}
	}
	it.ProjectedTo = targets
	h.Reg.UpsertItem(it)
	_ = h.Reg.Save()
	return nil
}

// OnRules 工具 rules 目录出现新 rule 文件：收敛 → 原副本换软链 → 询问 → 分发
func (h *Handler) OnRules(ctx context.Context, a adapter.Adapter, file string) error {
	log.Printf("[daemon] 检测到新 rule: %s (来源: %s)", file, a.Name())

	it, err := sync.IngestRule(ctx, h.Reg, a, file)
	if err != nil {
		return err
	}
	log.Printf("[daemon] 已收敛 rule 到 %s", it.Canonical)

	// 原工具副本替换为软链（目录式规则软链源文件）
	if err := sync.ReplaceWithSymlink(ctx, a, file, it); err != nil {
		log.Printf("[daemon] 替换源 rule 为软链失败: %v", err)
	}

	return h.projectRuleToDirs(ctx, a, it)
}

// OnRuleFile 工具单文件规则（AGENTS.md/SOUL.md 等）收敛：只复制进 canonical，
// 不软链源文件；分发只到有独立 rules 目录的工具（只收敛，不分发回单文件工具）。
func (h *Handler) OnRuleFile(ctx context.Context, a adapter.Adapter, file string) error {
	log.Printf("[daemon] 检测到单文件 rule: %s (来源: %s)", file, a.Name())

	it, err := sync.IngestRuleFile(ctx, h.Reg, a, file)
	if err != nil {
		return err
	}
	log.Printf("[daemon] 已收敛单文件 rule 到 %s", it.Canonical)

	return h.projectRuleToDirs(ctx, a, it)
}

// projectRuleToDirs 询问后把 rule item 分发到有独立 rules 目录的工具（源工具除外）。
// 单文件规则工具（codex/hermes/gemini/opencode）RulesDir 为空，天然不参与分发。
func (h *Handler) projectRuleToDirs(ctx context.Context, a adapter.Adapter, it *registry.Item) error {
	// 询问分发目标（只列已检测、支持 rules 且声明了 rules 目录的工具）
	candidates := []string{}
	for _, ad := range h.Adapters {
		if ad.Name() == a.Name() {
			continue
		}
		if ad.RulesDir() != "" && ad.KindSupported(registry.KindRules) {
			candidates = append(candidates, ad.Name())
		}
	}
	it.ProjectedTo = []string{}
	if len(candidates) > 0 {
		targets, err := ask.MultiSelect(
			"检测到以下工具支持 rules，同步到哪些？（空回车=全部）",
			candidates, candidates)
		if err != nil {
			return err
		}
		for _, ad := range h.Adapters {
			for _, t := range targets {
				if ad.Name() == t {
					if err := sync.ProjectRule(ctx, ad, it); err != nil {
						log.Printf("[daemon] 分发 rule 到 %s 失败: %v", ad.Name(), err)
					} else {
						log.Printf("[daemon] 已分发 rule 到 %s", ad.Name())
					}
				}
			}
		}
		it.ProjectedTo = targets
	}
	h.Reg.UpsertItem(it)
	_ = h.Reg.Save()
	return nil
}

// buildMcpBaseline 建立 MCP 的「无 adopt」基线：记录启动时各工具既有的 MCP server 名。
// 之后 OnMcpChange 跳过这些既有 server，只收敛启动后新增的。
func (h *Handler) buildMcpBaseline() {
	h.mcpBaseline = map[string]bool{}
	for _, ma := range mcpsync.Adapters() {
		f, err := mcpread.Read(ma.McpFile(), ma.Format(), ma.ServersKey())
		if err != nil {
			continue
		}
		for name := range f.Servers {
			h.mcpBaseline[name] = true
		}
	}
}

// OnMcpChange 工具 MCP 配置文件变化：读文件 → 对比 registry → 收敛 → 询问 → 分发
func (h *Handler) OnMcpChange(ctx context.Context, a adapter.Adapter) error {
	ma, ok := mcpsync.AdapterByName(a.Name())
	if !ok {
		return nil // 该工具无 MCP 适配器
	}
	f, err := mcpread.Read(ma.McpFile(), ma.Format(), ma.ServersKey())
	if err != nil {
		return err
	}
	// 无 adopt：移除启动前既有且尚未收敛的 server，只收敛启动后新增/变更的
	for name := range h.mcpBaseline {
		if h.Reg.GetItem("mcp:"+name) == nil {
			delete(f.Servers, name)
		}
	}
	diff := mcpsync.DetectDiff(f, h.Reg, a.Name())

	if len(diff.Removed) > 0 {
		// TODO(MCP 删除-询问流)：外部删除 server 目前只记录日志，不反向询问分发删除。
		// 原因：删除是破坏性操作，需先设计「询问-确认-分发删除」流程（防误删），暂缓实现。
		log.Printf("[daemon] MCP 外部删除 %s: %v（待后续处理）", a.Name(), diff.Removed)
	}
	if len(diff.Added) == 0 && len(diff.Changed) == 0 {
		return nil
	}

	// 收敛新增/变更 server 进 registry（Origin=来源工具）
	synced, err := mcpsync.SyncFromTool(ctx, h.Reg, a.Name(), f)
	if err != nil {
		return err
	}
	if len(synced) == 0 {
		return nil
	}
	log.Printf("[daemon] MCP 收敛 %d 个 server: %v（来源: %s）", len(synced), synced, a.Name())

	// 询问分发目标（其他支持 MCP 且具备配置适配器的工具）
	candidates := []string{}
	for _, ad := range h.Adapters {
		if ad.Name() == a.Name() || !ad.KindSupported(registry.KindMCP) {
			continue
		}
		if _, ok := mcpsync.AdapterByName(ad.Name()); ok {
			candidates = append(candidates, ad.Name())
		}
	}
	if len(candidates) > 0 {
		targets, err := ask.MultiSelect(
			"检测到以下工具支持 MCP，同步到哪些？（空回车=全部）",
			candidates, candidates)
		if err != nil {
			return err
		}
		for _, t := range targets {
			ma2, _ := mcpsync.AdapterByName(t)
			if err := mcpsync.ProjectTo(h.Reg, ma2, synced); err != nil {
				log.Printf("[daemon] MCP 分发到 %s 失败: %v", t, err)
			} else {
				log.Printf("[daemon] MCP 已分发到 %s", t)
			}
		}
		// 记录 projected_to
		for _, name := range synced {
			if it := h.Reg.GetItem("mcp:" + name); it != nil {
				it.ProjectedTo = targets
				h.Reg.UpsertItem(it)
			}
		}
	}
	_ = h.Reg.Save()
	return nil
}

// OnRescan 周期重扫：检测新 agent 并加入监听
func (h *Handler) OnRescan(ctx context.Context) error {
	adReg := adapter.NewRegistry()
	detected := adReg.DetectAll(ctx)
	// 找出新出现的工具
	have := map[string]bool{}
	for _, a := range h.Adapters {
		have[a.Name()] = true
	}
	for _, a := range detected {
		if !have[a.Name()] {
			log.Printf("[daemon] 检测到新 agent: %s，加入监听", a.Name())
			h.Adapters = append(h.Adapters, a)
			if h.W != nil {
				h.W.AddAdapter(a)
			}
		}
	}
	// 记录检测状态进 registry
	for _, a := range detected {
		h.Reg.Tools[a.Name()] = registry.ToolState{Detected: true, Enabled: true}
	}
	_ = h.Reg.Save()
	return nil
}

// EnsureCanonical 确保 canonical 目录存在
func EnsureCanonical() error {
	for _, d := range []string{sync.SkillsRoot(), sync.RulesRoot()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// Run 启动守护进程（阻塞）
func Run(ctx context.Context, reg *registry.Registry) error {
	if err := EnsureCanonical(); err != nil {
		return err
	}
	// 检测本机已装 agent
	adReg := adapter.NewRegistry()
	detected := adReg.DetectAll(ctx)
	log.Printf("[daemon] 检测到 %d 个 agent: %v", len(detected), adapterNames(detected))

	h := &Handler{Reg: reg, Adapters: detected}
	h.buildMcpBaseline() // 建立 MCP 无 adopt 基线
	w := watch.New(reg, detected, h, debounce)
	h.SetWatcher(w)
	return w.Run(ctx)
}

// adapterNames 返回适配器名列表
func adapterNames(as []adapter.Adapter) []string {
	names := make([]string, 0, len(as))
	for _, a := range as {
		names = append(names, a.Name())
	}
	return names
}
