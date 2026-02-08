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
- `$droid_upstream_kiro_force_compress` - Droid + Kiro 上下文超限检测
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
- `$upstream_kiro_chunked_write_hint` - 分段写入提示

完整列表请参考 `proxy/internal/transformer/builtin.go`。
