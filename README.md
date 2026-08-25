# agentsync

跨 Agent 工具配置自动同步工具（Go 实现，CLI + 后台守护进程）。

**核心模型：监听-收敛-分发。**

- 用户在任何已安装的 agent 工具（Claude Code / Cursor / Qoder / Hermes …）里安装 skill
- 守护进程监听各工具 skills 目录 → 收敛到统一副本 `~/.agents/skills/`
- 原副本替换为软链指向统一副本 → 询问分发到哪些工具 → 其他工具目录建软链
- 之后在任一工具里修改，因软链穿透，全部工具即时一致

## 快速开始

```bash
# 构建
go build -o agentsync .

# 启动守护进程（前台，调试用）
./agentsync daemon

# 状态/列表
./agentsync status
./agentsync list
```

## 目录结构

```
~/workspace/agentsync
├── main.go                     # CLI 入口
├── internal/
│   ├── adapter/                # 各工具适配器（claude-code/cursor/qoder/hermes…）
│   │   ├── adapter.go          #   Adapter 接口 + 通用辅助
│   │   ├── claude_code.go
│   │   ├── cursor.go
│   │   ├── qoder.go
│   │   └── hermes.go
│   ├── ask/                    # 交互询问（多选/确认）
│   ├── daemon/                 # 守护进程核心循环
│   ├── registry/               # 状态库 registry.json（含凭据，0600）
│   ├── sync/                   # 收敛 + 分发 + 软链替换
│   └── watch/                  # fsnotify 多目录监听 + debounce
```

## 同步矩阵（现状）

| 类型 | 状态 | 说明 |
|:---|:---|:---|
| Skill | ✅ M0 已实现 | 监听 → 收敛 `~/.agents/skills/` → 软链分发 |
| MCP | 🔜 M1 | 监听配置 → 合并 registry → 写回各工具 |
| Rules | 🔜 M4 | 收敛 `~/.agents/rules/` → 分发 |
| Memory/Hooks/Plugins | ❌ 不支持 | 各工具私有，仅提示 |

## 支持的工具

| 适配器 | 状态 |
|:---|:---|
| claude-code / cursor / qoder / hermes | ✅ 已实现（M0） |
| opencode / codex | 🔜 M3 |
| gemini / pi | 🔜 M3（规格已调研） |
| grok | 🔜 待实测 |

## 设计决策（详见 docs/agent-sync-prd.md）

- 软链单向同步：canonical 为唯一事实源，工具目录是投影
- 收敛后原副本替换为软链（避免双份漂移）
- 分发前询问同步范围（只列本机已检测工具 + 全部选项）
- 冲突每次询问三选一；外部改动每次询问
- MCP 凭据统一存 registry（0600）
- 未装 agentsync 前的既有配置不处理（无 adopt）
