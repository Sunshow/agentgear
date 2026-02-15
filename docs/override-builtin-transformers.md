# 覆盖内置 Transformer 配置

## 概述

从当前版本开始，AgentGear 支持通过用户配置文件覆盖内置的 transformer 定义和 mapping 规则。这使得你可以快速调整内置行为（如压缩阈值）而无需修改代码。

## 工作原理

在配置加载时，用户配置中与内置配置**同名**的 transformer definition 或 mapping rule 会自动覆盖内置配置。

- **Definitions**：同名的用户定义会替换内置定义
- **Mappings**：同名的用户规则会替换内置规则

## 快速调整压缩阈值示例

### 场景 1：调整错误转换器阈值（触发 Droid 自身压缩）

在 `proxy/configs/config.yaml` 中添加：

```yaml
transformers:
  definitions:
    # 覆盖内置的 $droid_upstream_kiro_force_compress
    - name: "$droid_upstream_kiro_force_compress"
      direction: "response"
      type: "error_transform"
      error_code: "context_length_exceeded"
      error_message: "prompt is too long: request size exceeds limit"
      request_size_threshold: 500000
      context_token_limit: 200000
      context_threshold_ratio: 0.6  # 从默认 70% 调整为 60%
      token_estimate_ratio: 3.5
      model_context_limits:
        - model_pattern: "claude-opus-4-6*"
          token_limit: 1000000
        - model_pattern: "claude-opus-4.6*"
          token_limit: 1000000
```

**效果**：当请求大小超过 120K tokens（200K × 60%）时触发错误，让 Droid 自己压缩。

### 场景 2：调整自动压缩转换器阈值（AgentGear 自动压缩）

```yaml
transformers:
  definitions:
    # 覆盖内置的 $auto_compress
    - name: "$auto_compress"
      type: "compress"
      direction: "request"
      context_token_limit: 200000
      context_threshold_ratio: 0.5  # 从默认 70% 调整为 50%
      token_estimate_ratio: 3.5
      model_context_limits:
        - model_pattern: "claude-opus-4-6*"
          token_limit: 1000000
      compress_target: "same"
      compress_model: ""
      preserve_budget: 40000
      summary_budget: 4000
      auto_retry: true
      max_retries: 1
```

**效果**：当请求大小超过 100K tokens（200K × 50%）时自动压缩上下文。

### 场景 3：禁用内置 Mapping

```yaml
transformers:
  mappings:
    # 禁用内置的压缩 mapping
    - name: "$droid_upstream_kiro_force_compress_mapping"
      enabled: false
      tags: ["$a_droid", "$u_kiro"]
      transformer: "$droid_upstream_kiro_force_compress"
```

**效果**：即使有同名 transformer 定义，也不会应用到任何请求。

### 场景 4：调整分段写入提示的行数阈值

```yaml
transformers:
  definitions:
    # 覆盖内置的 $upstream_kiro_chunked_write_hint，调整行数阈值
    - name: "$upstream_kiro_chunked_write_hint"
      template_ref: "$tpl_chunked_write_hint"
      template_args:
        line_threshold: "300"  # 从默认 150 行调整为 300 行
        chunk_size: "200"      # 从默认 100 行调整为 200 行
```

**效果**：当文件超过 300 行时提示分段写入，每块约 200 行。内置 mapping 自动生效，无需额外配置。

## 阈值参考表

| 阈值 | 200K 模型触发点 | 1M 模型触发点 | 适用场景 |
|------|----------------|--------------|---------|
| 0.5  | 100K tokens    | 500K tokens  | 激进压缩，频繁触发 |
| 0.6  | 120K tokens    | 600K tokens  | 较早压缩 |
| 0.7  | 140K tokens    | 700K tokens  | 默认配置 |
| 0.8  | 160K tokens    | 800K tokens  | 保守压缩 |
| 0.9  | 180K tokens    | 900K tokens  | 接近上限才压缩 |

## 验证配置生效

### 1. 查看当前生效的配置

```bash
curl http://localhost:9001/api/transformers | jq '.[] | select(.name | contains("compress"))'
```

### 2. 监控日志

```bash
tail -f proxy/logs/agentgear.log | grep -E "compress check|compression triggered|Preemptive context"
```

### 3. 重启服务

修改配置后需要重启服务：

```bash
cd proxy
./agentgear  # 或 docker-compose restart
```

## 注意事项

1. **完整配置**：覆盖时需要提供完整的 transformer 定义，不支持部分字段覆盖
2. **重启生效**：配置修改后需要重启服务才能生效
3. **API 保护**：通过 API 仍然无法修改内置配置，只能通过配置文件覆盖
4. **同名唯一**：每个 transformer 名称在最终配置中只会出现一次

## 内置 Transformer 列表

可以覆盖的内置 transformer：

### Error Transform 类型
- `$droid_upstream_kiro_force_compress` - Droid + Kiro 上下文超限检测（基于模板 `$tpl_force_hint_context_length_exceeded`）
- `$input_too_long_error_transform` - 上游错误模式匹配转换

### Compress 类型
- `$auto_compress` - 通用自动上下文压缩

### Tool 类型
- `$droid_warp_Write_to_ExitSpecMode` - Write 转 ExitSpecMode
- `$droid_warp_Edit_normalize` - Edit 参数标准化
- `$droid_warp_Glob_normalize` - Glob 参数标准化
- `$droid_warp_Bash_to_Execute` - Bash 转 Execute
- `$droid_warp_Write_to_Create` - Write 转 Create
- `$droid_warp_Create_to_Write` - Create 转 Write
- `$droid_warp_Execute_to_Bash` - Execute 转 Bash

### Message Inject 类型
- `$upstream_kiro_chunked_write_hint` - 分段写入提示（基于模板 `$tpl_chunked_write_hint`，默认 150 行/100 行分块）

### 模板列表

可用于创建自定义实例的内置模板：

- `$tpl_tool_alias` - 工具别名模板（参数：`direction`, `source`, `target`）
- `$tpl_chunked_write_hint` - 分段写入提示模板（参数：`line_threshold`, `chunk_size`）
- `$tpl_force_hint_context_length_exceeded` - 上下文超限提示模板（参数：`error_code`, `error_message`, `request_size_threshold`, `context_token_limit`, `context_threshold_ratio`, `token_estimate_ratio`, `image_token_estimate`）

## 使用模板自定义分段写入提示

内置的 `$upstream_kiro_chunked_write_hint` 默认提示文件超过 150 行时分段写入，每块约 100 行。你可以通过模板创建自定义实例，配置不同的阈值。

### 示例：自定义行数阈值

```yaml
transformers:
  definitions:
    # 更激进的阈值：100 行触发，每块 50 行
    - name: "my_chunked_write_aggressive"
      template_ref: "$tpl_chunked_write_hint"
      template_args:
        line_threshold: "100"
        chunk_size: "50"

    # 更宽松的阈值：200 行触发，每块 150 行
    - name: "my_chunked_write_relaxed"
      template_ref: "$tpl_chunked_write_hint"
      template_args:
        line_threshold: "200"
        chunk_size: "150"

  mappings:
    # 为特定 Agent 使用激进配置
    - name: "my_agent_chunked_write"
      enabled: true
      tags: ["$a_my_agent", "$u_kiro"]
      tools: ["Create", "Write"]
      tool_op: "any"
      transformer: "my_chunked_write_aggressive"

    # 禁用内置的默认分段写入提示（可选）
    - name: "$upstream_kiro_chunked_write"
      enabled: false
      tags: ["$u_kiro"]
      tools: ["Create", "Write"]
      tool_op: "any"
      transformer: "$upstream_kiro_chunked_write_hint"
```

### 模板参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `line_threshold` | 文件超过多少行时触发分段写入 | 150 |
| `chunk_size` | 每次 Edit 调用的建议行数 | 100 |

> **注意**：模板中的 `{{tool}}` 占位符会在实际注入时根据匹配的工具名自动替换，无需在 `template_args` 中配置。

## 使用模板自定义上下文超限提示

内置的 `$droid_upstream_kiro_force_compress` 默认绑定 `$a_droid` + `$u_kiro` 标签，70% 阈值触发。你可以通过模板为其他 Agent 创建新的实例。

### 示例：为不同 Agent 创建不同的触发策略

```yaml
transformers:
  definitions:
    # Cursor 使用 60% 阈值（更早触发）
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

    # OpenCode 使用 80% 阈值（更晚触发）
    - name: "opencode_force_compress"
      description: "OpenCode 触发压缩（80% 阈值）"
      template_ref: "$tpl_force_hint_context_length_exceeded"
      template_args:
        error_code: "context_length_exceeded"
        error_message: "context window exceeded"
        request_size_threshold: "300000"
        context_token_limit: "200000"
        context_threshold_ratio: "0.8"
        token_estimate_ratio: "3.5"
        image_token_estimate: "1600"

  mappings:
    - name: "cursor_kiro_force_compress_mapping"
      enabled: true
      tags: ["$a_cursor", "$u_kiro"]
      transformer: "cursor_kiro_force_compress"

    - name: "opencode_force_compress_mapping"
      enabled: true
      tags: ["$a_opencode", "$g_my_gateway"]
      transformer: "opencode_force_compress"
```

### 模板参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `error_code` | 返回的错误码 | 无（必填） |
| `error_message` | 返回的错误消息 | 无（必填） |
| `request_size_threshold` | 响应后兜底检测阈值（字节） | 无（必填） |
| `context_token_limit` | token 限制 | 无（必填） |
| `context_threshold_ratio` | 触发比例（0-1），越小越早触发 | 无（必填） |
| `token_estimate_ratio` | 字节到 token 的估算比率 | 无（必填） |
| `image_token_estimate` | 单张图片估算 token 数 | 无（必填） |

### 阈值参考表

| 阈值 | 200K 模型触发点 | 适用场景 |
|------|----------------|---------|
| 0.5  | 100K tokens    | 激进，频繁触发 |
| 0.6  | 120K tokens    | 较早触发 |
| 0.7  | 140K tokens    | 默认配置 |
| 0.8  | 160K tokens    | 保守 |
| 0.9  | 180K tokens    | 接近上限才触发 |

完整列表请参考 `proxy/internal/transformer/builtin.go`。
