package transformer

type ParamMapping struct {
	From      string `mapstructure:"from" json:"from" yaml:"from"`
	To        string `mapstructure:"to" json:"to" yaml:"to"`
	Transform string `mapstructure:"transform" json:"transform,omitempty" yaml:"transform,omitempty"`
}

// ContentReplacePattern defines a text replacement rule for response content
type ContentReplacePattern struct {
	Match          string `mapstructure:"match" json:"match" yaml:"match"`                                        // Marker text to match, e.g. "【ADTEST】"
	ReplaceWith    string `mapstructure:"replace_with" json:"replace_with" yaml:"replace_with"`                    // Replacement text (empty string = delete)
	TrimAfter      string `mapstructure:"trim_after" json:"trim_after,omitempty" yaml:"trim_after,omitempty"`      // Extend match forward until this separator, e.g. "\n\n"
	StripZeroWidth bool   `mapstructure:"strip_zero_width" json:"strip_zero_width,omitempty" yaml:"strip_zero_width,omitempty"` // Strip zero-width Unicode chars before matching
}

// ParamCondition defines a condition for matching tool parameters
type ParamCondition struct {
	Param string `mapstructure:"param" json:"param" yaml:"param"` // 参数路径，如 "file_path"
	Op    string `mapstructure:"op" json:"op" yaml:"op"`          // 操作符: prefix, suffix, contains, equals
	Value string `mapstructure:"value" json:"value" yaml:"value"` // 匹配值
}

// ModelContextLimit defines model-specific context token limits
type ModelContextLimit struct {
	ModelPattern string `mapstructure:"model_pattern" json:"model_pattern" yaml:"model_pattern"` // 模型匹配模式（支持通配符 *）
	TokenLimit   int    `mapstructure:"token_limit" json:"token_limit" yaml:"token_limit"`       // 该模型的 token 限制
}

// HeaderInjection defines a custom HTTP header to inject
type HeaderInjection struct {
	Key   string `mapstructure:"key" json:"key" yaml:"key"`       // Header name (e.g., "X-Custom-Agent")
	Value string `mapstructure:"value" json:"value" yaml:"value"` // Header value (supports {{placeholders}})
}

// TransformerDef defines a transformer's logic (source tool -> target tool mapping)
type TransformerDef struct {
	Name         string                 `mapstructure:"name" json:"name" yaml:"name"`
	Description  string                 `mapstructure:"description" json:"description,omitempty" yaml:"description,omitempty"`
	Direction    string                 `mapstructure:"direction" json:"direction,omitempty" yaml:"direction,omitempty"` // "request" or "response"
	Type         string                 `mapstructure:"type" json:"type,omitempty" yaml:"type,omitempty"`                // "tool" (默认) | "message_inject" | "error_transform" | "header_inject"
	SourceTool   string                 `mapstructure:"source_tool" json:"source_tool,omitempty" yaml:"source_tool,omitempty"`
	TargetTool   string                 `mapstructure:"target_tool" json:"target_tool,omitempty" yaml:"target_tool,omitempty"`
	Accumulate   bool                   `mapstructure:"accumulate" json:"accumulate" yaml:"accumulate"`
	ParamMapping []ParamMapping         `mapstructure:"param_mapping" json:"param_mapping" yaml:"param_mapping"`
	InputSchema  map[string]interface{} `mapstructure:"input_schema" json:"input_schema,omitempty" yaml:"input_schema,omitempty"`    // 请求方向：替换工具的 input_schema
	InjectText   string                 `mapstructure:"inject_text" json:"inject_text,omitempty" yaml:"inject_text,omitempty"`       // 注入的文本内容
	InjectFormat string                 `mapstructure:"inject_format" json:"inject_format,omitempty" yaml:"inject_format,omitempty"` // "system-reminder" | "plain"
	// error_transform 类型专用字段
	ErrorCode             string              `mapstructure:"error_code" json:"error_code,omitempty" yaml:"error_code,omitempty"`
	ErrorMessage          string              `mapstructure:"error_message" json:"error_message,omitempty" yaml:"error_message,omitempty"`
	ErrorPatterns         []string            `mapstructure:"error_patterns" json:"error_patterns,omitempty" yaml:"error_patterns,omitempty"` // 响应体正则匹配模式，匹配到任一则触发错误转换
	RequestSizeThreshold  int                 `mapstructure:"request_size_threshold" json:"request_size_threshold,omitempty" yaml:"request_size_threshold,omitempty"`
	ContextTokenLimit     int                 `mapstructure:"context_token_limit" json:"context_token_limit,omitempty" yaml:"context_token_limit,omitempty"`
	ModelContextLimits    []ModelContextLimit `mapstructure:"model_context_limits" json:"model_context_limits,omitempty" yaml:"model_context_limits,omitempty"`
	ContextThresholdRatio float64             `mapstructure:"context_threshold_ratio" json:"context_threshold_ratio,omitempty" yaml:"context_threshold_ratio,omitempty"`
	TokenEstimateRatio    float64             `mapstructure:"token_estimate_ratio" json:"token_estimate_ratio,omitempty" yaml:"token_estimate_ratio,omitempty"`
	ImageTokenEstimate    int                 `mapstructure:"image_token_estimate" json:"image_token_estimate,omitempty" yaml:"image_token_estimate,omitempty"` // 单张图片估算 token 数，默认 1600
	ContentPatterns       []ContentReplacePattern `mapstructure:"content_patterns" json:"content_patterns,omitempty" yaml:"content_patterns,omitempty"`
	ParamConditions       []ParamCondition    `mapstructure:"param_conditions" json:"param_conditions,omitempty" yaml:"param_conditions,omitempty"`
	HeaderInjections      []HeaderInjection   `mapstructure:"header_injections" json:"header_injections,omitempty" yaml:"header_injections,omitempty"`
	// compress 类型专用字段
	CompressTarget       string `mapstructure:"compress_target" json:"compress_target,omitempty" yaml:"compress_target,omitempty"`             // "same" | "gateway:name" | "url:https://..."
	CompressModel        string `mapstructure:"compress_model" json:"compress_model,omitempty" yaml:"compress_model,omitempty"`                // 压缩使用的模型
	CompressSystemPrompt string `mapstructure:"compress_system_prompt" json:"compress_system_prompt,omitempty" yaml:"compress_system_prompt,omitempty"` // 压缩 system prompt
	CompressUserPrompt   string `mapstructure:"compress_user_prompt" json:"compress_user_prompt,omitempty" yaml:"compress_user_prompt,omitempty"`       // 压缩 user prompt
	PreserveBudget       int    `mapstructure:"preserve_budget" json:"preserve_budget,omitempty" yaml:"preserve_budget,omitempty"`             // 保留最近 N tokens
	SummaryBudget        int    `mapstructure:"summary_budget" json:"summary_budget,omitempty" yaml:"summary_budget,omitempty"`                // 摘要预算 tokens
	AutoRetry            bool   `mapstructure:"auto_retry" json:"auto_retry,omitempty" yaml:"auto_retry,omitempty"`                            // 自动重试原请求
	MaxRetries           int    `mapstructure:"max_retries" json:"max_retries,omitempty" yaml:"max_retries,omitempty"`                         // 最大重试次数
	Builtin              bool   `mapstructure:"builtin" json:"builtin" yaml:"builtin,omitempty"`
	IsTemplate           bool   `mapstructure:"is_template" json:"is_template,omitempty" yaml:"is_template,omitempty"`
	TemplateRef          string `mapstructure:"template_ref" json:"template_ref,omitempty" yaml:"template_ref,omitempty"`
	TemplateArgs         map[string]string `mapstructure:"template_args" json:"template_args,omitempty" yaml:"template_args,omitempty"`
}

// MappingRule binds a transformer to conditions (tags/gateways)
type MappingRule struct {
	Name        string   `mapstructure:"name" json:"name" yaml:"name"`
	Description string   `mapstructure:"description" json:"description,omitempty" yaml:"description,omitempty"`
	Enabled     bool     `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
	Tags        []string `mapstructure:"tags" json:"tags" yaml:"tags"`
	ExcludeTags []string `mapstructure:"exclude_tags" json:"exclude_tags,omitempty" yaml:"exclude_tags,omitempty"` // Tags that must NOT be present
	Gateways    []string `mapstructure:"gateways" json:"gateways" yaml:"gateways"`
	Tools       []string `mapstructure:"tools" json:"tools,omitempty" yaml:"tools,omitempty"`       // Tool names to match
	ToolOp      string   `mapstructure:"tool_op" json:"tool_op,omitempty" yaml:"tool_op,omitempty"` // "all" (default) or "any"
	Transformer string   `mapstructure:"transformer" json:"transformer" yaml:"transformer"`
	Builtin     bool     `mapstructure:"builtin" json:"builtin" yaml:"builtin,omitempty"`
}

// TransformerConfig is kept for backward compatibility during migration
type TransformerConfig struct {
	Name         string                 `mapstructure:"name" json:"name" yaml:"name"`
	Enabled      bool                   `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
	Tags         []string               `mapstructure:"tags" json:"tags" yaml:"tags"`
	Gateways     []string               `mapstructure:"gateways" json:"gateways" yaml:"gateways"`
	SourceTool   string                 `mapstructure:"source_tool" json:"source_tool" yaml:"source_tool"`
	TargetTool   string                 `mapstructure:"target_tool" json:"target_tool" yaml:"target_tool"`
	Accumulate   bool                   `mapstructure:"accumulate" json:"accumulate" yaml:"accumulate"`
	ParamMapping []ParamMapping         `mapstructure:"param_mapping" json:"param_mapping" yaml:"param_mapping"`
	InputSchema  map[string]interface{} `mapstructure:"input_schema" json:"input_schema,omitempty" yaml:"input_schema,omitempty"`
}

// Config holds transformer configuration
type Config struct {
	Definitions []TransformerDef `mapstructure:"definitions" json:"definitions" yaml:"definitions"`
	Mappings    []MappingRule    `mapstructure:"mappings" json:"mappings" yaml:"mappings"`
}
