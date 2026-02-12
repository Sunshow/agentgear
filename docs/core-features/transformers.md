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

## 详细设计文档

- [模板化转换器设计](../模板化转换器设计.md) - 转换器模板化设计方案
- [覆盖内置转换器](../override-builtin-transformers.md) - 如何覆盖内置转换器行为
