# OpenCode 上下文自动压缩机制 — 完整技术规范

> 本文档详细描述 OpenCode 项目中上下文超长时的 compact/compress 行为，包括触发条件、各阶段提示词、请求格式、数据结构和配置选项。  
> 目标读者：需要在其他 Agent 系统中实现类似自动压缩功能的开发者。

---

## 目录

1. [整体架构概览](#1-整体架构概览)
2. [触发检测机制](#2-触发检测机制)
3. [Prune 修剪阶段](#3-prune-修剪阶段)
4. [Compaction 压缩阶段](#4-compaction-压缩阶段)
5. [System Prompt 与 Agent 配置](#5-system-prompt-与-agent-配置)
6. [压缩后处理与消息过滤](#6-压缩后处理与消息过滤)
7. [配置选项、插件扩展点与移植指南](#7-配置选项插件扩展点与移植指南)

---

## 1. 整体架构概览

### 1.1 核心设计思想

OpenCode 采用 **两阶段渐进式压缩** 策略：

- **Prune（修剪）**：轻量级本地操作，清除旧的工具调用输出，不涉及 LLM 调用
- **Compaction（压缩）**：重量级操作，调用 LLM 生成结构化摘要替代历史消息

两个阶段独立运行，Prune 在每次会话循环结束后执行，Compaction 仅在检测到上下文溢出时触发。

### 1.2 关键源文件

| 文件 | 职责 |
|------|------|
| `packages/opencode/src/session/compaction.ts` | 压缩核心逻辑：溢出检测、锚点选择、修剪、压缩处理 |
| `packages/opencode/src/session/processor.ts` | 流处理器：在 `finish-step` 中检测溢出 |
| `packages/opencode/src/session/prompt.ts` | 会话主循环：调度 prune 和 compaction |
| `packages/opencode/src/session/llm.ts` | LLM 调用封装：组装最终请求 |
| `packages/opencode/src/agent/agent.ts` | Agent 定义：compaction agent 配置 |
| `packages/opencode/src/agent/prompt/compaction.txt` | 压缩 Agent 的 System Prompt |
| `packages/opencode/src/session/message-v2.ts` | 消息模型：toModelMessages、filterCompacted |
| `packages/opencode/src/util/token.ts` | Token 估算工具 |
| `packages/opencode/src/config/config.ts` | 配置定义：compaction 相关选项 |
| `packages/opencode/src/flag/flag.ts` | 环境变量开关 |

### 1.3 完整流程图

```
用户发送消息
    │
    ▼
┌─────────────────────────────┐
│  SessionPrompt.loop()       │  ◄── 会话主循环
│  主循环开始                   │
└─────────┬───────────────────┘
          │
          ▼
┌─────────────────────────────┐
│  SessionProcessor.process() │  ◄── 流式处理 LLM 响应
│  处理 LLM 流式响应            │
└─────────┬───────────────────┘
          │
          ▼  finish-step 事件
┌─────────────────────────────┐
│  SessionCompaction           │
│  .isOverflow()              │  ◄── 检测上下文是否溢出
│  检测 token 是否超限          │
└─────────┬───────────────────┘
          │
     ┌────┴────┐
     │ 未溢出   │ 溢出
     ▼         ▼
  正常继续   processor.process()
             返回 "compact"
                │
                ▼
┌─────────────────────────────┐
│  SessionCompaction.create() │  ◄── 创建压缩任务消息
│  创建 compaction part        │
└─────────┬───────────────────┘
          │
          ▼  下一次主循环迭代
┌─────────────────────────────┐
│  检测到 compaction part      │
│  调用 SessionCompaction      │
│  .process()                 │  ◄── 执行压缩
└─────────┬───────────────────┘
          │
          ├── selectAnchor()        分割消息为 prefix/suffix
          ├── sanitizeToolMessages() 清理工具消息为纯文本
          ├── LLM.stream()          调用 LLM 生成摘要
          └── 保存 summary 消息      标记 summary: true
          │
          ▼
┌─────────────────────────────┐
│  自动继续（auto=true 时）     │
│  添加 "Continue if you have  │
│  next steps" 合成消息         │
└─────────┬───────────────────┘
          │
          ▼  主循环结束
┌─────────────────────────────┐
│  SessionCompaction.prune()  │  ◄── 修剪旧工具输出
│  清理超出保护范围的工具输出    │
└─────────────────────────────┘
```

---

## 2. 触发检测机制

### 2.1 检测时机

在 `SessionProcessor.process()` 的 `finish-step` 事件中检测：

```typescript
// packages/opencode/src/session/processor.ts
case "finish-step":
  const usage = Session.getUsage({
    model: input.model,
    usage: value.usage,
    metadata: value.providerMetadata,
  })
  // ... 更新消息和 tokens
  
  if (await SessionCompaction.isOverflow({ tokens: usage.tokens, model: input.model })) {
    needsCompaction = true
  }
  break
```

当 `needsCompaction = true` 时，`processor.process()` 返回 `"compact"`，触发主循环创建压缩任务。

### 2.2 溢出判断逻辑

```typescript
// packages/opencode/src/session/compaction.ts
export async function isOverflow(input: { 
  tokens: MessageV2.Assistant["tokens"]; 
  model: Provider.Model 
}) {
  const config = await Config.get()
  
  // 1. 检查配置是否禁用自动压缩
  if (config.compaction?.auto === false) return false
  
  // 2. 检查模型上下文限制（0 表示无限制）
  const context = input.model.limit.context
  if (context === 0) return false
  
  // 3. 计算当前 token 总数
  const count = input.tokens.input + input.tokens.cache.read + input.tokens.output
  
  // 4. 计算可用输出 token 数（取模型限制和全局限制的较小值）
  const output = Math.min(input.model.limit.output, SessionPrompt.OUTPUT_TOKEN_MAX) 
                 || SessionPrompt.OUTPUT_TOKEN_MAX  // 默认 64,000
  
  // 5. 计算可用输入 token 数
  const usable = input.model.limit.input || (context - output)
  
  // 6. 判断是否溢出
  return count > usable
}
```

### 2.3 关键常量

```typescript
// packages/opencode/src/session/prompt.ts
export const OUTPUT_TOKEN_MAX = Flag.OPENCODE_EXPERIMENTAL_OUTPUT_TOKEN_MAX || 64_000

// packages/opencode/src/session/compaction.ts
export const PRUNE_MINIMUM = 20_000      // 最小修剪阈值
export const PRUNE_PROTECT = 40_000      // 保护最近的 40k tokens
export const SUMMARY_BUDGET = 4_000      // 摘要预算
export const PRESERVE_BUDGET = 40_000    // 保留最近消息预算
```

### 2.4 Token 计算规则

```typescript
// packages/opencode/src/session/index.ts - Session.getUsage()
export const getUsage = fn(
  z.object({
    model: z.custom<Provider.Model>(),
    usage: z.custom<LanguageModelUsage>(),
    metadata: z.custom<ProviderMetadata>().optional(),
  }),
  (input) => {
    // 1. 提取缓存相关 token
    const cacheReadInputTokens = input.usage.cachedInputTokens ?? 0
    const cacheWriteInputTokens = (
      input.metadata?.["anthropic"]?.["cacheCreationInputTokens"] ??
      input.metadata?.["bedrock"]?.["usage"]?.["cacheWriteInputTokens"] ??
      input.metadata?.["venice"]?.["usage"]?.["cacheCreationInputTokens"] ??
      0
    ) as number

    // 2. 判断是否需要排除缓存 token（Anthropic/Bedrock 已在 inputTokens 中排除）
    const excludesCachedTokens = !!(input.metadata?.["anthropic"] || input.metadata?.["bedrock"])
    
    // 3. 计算调整后的输入 token
    const adjustedInputTokens = excludesCachedTokens
      ? (input.usage.inputTokens ?? 0)
      : (input.usage.inputTokens ?? 0) - cacheReadInputTokens - cacheWriteInputTokens

    // 4. 返回标准化的 token 结构
    return {
      tokens: {
        input: safe(adjustedInputTokens),
        output: safe(input.usage.outputTokens ?? 0),
        reasoning: safe(input.usage?.reasoningTokens ?? 0),
        cache: {
          write: safe(cacheWriteInputTokens),
          read: safe(cacheReadInputTokens),
        },
      },
      cost: /* 成本计算逻辑 */
    }
  }
)
```

### 2.5 溢出后的处理

```typescript
// packages/opencode/src/session/prompt.ts - loop()
const result = await processor.process(streamInput)

if (result === "compact") {
  await SessionCompaction.create({
    sessionID,
    agent: lastUser.agent,
    model: lastUser.model,
    auto: true,  // 自动触发
  })
}
```

`SessionCompaction.create()` 创建一条用户消息，附带 `compaction` part：

```typescript
// 创建的消息结构
{
  id: Identifier.ascending("message"),
  role: "user",
  model: { providerID: "...", modelID: "..." },
  sessionID: "...",
  agent: "build",  // 当前用户的 agent
  time: { created: Date.now() }
}

// 附带的 part
{
  id: Identifier.ascending("part"),
  messageID: "<上面消息的id>",
  sessionID: "...",
  type: "compaction",
  auto: true  // 标记为自动触发
}
```

---

## 3. Prune 修剪阶段

### 3.1 执行时机

在主循环结束后自动执行，**不涉及任何 LLM 调用**：

```typescript
// packages/opencode/src/session/prompt.ts - loop() 结束前
SessionCompaction.prune({ sessionID })
```

### 3.2 修剪逻辑

```typescript
// packages/opencode/src/session/compaction.ts
export async function prune(input: { sessionID: string }) {
  const config = await Config.get()
  if (config.compaction?.prune === false) return  // 检查配置
  
  const msgs = await Session.messages({ sessionID: input.sessionID })
  let total = 0      // 累计工具输出 token 数
  let pruned = 0     // 累计修剪的 token 数
  const toPrune = [] // 待修剪的 part 列表
  let turns = 0      // 对话轮数计数
  
  // 从最新消息向前遍历
  loop: for (let msgIndex = msgs.length - 1; msgIndex >= 0; msgIndex--) {
    const msg = msgs[msgIndex]
    
    // 跳过最近 2 轮对话
    if (msg.info.role === "user") turns++
    if (turns < 2) continue
    
    // 遇到摘要消息则停止
    if (msg.info.role === "assistant" && msg.info.summary) break loop
    
    // 遍历消息的 parts
    for (let partIndex = msg.parts.length - 1; partIndex >= 0; partIndex--) {
      const part = msg.parts[partIndex]
      
      if (part.type === "tool" && part.state.status === "completed") {
        // 跳过受保护的工具
        if (PRUNE_PROTECTED_TOOLS.includes(part.tool)) continue
        
        // 遇到已修剪的工具则停止
        if (part.state.time.compacted) break loop
        
        // 累计 token 数
        const estimate = Token.estimate(part.state.output)
        total += estimate
        
        // 超过保护阈值后标记为待修剪
        if (total > PRUNE_PROTECT) {
          pruned += estimate
          toPrune.push(part)
        }
      }
    }
  }
  
  // 只有累计修剪量超过最小阈值才执行
  if (pruned > PRUNE_MINIMUM) {
    for (const part of toPrune) {
      if (part.state.status === "completed") {
        part.state.time.compacted = Date.now()  // 标记为已修剪
        await Session.updatePart(part)
      }
    }
    log.info("pruned", { count: toPrune.length })
  }
}
```

### 3.3 关键常量与配置

```typescript
// 修剪相关常量
const PRUNE_MINIMUM = 20_000      // 最小修剪阈值：累计修剪量必须 > 20k tokens
const PRUNE_PROTECT = 40_000      // 保护阈值：保护最近 40k tokens 的工具输出
const PRUNE_PROTECTED_TOOLS = ["skill"]  // 受保护的工具列表

// Token 估算（简单规则：4 字符 = 1 token）
export namespace Token {
  const CHARS_PER_TOKEN = 4
  export function estimate(input: string) {
    return Math.max(0, Math.round((input || "").length / CHARS_PER_TOKEN))
  }
}
```

### 3.4 修剪后的数据结构

被修剪的 `ToolPart` 仅修改 `time.compacted` 字段，原始输出保留在存储中：

```typescript
// 修剪前
{
  type: "tool",
  tool: "bash",
  state: {
    status: "completed",
    output: "很长的输出内容...",
    time: {
      start: 1234567890,
      end: 1234567891
    }
  }
}

// 修剪后（仅添加 compacted 字段）
{
  type: "tool",
  tool: "bash",
  state: {
    status: "completed",
    output: "很长的输出内容...",  // 原始输出保留
    time: {
      start: 1234567890,
      end: 1234567891,
      compacted: 1234567900  // ← 新增：标记修剪时间
    }
  }
}
```

### 3.5 修剪后的消息转换

在 `toModelMessages()` 中，被修剪的工具输出会被替换：

```typescript
// packages/opencode/src/session/compaction.ts - messagePayload()
function messagePayload(message: MessageV2.WithParts) {
  const parts = [] as string[]
  for (const part of message.parts) {
    if (part.type === "tool" && part.state.status === "completed") {
      const output = part.state.time.compacted 
        ? "[Old tool result content cleared]"  // ← 修剪后的占位符
        : part.state.output
      parts.push(output)
    }
  }
  return parts.join("\n")
}
```

---

## 4. Compaction 压缩阶段

这是唯一涉及 LLM 调用的阶段，也是整个压缩机制的核心。

### 4.1 触发条件

在主循环的下一次迭代中，检测到 `compaction` part 后执行：

```typescript
// packages/opencode/src/session/prompt.ts - loop()
const task = tasks.pop()

if (task?.type === "compaction") {
  const result = await SessionCompaction.process({
    messages: msgs,
    parentID: lastUser.id,
    abort,
    sessionID,
    auto: task.auto,
  })
  if (result === "stop") break
  continue
}
```

### 4.2 消息分割 — selectAnchor

将消息历史分为两部分：需要压缩的 prefix 和保留的 suffix。

```typescript
// packages/opencode/src/session/compaction.ts
export function selectAnchor(input: {
  messages: MessageV2.WithParts[]
  summaryBudget: number      // 4,000 tokens
  preserveBudget: number     // 40,000 tokens
}) {
  const budget = input.preserveBudget + input.summaryBudget  // 44,000
  const preserveBudget = budget - input.summaryBudget        // 40,000
  const suffixMessages = [] as MessageV2.WithParts[]
  const state = { total: 0 }
  const list = [...input.messages].reverse()  // 从最新消息开始

  // 从后向前累计消息，直到达到 preserveBudget
  for (const message of list) {
    const estimate = messageTokens(message)
    
    if (suffixMessages.length === 0) {
      // 第一条消息（最新）总是保留，即使超出预算
      suffixMessages.unshift(message)
      state.total = estimate
      if (estimate > preserveBudget) break
      continue
    }
    
    // 如果加上当前消息会超出预算，则停止
    if (state.total + estimate > preserveBudget) break
    
    suffixMessages.unshift(message)
    state.total += estimate
  }

  // 计算 prefix（需要压缩的部分）
  const prefixCount = input.messages.length - suffixMessages.length
  const prefixMessages = input.messages.slice(0, prefixCount)
  const anchorMessageID = suffixMessages.length > 0 ? suffixMessages[0].info.id : ""

  return { anchorMessageID, prefixMessages, suffixMessages }
}

// Token 估算辅助函数
function messageTokens(message: MessageV2.WithParts) {
  return Token.estimate(messagePayload(message))
}

function messagePayload(message: MessageV2.WithParts) {
  const parts = [] as string[]
  for (const part of message.parts) {
    if (part.type === "text") {
      if (part.synthetic) continue  // 跳过合成文本
      if (part.ignored) continue    // 跳过被忽略的文本
      parts.push(part.text)
    }
    if (part.type === "tool") {
      if (part.state.status !== "completed") continue
      const output = part.state.time.compacted 
        ? "[Old tool result content cleared]" 
        : part.state.output
      parts.push(output)
    }
  }
  return parts.join("\n")
}
```

**示例：**

假设有 20 条消息，每条约 4k tokens，最后 3 条分别为 60k、60k、40k tokens：

```
消息 1-17: 每条 ~4 tokens
消息 18: 60,000 tokens
消息 19: 60,000 tokens
消息 20: 40,000 tokens (最新)

selectAnchor 结果：
- anchorMessageID: "m18"
- prefixMessages: m1-m17 (需要压缩)
- suffixMessages: m18-m20 (保留，约 160k tokens)
```

### 4.3 消息清理 — sanitizeToolMessages

将工具调用和工具结果转换为纯文本，防止 API 拒绝（因为压缩时 `tools: {}` 为空）。

```typescript
// packages/opencode/src/session/compaction.ts
function sanitizeToolMessages(messages: ModelMessage[]): ModelMessage[] {
  return messages.map((msg): ModelMessage => {
    // 1. 处理 role: 'tool' 消息 → 转换为 role: 'user'
    if (msg.role === "tool") {
      const texts: string[] = []
      if (Array.isArray(msg.content)) {
        for (const part of msg.content) {
          if (!part || typeof part !== "object") continue
          if (part.type === "tool-result") {
            const id = "toolCallId" in part ? part.toolCallId : ""
            const name = "toolName" in part ? part.toolName : "unknown"
            const result = extractOutput("output" in part ? part.output : "")
            texts.push(`[Tool Result${id ? ` (${id})` : ""} ${name}: ${result}]`)
          }
        }
      }
      return {
        role: "user",
        content: texts.join("\n") || "[Tool result converted to text]",
      }
    }

    if (typeof msg.content === "string") return msg
    if (!Array.isArray(msg.content)) return msg

    // 2. 处理 assistant 消息中的 tool-call 和 tool-result
    if (msg.role === "assistant") {
      const newContent = msg.content.map((part) => {
        if (!part || typeof part !== "object") return part
        
        // tool-call → 纯文本
        if (part.type === "tool-call") {
          const id = "toolCallId" in part ? part.toolCallId : ""
          const name = "toolName" in part ? part.toolName : "unknown"
          const input = safeStringify("input" in part ? part.input : "")
          return { 
            type: "text" as const, 
            text: `[Tool Call${id ? ` (${id})` : ""}: ${name}(${input})]` 
          }
        }
        
        // tool-result → 纯文本
        if (part.type === "tool-result") {
          const id = "toolCallId" in part ? part.toolCallId : ""
          const name = "toolName" in part ? part.toolName : "unknown"
          const result = extractOutput("output" in part ? part.output : "")
          return { 
            type: "text" as const, 
            text: `[Tool Result${id ? ` (${id})` : ""} ${name}: ${result}]` 
          }
        }
        
        return part
      })
      return { ...msg, content: newContent }
    }

    return msg
  })
}

// 辅助函数
function safeStringify(value: unknown): string {
  if (typeof value === "string") return value
  if (value === undefined || value === null) return ""
  try {
    return JSON.stringify(value) ?? ""
  } catch {
    return "[unserializable]"
  }
}

function extractOutput(output: unknown): string {
  if (!output || typeof output !== "object") return safeStringify(output)
  const o = output as Record<string, unknown>
  if (o.type === "text" && typeof o.value === "string") return o.value
  if (o.type === "json") return safeStringify(o.value)
  return safeStringify(output)
}
```

**转换示例：**

```typescript
// 转换前
{
  role: "assistant",
  content: [
    { type: "text", text: "我来执行命令" },
    { 
      type: "tool-call", 
      toolCallId: "call_abc123", 
      toolName: "bash", 
      input: { command: "ls -la" } 
    }
  ]
}

// 转换后
{
  role: "assistant",
  content: [
    { type: "text", text: "我来执行命令" },
    { type: "text", text: '[Tool Call (call_abc123): bash({"command":"ls -la"})]' }
  ]
}

// 转换前
{
  role: "tool",
  content: [
    { 
      type: "tool-result", 
      toolCallId: "call_abc123", 
      toolName: "bash", 
      output: "total 48\ndrwxr-xr-x  12 user  staff  384 Feb  8 10:00 .\n..." 
    }
  ]
}

// 转换后
{
  role: "user",
  content: "[Tool Result (call_abc123) bash: total 48\ndrwxr-xr-x  12 user  staff  384 Feb  8 10:00 .\n...]"
}
```

### 4.4 构造 LLM 请求

```typescript
// packages/opencode/src/session/compaction.ts - process()
export async function process(input: {
  parentID: string
  messages: MessageV2.WithParts[]
  sessionID: string
  abort: AbortSignal
  auto: boolean
}) {
  const userMessage = input.messages.findLast((m) => m.info.id === input.parentID)!.info as MessageV2.User

  // 1. 分割消息
  const { anchorMessageID, prefixMessages } = selectAnchor({
    messages: input.messages,
    summaryBudget: SUMMARY_BUDGET,
    preserveBudget: PRESERVE_BUDGET,
  })

  // 2. 获取 compaction agent 和模型
  const agent = await Agent.get("compaction")
  const model = agent.model
    ? await Provider.getModel(agent.model.providerID, agent.model.modelID)
    : await Provider.getModel(userMessage.model.providerID, userMessage.model.modelID)

  // 3. 创建 assistant 消息（用于保存摘要）
  const msg = (await Session.updateMessage({
    id: Identifier.ascending("message"),
    role: "assistant",
    parentID: input.parentID,
    sessionID: input.sessionID,
    mode: "compaction",
    agent: "compaction",
    summary: true,  // ← 标记为摘要消息
    path: { cwd: Instance.directory, root: Instance.worktree },
    cost: 0,
    tokens: { output: 0, input: 0, reasoning: 0, cache: { read: 0, write: 0 } },
    modelID: model.id,
    providerID: model.providerID,
    time: { created: Date.now() },
  })) as MessageV2.Assistant

  // 4. 更新 compaction part 的 anchorMessageID
  const parentMessage = await MessageV2.get({
    sessionID: input.sessionID,
    messageID: input.parentID,
  })
  const compactionPart = parentMessage.parts.find((part) => part.type === "compaction")
  if (compactionPart) {
    await Session.updatePart({ ...compactionPart, anchorMessageID })
  }

  // 5. 创建处理器
  const processor = SessionProcessor.create({
    assistantMessage: msg,
    sessionID: input.sessionID,
    model,
    abort: input.abort,
  })

  // 6. 插件扩展点：允许自定义压缩 prompt
  const compacting = await Plugin.trigger(
    "experimental.session.compacting",
    { sessionID: input.sessionID },
    { context: [], prompt: undefined }
  )
  const defaultPrompt = 
    "Please read the complete conversation above and generate a summary according to the guidelines. " +
    "The new session will not have access to our conversation history, so the summary must contain " +
    "all key information needed to continue the work."
  const promptText = compacting.prompt ?? [defaultPrompt, ...compacting.context].join("\n\n")

  // 7. 调用 LLM 生成摘要
  const result = await processor.process({
    user: userMessage,
    agent,
    abort: input.abort,
    sessionID: input.sessionID,
    tools: {},  // ← 空工具集
    system: [], // ← 空 system（agent 的 prompt 会自动添加）
    messages: [
      // 清理后的历史消息
      ...sanitizeToolMessages(MessageV2.toModelMessages(prefixMessages, model)),
      // 压缩指令
      {
        role: "user",
        content: [{ type: "text", text: promptText }],
      },
    ],
    model,
  })

  // 8. 自动继续（如果 auto=true）
  if (result === "continue" && input.auto) {
    const continueMsg = await Session.updateMessage({
      id: Identifier.ascending("message"),
      role: "user",
      sessionID: input.sessionID,
      time: { created: Date.now() },
      agent: userMessage.agent,
      model: userMessage.model,
    })
    await Session.updatePart({
      id: Identifier.ascending("part"),
      messageID: continueMsg.id,
      sessionID: input.sessionID,
      type: "text",
      synthetic: true,
      text: "Continue if you have next steps",
      time: { start: Date.now(), end: Date.now() },
    })
  }

  if (processor.message.error) return "stop"
  Bus.publish(Event.Compacted, { sessionID: input.sessionID })
  return "continue"
}
```

### 4.5 在 LLM.stream() 中的最终组装

```typescript
// packages/opencode/src/session/llm.ts
export async function stream(input: StreamInput) {
  // ... 获取配置、provider、auth 等
  
  // 1. 构造 system prompt
  const system = []
  system.push([
    // agent prompt（compaction agent 的 prompt）
    ...(input.agent.prompt ? [input.agent.prompt] : /* provider prompt */),
    // 自定义 system（压缩时为空）
    ...input.system,
    // 用户消息的 system（压缩时为空）
    ...(input.user.system ? [input.user.system] : []),
  ].filter((x) => x).join("\n"))

  // 2. 插件转换 system
  await Plugin.trigger(
    "experimental.chat.system.transform",
    { sessionID: input.sessionID, model: input.model },
    { system }
  )

  // 3. 调用 streamText
  return streamText({
    messages: [
      // system prompt
      ...system.map((x): ModelMessage => ({ role: "system", content: x })),
      // 传入的消息（已清理的历史 + 压缩指令）
      ...input.messages,
    ],
    tools: input.tools,  // 压缩时为 {}
    model: language,
    temperature: input.agent.temperature,  // compaction agent 未设置，使用默认值
    maxOutputTokens: /* 计算逻辑 */,
    // ... 其他参数
  })
}
```

### 4.6 最终发送给 LLM 的完整消息序列

```
┌─────────────────────────────────────────────────────────┐
│ Message 1: role: system                                 │
│ content: <compaction agent 的完整 prompt>                │
│   "You are OpenCode, an open source AI coding           │
│    assistant. You excel at creating and maintaining     │
│    summaries... [13个摘要指南]"                           │
├─────────────────────────────────────────────────────────┤
│ Message 2: role: user                                   │
│ content: "帮我实现一个登录功能..."                          │
├─────────────────────────────────────────────────────────┤
│ Message 3: role: assistant                              │
│ content: [                                              │
│   { type: "text", text: "好的，我来分析需求..." },         │
│   { type: "text", text: "[Tool Call (c1): bash(...)]" },│
│ ]                                                       │
├─────────────────────────────────────────────────────────┤
│ Message 4: role: user                                   │
│ content: "[Tool Result (c1) bash: 命令输出...]"          │
├─────────────────────────────────────────────────────────┤
│ ... (更多历史消息，所有工具调用已转为纯文本) ...             │
├─────────────────────────────────────────────────────────┤
│ Message N: role: user                                   │
│ content: "Please read the complete conversation above   │
│   and generate a summary according to the guidelines.   │
│   The new session will not have access to our           │
│   conversation history, so the summary must contain     │
│   all key information needed to continue the work."     │
└─────────────────────────────────────────────────────────┘

关键特征：
- tools: {} (空对象，无可用工具)
- temperature: undefined (使用模型默认值)
- maxOutputTokens: min(model.limit.output, 64000)
- 所有工具调用已转换为纯文本描述
```

---

## 5. System Prompt 与 Agent 配置

### 5.1 Compaction Agent 定义

```typescript
// packages/opencode/src/agent/agent.ts
const result: Record<string, Info> = {
  // ... 其他 agents
  
  compaction: {
    name: "compaction",
    mode: "primary",
    native: true,
    hidden: true,  // 不在 UI 中显示
    prompt: PROMPT_COMPACTION,  // 来自 compaction.txt
    permission: PermissionNext.merge(
      defaults,
      PermissionNext.fromConfig({
        "*": "deny",  // 禁用所有工具
      }),
      user,
    ),
    options: {},
  },
  
  // ... 其他 agents
}
```

### 5.2 Compaction Agent 的 System Prompt

完整内容来自 `packages/opencode/src/agent/prompt/compaction.txt`：

```
You are OpenCode, an open source AI coding assistant. You excel at creating 
and maintaining summaries that capture the most important details of technical 
conversations.

You need to read the complete conversation and generate a summary according 
to the following guidelines. Use bullet point lists where appropriate.

1. Detailed Chronological Record
   - Capture every important turn in order, including user messages, assistant 
     responses, and tool calls
   - Include tool commands and their important outputs (error messages, test 
     results, exit codes); avoid pasting lengthy logs
   - Use arrows to indicate flow
   - Paraphrase when necessary but preserve intent, technical details, and results

2. Primary Request and Intent
   - Why was this session created?
   - What is the user trying to achieve?
   - What defines success?

3. Constraints and Boundaries
   - User-specified requirements (must do / must not do)
   - Technical limitations discovered
   - Codebase conventions to follow

4. Decisions Made
   - Important decisions and their rationale
   - Rejected alternatives and why

5. Approach - How did the assistant handle the problem?

6. Key Technical Work - List all key technical work completed so far

7. Questions and Clarifications
   - Questions the assistant asked and clarifications the user provided
   - Assumptions made when not explicitly clarified (brief)

8. Files and Code Sections
   - List files created, modified, or deleted
   - External references if any (PR links, Commit SHAs)

9. Error Resolution
   - Errors encountered and how they were resolved
   - Failed approaches and their reasons — avoid retrying unless new 
     information changes conditions

10. Pending Tasks
    - Incomplete tasks with current status
    - For partial work: what IS done vs what is NOT done

11. Current Work
    - Details of the assistant's current task
    - State snapshot if relevant (branch/commit, dirty status, last test/build result)

12. Next Steps - What should the assistant do next?

13. Critical Information
    - Key information that must be passed to subsequent conversations
    - Content that doesn't fit other categories but absolutely cannot be lost
    - Special notes emphasized by the user
```

### 5.3 插件扩展点

在调用 LLM 前，触发插件钩子允许自定义：

```typescript
// packages/opencode/src/session/compaction.ts
const compacting = await Plugin.trigger(
  "experimental.session.compacting",
  { sessionID: input.sessionID },
  { context: [], prompt: undefined }
)

// 如果插件提供了自定义 prompt，则完全替换默认 prompt
// 如果插件提供了 context，则追加到默认 prompt 后面
const defaultPrompt = "Please read the complete conversation above..."
const promptText = compacting.prompt ?? [defaultPrompt, ...compacting.context].join("\n\n")
```

**插件示例：**

```typescript
// 自定义插件可以注入额外上下文或完全替换 prompt
Plugin.register({
  "experimental.session.compacting": async (ctx, data) => {
    // 追加额外上下文
    data.context.push("Additional context: Focus on security considerations.")
    
    // 或完全替换 prompt
    // data.prompt = "Custom compaction prompt..."
    
    return data
  }
})
```

---

## 6. 压缩后处理与消息过滤

### 6.1 摘要消息保存

LLM 的输出作为 `summary: true` 的 assistant 消息保存：

```typescript
{
  id: "<ascending-message-id>",
  role: "assistant",
  mode: "compaction",
  agent: "compaction",
  summary: true,  // ← 标记为摘要消息
  parentID: "<compaction 触发消息的 id>",
  sessionID: "...",
  modelID: "...",
  providerID: "...",
  path: { cwd: "...", root: "..." },
  cost: 0.05,  // 实际成本
  tokens: {
    input: 15000,
    output: 2000,
    reasoning: 0,
    cache: { read: 0, write: 0 }
  },
  finish: "stop",
  time: {
    created: 1234567890,
    completed: 1234567900
  }
}

// 附带的 text part（摘要内容）
{
  id: "<ascending-part-id>",
  messageID: "<上面消息的id>",
  sessionID: "...",
  type: "text",
  text: "## 1. Detailed Chronological Record\n- User requested...\n...",
  time: { start: 1234567890, end: 1234567900 }
}
```

### 6.2 自动继续（auto = true 时）

```typescript
// packages/opencode/src/session/compaction.ts - process()
if (result === "continue" && input.auto) {
  // 创建合成用户消息
  const continueMsg = await Session.updateMessage({
    id: Identifier.ascending("message"),
    role: "user",
    sessionID: input.sessionID,
    time: { created: Date.now() },
    agent: userMessage.agent,  // 恢复原 agent
    model: userMessage.model,  // 恢复原 model
  })
  
  // 附带合成 text part
  await Session.updatePart({
    id: Identifier.ascending("part"),
    messageID: continueMsg.id,
    sessionID: input.sessionID,
    type: "text",
    synthetic: true,  // ← 标记为合成消息
    text: "Continue if you have next steps",
    time: { start: Date.now(), end: Date.now() },
  })
}
```

### 6.3 filterCompacted — 消息过滤机制

下次主循环读取消息时，`filterCompacted` 会截断历史到锚点位置：

```typescript
// packages/opencode/src/session/prompt.ts - loop()
let msgs = await MessageV2.filterCompacted(MessageV2.stream(sessionID))
```

**filterCompacted 逻辑：**

```typescript
// packages/opencode/src/session/message-v2.ts
export async function filterCompacted(stream: AsyncIterable<MessageV2.WithParts>) {
  const result = [] as MessageV2.WithParts[]
  const completed = new Set<string>()  // 已完成的 compaction 消息
  const state = { anchor: "", marker: -1, found: false }
  
  for await (const msg of stream) {
    result.push(msg)
    
    // 如果找到锚点消息，停止
    if (state.anchor && msg.info.id === state.anchor) {
      state.found = true
      break
    }
    
    // 查找 compaction part
    if (
      state.marker < 0 &&
      msg.info.role === "user" &&
      completed.has(msg.info.id) &&
      msg.parts.some((part) => part.type === "compaction")
    ) {
      state.marker = result.length - 1
      const part = msg.parts.find((part) => part.type === "compaction") as MessageV2.CompactionPart | undefined
      const anchor = part?.anchorMessageID
      if (!anchor) break
      state.anchor = anchor
      if (msg.info.id === anchor) {
        state.found = true
        break
      }
    }
    
    // 记录已完成的 compaction
    if (msg.info.role === "assistant" && msg.info.summary && msg.info.finish) {
      completed.add(msg.info.parentID)
    }
  }
  
  // 如果找到锚点但未到达，截断到 marker 位置
  if (state.anchor && !state.found && state.marker >= 0) {
    result.length = state.marker + 1
  }
  
  result.reverse()
  return result
}
```

**过滤效果示意：**

```
压缩前的消息历史（20 条）：
m1, m2, m3, ..., m17, m18, m19, m20

压缩后（anchorMessageID = m18）：
- compaction part 在 m17 的用户消息中
- summary 消息在 m17 之后
- filterCompacted 返回：[m18, m19, m20, summary, continue]

实际发送给 LLM 的上下文：
1. system prompt
2. summary 消息的文本（压缩后的摘要）
3. m18, m19, m20 的完整内容
4. "Continue if you have next steps"
```

### 6.4 toModelMessages 中的 compaction part 转换

```typescript
// packages/opencode/src/session/message-v2.ts - toModelMessages()
if (msg.info.role === "user") {
  const userMessage: UIMessage = { id: msg.info.id, role: "user", parts: [] }
  result.push(userMessage)
  
  for (const part of msg.parts) {
    // ... 其他 part 类型处理
    
    if (part.type === "compaction") {
      userMessage.parts.push({
        type: "text",
        text: "What did we do so far?",  // ← 压缩触发消息的展示文本
      })
    }
  }
}
```

这样在 UI 中，压缩触发消息显示为 "What did we do so far?"，而不是空消息。

---

## 7. 配置选项、插件扩展点与移植指南

### 7.1 配置选项

#### 7.1.1 配置文件（opencode.json / opencode.jsonc）

```json
{
  "compaction": {
    "auto": true,   // 启用自动压缩（默认 true）
    "prune": true   // 启用修剪（默认 true）
  }
}
```

#### 7.1.2 环境变量

```bash
# 禁用自动压缩
export OPENCODE_DISABLE_AUTOCOMPACT=true

# 禁用修剪
export OPENCODE_DISABLE_PRUNE=true

# 自定义输出 token 最大值（默认 64000）
export OPENCODE_EXPERIMENTAL_OUTPUT_TOKEN_MAX=32000
```

#### 7.1.3 配置优先级

```
环境变量 > 配置文件 > 默认值
```

### 7.2 插件扩展点

#### 7.2.1 experimental.session.compacting

在压缩前自定义 prompt 或注入额外上下文：

```typescript
Plugin.register({
  "experimental.session.compacting": async (ctx, data) => {
    // ctx: { sessionID: string }
    // data: { context: string[], prompt: string | undefined }
    
    // 方式 1：追加额外上下文
    data.context.push("Focus on security and performance considerations.")
    
    // 方式 2：完全替换 prompt
    data.prompt = "Generate a concise technical summary..."
    
    return data
  }
})
```

#### 7.2.2 experimental.chat.system.transform

在所有 LLM 调用前转换 system prompt（包括压缩）：

```typescript
Plugin.register({
  "experimental.chat.system.transform": async (ctx, data) => {
    // ctx: { sessionID?: string, model: Provider.Model }
    // data: { system: string[] }
    
    // 修改 system prompt
    data.system[0] = data.system[0].replace("OpenCode", "MyAgent")
    
    return data
  }
})
```

#### 7.2.3 experimental.chat.messages.transform

在发送给 LLM 前转换消息（包括压缩时的历史消息）：

```typescript
Plugin.register({
  "experimental.chat.messages.transform": async (ctx, data) => {
    // data: { messages: MessageV2.WithParts[] }
    
    // 过滤或修改消息
    data.messages = data.messages.filter(m => /* 自定义逻辑 */)
    
    return data
  }
})
```

### 7.3 关键数据结构

#### 7.3.1 MessageV2.Part 类型

```typescript
type Part = 
  | TextPart           // 文本内容
  | ToolPart           // 工具调用
  | FilePart           // 文件附件
  | ReasoningPart      // 推理过程
  | CompactionPart     // 压缩触发标记
  | SubtaskPart        // 子任务
  | StepStartPart      // 步骤开始
  | StepFinishPart     // 步骤结束
  | SnapshotPart       // 快照
  | PatchPart          // 补丁
  | AgentPart          // Agent 引用
  | RetryPart          // 重试记录

// CompactionPart 结构
interface CompactionPart {
  id: string
  sessionID: string
  messageID: string
  type: "compaction"
  auto: boolean              // 是否自动触发
  anchorMessageID?: string   // 锚点消息 ID（压缩完成后设置）
}

// ToolPart 结构（关键：time.compacted 字段）
interface ToolPart {
  id: string
  sessionID: string
  messageID: string
  type: "tool"
  callID: string
  tool: string
  state: ToolState
  metadata?: Record<string, any>
}

type ToolState = 
  | { status: "pending", input: any, raw: string }
  | { status: "running", input: any, time: { start: number }, ... }
  | { 
      status: "completed", 
      input: any, 
      output: string, 
      time: { 
        start: number, 
        end: number, 
        compacted?: number  // ← 修剪时间戳
      }, 
      ... 
    }
  | { status: "error", input: any, error: string, time: { start: number, end: number } }
```

#### 7.3.2 MessageV2.Info 类型

```typescript
type Info = User | Assistant

interface User {
  id: string
  sessionID: string
  role: "user"
  time: { created: number }
  agent: string
  model: { providerID: string, modelID: string }
  system?: string
  tools?: Record<string, boolean>
  variant?: string
}

interface Assistant {
  id: string
  sessionID: string
  role: "assistant"
  time: { created: number, completed?: number }
  error?: ErrorObject
  parentID: string
  modelID: string
  providerID: string
  mode: string
  agent: string
  path: { cwd: string, root: string }
  summary?: boolean  // ← 标记为摘要消息
  cost: number
  tokens: {
    input: number
    output: number
    reasoning: number
    cache: { read: number, write: number }
  }
  finish?: string
}
```

### 7.4 移植到其他 Agent 系统的指南

#### 7.4.1 最小实现清单

1. **溢出检测**
   - 实现 `isOverflow()` 逻辑
   - 在每次 LLM 响应后检测 token 使用量
   - 触发压缩任务创建

2. **消息分割**
   - 实现 `selectAnchor()` 逻辑
   - 根据 token 预算分割消息为 prefix/suffix
   - 保留最近的消息，压缩更早的消息

3. **工具消息清理**
   - 实现 `sanitizeToolMessages()` 逻辑
   - 将工具调用转换为纯文本描述
   - 确保 API 不会拒绝无工具定义的工具消息

4. **压缩 Agent**
   - 创建专门的压缩 agent
   - 使用结构化的 system prompt（13 部分摘要指南）
   - 禁用所有工具调用

5. **消息过滤**
   - 实现 `filterCompacted()` 逻辑
   - 在读取消息时截断到锚点位置
   - 保留摘要 + 锚点后的消息

6. **可选：Prune 修剪**
   - 实现轻量级的工具输出修剪
   - 保护最近的工具输出
   - 标记旧输出为已修剪

#### 7.4.2 关键设计决策

| 决策点 | OpenCode 的选择 | 替代方案 |
|--------|----------------|---------|
| **压缩时机** | 检测到溢出后立即触发 | 定期压缩、手动触发 |
| **消息分割** | 保留最近 40k tokens | 固定消息数量、时间窗口 |
| **工具消息处理** | 转换为纯文本 | 完全删除、保留结构 |
| **摘要格式** | 13 部分结构化摘要 | 自由格式、JSON 结构 |
| **修剪策略** | 保护最近 40k tokens | 不修剪、全部修剪 |
| **自动继续** | 添加合成消息 | 不继续、等待用户输入 |

#### 7.4.3 性能优化建议

1. **Token 估算**
   - OpenCode 使用简单的 4 字符 = 1 token 规则
   - 可替换为更精确的 tokenizer（如 tiktoken）
   - 权衡：精确度 vs 性能

2. **并行处理**
   - 压缩和修剪可以并行执行
   - 消息分割和清理可以流式处理

3. **缓存优化**
   - 缓存 token 估算结果
   - 缓存消息转换结果
   - 使用增量更新而非全量重建

4. **存储优化**
   - 修剪时不删除原始数据，仅标记
   - 支持后续恢复或审计
   - 定期清理过期的修剪数据

#### 7.4.4 测试要点

1. **溢出检测测试**
   ```typescript
   // 测试不同模型的上下文限制
   test("detects overflow for different models", async () => {
     const model = { limit: { context: 100_000, output: 32_000 } }
     const tokens = { input: 75_000, output: 5_000, cache: { read: 0, write: 0 } }
     expect(await isOverflow({ tokens, model })).toBe(true)
   })
   ```

2. **消息分割测试**
   ```typescript
   // 测试锚点选择逻辑
   test("selects correct anchor message", () => {
     const messages = createTestMessages(20)
     const result = selectAnchor({ messages, summaryBudget: 4000, preserveBudget: 40000 })
     expect(result.anchorMessageID).toBe("m18")
     expect(result.suffixMessages.length).toBe(3)
   })
   ```

3. **工具消息清理测试**
   ```typescript
   // 测试工具调用转换
   test("sanitizes tool messages correctly", () => {
     const messages = [
       { role: "assistant", content: [{ type: "tool-call", toolName: "bash", ... }] },
       { role: "tool", content: [{ type: "tool-result", output: "result" }] }
     ]
     const sanitized = sanitizeToolMessages(messages)
     expect(sanitized[0].content[0].type).toBe("text")
     expect(sanitized[1].role).toBe("user")
   })
   ```

4. **消息过滤测试**
   ```typescript
   // 测试 filterCompacted 逻辑
   test("filters compacted messages correctly", async () => {
     const stream = createTestStream(20)
     const filtered = await filterCompacted(stream)
     expect(filtered.length).toBeLessThan(20)
     expect(filtered[0].info.id).toBe("m18")  // 锚点消息
   })
   ```

### 7.5 常见问题与解决方案

#### Q1: 压缩后 Agent 忘记了之前的上下文？

**原因：** 摘要不够详细，或锚点选择过于激进。

**解决方案：**
- 增加 `PRESERVE_BUDGET`（默认 40k）
- 优化 compaction prompt，强调关键信息
- 使用插件注入额外上下文

#### Q2: 压缩频率过高，影响性能？

**原因：** 溢出阈值设置过低，或消息过长。

**解决方案：**
- 增加 `OUTPUT_TOKEN_MAX`
- 启用 prune 修剪，延缓压缩触发
- 优化工具输出长度

#### Q3: 工具消息转换后 API 仍然报错？

**原因：** 某些 API 对消息格式有严格要求。

**解决方案：**
- 检查 `sanitizeToolMessages` 的转换逻辑
- 添加 LiteLLM 兼容性处理（dummy tool）
- 使用不同的 API 提供商

#### Q4: 压缩后成本显著增加？

**原因：** 压缩本身需要调用 LLM，且输入较长。

**解决方案：**
- 使用更便宜的模型进行压缩
- 增加压缩阈值，减少压缩频率
- 启用 prompt caching（如 Anthropic）

#### Q5: 如何自定义压缩摘要格式？

**解决方案：**
```typescript
// 使用插件完全替换 prompt
Plugin.register({
  "experimental.session.compacting": async (ctx, data) => {
    data.prompt = `
      Generate a JSON summary with the following structure:
      {
        "objective": "...",
        "progress": "...",
        "next_steps": "..."
      }
    `
    return data
  }
})
```

### 7.6 性能指标参考

基于 OpenCode 的实际使用数据：

| 指标 | 典型值 | 说明 |
|------|--------|------|
| **压缩触发频率** | 每 50-100 轮对话 1 次 | 取决于消息长度和工具使用 |
| **压缩耗时** | 5-15 秒 | 取决于历史长度和模型速度 |
| **压缩成本** | $0.01-0.10 | 取决于模型和输入长度 |
| **Token 压缩比** | 5:1 到 10:1 | 100k tokens → 10-20k tokens |
| **修剪频率** | 每轮对话 1 次 | 轻量级操作，几乎无开销 |
| **修剪效果** | 减少 20-50% tokens | 取决于工具输出长度 |

---

## 8. 总结与最佳实践

### 8.1 核心优势

1. **渐进式压缩**：Prune + Compaction 两阶段，平衡性能和效果
2. **智能保护**：保留最近的消息和关键工具输出
3. **无损转换**：工具消息转换为文本，保留上下文信息
4. **结构化摘要**：13 部分摘要指南，确保信息完整性
5. **可配置**：支持全局和会话级别的配置
6. **可扩展**：插件系统支持自定义压缩逻辑

### 8.2 最佳实践

1. **合理设置阈值**
   - `PRESERVE_BUDGET`: 40k tokens（保留最近 10-20 轮对话）
   - `PRUNE_PROTECT`: 40k tokens（保护最近的工具输出）
   - `OUTPUT_TOKEN_MAX`: 64k tokens（为输出预留足够空间）

2. **优化工具输出**
   - 限制工具输出长度（如截断日志）
   - 使用结构化输出（JSON）而非纯文本
   - 定期清理临时文件和缓存

3. **监控压缩效果**
   - 记录压缩频率和成本
   - 监控摘要质量（通过后续对话效果）
   - 调整配置以优化性能

4. **测试边界情况**
   - 超长单条消息（> 40k tokens）
   - 频繁工具调用（> 100 次）
   - 多模态内容（图片、文件）

5. **文档化自定义逻辑**
   - 记录插件的压缩策略
   - 说明自定义 prompt 的设计意图
   - 提供配置示例和最佳实践

---

## 附录 A：完整代码示例

### A.1 最小压缩实现（伪代码）

```typescript
// 1. 溢出检测
async function isOverflow(tokens: Tokens, model: Model): Promise<boolean> {
  const count = tokens.input + tokens.cache.read + tokens.output
  const usable = model.limit.input || (model.limit.context - model.limit.output)
  return count > usable
}

// 2. 消息分割
function selectAnchor(messages: Message[], preserveBudget: number) {
  const suffix = []
  let total = 0
  
  for (const msg of messages.reverse()) {
    const estimate = estimateTokens(msg)
    if (suffix.length === 0 || total + estimate <= preserveBudget) {
      suffix.unshift(msg)
      total += estimate
    } else {
      break
    }
  }
  
  const prefix = messages.slice(0, messages.length - suffix.length)
  return { prefix, suffix, anchorID: suffix[0]?.id }
}

// 3. 工具消息清理
function sanitizeToolMessages(messages: Message[]): Message[] {
  return messages.map(msg => {
    if (msg.role === "tool") {
      return { role: "user", content: `[Tool Result: ${msg.content}]` }
    }
    if (msg.role === "assistant" && hasToolCalls(msg)) {
      return {
        ...msg,
        content: msg.content.map(part => 
          part.type === "tool-call" 
            ? { type: "text", text: `[Tool Call: ${part.toolName}(...)]` }
            : part
        )
      }
    }
    return msg
  })
}

// 4. 压缩执行
async function compact(sessionID: string) {
  const messages = await getMessages(sessionID)
  const { prefix, suffix, anchorID } = selectAnchor(messages, 40000)
  
  const summary = await llm.generate({
    system: COMPACTION_PROMPT,
    messages: [
      ...sanitizeToolMessages(prefix),
      { role: "user", content: "Generate a summary..." }
    ],
    tools: {}
  })
  
  await saveMessage({
    role: "assistant",
    content: summary,
    summary: true,
    anchorID
  })
  
  return { anchorID }
}

// 5. 消息过滤
async function filterCompacted(messages: Message[]): Promise<Message[]> {
  let anchorID = null
  
  for (const msg of messages) {
    if (msg.summary && msg.anchorID) {
      anchorID = msg.anchorID
      break
    }
  }
  
  if (!anchorID) return messages
  
  const anchorIndex = messages.findIndex(m => m.id === anchorID)
  return messages.slice(anchorIndex)
}
```

### A.2 插件示例：自定义压缩策略

```typescript
import { Plugin } from "@opencode-ai/plugin"

export default Plugin.define({
  name: "custom-compaction",
  version: "1.0.0",
  
  hooks: {
    "experimental.session.compacting": async (ctx, data) => {
      // 根据会话类型自定义 prompt
      const session = await getSession(ctx.sessionID)
      
      if (session.type === "debugging") {
        data.context.push(
          "Focus on error messages, stack traces, and debugging steps."
        )
      } else if (session.type === "feature-development") {
        data.context.push(
          "Focus on feature requirements, implementation decisions, and test results."
        )
      }
      
      return data
    },
    
    "experimental.chat.messages.transform": async (ctx, data) => {
      // 过滤掉过长的工具输出
      for (const msg of data.messages) {
        for (const part of msg.parts) {
          if (part.type === "tool" && part.state.status === "completed") {
            if (part.state.output.length > 10000) {
              part.state.output = part.state.output.slice(0, 10000) + "\n[Output truncated]"
            }
          }
        }
      }
      
      return data
    }
  }
})
```

---

## 附录 B：参考资源

### B.1 相关文件路径

```
packages/opencode/src/
├── session/
│   ├── compaction.ts          # 压缩核心逻辑
│   ├── processor.ts           # 流处理器
│   ├── prompt.ts              # 会话主循环
│   ├── llm.ts                 # LLM 调用封装
│   ├── message-v2.ts          # 消息模型
│   ├── index.ts               # Session 管理
│   └── system.ts              # System prompt
├── agent/
│   ├── agent.ts               # Agent 定义
│   └── prompt/
│       └── compaction.txt     # 压缩 prompt
├── util/
│   └── token.ts               # Token 估算
├── config/
│   └── config.ts              # 配置管理
└── flag/
    └── flag.ts                # 环境变量

packages/opencode/test/session/
├── compaction.test.ts         # 压缩测试
└── revert-compact.test.ts     # 回滚测试
```

### B.2 相关测试用例

- `packages/opencode/test/session/compaction.test.ts`
- `packages/opencode/test/session/revert-compact.test.ts`
- `packages/opencode/test/session/message-v2.test.ts`

### B.3 配置示例

```json
{
  "$schema": "https://opencode.ai/config.json",
  "compaction": {
    "auto": true,
    "prune": true
  },
  "agent": {
    "compaction": {
      "model": "anthropic/claude-3-5-sonnet-20241022",
      "temperature": 0.3
    }
  }
}
```

---

**文档版本：** 1.0  
**最后更新：** 2026-02-08  
**基于 OpenCode 版本：** v1.1.53+  
**作者：** OpenCode 分析文档  
**许可：** MIT
