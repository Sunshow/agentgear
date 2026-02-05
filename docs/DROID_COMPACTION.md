# Droid 自动压缩 / 自动摘要（触发机制 + 匹配规则）

本文记录 droid（在 Windows、droid `0.56.3` 上观察到的行为）是如何决定执行会话压缩（compaction / auto-summarization）的。内容来自对 `droid.exe` 二进制中打包的 JS 片段进行字符串与控制流层面的提取与比对。

注意：二进制内的符号名已被 minify/混淆；本文中的函数名为二进制里实际可见的“最小化后的名字”。

## 总体结论（最重要的一点）

compaction 是“被动触发（reactive）”，不是“主动触发（proactive）”：

- droid 先按当前会话内容直接发起一次 LLM 请求。
- 只有当该请求返回“上下文超限/长度超限”类错误时，droid 才会执行 compaction 并重试。
- 因此不存在一个固定的“发送前 token 数达到 X 就一定会自动压缩”的阈值。

## 主要调用路径（Agent 循环）

Agent 发送请求的大致结构如下：

- 构造 conversationHistory（历史消息）与 systemMessage/tools（系统提示与工具定义）
- `try { await doSend() } catch (err) { ... }`
- 如果 `tb(err)` 为 false：直接抛错（不做 compaction）
- 如果 `tb(err)` 为 true：执行 compaction，再重新发送
  - `await zH("context_limit", { conversationHistory, systemMessage, allTools, sessionId })`
  - 用 compaction 后的会话替换原会话，随后再次 `doSend()`

其中 compaction 的 reason 文字为 `"context_limit"`（用于日志/遥测标记）。

## compaction 的实现（它做了什么）

### 核心函数

核心 compaction 逻辑函数为：

- `Jr$({ messages, system, tools, summarize, lastSummary, systemInfo, signal, thresholds? })`

它会计算一个“预算”（近似 token），选择要保留的后缀消息，对前缀进行摘要，然后把“摘要 + 后缀消息”拼回新的会话。

### 预算参数（postAbsolute / summarySoftCap / summaryReserve）

在 `Jr$` 内部，最终使用的参数为：

- `postAbsolute = thresholds?.postAbsolute ?? 40000`
- `summarySoftCap = thresholds?.summarySoftCap ?? Tr$`
- `summaryReserve = thresholds?.summaryReserve ?? Vr$`

默认常量：

- `Tr$ = 2000`
- `Vr$ = 4000`

重点：

- `thresholds.postAbsolute` 是 compaction 内部的一个可选配置字段。
- 它不是 LLM 提供商统计的 tokenizer 上下文上限。

### token 计数方式（approx，不是真实 tokenizer）

droid 在 compaction 里使用的是“近似 token”估算，不调用模型的真实 tokenizer：

- `_Y(chars) = ceil(chars / 4)`
- `Xr$(message)`：对每条 message 的 content 做字符长度统计（含 text/thinking/tool_use/tool_result 的 JSON 序列化长度等），然后除以 4。
- `Gm0(system, tools)`：对 system prompt + tool schemas 的字符长度做统计，然后除以 4。

所以 provider 日志里看到的真实 input tokens（比如 100k+）与 droid 内部的 approx token 可能差异很大。

## “什么时候触发自动压缩”的真正条件（context-limit 判定）

触发条件是：`tb(err)` 返回 true。

`tb(err)` 会尝试用“错误码/错误对象结构/错误文案”来判断是否属于“上下文超限”。它本质上是一个错误分类器，而不是一个数值阈值判断。

### 结构化错误码匹配

当 `err` 是对象且不是 `Error` 实例时（例如 SDK 返回的结构化错误对象），满足任一条件即判定为 true：

- `err.code === "context_length_exceeded"`
- `err.error.code === "context_length_exceeded"`
- 若 `err.type === "response.error"`：从 `err.message` 或 `err.error.message` 取到字符串后，进一步用 `jHI(message)` 正则判定

当 `err` 是 `Error` 实例时，满足任一条件即判定为 true：

- `err.name === "LLMContextLengthExceededError"`
- `err.code === "context_length_exceeded"`
- 若 `err.cause` 存在：
  - `err.cause.code === "context_length_exceeded"`
  - 或递归 `tb(err.cause)`
- 否则回退为 `jHI(err.message || "")`

### 错误文案/子串正则匹配（jHI）

`jHI(message)` 为 true 的条件（全部为不区分大小写匹配）：

- `/context length/i`
- `/context limit/i`
- `/exceed\s.*context\s.*limit/i`
- `/prompt is too long/i`
- `/maximum context/i`
- `/input is too long/i`
- `/over the maximum length/i`
- `/is too many tokens/i`
- `/context_length_exceeded/i`
- `/exceeds? the .*context window/i`
- `/input exceeds? the.*context/i`
- `/context window exceeds limit/i`
- `/request too large for (the )?context window/i`

### 常见“明明超限但不触发 compaction”的原因（自定义/代理场景）

有些自定义 LLM 提供商在上下文超限时会返回 HTTP 400，但错误对象形态类似：

```text
Error code: 400
type: "error"
error.type: "invalid_request_error"
error.message: "prompt is too long: ... tokens > ... maximum"
```

这种形态如果最终被 droid 当成“普通对象”（不是 `Error` 实例）抛出，并且又不满足以下任一条件：

- 顶层 `code === "context_length_exceeded"`
- `error.code === "context_length_exceeded"`
- 顶层 `type === "response.error"`（且能取到 `message` / `error.message` 字符串）

那么 `tb(err)` 可能不会进入 `jHI(message)` 的正则判断，从而导致“没有触发自动压缩/摘要”。

### 解决办法：在 proxy 中改写错误以命中 tb/jHI

如果你能在自定义后端或代理层拦截错误，可以把“提示词过长”类错误改写为 droid 能识别的形态再返回，例如任选其一：

1) 保持 HTTP 400，但补充错误码：

- 设置 `code = "context_length_exceeded"`（顶层或 `error.code`）

2) 改为 `response.error` 形态（并确保 `message` 是字符串且包含 `prompt is too long` / `context limit` 等关键字）：

- 设置顶层 `type = "response.error"`

3) 让最终抛到 droid 的异常是 `Error`，并且 `err.message` 包含 `prompt is too long`（这样会命中 `jHI`）。

## 可用于排查的日志标记

二进制内存在如下日志标记，可用于确认 compaction 是否实际发生：

- `[Compaction] Start`
- `[Compaction] Context & message tokens (approx)`
- `[Compaction] Suffix selection`
- `[Compaction] New summary created`
- `[Compaction] End`

如果你的环境中 droid 日志里从未出现这些标记，通常意味着：

- 没有发生“上下文超限”错误；或
- `tb(err)` 没识别到 provider 返回的错误形态；或
- compaction 被其他逻辑拦截/短路。

## 二进制偏移（droid.exe 0.56.3，尽力而为）

以下 offset 仅对某一 Windows 版本的 `droid.exe` 有效，不保证跨版本一致：

- `Jr$`（compaction 核心函数）：约 `0x75599f8`
- `jHI` / `tb`（上下文超限错误识别）：约 `0x7620a36`
- Agent 捕获错误并触发 compaction 的流程（调用 `zH("context_limit", ...)`）：约 `0x7765b59`
