# 转换器 (Transformer)

转换器是 AgentGear 的核心功能，用于对 API 请求和响应进行转换处理。

## 基本配置

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

### 配置字段说明

| 字段 | 说明 |
|------|------|
| `name` | 转换器名称，唯一标识 |
| `enabled` | 是否启用 |
| `tags` | 绑定的标签列表，匹配任一标签即生效 |
| `source_tool` | 源工具名称（需要被转换的工具） |
| `target_tool` | 目标工具名称（转换后的工具） |
| `accumulate` | 是否需要累积流式 delta 后再转换 |
| `param_mapping` | 参数映射规则 |

## 流式处理注意事项

流式响应需要累积工具调用的所有 delta 后再转换。详见 [streaming.md](./streaming.md)。

## content_replace 类型：响应文本替换

用于对上游响应中的文本内容进行匹配替换，常用于移除渠道注入的广告或水印文本。

### 配置字段

| 字段 | 说明 |
|------|------|
| `match` | 匹配标记文本，如 `【广告】` |
| `replace_with` | 替换为的内容（空字符串=删除） |
| `trim_after` | 匹配标记后，向后扩展到该分隔符并一并删除（如 `\n\n`） |

### 配置示例

```yaml
transformers:
  definitions:
    - name: "remove_channel_ad"
      type: "content_replace"
      direction: "response"
      content_patterns:
        - match: "【广告】"
          trim_after: "\n\n"
          replace_with: ""
  mappings:
    - name: "channel_ad_removal"
      enabled: true
      tags: ["$g_my_gateway"]
      transformer: "remove_channel_ad"
```

### 工作原理

- 在流式 SSE 响应中，对每个 text content block 的第一个 `text_delta` 进行检测
- 找到 `match` 标记后，从标记位置开始，向后查找 `trim_after` 分隔符，将整段（含尾部空行）替换为 `replace_with`
- 后续 delta 直接透传，不再检测

## error_transform 类型：触发 Agent 自身压缩

通过返回 `context_length_exceeded` 错误，触发 Agent（如 Droid）自身的压缩机制。

### 内置模板 `$tpl_force_hint_context_length_exceeded`

系统提供了模板，用户可基于模板创建多个实例，为不同 Agent + Upstream 组合配置不同的触发策略。

### 模板参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `error_code` | 返回的错误码 | `context_length_exceeded` |
| `error_message` | 返回的错误消息 | `prompt is too long: request size exceeds limit` |
| `request_size_threshold` | 响应后兜底检测阈值（字节） | `500000` |
| `context_token_limit` | token 限制 | `200000` |
| `context_threshold_ratio` | 触发比例（0-1） | `0.7` |
| `token_estimate_ratio` | 字节到 token 的估算比率 | `3.5` |
| `image_token_estimate` | 单张图片估算 token 数 | `1600` |

### 配置示例

```yaml
transformers:
  definitions:
    - name: "cursor_kiro_force_compress"
      description: "Cursor+Kiro 触发压缩（60% 阈值）"
      template_ref: "$tpl_force_hint_context_length_exceeded"
      template_args:
        error_code: "context_length_exceeded"
        error_message: "prompt is too long: request size exceeds limit"
        request_size_threshold: "500000"
        context_token_limit: "200000"
        context_threshold_ratio: "0.6"
        token_estimate_ratio: "3.5"
        image_token_estimate: "1600"

  mappings:
    - name: "cursor_kiro_force_compress_mapping"
      enabled: true
      tags: ["$a_cursor", "$u_kiro"]
      transformer: "cursor_kiro_force_compress"
```

### 内置实例

- `$droid_upstream_kiro_force_compress`：Droid + Kiro 场景，70% 阈值，200K token 限制
- 对应 mapping：`$droid_upstream_kiro_force_compress_mapping`（tags: `$a_droid` + `$u_kiro`）

## cache_strip 类型：缓存 Token 擦除

用于擦除 Anthropic 协议响应中的缓存 token 计数，将缓存读取的 token 数量合并到输入 token 中。适用于不希望下游 Agent 感知到缓存命中情况的场景。

### 转换逻辑

对响应中的 `usage` 对象执行以下操作：

```
input_tokens = input_tokens + cache_read_input_tokens + cache_creation_input_tokens
cache_creation_input_tokens = 0
cache_read_input_tokens = 0
```

### 作用范围

- 流式响应：`message_start` 事件中的 `message.usage` 和 `message_delta` 事件中的 `usage`
- 非流式响应：响应体顶层 `usage` 字段

### 配置示例

```yaml
transformers:
  definitions:
    - name: "my_cache_strip"
      type: "cache_strip"
      direction: "response"

  mappings:
    - name: "my_cache_strip_mapping"
      enabled: true
      tags: ["$a_droid"]
      transformer: "my_cache_strip"
```

## 详细设计文档

- [模板化转换器设计](../模板化转换器设计.md) - 转换器模板化设计方案
- [覆盖内置转换器](../override-builtin-transformers.md) - 如何覆盖内置转换器行为
