// Package tui 状态可视化：bubbletea + lipgloss 交互式状态面板。
// 展示本机已检测 agent、各工具支持的类型，以及已收敛的配置清单。
package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/xmly/agentsync/internal/adapter"
	"github.com/xmly/agentsync/internal/registry"
)

// Run 启动状态 TUI（阻塞直到退出）
func Run(dir string, reg *registry.Registry) error {
	p := tea.NewProgram(newModel(dir, reg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// agentInfo 一个 agent 工具的展示信息
type agentInfo struct {
	name     string
	detected bool
	skills   bool
	rules    bool
	mcp      bool
}

type model struct {
	dir    string
	reg    *registry.Registry
	agents []agentInfo
	tab    int // 0=工具 1=配置
	off    int
	width  int
	height int
}

func newModel(dir string, reg *registry.Registry) model {
	return model{dir: dir, reg: reg, agents: detectAgents()}
}

// detectAgents 实时检测全部已注册适配器
func detectAgents() []agentInfo {
	ctx := context.Background()
	out := make([]agentInfo, 0, len(adapter.All()))
	for _, a := range adapter.All() {
		ok, _ := a.Detect(ctx)
		out = append(out, agentInfo{
			name:     a.Name(),
			detected: ok,
			skills:   a.KindSupported(registry.KindSkill),
			rules:    a.KindSupported(registry.KindRules),
			mcp:      a.KindSupported(registry.KindMCP),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l", "left", "h":
			m.tab = (m.tab + 1) % 2
			m.off = 0
		case "up", "k":
			if m.off > 0 {
				m.off--
			}
		case "down", "j":
			m.off++
		case "r":
			if reg, err := registry.Load(m.dir); err == nil {
				m.reg = reg
			}
			m.agents = detectAgents()
			m.off = 0
		}
	}
	return m, nil
}

func (m model) View() string {
	lines := m.bodyLines()
	visible := m.height - 5 // 头部 + tab + footer + 留白
	if visible < 5 {
		visible = 20
	}
	maxOff := len(lines) - visible
	if maxOff < 0 {
		maxOff = 0
	}
	off := m.off
	if off > maxOff {
		off = maxOff
	}
	end := off + visible
	if end > len(lines) {
		end = len(lines)
	}
	body := strings.Join(lines[off:end], "\n")

	return lipgloss.JoinVertical(lipgloss.Left,
		m.header(),
		m.tabs(),
		body,
		m.footer(),
	)
}

// --- 样式 ---

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	badStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	tabActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("205")).Padding(0, 1)
	tabInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1)
	badgeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
)

// --- 视图块 ---

func (m model) header() string {
	s, r, c := m.counts()
	line := titleStyle.Render("agentsync") +
		"  " + dimStyle.Render(fmt.Sprintf("已收敛 skill×%d rules×%d mcp×%d", s, r, c)) +
		"  " + dimStyle.Render("registry: "+m.reg.Path())
	return line
}

func (m model) tabs() string {
	a, b := tabInactive.Render("1 工具"), tabInactive.Render("2 配置")
	if m.tab == 0 {
		a = tabActive.Render("1 工具")
	} else {
		b = tabActive.Render("2 配置")
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, a, " ", b)
}

func (m model) footer() string {
	hint := dimStyle.Render("q 退出 · r 刷新 · tab 切换 · ↑↓ 滚动")
	return hint
}

func (m model) counts() (skill, rules, mcp int) {
	for _, it := range m.reg.Items {
		switch it.Kind {
		case registry.KindSkill:
			skill++
		case registry.KindRules:
			rules++
		case registry.KindMCP:
			mcp++
		}
	}
	return
}

// bodyLines 当前 tab 的内容行（供滚动裁剪）
func (m model) bodyLines() []string {
	if m.tab == 0 {
		return m.agentLines()
	}
	return m.itemLines()
}

func (m model) agentLines() []string {
	if len(m.agents) == 0 {
		return []string{dimStyle.Render("  （无已注册 agent 适配器）")}
	}
	lines := make([]string, 0, len(m.agents))
	for _, a := range m.agents {
		status := badStyle.Render("✗ 未检测")
		if a.detected {
			status = okStyle.Render("✓ 已检测")
		}
		// 类型徽章
		badges := []string{}
		if a.skills {
			badges = append(badges, badgeStyle.Render("skill"))
		}
		if a.rules {
			badges = append(badges, badgeStyle.Render("rules"))
		}
		if a.mcp {
			badges = append(badges, badgeStyle.Render("mcp"))
		}
		kind := dimStyle.Render("无")
		if len(badges) > 0 {
			kind = strings.Join(badges, " ")
		}
		lines = append(lines, fmt.Sprintf("  %-12s %s   支持: %s", a.name, status, kind))
	}
	return lines
}

func (m model) itemLines() []string {
	if len(m.reg.Items) == 0 {
		return []string{dimStyle.Render("  （暂无已收敛配置）")}
	}
	lines := make([]string, 0, len(m.reg.Items))
	for _, it := range m.reg.Items {
		kind := badgeStyle.Render(string(it.Kind))
		proj := "—"
		if len(it.ProjectedTo) > 0 {
			proj = strings.Join(it.ProjectedTo, ",")
		}
		lines = append(lines, fmt.Sprintf("  [%s] %-24s 来源=%-10s 分发=%s", kind, it.ID, it.Origin, proj))
	}
	return lines
}
