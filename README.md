# agentsync

跨 Agent 工具配置自动同步工具（Go 实现，CLI + 后台守护进程）。

**核心模型：监听-收敛-分发。**

- 用户在任何已安装的 agent 工具（Claude Code / Cursor / Qoder / Hermes …）里安装 skill
- 守护进程监听各工具 skills 目录 → 收敛到统一副本 `~/.agents/skills/`
- 原副本替换为软链指向统一副本 → 询问分发到哪些工具 → 其他工具目录建软链
- 之后在任一工具里修改，因软链穿透，全部工具即时一致

## 安装

**方式一：一键脚本（推荐，无需 Go）**

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/AdamsShen/agentsync/master/install.sh | bash
```

Windows（PowerShell）：

```powershell
irm https://raw.githubusercontent.com/AdamsShen/agentsync/master/install.ps1 | iex
```

**方式二：go install（需要 Go，三平台通用）**

```bash
go install github.com/AdamsShen/agentsync@latest
```

一键脚本会自动注册开机自启并启动；`go install` 只装二进制，需手动：

```bash
agentsync install    # 注册开机自启服务 + 立即启动
agentsync daemon     # 或前台启动守护进程（调试用）
```

> 说明：agentsync 只处理「安装后新增」的配置，安装前已存在的配置不处理（无 adopt）。
> Windows 的开机自启（`sc create` 系统服务）需管理员权限。

## 快速开始（本地构建/开发）

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
├── main.go                      # CLI 入口
├── install.sh                   # 一键安装脚本（curl | bash）
├── .goreleaser.yaml             # 跨平台构建配置
├── .github/workflows/release.yml # 打 tag 自动发布 GitHub Release
├── internal/
│   ├── adapter/                 # 各工具适配器（claude-code/cursor/qoder/hermes/codex/opencode/gemini/pi/grok）
│   ├── ask/                     # 交互询问（多选/确认）
│   ├── daemon/                  # 守护进程核心循环
│   ├── mcpread/                 # MCP 多格式读写（JSON/TOML/YAML）
│   ├── mcpsync/                 # MCP 跨工具同步
│   ├── registry/                # 状态库 registry.json（含凭据，0600）
│   ├── sync/                    # 收敛 + 分发 + 软链替换
│   ├── tui/                     # 交互式状态面板
│   └── watch/                   # fsnotify 多目录监听 + debounce
```

## 同步矩阵（现状）

| 类型 | 状态 | 说明 |
|:---|:---|:---|
| Skill | ✅ 已实现 | 监听 → 收敛 `~/.agents/skills/` → 软链分发 |
| MCP | ✅ 已实现 | 多格式（JSON/TOML/YAML）监听 → 合并 registry → 写回各工具 |
| Rules | ✅ 已实现 | 目录式收敛 + 单文件规则（AGENTS.md/SOUL.md）只收敛 |
| Memory/Hooks/Plugins | ❌ 不支持 | 各工具私有，仅提示 |

## 支持的工具

| 适配器 | 状态 |
|:---|:---|
| claude-code / cursor / qoder / hermes / codex / opencode / gemini / pi / grok | ✅ 已实现 |

> codex / opencode / grok 的软链与 MCP 写回待真机验证；grok 因官方安装源被墙，配置格式经文档确认。

## 设计决策（详见 docs/agent-sync-prd.md）

- 软链单向同步：canonical 为唯一事实源，工具目录是投影
- 收敛后原副本替换为软链（避免双份漂移）
- 分发前询问同步范围（只列本机已检测工具 + 全部选项）
- 冲突每次询问三选一；外部改动每次询问
- MCP 凭据统一存 registry（0600）
- 未装 agentsync 前的既有配置不处理（无 adopt）
