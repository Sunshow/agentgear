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

## 详细设计文档

- [模板化转换器设计](../模板化转换器设计.md) - 转换器模板化设计方案
- [覆盖内置转换器](../override-builtin-transformers.md) - 如何覆盖内置转换器行为
