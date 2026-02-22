<coding_guidelines>
# AgentGear 开发指南

## 项目定位

AgentGear 是 Agent API 适配层，解决不同 Agent 之间的协议兼容性问题。主要用于：
- 本地 Agent（如 Droid）调用远程 API（如通过 Warp 反向代理的 Anthropic API）
- 处理上游返回本地 Agent 不支持的工具调用
- 记录请求/响应进行问题分析
- 通过图形界面管理和配置转换规则

## 项目结构

```
agentgear/
├── proxy/                         # Go CLI 代理模块 (独立可部署)
│   ├── cmd/agentgear/             # 入口
│   ├── internal/
│   │   ├── config/                # 配置管理
│   │   ├── proxy/                 # 代理处理
│   │   ├── transformer/           # 转换器引擎
│   │   ├── tagging/               # 标签匹配引擎
│   │   ├── memory/                # 内存连接管理
│   │   ├── api/                   # 内部 API
│   │   └── logger/                # 日志
│   ├── configs/                   # 配置文件
│   ├── Dockerfile
│   └── go.mod
├── gui/                           # 图形界面模块 (Tauri + React)
├── docs/                          # 文档
└── docker-compose.yaml
```

## 核心规则

1. 敏感头信息（Authorization, X-Api-Key, Anthropic-Api-Key）需脱敏
2. 流式响应需要累积工具调用的所有 delta 后再转换
3. 会话 ID 通过 `X-Session-Id` 请求头传递，没有则自动生成
4. 内存存储有容量限制，自动淘汰旧连接
5. GUI 对话框**禁止**使用手写的 `fixed inset-0` modal 实现，必须使用 `ResizableDialog`
6. GUI 优先使用项目已有的 UI 组件（Button, Input, Checkbox, Select, Tabs, ScrollArea），而非原生 HTML 元素
7. 日志分两类：
   - **Business Logger**：始终启用，记录业务逻辑（tagging 匹配、transformer 检测/执行、mapping 应用、压缩触发、错误转换、header 注入等），输出到控制台 + `agentgear.log` 文件（按天 rotate，保留 3 个）
   - **Session Logger**：受 `logging.enabled` 控制，记录完整 session 数据（request/response body 文件）。`logging.enabled` 仅控制 session 完整日志，不影响业务日志

## 文档查阅指引

在开发前，根据你要修改的模块查阅对应的详细文档：

| 修改内容 | 查阅文档 |
|---------|---------|
| 项目架构、模块关系 | `docs/architecture/overview.md` |
| 标签系统（Tagging） | `docs/core-features/tagging.md` |
| 转换器（Transformer） | `docs/core-features/transformers.md` |
| 流式处理 | `docs/core-features/streaming.md` |
| 内部 API 接口 | `docs/core-features/internal-api.md` |
| 部署与构建 | `docs/integration/deployment.md` |
| 上下文压缩 | `docs/integration/compression.md` |
| GUI 对话框开发 | `docs/gui/dialogs.md` |
| GUI 组件使用 | `docs/gui/components.md` |

> **核心规则：渐进式文档发现**
> AGENTS.md 仅保留核心规则和文档索引。详细的设计文档、配置示例、API 说明等内容
> 存放在 `docs/` 对应子目录中。新增文档时，应放入合适的子目录并在此表格中添加索引。

## 架构概览

```
┌─────────────┐     ┌─────────────────────────────────┐     ┌─────────────┐
│  Local      │     │  AgentGear Proxy (9000)         │     │  Upstream   │
│  Agent      │────▶│  ┌─────────┐ ┌───────────────┐  │────▶│  API        │
│  (Droid)    │◀────│  │ Tagging │ │ Transformer   │  │◀────│  (Anthropic)│
└─────────────┘     │  └─────────┘ └───────────────┘  │     └─────────────┘
                    │  ┌─────────┐ ┌───────────────┐  │
                    │  │ Memory  │ │ API (9001)    │  │
                    │  │ Store   │ │ + WebSocket   │  │
                    │  └─────────┘ └───────────────┘  │
                    └─────────────────────────────────┘
                                    │
                                    ▼
                    ┌─────────────────────────────────┐
                    │  GUI (Tauri + React)            │
                    │  - 连接监控                      │
                    │  - 转换器管理                    │
                    │  - 标签规则配置                  │
                    └─────────────────────────────────┘
```
</coding_guidelines>
