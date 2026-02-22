package tagging

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

type MatcherType string

const (
	MatcherTypeHeader   MatcherType = "header"
	MatcherTypeBodyJSON MatcherType = "body_json"
	MatcherTypeTag      MatcherType = "tag"
	MatcherTypeTags     MatcherType = "tags"
	MatcherTypeTool     MatcherType = "tool"  // 检测请求中是否存在某个 tool
	MatcherTypeTools    MatcherType = "tools" // 检测多个 tools (复用 TagOp: all/any)
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
	Tool  string       `mapstructure:"tool" json:"tool"`   // 单个 tool 名称
	Tools []string     `mapstructure:"tools" json:"tools"` // 多个 tool 名称 (复用 TagOp: all/any)
}

type Rule struct {
	Name            string                              `mapstructure:"name" json:"name"`
	Priority        int                                 `mapstructure:"priority" json:"priority"`
	Enabled         *bool                               `mapstructure:"enabled" json:"enabled"`
	Builtin         bool                                `mapstructure:"-" json:"builtin"`
	Matchers        []Matcher                           `mapstructure:"matchers" json:"matchers"`
	Tags            []string                            `mapstructure:"tags" json:"tags"`
	DynamicTagsFunc func(ctx *RequestContext) []string  `mapstructure:"-" json:"-"`
}

func (r *Rule) IsEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

func (r *Rule) HasTagMatcher() bool {
	for _, m := range r.Matchers {
		if m.Type == MatcherTypeTag || m.Type == MatcherTypeTags {
			return true
		}
	}
	return false
}

type Config struct {
	Rules           []Rule `mapstructure:"rules" json:"rules"`
	SkipMCPToolTags *bool  `mapstructure:"skip_mcp_tool_tags" json:"skip_mcp_tool_tags"`
}
