# Thinking Preserve 转换器 — 保留 Thinking Blocks

## 问题背景

部分上游 API（如 DeepSeek 兼容 Anthropic 协议）的 extended thinking 功能会在 assistant 消息中返回 `type: "thinking"` 的 content block。部分情况下 thinking 内容为空，但带有 signature：

```json
{
  "role": "assistant",
  "content": [
    {"type": "thinking", "thinking": "", "signature": "55f95082-3e70-..."},
    {"type": "tool_use", "id": "call_00_xxx", "name": "Edit", "input": {...}}
  ]
}
```

部分下游 Agent 会丢弃空的 thinking block。当下次请求携带 conversation history 时，由于缺失 signature，上游 API 校验失败返回 **400 Bad Request**：

```
"The `content[].thinking` in the thinking mode must be passed back to the API."
```

```
┌──────────┐     ┌──────────────┐     ┌───────────┐
│  Agent   │────▶│  AgentGear   │────▶│  Upstream │
│          │◀────│  Proxy       │◀────│  API      │
└──────────┘     └──────────────┘     └───────────┘

  第 1 轮：上游返回 thinking block
  {type:"thinking", thinking:"", signature:"abc123"}
     │
     ▼
  Agent 丢弃空 thinking block
     │
     ▼
  第 2 轮：发送 conversation history（缺少 thinking block）
     │
     ▼
  上游校验 signature 缺失 → 400 ❌
```

## 解决方案

在 AgentGear 代理层缓存上游响应中的 thinking blocks，后续请求中自动补全缺失的 thinking blocks。

**不需要缓存整个会话**，只需要缓存 assistant 消息中 `type: "thinking"` 的 content blocks。

```
Response 方向：
  上游响应 → 解析 content blocks → 提取 thinking blocks → 存入 ThinkingStore

Request 方向：
  下游请求 → 遍历 assistant messages → 查找缓存 → 注入缺失的 thinking blocks → 转发上游
```

## 缓存设计：全局 Content Hash

### 为什么不用 Session 分区

Agent（如 factory-cli）**不发送** `X-Session-Id` header，每次请求都会自动生成新的 session_id。因此无法按 session 分区缓存。

### 全局 Hash 缓存

使用全局的 `ThinkingStore`，以 content hash 作为 key：

```go
type ThinkingStore struct {
    mu      sync.RWMutex
    entries map[string]*ThinkingEntry // contentHash -> entry（全局）
    config  ThinkingStoreConfig
}

type ThinkingStoreConfig struct {
    MaxEntries      int    // 最大条目数，默认 5000
    EntryTTLMinutes int    // 条目过期时间，默认 1440 分钟（24 小时）
    PersistPath     string // 持久化快照路径，默认 ./data/thinking_store.json
}
```

**理由**：
- Content hash（非 thinking blocks 的 SHA256）本身就是全局唯一的语义指纹
- 不同会话不可能产生相同的 text + tool_use 组合（tool_use 包含唯一 ID）
- 简化实现，无需处理 session 关联

### 缓存生命周期

- 默认会将快照持久化到 `./data/thinking_store.json`
- 启动时自动加载持久化快照，关闭时强制 flush
- 运行中采用 debounce 原子刷盘（临时文件 + rename），访问时间只按粗粒度同步到磁盘，避免命中请求触发高频全量重写
- TTL 默认 24 小时，最多 5000 条，自动过期淘汰
- 命中条目会刷新访问时间，避免长会话中仍在使用的旧 assistant 快照被过早淘汰

## 匹配策略

对 assistant 消息的**非 thinking content blocks** 做 SHA256 hash 作为缓存 key。

Hash 计算时只使用稳定字段，排除 `cache_control` 等可变字段：
- `tool_use`: 使用 `type`, `id`, `name`, `input`
- `text`: 使用 `type`, `text`
- 其他: 使用 `type`, `id`

| 场景 | 行为 |
|------|------|
| hash 命中，thinking 缺失 | 注入缓存的 thinking blocks |
| hash 命中，thinking 已存在 | 不修改（下游 Agent 保留了 thinking） |
| hash 未命中 | 透传（首次请求或缓存已淘汰） |

### 为什么用 content hash 而不是位置索引

- 部分 Agent 可能在多轮对话中重排或合并消息，位置不可靠
- thinking 内容可能为空，无法用 thinking 自身做匹配
- text + tool_use 的实际内容是该消息的"语义指纹"，确定性高

### 多次空 thinking 支持

每个 assistant 响应的 content hash 是独立的（因为 tool_use ID 不同），所以多次空 thinking 各自独立缓存，互不影响。

## 核心流程

### Response 方向（缓存写入）

```
1. 流式响应完成后（message_stop 之后），解析 originalBuffer 中的 SSE 数据
2. 累积完整的 content blocks（thinking_delta → thinking, signature_delta → signature, input_json_delta → input）
3. 提取 type:"thinking" 的 blocks（含 thinking 内容和 signature）
4. 对非 thinking blocks 序列化后计算 SHA256 hash
5. 以 hash 为 key，将 thinking blocks 存入 ThinkingStore
```

关键点：
- 缓存在流式输出完成后执行，**不影响流式输出的实时性**
- 解析的是 `originalBuffer`（通过 TeeReader 记录的副本），不干扰客户端数据流
- 所有包含 thinking blocks 的响应都会缓存（不区分 thinking 是否为空）

### Request 方向（缓存注入）

```
1. 在消息格式修正之前，解析请求 body 的 messages 数组
2. 遍历每条 role:"assistant" 的消息：
   a. 检查是否已有 thinking blocks → 有则跳过
   b. 提取非 thinking content blocks，计算 hash
   c. 在 ThinkingStore 中查找缓存
   d. 若命中 → 将缓存的 thinking blocks 注入到 content 数组最前面
3. 重新序列化请求 body
```

### 注入前后对比

**注入前（Agent 发送的请求，缺少 thinking）**：
```json
{
  "role": "assistant",
  "content": [
    {"type": "tool_use", "id": "call_00_xxx", "name": "Edit", "input": {...}}
  ]
}
```

**注入后（转发给上游，补全 thinking）**：
```json
{
  "role": "assistant",
  "content": [
    {"type": "thinking", "thinking": "", "signature": "55f95082-3e70-..."},
    {"type": "tool_use", "id": "call_00_xxx", "name": "Edit", "input": {...}}
  ]
}
```

## 转换器配置

作为内置转换器提供，默认对所有请求生效（无 tag 限制）：

```yaml
# 内置 definition（自动注册，无需手动配置）
transformers:
  definitions:
    - name: "$preserve_thinking_blocks"
      description: "保留 thinking blocks signature，防止下游 Agent 丢弃后导致 400 错误"
      type: "thinking_preserve"
      direction: "both"
      builtin: true
  mappings:
    - name: "$preserve_thinking_blocks_mapping"
      enabled: true
      tags: []              # 空 tags = 对所有请求生效
      transformer: "$preserve_thinking_blocks"
      builtin: true
```

该功能对所有请求安全生效：
- 响应无 thinking blocks → 不缓存
- 请求中 assistant 消息已有 thinking → 不修改
- 缓存未命中 → 透传

## 日志

通过 Business Logger 输出，不受 `logging.enabled` 控制：

| 级别 | 场景 | 日志内容 |
|------|------|---------|
| INFO | 缓存空 thinking block | `thinking_preserve: caching empty thinking block (agent may drop this)` |
| DEBUG | 缓存非空 thinking block | `thinking_preserve: caching thinking block` |
| INFO | 缓存写入完成 | `thinking_preserve: cached thinking blocks from response` + hash、store_size |
| INFO | 请求注入成功 | `thinking_preserve: injected thinking blocks into request` + hash、store_size |
| DEBUG | 缓存未命中 | `thinking_preserve: cache miss for assistant message without thinking` |

## 边界情况

| 情况 | 处理 |
|------|------|
| 消息无 thinking blocks | 不缓存，请求方向也不修改 |
| 消息 thinking 非空 | 正常缓存（Agent 也可能丢弃非空 thinking） |
| 缓存未命中 | 透传不做任何修改 |
| 缓存过期淘汰 | 自动淘汰，后续响应会重新缓存 |
| 流式响应中途断开/error | 不写入缓存（非 200 响应不处理） |
| 并发请求 | ThinkingStore 使用读写锁保护 |
| 同一条消息被多次请求 | hash 匹配保证幂等注入 |
| 进程重启 | 默认从持久化快照恢复；若快照不存在则新响应会重新缓存 |
| Agent 修改了 text 内容 | hash 不匹配，透传不修改 |
| content 含 cache_control 等额外字段 | hash 计算时排除，不影响匹配 |

## 涉及文件

| 文件 | 改动 |
|------|------|
| `proxy/internal/memory/thinking_store.go` | **新建** — 全局 ThinkingStore 实现 |
| `proxy/internal/transformer/thinking_preserve.go` | **新建** — ThinkingPreserver（缓存写入 + 请求注入 + hash 计算） |
| `proxy/internal/transformer/registry.go` | 添加 `GetThinkingPreserveTransformer()` 方法 |
| `proxy/internal/transformer/builtin.go` | 添加内置 definition + mapping |
| `proxy/internal/proxy/handler.go` | 集成：请求方向注入 + 响应方向 SSE 解析缓存 |
| `proxy/cmd/agentgear/cmd/serve.go` | 初始化 ThinkingStore 并注入 Handler |
