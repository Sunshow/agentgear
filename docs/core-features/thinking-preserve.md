# Thinking Preserve 转换器 — 保留 Anthropic Thinking Blocks

## 问题背景

Anthropic API 的 extended thinking 功能会在 assistant 消息中返回 `type: "thinking"` 的 content block。部分情况下 thinking 内容为空，但带有加密 signature：

```json
{
  "role": "assistant",
  "content": [
    {"type": "thinking", "thinking": "", "signature": "ErQBCkYI..."},
    {"type": "text", "text": "这是实际的回复内容"}
  ]
}
```

部分下游 Agent 会丢弃空的 thinking block。当下次请求携带 conversation history 时，由于缺失 signature，Anthropic API 校验失败返回 **400 Bad Request**。

```
┌──────────┐     ┌──────────────┐     ┌───────────┐
│  Agent   │────▶│  AgentGear   │────▶│  Upstream │
│          │◀────│  Proxy       │◀────│  (Anthropic)
└──────────┘     └──────────────┘     └───────────┘
                                          │
  第 1 轮：返回 thinking block              │
  {type:"thinking", thinking:"",           │
   signature:"abc123"}                     │
     │                                      │
     ▼                                      │
  Agent 丢弃空 thinking block               │
     │                                      │
     ▼                                      │
  第 2 轮：发送 conversation history         │
  (缺少 thinking block + signature)         │
     │                                      │
     ▼                                      │
  上游校验 signature 缺失 → 400 ❌           │
```

## 解决思路

在 AgentGear 代理层缓存每会话的 assistant thinking blocks，后续请求中自动补全缺失的 thinking blocks。

**不需要缓存整个会话**，只需要缓存 assistant 消息中 `type: "thinking"` 的 content blocks。

```
Response 方向：
  上游响应 → 解析 assistant 消息 → 提取 thinking blocks → 存入 SessionThinkingStore

Request 方向：
  下游请求 → 解析 messages 数组 → 查找缓存 → 注入缺失的 thinking blocks → 转发上游
```

## 匹配策略

对 assistant 消息的**非 thinking content blocks**（text + tool_use）做 SHA256 hash 作为缓存 key。

| 场景 | 行为 |
|------|------|
| hash 命中，thinking 缺失 | 注入缓存的 thinking blocks |
| hash 命中，thinking 已存在 | 不修改（下游 Agent 保留了 thinking） |
| hash 未命中 | 透传（首次请求或缓存已淘汰） |

### 为什么用 content hash 而不是位置索引

- 部分 Agent 可能在多轮对话中重排或合并消息，位置不可靠
- thinking 内容可能为空，无法用 thinking 自身做匹配
- text + tool_use 的实际内容是该消息的"语义指纹"，确定性高

## 数据结构

```go
// SessionThinkingStore — 会话级 thinking block 缓存
type SessionThinkingStore struct {
    mu       sync.RWMutex
    sessions map[string]*SessionCache  // sessionID -> cache
    config   ThinkingStoreConfig
}

type SessionCache struct {
    entries    map[string]*ThinkingEntry  // contentHash -> entry
    lastAccess time.Time
}

type ThinkingEntry struct {
    ThinkingBlocks []ContentBlock  // 仅 type:"thinking" 的 blocks
    CachedAt       time.Time
}

type ContentBlock struct {
    Type      string `json:"type"`
    Thinking  string `json:"thinking"`
    Signature string `json:"signature"`
}

type ThinkingStoreConfig struct {
    MaxSessions          int           // 最大会话数，默认 500
    MaxEntriesPerSession int           // 每会话最大条目数，默认 10
    SessionTTL           time.Duration // 会话过期时间，默认 30min
}
```

## 核心流程

### Response 方向（缓存写入）

```
1. 上游返回完整响应 body（非流式）或累积到 message_stop（流式）
2. 解析 assistant message 的 content 数组
3. 提取 type:"thinking" 的 blocks（含 thinking 内容和 signature）
4. 对非 thinking blocks（text + tool_use）序列化后计算 SHA256 hash
5. 以 hash 为 key，将 thinking blocks 存入 SessionThinkingStore
```

关键点：
- 流式响应需累积完整的 content block 列表后才能写入缓存，不能每个 delta 单独处理
- 仅 assistant role 消息需要缓存，user/tool 消息忽略

### Request 方向（缓存注入）

```
1. 解析请求 body 的 messages 数组
2. 遍历每条 role:"assistant" 的消息：
   a. 提取非 thinking content blocks，序列化计算 hash
   b. 在 SessionThinkingStore 中查找对应 session 的缓存
   c. 若命中 且 消息中缺少 thinking blocks → 将缓存的 thinking blocks 注入
   d. 注入位置：assistant content 数组的最前面（thinking block 应在 text/tool_use 之前）
3. 重新序列化请求 body
```

### 注入前后对比

**注入前（Agent 发送的请求，缺少 thinking）**：
```json
{
  "role": "assistant",
  "content": [
    {"type": "text", "text": "这是回复内容"}
  ]
}
```

**注入后（转发给上游，补全 thinking）**：
```json
{
  "role": "assistant",
  "content": [
    {"type": "thinking", "thinking": "", "signature": "ErQBCkYI..."},
    {"type": "text", "text": "这是回复内容"}
  ]
}
```

## 转换器配置

作为内置转换器提供，通过 mapping 绑定到特定上游标签来启用：

```yaml
transformers:
  definitions:
    - name: "$preserve_thinking_blocks"
      description: "保留 Anthropic thinking blocks signature，防止下游 Agent 丢弃后导致 400 错误"
      type: "thinking_preserve"
      direction: "both"       # 同时处理 request 和 response
      builtin: true
  mappings:
    - name: "$preserve_thinking_blocks_mapping"
      enabled: true
      tags: ["$u_kiro"]       # 绑定到特定上游标签
      transformer: "$preserve_thinking_blocks"
```

### 为什么需要 mapping

- thinking_preserve 需要响应缓存 + 请求注入，仅对支持 extended thinking 的上游（如 Anthropic/Kiro）有意义
- 通过 tags 精确控制生效范围，避免不必要的性能开销
- 满足 AgentGear 现有的 transformer + mapping 架构约束

## 边界情况

| 情况 | 处理 |
|------|------|
| 消息无 thinking blocks | 不缓存该消息，请求方向也不修改 |
| 消息 thinking 非空（正常 content） | 正常缓存完整 thinking block（含 signature），后续请求注入 |
| 缓存未命中（新消息/key 不匹配） | 透传不做任何修改 |
| 会话过期淘汰 | 自动淘汰，下次命中时重新缓存即可 |
| 流式响应中途断开/error | 不写入缓存（非完整响应不可信） |
| 非流式响应无 assistant message | 不缓存 |
| 并发请求同一会话 | SessionThinkingStore 使用读写锁保护 |
| 同一条 assistant 消息被多次请求 | hash 匹配保证幂等注入 |

## 关于记忆整个会话的问题

**Q: 是否需要记忆整个会话？**

A: 不需要。只需要缓存 assistant 消息中 thinking blocks 的 signature。下游 Agent 仅删除 thinking block 本身，不会修改 text/tool_use 内容，因此用 content hash 匹配即可。

**Q: 如果 Agent 修改了 text 内容怎么办？**

A: hash 会不匹配，视为未命中，透传不做修改。这种情况上游本身就会 400（内容被篡改），不属于本转换器要解决的问题。

## 涉及文件

| 文件 | 改动 |
|------|------|
| `proxy/internal/memory/thinking_store.go` | **新建** — SessionThinkingStore 实现 |
| `proxy/internal/transformer/thinking_preserve.go` | **新建** — 转换器实现 |
| `proxy/internal/transformer/registry.go` | 注册 thinking_preserve 类型 |
| `proxy/internal/transformer/builtin.go` | 添加内置定义和 mapping |
| `proxy/internal/proxy/handler.go` | 注入 ThinkingStore，集成请求/响应处理 |
| `proxy/internal/config/config.go` | 添加 ThinkingStoreConfig 配置项 |
| `proxy/configs/config.example.yaml` | 添加配置示例 |

## 后续工作

1. 收集实际请求/响应日志，确认 thinking block 的具体格式和 signature 特征
2. 确认下游 Agent 丢弃 thinking block 的具体位置和条件
3. 根据日志二次确认本方案可行后再开始编码实现
