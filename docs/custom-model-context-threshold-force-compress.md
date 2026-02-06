# 支持模型特定上下文阈值配置

## 背景
当前 Kiro 上下文限制检测使用固定的 `ContextTokenLimit: 200000`，但新模型 `claude-opus-4-6` 和 `claude-opus-4.6` 支持 1M 上下文，需要支持根据不同模型设置不同的阈值。

## 当前实现分析

### 现有机制
位于 `proxy/internal/transformer/builtin.go` 的 `$droid_upstream_kiro_force_compress` 转换器：
```go
{
    Name:                  "$droid_upstream_kiro_force_compress",
    Direction:             "response",
    Type:                  "error_transform",
    ErrorCode:             "context_length_exceeded",
    ErrorMessage:          "prompt is too long: request size exceeds limit",
    RequestSizeThreshold:  500000,  // 响应后兜底检测阈值 500KB
    ContextTokenLimit:     200000,  // 固定 200K tokens
    ContextThresholdRatio: 0.85,    // 85% 阈值
    TokenEstimateRatio:    3.5,     // 字节到 token 估算比例
}
```

### 检测逻辑
在 `handler.go` 的 `shouldPreemptContextError()` 中：
```go
estimatedTokens := float64(len(reqCtx.reqBody)) / ratio
threshold := float64(handler.ContextTokenLimit) * thresholdRatio
if estimatedTokens > threshold {
    // 返回 context_length_exceeded 错误
}
```

## 实现方案

### 1. 扩展 TransformerDef 配置结构

在 `internal/transformer/config.go` 中添加模型特定配置：

```go
type ModelContextLimit struct {
    ModelPattern string `mapstructure:"model_pattern" json:"model_pattern" yaml:"model_pattern"` // 模型匹配模式（支持通配符）
    TokenLimit   int    `mapstructure:"token_limit" json:"token_limit" yaml:"token_limit"`       // 该模型的 token 限制
}

type TransformerDef struct {
    // ... 现有字段 ...
    
    // error_transform 类型专用字段
    ContextTokenLimit     int                  // 默认 token 限制（向后兼容）
    ModelContextLimits    []ModelContextLimit  // 模型特定 token 限制（优先级更高）
    ContextThresholdRatio float64
    TokenEstimateRatio    float64
}
```

### 2. 更新内置转换器配置

在 `internal/transformer/builtin.go` 中更新，只为已知的 1M 上下文模型配置特殊阈值：

```go
{
    Name:                  "$droid_upstream_kiro_force_compress",
    Direction:             "response",
    Type:                  "error_transform",
    ErrorCode:             "context_length_exceeded",
    ErrorMessage:          "prompt is too long: request size exceeds limit",
    RequestSizeThreshold:  500000,
    ContextTokenLimit:     200000,  // 默认值（适用于大部分模型）
    ModelContextLimits: []ModelContextLimit{
        {ModelPattern: "claude-opus-4-6*", TokenLimit: 1000000},   // claude-opus-4-6, claude-opus-4-6-20260101 等
        {ModelPattern: "claude-opus-4.6*", TokenLimit: 1000000},   // claude-opus-4.6, claude-opus-4.6-20260101 等
        // 未来有新的 1M 模型时在此添加
    },
    ContextThresholdRatio: 0.85,
    TokenEstimateRatio:    3.5,
}
```

### 3. 修改检测逻辑

在 `internal/proxy/handler.go` 中：

**a) 添加模型匹配函数：**
```go
// matchModelPattern 检查模型名是否匹配模式（支持通配符 *）
func matchModelPattern(model, pattern string) bool {
    // 简单通配符匹配：* 匹配任意字符
    // 例如：claude-opus-4-6* 匹配 claude-opus-4-6, claude-opus-4-6-20260101 等
    if pattern == "" {
        return false
    }
    
    // 精确匹配
    if model == pattern {
        return true
    }
    
    // 通配符匹配
    if strings.HasSuffix(pattern, "*") {
        prefix := strings.TrimSuffix(pattern, "*")
        return strings.HasPrefix(model, prefix)
    }
    
    return false
}

// getContextTokenLimit 根据模型名获取对应的 token 限制
func getContextTokenLimit(handler *transformer.TransformerDef, model string) int {
    if model != "" && len(handler.ModelContextLimits) > 0 {
        for _, limit := range handler.ModelContextLimits {
            if matchModelPattern(model, limit.ModelPattern) {
                return limit.TokenLimit
            }
        }
    }
    // 返回默认值
    return handler.ContextTokenLimit
}
```

**b) 修改 `shouldPreemptContextError()` 函数：**
```go
func (h *Handler) shouldPreemptContextError(reqCtx *requestContext) *transformer.TransformerDef {
    // 跳过压缩请求（避免死循环）
    if h.isCompressRequest(reqCtx.reqBody) {
        return nil
    }
    
    // 获取错误转换器
    if h.transformerRegistry == nil {
        return nil
    }
    handler := h.transformerRegistry.GetErrorTransformer(reqCtx.tags)
    if handler == nil {
        return nil
    }
    
    // 从请求体中提取模型名
    var req map[string]interface{}
    var model string
    if err := json.Unmarshal(reqCtx.reqBody, &req); err == nil {
        if m, ok := req["model"].(string); ok {
            model = m
        }
    }
    
    // 根据模型获取对应的 token 限制
    contextTokenLimit := getContextTokenLimit(handler, model)
    if contextTokenLimit == 0 {
        return nil
    }
    
    // 设置默认值
    ratio := handler.TokenEstimateRatio
    if ratio == 0 {
        ratio = 3.5
    }
    thresholdRatio := handler.ContextThresholdRatio
    if thresholdRatio == 0 {
        thresholdRatio = 0.85
    }
    
    // 估算 token 数
    estimatedTokens := float64(len(reqCtx.reqBody)) / ratio
    threshold := float64(contextTokenLimit) * thresholdRatio
    
    if estimatedTokens > threshold {
        h.logger.Warn("preemptive context limit check triggered",
            zap.String("model", model),
            zap.Int("context_token_limit", contextTokenLimit),
            zap.Int("request_size", len(reqCtx.reqBody)),
            zap.Float64("estimated_tokens", estimatedTokens),
            zap.Float64("threshold", threshold),
            zap.String("transformer", handler.Name))
        return handler
    }
    
    return nil
}
```

### 4. 配置文件示例

用户可在 `configs/config.yaml` 中自定义扩展：

```yaml
transformers:
  definitions:
    - name: "custom_kiro_context_check"
      direction: "response"
      type: "error_transform"
      error_code: "context_length_exceeded"
      error_message: "prompt is too long: request size exceeds limit"
      request_size_threshold: 500000
      context_token_limit: 200000  # 默认值
      model_context_limits:
        - model_pattern: "claude-opus-4-6*"
          token_limit: 1000000
        - model_pattern: "claude-opus-4.6*"
          token_limit: 1000000
        - model_pattern: "claude-opus-5*"  # 未来新模型
          token_limit: 2000000
      context_threshold_ratio: 0.85
      token_estimate_ratio: 3.5
```

### 5. 更新配置示例文档

在 `configs/config.example.yaml` 中添加说明：

```yaml
# Example: Error transform with model-specific context limits
# - name: "kiro_context_check"
#   direction: "response"
#   type: "error_transform"
#   error_code: "context_length_exceeded"
#   error_message: "prompt is too long: request size exceeds limit"
#   request_size_threshold: 500000
#   context_token_limit: 200000  # Default limit for most models
#   model_context_limits:         # Model-specific limits (higher priority)
#     - model_pattern: "claude-opus-4-6*"   # Matches claude-opus-4-6, claude-opus-4-6-20260101, etc.
#       token_limit: 1000000
#     - model_pattern: "claude-opus-4.6*"   # Matches claude-opus-4.6, claude-opus-4.6-20260101, etc.
#       token_limit: 1000000
#   context_threshold_ratio: 0.85  # Trigger at 85% of limit
#   token_estimate_ratio: 3.5      # Bytes to tokens estimation ratio
```

## 实现步骤

1. **修改配置结构** (`internal/transformer/config.go`)
   - 添加 `ModelContextLimit` 结构体
   - 在 `TransformerDef` 中添加 `ModelContextLimits []ModelContextLimit` 字段

2. **更新内置转换器** (`internal/transformer/builtin.go`)
   - 为 `$droid_upstream_kiro_force_compress` 添加 `ModelContextLimits` 配置
   - 只配置 `claude-opus-4-6*` 和 `claude-opus-4.6*` 为 1M，其他使用默认 200K

3. **实现模型匹配逻辑** (`internal/proxy/handler.go`)
   - 添加 `matchModelPattern()` 函数（支持后缀通配符 `*`）
   - 添加 `getContextTokenLimit()` 函数（按顺序匹配，返回第一个匹配的限制）

4. **修改检测函数** (`internal/proxy/handler.go`)
   - 更新 `shouldPreemptContextError()` 提取模型名并使用动态阈值
   - 日志输出包含模型名和使用的 token 限制

5. **更新配置示例** (`configs/config.example.yaml`)
   - 添加 `model_context_limits` 配置说明和示例

## 向后兼容性

- 保留 `ContextTokenLimit` 字段作为默认值
- 如果未配置 `ModelContextLimits` 或模型不匹配任何模式，使用默认的 `ContextTokenLimit`
- 现有配置无需修改即可继续工作

## 测试验证

1. 使用 `claude-opus-4-6` 模型发送大请求（约 3MB），应使用 1M token 阈值，不触发错误
2. 使用 `claude-opus-4.6-20260101` 模型发送相同请求，应使用 1M token 阈值
3. 使用 `claude-sonnet-4` 模型发送相同请求，应使用默认 200K token 阈值，触发错误
4. 检查日志输出包含模型名和使用的 token 限制