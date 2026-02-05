## 方案 B: 扩展 transformer 系统支持消息注入

### 1. 扩展 tagging matcher 支持检测 tools

**文件**: `proxy/internal/tagging/rules.go`

```go
const (
    // ... 现有类型
    MatcherTypeTool  MatcherType = "tool"   // 检测请求中是否存在某个 tool
    MatcherTypeTools MatcherType = "tools"  // 检测多个 tools (复用 TagOp: all/any)
)
```

**文件**: `proxy/internal/tagging/matcher.go`

在 `matchMatcher` 中添加：
```go
case MatcherTypeTool:
    return e.matchTool(m, ctx)
case MatcherTypeTools:
    return e.matchTools(m, ctx)
```

实现 `matchTool` 和 `matchTools`：从 `ctx.bodyJSON["tools"]` 数组中提取工具名称进行匹配。

**文件**: `proxy/internal/tagging/validate.go`

添加校验逻辑。

**文件**: `proxy/internal/tagging/builtin.go`

添加 `$U_Kiro` 规则：
```go
{
    Name:     "$U_Kiro",
    Priority: -1000,
    Builtin:  true,
    Matchers: []Matcher{
        {
            Type: MatcherTypeTool,
            Tool: "Write",
        },
    },
    Tags: []string{"$u_kiro"},
},
```

### 2. 扩展 transformer 支持消息注入类型

**文件**: `proxy/internal/transformer/config.go`

扩展 `TransformerDef`：
```go
type TransformerDef struct {
    // ... 现有字段
    Type         string `mapstructure:"type" json:"type,omitempty" yaml:"type,omitempty"` // "tool" (默认) | "message_inject"
    InjectText   string `mapstructure:"inject_text" json:"inject_text,omitempty" yaml:"inject_text,omitempty"` // 注入的文本内容
    InjectFormat string `mapstructure:"inject_format" json:"inject_format,omitempty" yaml:"inject_format,omitempty"` // "system-reminder" | "plain"
}
```

**文件**: `proxy/internal/transformer/registry.go`

添加方法获取消息注入转换器：
```go
func (r *Registry) GetMessageInjectTransformers(tags []string) []*TransformerDef
```

### 3. 在 handler 中应用消息注入转换器

**文件**: `proxy/internal/proxy/handler.go`

修改 `transformRequestBody`，在处理 tools 转换之前，先获取并应用消息注入转换器：
```go
// Apply message inject transformers
if h.transformerRegistry != nil {
    injectTransformers := h.transformerRegistry.GetMessageInjectTransformers(tags)
    for _, t := range injectTransformers {
        h.injectMessage(req, t.InjectText, t.InjectFormat)
        transformed = true
    }
}
```

`injectMessage` 方法：在第一条 user 消息的 content 开头插入 `<system-reminder>` 块。

### 4. 添加内建定义和映射

**文件**: `proxy/internal/transformer/builtin.go`

```go
// BuiltinDefinitions 添加
{
    Name:         "$droid_chunked_write_hint",
    Description:  "提示模型分段创建长文件",
    Direction:    "request",
    Type:         "message_inject",
    InjectFormat: "system-reminder",
    InjectText:   `IMPORTANT: When using the Write tool to create files, if the content exceeds 150 lines:
1. First create the file with a skeleton (imports, class/function signatures, brief TODO comments)
2. Then use multiple Edit tool calls to add the implementation in chunks of ~100 lines each
This prevents output truncation with long content.`,
    Builtin: true,
},

// BuiltinMappings 添加
{
    Name:        "$droid_kiro_chunked_write",
    Description: "Droid+Kiro: 分段写入提示",
    Enabled:     true,
    Tags:        []string{"$a_droid", "$u_kiro"},
    Transformer: "$droid_chunked_write_hint",
    Builtin:     true,
},
```

### 实现文件清单

1. `proxy/internal/tagging/rules.go` - 添加 MatcherTypeTool/MatcherTypeTools 常量
2. `proxy/internal/tagging/matcher.go` - 实现 matchTool/matchTools 方法
3. `proxy/internal/tagging/validate.go` - 添加校验逻辑
4. `proxy/internal/tagging/builtin.go` - 添加 $U_Kiro 规则
5. `proxy/internal/transformer/config.go` - 扩展 TransformerDef 添加 Type/InjectText/InjectFormat 字段
6. `proxy/internal/transformer/registry.go` - 添加 GetMessageInjectTransformers 方法
7. `proxy/internal/transformer/builtin.go` - 添加分段写入提示的定义和映射
8. `proxy/internal/proxy/handler.go` - 修改 transformRequestBody 应用消息注入