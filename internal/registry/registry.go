package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Kind 配置类型
type Kind string

const (
	KindSkill Kind = "skill"
	KindMCP   Kind = "mcp"
	KindRules Kind = "rules"
)

// Item 一条已收敛配置的记录（registry 中的事实源）
type Item struct {
	ID          string            `json:"id"`                      // 形如 "skill:foo"
	Kind        Kind              `json:"kind"`                    // skill | mcp | rules
	Canonical   string            `json:"canonical,omitempty"`     // 统一副本绝对路径（skill/rules）
	Origin      string            `json:"origin"`                  // 来自哪个工具
	ProjectedTo []string          `json:"projected_to,omitempty"`  // 分发到了哪些工具
	Config      map[string]any    `json:"config,omitempty"`        // MCP 等结构化定义
	LastHash    string            `json:"last_hash,omitempty"`     // 出站写入后的内容指纹（防循环）
	Ignore      map[string]bool   `json:"ignore,omitempty"`        // 按工具忽略（key=工具名）
	CreatedAt   time.Time         `json:"created_at"`
	Meta        map[string]string `json:"meta,omitempty"`
}

// Secrets 敏感凭据统一存放（0600）
type Secrets struct {
	MCP map[string]map[string]string `json:"mcp,omitempty"` // server名 -> header键值
}

// ToolState 工具检测状态
type ToolState struct {
	Detected bool `json:"detected"`
	Enabled  bool `json:"enabled"`
}

// Config 全局默认策略
type Config struct {
	AskOnConflict      bool   `json:"ask_on_conflict"`
	AskOnExternal      bool   `json:"ask_on_external_change"`
	AskOnPropagate     bool   `json:"ask_on_propagate"`
	DebounceMS         int    `json:"debounce_ms"`
	NoTTYPolicy        string `json:"no_tty_policy"` // log | ignore | apply_default
	RescanIntervalSec  int    `json:"rescan_interval_sec"`
}

// Registry 状态库
type Registry struct {
	Version  int                  `json:"version"`
	Defaults Config               `json:"defaults"`
	Tools    map[string]ToolState `json:"tools"`
	Secrets  Secrets              `json:"secrets,omitempty"`
	Items    []*Item              `json:"items"`
	path     string               // 文件路径（不序列化）
}

// Load 读取 registry；不存在则返回空 registry（不落盘）
func Load(dir string) (*Registry, error) {
	r := &Registry{
		Version:  3,
		Defaults: defaultConfig(),
		Tools:    map[string]ToolState{},
		Secrets:  Secrets{MCP: map[string]map[string]string{}},
		Items:    []*Item{},
	}
	r.path = filepath.Join(dir, "registry.json")
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil // 首次使用，空状态
		}
		return nil, err
	}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("解析 registry %s 失败: %w", r.path, err)
	}
	r.path = filepath.Join(dir, "registry.json")
	return r, nil
}

func defaultConfig() Config {
	return Config{
		AskOnConflict:     true,
		AskOnExternal:     true,
		AskOnPropagate:    true,
		DebounceMS:        2000,
		NoTTYPolicy:       "log",
		RescanIntervalSec: 300,
	}
}

// Save 落盘（0600，因含凭据）
func (r *Registry) Save() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0o600)
}

// Path 返回 registry 文件路径
func (r *Registry) Path() string { return r.path }

// GetItem 按 ID 查 item
func (r *Registry) GetItem(id string) *Item {
	for _, it := range r.Items {
		if it.ID == id {
			return it
		}
	}
	return nil
}

// UpsertItem 新增或更新
func (r *Registry) UpsertItem(it *Item) {
	for i, e := range r.Items {
		if e.ID == it.ID {
			r.Items[i] = it
			return
		}
	}
	r.Items = append(r.Items, it)
}

// RemoveItem 删除
func (r *Registry) RemoveItem(id string) {
	out := r.Items[:0]
	for _, it := range r.Items {
		if it.ID != id {
			out = append(out, it)
		}
	}
	r.Items = out
}

// IsOwnedSymlink 判断 path 是否是我方投影（软链且指向 canonical）
func (it *Item) IsOwnedSymlink(p string) bool {
	if it.Canonical == "" {
		return false
	}
	target, err := os.Readlink(p)
	if err != nil {
		return false
	}
	return samePath(target, it.Canonical)
}

func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 == nil && err2 == nil {
		return filepath.Clean(ra) == filepath.Clean(rb)
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
