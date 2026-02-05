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

## 核心功能

1. **透明代理** - 转发 API 请求到上游 (端口 9000)
2. **内部 API** - 管理和监控接口 (端口 9001)
3. **内存存储** - 连接信息存储，支持实时查看
4. **标签系统** - 根据请求特征自动打标签
5. **工具转换** - 基于标签的配置化转换
6. **流式支持** - SSE 流式响应处理和转换

## 标签系统 (Tagging)

根据请求特征自动匹配标签，用于选择不同的转换器：

```yaml
tagging:
  rules:
    - name: "droid-agent"
      priority: 100
      matchers:
        - type: "header"
          key: "User-Agent"
          pattern: ".*Droid.*"
      tags: ["droid"]
```

匹配器类型：`header` | `body_json` | `query` | `path`

## 转换器配置

转换器支持按标签绑定，实现不同 Agent 使用不同转换规则：

```yaml
transformers:
  response:
    - name: "create_documents_to_ExitSpecMode"
      enabled: true
      tags: ["droid"]
      source_tool: "create_documents"
      target_tool: "ExitSpecMode"
      accumulate: true
      param_mapping:
        - from: "new_documents[0].content"
          to: "plan"
```

## 内部 API

```
GET  /api/connections              # 连接列表
GET  /api/connections/:id          # 连接详情
DELETE /api/connections            # 清空连接

GET  /api/tags                     # 标签统计
GET  /api/tagging/rules            # 标签规则
POST /api/tagging/test             # 测试标签匹配

GET  /api/transformers             # 转换器列表
PUT  /api/transformers/:name       # 更新转换器

GET  /api/stats                    # 统计信息
WS   /api/ws                       # WebSocket 实时推送
```

## 部署模式

### 服务器部署 (仅 CLI)
```bash
cd proxy && go build -o agentgear ./cmd/agentgear
docker build -t agentgear:latest ./proxy
```

### 桌面使用 (GUI + CLI)
GUI 可连接本地或远程 CLI 服务进行管理。

## 开发规范

- 敏感头信息（Authorization, X-Api-Key, Anthropic-Api-Key）需脱敏
- 流式响应需要累积工具调用的所有 delta 后再转换
- 会话 ID 通过 `X-Session-Id` 请求头传递，没有则自动生成
- 内存存储有容量限制，自动淘汰旧连接

## GUI 对话框规范

表单对话框统一使用 `ResizableDialog` 组件（可拖拽、可调整大小）：

```tsx
import {
  ResizableDialog,
  ResizableDialogContent,
  ResizableDialogHeader,
  ResizableDialogBody,
  ResizableDialogFooter,
  ResizableDialogTitle,
} from '../ui/resizable-dialog'
import { Input, Checkbox, Button } from '../ui/dialog'
```

结构模板：
```tsx
<ResizableDialog open={open} onOpenChange={(o) => !o && onClose()}>
  <ResizableDialogContent defaultWidth={450} defaultHeight={480} minWidth={350} minHeight={400}>
    <ResizableDialogHeader>
      <ResizableDialogTitle>标题</ResizableDialogTitle>
    </ResizableDialogHeader>
    <ResizableDialogBody>
      <form id="form-id" onSubmit={handleSubmit}>...</form>
    </ResizableDialogBody>
    <ResizableDialogFooter>
      <Button variant="outline" onClick={onClose}>Cancel</Button>
      <Button type="submit" form="form-id">Submit</Button>
    </ResizableDialogFooter>
  </ResizableDialogContent>
</ResizableDialog>
```

**禁止**使用手写的 `fixed inset-0` modal 实现。

## UI 组件使用规范

优先使用项目已有的 UI 组件，而非原生 HTML 元素：

- **Button**: `import { Button } from '../ui/dialog'` - 支持 `variant="default|outline|destructive"`
- **Input**: `import { Input } from '../ui/dialog'` - 带 label 的输入框
- **Checkbox**: `import { Checkbox } from '../ui/dialog'`
- **Select**: `import { Select } from '../ui/dialog'`
- **Tabs**: `import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'`
- **ScrollArea**: `import { ScrollArea } from '../ui/scroll-area'`

这些组件已包含完整的交互反馈（hover、focus、disabled 状态）和一致的视觉风格。

## 架构

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
