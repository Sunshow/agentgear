# Tagging 系统设计文档

## 概述

Tagging 系统是 AgentGear 的核心功能之一，用于根据请求特征自动为连接打标签。标签可用于：

- **条件化转换器**：根据标签选择性应用不同的 Transformer
- **连接过滤**：在 GUI 和 API 中按标签筛选连接
- **统计分析**：按标签维度统计请求分布

## 标签类型

系统定义了 6 种标签类型，通过前缀区分：

| 前缀 | 类型 | 含义 | 示例 | 生成方式 |
|------|------|------|------|----------|
| `$a_` | Agent | 客户端/Agent 类型 | `$a_droid` | Builtin 规则 |
| `$p_` | Protocol | 协议类型 | `$p_anthropic` | Builtin 规则 |
| `$u_` | Upstream | 上游服务类型 | `$u_warp`, `$u_kiro` | Gateway 配置 |
| `$g_` | Gateway | 网关标识 | `$g_mygateway` | Gateway 配置 |
| `$t_` | Tool | 工具名称 | `$t_read`, `$t_edit` | 自动提取 |
| 无前缀 | Custom | 用户自定义标签 | `mobile_client` | 用户规则 |

### 标签命名规范

- **系统标签**：以 `$` 开头，由系统自动生成，用户规则不能定义
- **用户标签**：以字母或 `_` 开头，只能包含字母、数字、下划线
- **标签存储**：所有标签统一转为小写存储

## 自动标签生成

### 1. 工具标签 ($t_)

系统自动从请求 body 中提取 `tools` 数组，为每个工具生成 `$t_<toolname>` 标签：

```json
{
  "tools": [
    { "name": "Read" },
    { "name": "Edit" }
  ]
}
```

生成标签：`$t_read`, `$t_edit`

**MCP 工具过滤**：默认跳过包含 `___` 的 MCP 工具名（如 `chrome-devtools___click`），可通过配置关闭：

```yaml
tagging:
  skip_mcp_tool_tags: false  # 默认 true
```

### 2. 上游标签 ($u_)

当请求通过 Gateway 路由时，根据 Gateway 的 `type` 字段自动注入：

```yaml
gateways:
  - name: "my-warp"
    type: "warp"  # 生成 $u_warp
```

### 3. 网关标签 ($g_)

根据 Gateway 的 `name` 字段自动注入：

```yaml
gateways:
  - name: "production"  # 生成 $g_production
```

## 内置规则 (Builtin)

系统预定义了以下内置规则，优先级为 -1000（最高优先级）：

### $A_Droid

检测 Factory Droid 客户端：

```yaml
name: "$A_Droid"
priority: -1000
builtin: true
matchers:
  - type: header
    key: "User-Agent"
    match:
      op: regex
      value: "^factory-cli/\\d+\\.\\d+\\.\\d+"
tags:
  - "$a_droid"
```

### $P_Anthropic

检测 Anthropic 协议：

```yaml
name: "$P_Anthropic"
priority: -1000
builtin: true
matchers:
  - type: header
    key: "Anthropic-Version"
    match:
      op: exists
tags:
  - "$p_anthropic"
```

## 匹配器类型 (Matcher Types)

### 1. header

匹配 HTTP 请求头：

```yaml
matchers:
  - type: header
    key: "User-Agent"
    match:
      op: contains
      value: "Mobile"
```

### 2. body_json

匹配请求 body 中的 JSON 字段（支持点号路径）：

```yaml
matchers:
  - type: body_json
    key: "model"
    match:
      op: prefix
      value: "claude-"
```

### 3. tag

匹配已存在的单个标签：

```yaml
matchers:
  - type: tag
    tag: "$a_droid"
```

### 4. tags

匹配多个标签（支持 all/any 逻辑）：

```yaml
matchers:
  - type: tags
    tags: ["$a_droid", "$p_anthropic"]
    tag_op: all  # all: 全部匹配, any: 任一匹配
```

### 5. tool

匹配请求中是否包含指定工具：

```yaml
matchers:
  - type: tool
    tool: "Read"
```

### 6. tools

匹配多个工具（支持 all/any 逻辑）：

```yaml
matchers:
  - type: tools
    tools: ["Read", "Edit", "Create"]
    tag_op: any
```

## 匹配操作符 (Match Operations)

| 操作符 | 说明 | 示例 |
|--------|------|------|
| `exists` | 值存在（非空） | 检测 header 是否存在 |
| `not_exists` | 值不存在（为空） | 检测 header 不存在 |
| `eq` | 完全相等 | `value: "claude-3"` |
| `ne` | 不相等 | `value: "gpt-4"` |
| `contains` | 包含子串（忽略大小写） | `value: "mobile"` |
| `not_contains` | 不包含子串 | `value: "bot"` |
| `prefix` | 前缀匹配 | `value: "claude-"` |
| `suffix` | 后缀匹配 | `value: "-preview"` |
| `regex` | 正则表达式匹配 | `value: "^v\\d+"` |
| `in` | 值在列表中 | `values: ["a", "b", "c"]` |
| `not_in` | 值不在列表中 | `values: ["x", "y"]` |

## 规则配置

### 配置结构

```yaml
tagging:
  skip_mcp_tool_tags: true  # 跳过 MCP 工具标签生成
  rules:
    - name: "rule_name"       # 规则名称（必填）
      priority: 0             # 优先级，越小越先执行（用户规则 >= 0）
      enabled: true           # 是否启用
      matchers:               # 匹配器列表（AND 逻辑）
        - type: header
          key: "X-Custom"
          match:
            op: exists
      tags:                   # 匹配成功后添加的标签
        - my_custom_tag
```

### 优先级说明

- **Builtin 规则**：priority < 0（如 -1000）
- **用户规则**：priority >= 0
- 数值越小，优先级越高，越先执行
- 规则按优先级排序后依次执行，标签累积

### 两阶段匹配

1. **第一阶段**：执行所有不包含 tag/tags 匹配器的规则
2. **第二阶段**：执行包含 tag/tags 匹配器的规则（可基于第一阶段产生的标签）

## API 接口

### 获取标签统计

```
GET /api/tags
```

响应：
```json
{
  "tags": [
    { "name": "$a_droid", "count": 150 },
    { "name": "$p_anthropic", "count": 120 }
  ]
}
```

### 获取标签规则

```
GET /api/tagging/rules
```

响应：
```json
{
  "rules": [
    {
      "name": "$A_Droid",
      "priority": -1000,
      "enabled": true,
      "builtin": true,
      "matchers": [...],
      "tags": ["$a_droid"]
    }
  ]
}
```

### 创建/更新规则

```
PUT /api/tagging/rules/:name
```

请求体：
```json
{
  "name": "my_rule",
  "priority": 100,
  "enabled": true,
  "matchers": [
    {
      "type": "header",
      "key": "X-Custom",
      "match": { "op": "exists" }
    }
  ],
  "tags": ["custom_tag"]
}
```

### 删除规则

```
DELETE /api/tagging/rules/:name
```

注意：Builtin 规则不能删除。

### 测试标签匹配

```
POST /api/tagging/test
```

请求体：
```json
{
  "method": "POST",
  "path": "/v1/messages",
  "headers": {
    "User-Agent": "factory-cli/1.0.0",
    "Anthropic-Version": "2023-06-01"
  },
  "body": "{\"model\":\"claude-3\"}"
}
```

响应：
```json
{
  "matched_rules": ["$A_Droid", "$P_Anthropic"],
  "tags": ["$a_droid", "$p_anthropic"]
}
```

### 按标签过滤连接

```
GET /api/connections?tags=$a_droid,$p_anthropic&limit=50
```

## GUI 展示

### 标签颜色

GUI 中按标签类型显示不同颜色：

| 类型 | 浅色模式 | 深色模式 |
|------|----------|----------|
| Agent ($a_) | 紫色 (purple-700) | 紫色 (purple-400) |
| Protocol ($p_) | 蓝色 (blue-700) | 蓝色 (blue-400) |
| Upstream ($u_) | 琥珀色 (amber-700) | 琥珀色 (amber-400) |
| Gateway ($g_) | 青色 (cyan-700) | 青色 (cyan-400) |
| Tool ($t_) | 绿色 (green-700) | 绿色 (green-400) |
| Custom | 玫红色 (rose-700) | 玫红色 (rose-400) |

### 分组展示

标签按类型分组显示，顺序为：Agent → Protocol → Upstream → Gateway → Tool → Custom

### 紧凑模式

连接列表中最多显示 3 个标签，按优先级排序，超出部分显示 `+N`。

## 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                      Request                                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   Tagging Engine                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ 1. 自动提取工具标签 ($t_)                            │    │
│  └─────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ 2. 第一阶段：执行非 tag 匹配器规则                   │    │
│  │    - Builtin 规则 ($a_, $p_)                        │    │
│  │    - 用户 header/body_json/tool 规则                │    │
│  └─────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ 3. 第二阶段：执行 tag/tags 匹配器规则                │    │
│  │    - 基于已有标签的派生规则                          │    │
│  └─────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ 4. 注入 Gateway 标签 ($u_, $g_)                      │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Final Tags                                │
│  [$a_droid, $p_anthropic, $u_warp, $g_prod, $t_read, ...]   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│              Transformer Registry                            │
│  根据 tags 选择匹配的 Transformer 进行请求/响应转换          │
└─────────────────────────────────────────────────────────────┘
```

## 实现参考

### Tag 验证函数

```go
package tagging

import (
    "fmt"
    "regexp"
    "strings"
)

var userTagRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// IsSystemTag 判断是否为系统预留 Tag
func IsSystemTag(tag string) bool {
    return strings.HasPrefix(tag, "$")
}

// IsValidUserTag 验证用户自定义 Tag 格式
func IsValidUserTag(tag string) bool {
    if IsSystemTag(tag) {
        return false
    }
    return userTagRegex.MatchString(tag)
}

// NormalizeTag 标准化 Tag（转小写）
func NormalizeTag(tag string) string {
    return strings.ToLower(tag)
}

// ValidateRuleTags 验证规则中的 Tags
func ValidateRuleTags(tags []string, isBuiltin bool) error {
    for _, tag := range tags {
        if IsSystemTag(tag) {
            if !isBuiltin {
                return fmt.Errorf("user rule cannot define system tag: %s", tag)
            }
        } else {
            if !IsValidUserTag(tag) {
                return fmt.Errorf("invalid tag format: %s", tag)
            }
        }
    }
    return nil
}

// ValidateRulePriority 验证规则优先级
func ValidateRulePriority(priority int, isBuiltin bool) error {
    if !isBuiltin && priority < 0 {
        return fmt.Errorf("user rule priority must be >= 0, got: %d", priority)
    }
    return nil
}
```

### ValueMatcher 实现

```go
type MatchOp string

const (
    MatchOpExists      MatchOp = "exists"
    MatchOpNotExists   MatchOp = "not_exists"
    MatchOpEquals      MatchOp = "eq"
    MatchOpNotEquals   MatchOp = "ne"
    MatchOpContains    MatchOp = "contains"
    MatchOpNotContains MatchOp = "not_contains"
    MatchOpPrefix      MatchOp = "prefix"
    MatchOpSuffix      MatchOp = "suffix"
    MatchOpRegex       MatchOp = "regex"
    MatchOpIn          MatchOp = "in"
    MatchOpNotIn       MatchOp = "not_in"
)

type ValueMatcher struct {
    Op     MatchOp  `mapstructure:"op" json:"op"`
    Value  string   `mapstructure:"value" json:"value"`
    Values []string `mapstructure:"values" json:"values"`
}

func (vm *ValueMatcher) Match(actual string) bool {
    switch vm.Op {
    case MatchOpExists:
        return actual != ""
    case MatchOpNotExists:
        return actual == ""
    case MatchOpEquals:
        return actual == vm.Value
    case MatchOpNotEquals:
        return actual != vm.Value
    case MatchOpContains:
        return strings.Contains(actual, vm.Value)
    case MatchOpNotContains:
        return !strings.Contains(actual, vm.Value)
    case MatchOpPrefix:
        return strings.HasPrefix(actual, vm.Value)
    case MatchOpSuffix:
        return strings.HasSuffix(actual, vm.Value)
    case MatchOpRegex:
        re, err := regexp.Compile(vm.Value)
        if err != nil {
            return false
        }
        return re.MatchString(actual)
    case MatchOpIn:
        for _, v := range vm.Values {
            if actual == v {
                return true
            }
        }
        return false
    case MatchOpNotIn:
        for _, v := range vm.Values {
            if actual == v {
                return false
            }
        }
        return true
    default:
        return false
    }
}
```

### Matcher 结构

```go
type MatcherType string

const (
    MatcherTypeHeader   MatcherType = "header"
    MatcherTypeBodyJSON MatcherType = "body_json"
    MatcherTypeTag      MatcherType = "tag"
    MatcherTypeTags     MatcherType = "tags"
    MatcherTypeTool     MatcherType = "tool"
    MatcherTypeTools    MatcherType = "tools"
)

type TagMatchOp string

const (
    TagMatchOpAll TagMatchOp = "all"
    TagMatchOpAny TagMatchOp = "any"
)

type Matcher struct {
    Type  MatcherType  `mapstructure:"type" json:"type"`
    Key   string       `mapstructure:"key" json:"key"`
    Match ValueMatcher `mapstructure:"match" json:"match"`
    Tag   string       `mapstructure:"tag" json:"tag"`
    Tags  []string     `mapstructure:"tags" json:"tags"`
    TagOp TagMatchOp   `mapstructure:"tag_op" json:"tag_op"`
    Tool  string       `mapstructure:"tool" json:"tool"`
    Tools []string     `mapstructure:"tools" json:"tools"`
}
```

### Rule 结构

```go
type Rule struct {
    Name     string    `mapstructure:"name" json:"name"`
    Priority int       `mapstructure:"priority" json:"priority"`
    Enabled  bool      `mapstructure:"enabled" json:"enabled"`
    Builtin  bool      `mapstructure:"-" json:"builtin"`
    Matchers []Matcher `mapstructure:"matchers" json:"matchers"`
    Tags     []string  `mapstructure:"tags" json:"tags"`
}

func (r *Rule) HasTagMatcher() bool {
    for _, m := range r.Matchers {
        if m.Type == MatcherTypeTag || m.Type == MatcherTypeTags {
            return true
        }
    }
    return false
}
```

### 内置规则定义

```go
// proxy/internal/tagging/builtin.go

var BuiltinRules = []Rule{
    {
        Name:     "$A_Droid",
        Priority: -1000,
        Enabled:  true,
        Builtin:  true,
        Matchers: []Matcher{
            {
                Type: MatcherTypeHeader,
                Key:  "User-Agent",
                Match: ValueMatcher{
                    Op:    MatchOpRegex,
                    Value: `^factory-cli/\d+\.\d+\.\d+`,
                },
            },
        },
        Tags: []string{"$a_droid"},
    },
    {
        Name:     "$P_Anthropic",
        Priority: -1000,
        Enabled:  true,
        Builtin:  true,
        Matchers: []Matcher{
            {
                Type: MatcherTypeHeader,
                Key:  "Anthropic-Version",
                Match: ValueMatcher{
                    Op: MatchOpExists,
                },
            },
        },
        Tags: []string{"$p_anthropic"},
    },
}
```

### 两阶段匹配实现

```go
func (e *Engine) Match(ctx *RequestContext) []string {
    e.mu.RLock()
    defer e.mu.RUnlock()

    tagSet := make(map[string]bool)

    // 阶段1：执行非 tag 匹配器规则
    for _, rule := range e.rules {
        if !rule.Enabled {
            continue
        }
        if rule.HasTagMatcher() {
            continue
        }
        if e.matchRule(rule, ctx, tagSet) {
            for _, tag := range rule.Tags {
                tagSet[NormalizeTag(tag)] = true
            }
        }
    }

    // 阶段2：执行 tag 匹配器规则
    for _, rule := range e.rules {
        if !rule.Enabled {
            continue
        }
        if !rule.HasTagMatcher() {
            continue
        }
        if e.matchRule(rule, ctx, tagSet) {
            for _, tag := range rule.Tags {
                tagSet[NormalizeTag(tag)] = true
            }
        }
    }

    tags := make([]string, 0, len(tagSet))
    for tag := range tagSet {
        tags = append(tags, tag)
    }
    sort.Strings(tags)

    return tags
}
```

### 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `proxy/internal/tagging/rules.go` | 修改 | 类型定义：`MatchOp`、`ValueMatcher`、`TagMatchOp`、`Matcher`、`Rule` |
| `proxy/internal/tagging/matcher.go` | 修改 | `ValueMatcher.Match()`、两阶段匹配逻辑 |
| `proxy/internal/tagging/builtin.go` | 新建 | `BuiltinRules` 内置规则列表 |
| `proxy/internal/tagging/validate.go` | 新建 | Tag/Priority/Rule 验证函数 |
| `proxy/configs/config.yaml` | 修改 | 更新为新配置格式 |
