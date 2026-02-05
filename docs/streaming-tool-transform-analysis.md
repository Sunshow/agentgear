# AgentGear 流式工具转换问题分析

## 问题现象

通过AgentGear中转后，Droid会话在中间停止，显示的最后内容在sessions日志里能找到，但工具调用没有正确转发给客户端。

## 问题定位

### 测试Session: `20260127-181748_167f98be`

**原始响应** (`001_response.body`) 包含：
1. `content_block_start` index=0 (text block)
2. text deltas（"现在我已经收集了足够的信息..."）
3. `content_block_stop` index=0
4. `content_block_start` index=1 (tool_use: `create_documents`)
5. tool input deltas (partial_json 片段)
6. `content_block_stop` index=1
7. `message_delta` (stop_reason: tool_use)
8. `message_stop`

**转换后响应** (`001_response_transformed.body`) 只有：
1. `content_block_start` index=0 (text block)
2. text deltas
3. `content_block_stop` index=0
4. `message_delta` (stop_reason: tool_use)
5. `message_stop`

**结论**：`create_documents` 工具调用没有被转换成 `ExitSpecMode` 并输出。

## 代码分析

### 当前实现逻辑

1. **`handleBlockStart`** (create_documents):
   - 创建 `accumulator`，`blockCount=0`
   - 返回 `nil`（抑制输出）

2. **`handleBlockDelta`** (partial_json):
   - `block.inputParts` 累积每个片段 ✓
   - 调用 `extractAndAccumulate(partialJSON, accumulator)` 试图从每个小片段提取 title/content
   - 片段如 `{"new_documents": [{"` 无法被解析
   - `acc.titleParts` 和 `acc.contentParts` 保持为空
   - `blockCount` 始终为 0

3. **`handleBlockStop`**:
   - **没有合并 `block.inputParts` 并解析的逻辑**
   - 直接删除 block，返回 `nil`

4. **`message_stop` 处理**:
   - 检查 `blockCount > 0`，失败（因为是0）
   - 不调用 `flushAccumulator`
   - 工具调用丢失

### 可能的设计意图

现有的 `extractAndAccumulate` 实现尝试从每个 partial_json 片段中提取内容，这可能是为了适配 **WARP 返回的 create_documents 格式**。WARP 可能以不同的方式发送 partial_json，比如：
- 每个 delta 是完整的 JSON 对象
- 或者 title/content 值在单个 delta 中完整出现

而当前上游（Anthropic API 直接返回）的 partial_json 是逐字符/逐词流式的，每个 delta 只是 JSON 的一小部分。

## 待确认

需要抓取 WARP 返回的 `create_documents` 响应格式，对比分析：
1. WARP 的 partial_json 片段格式是什么？
2. 是否每个 delta 包含完整的 title 或 content 值？
3. 现有实现是否能正确处理 WARP 响应？

## 修复方向（待WARP数据确认后）

### 方案A：在 `handleBlockStop` 中合并并解析

在 block 结束时，合并所有 `inputParts` 片段，解析完整 JSON，提取 title/content。

```go
func (h *Handler) handleBlockStop(data string, toolBlocks map[int]*toolBlockState, accumulator **toolBlockAccumulator) []sseEvent {
    // ...
    if needsAccumulate && *accumulator != nil && len(block.inputParts) > 0 {
        fullJSON := strings.Join(block.inputParts, "")
        // 解析完整 JSON 并提取 title/content
        // ...
        (*accumulator).blockCount++
    }
    // ...
}
```

### 方案B：保留现有逻辑但修复 blockCount 判断

如果 WARP 格式确实能被现有 `extractAndAccumulate` 正确处理，只需修复 `flushAccumulator` 的判断条件：

```go
// 改为检查是否有实际内容，而不是 blockCount
if acc == nil || (len(acc.titleParts) == 0 && len(acc.contentParts) == 0) {
    return nil
}
```

### 方案C：同时支持两种格式

检测 partial_json 格式，WARP格式用现有逻辑，直接API格式用合并后解析。

## 相关文件

- `internal/proxy/handler.go`: 流式响应处理和工具转换
- `internal/transformer/tool_mapper.go`: 工具映射规则
- `logs/sessions/`: 请求/响应日志

## 日期

2026-01-27
