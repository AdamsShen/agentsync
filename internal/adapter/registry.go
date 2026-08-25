package adapter

import (
	"context"
	"sort"
)

// all 已注册的所有适配器（按名字排序）
var all = []Adapter{
	ClaudeCode{},
	Codex{},
	Cursor{},
	Gemini{},
	Grok{},
	Hermes{},
	OpenCode{},
	Pi{},
	Qoder{},
}

// All 返回全部适配器
func All() []Adapter { return all }

// Registry 提供"检测 + 过滤 + 缓存"能力
type Registry struct {
	detected map[string]bool // 工具名 -> 是否检测到
}

// NewRegistry 创建检测注册表
func NewRegistry() *Registry {
	return &Registry{detected: map[string]bool{}}
}

// DetectAll 检测所有适配器，返回"已检测到"的适配器集合（含缓存）
func (r *Registry) DetectAll(ctx context.Context) []Adapter {
	got := []Adapter{}
	for _, a := range all {
		if ok, _ := a.Detect(ctx); ok {
			r.detected[a.Name()] = true
			got = append(got, a)
		}
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name() < got[j].Name() })
	return got
}

// Detected 返回是否已检测到某工具（上次 DetectAll 的结果）
func (r *Registry) Detected(name string) bool { return r.detected[name] }

// SupportedFor 过滤：支持某 kind 的已检测工具
func SupportedFor(ctx context.Context, kind Kind) []Adapter {
	out := []Adapter{}
	for _, a := range all {
		if ok, _ := a.Detect(ctx); ok && a.KindSupported(kind) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
