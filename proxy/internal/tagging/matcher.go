package tagging

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type Engine struct {
	rules           []Rule
	regexes         map[string]*regexp.Regexp
	skipMCPToolTags bool
	mu              sync.RWMutex
}

func NewEngine(cfg Config) *Engine {
	skipMCPToolTags := true
	if cfg.SkipMCPToolTags != nil {
		skipMCPToolTags = *cfg.SkipMCPToolTags
	}
	e := &Engine{
		rules:           make([]Rule, 0),
		regexes:         make(map[string]*regexp.Regexp),
		skipMCPToolTags: skipMCPToolTags,
	}

	for _, rule := range BuiltinRules {
		r := rule
		enabled := true
		r.Enabled = &enabled
		e.rules = append(e.rules, r)
	}

	for _, rule := range cfg.Rules {
		if err := ValidateRule(rule, false); err != nil {
			log.Printf("[tagging] skip invalid rule %q: %v", rule.Name, err)
			continue
		}
		e.rules = append(e.rules, rule)
	}

	sort.SliceStable(e.rules, func(i, j int) bool {
		return e.rules[i].Priority < e.rules[j].Priority
	})

	e.compileRegexes()

	return e
}

func (e *Engine) compileRegexes() {
	for _, rule := range e.rules {
		for _, m := range rule.Matchers {
			if m.Match.Op == MatchOpRegex && m.Match.Value != "" {
				if _, exists := e.regexes[m.Match.Value]; !exists {
					if re, err := regexp.Compile(m.Match.Value); err == nil {
						e.regexes[m.Match.Value] = re
					}
				}
			}
		}
	}
}

type RequestContext struct {
	Method   string
	Path     string
	Query    url.Values
	Headers  http.Header
	Body     []byte
	bodyJSON map[string]interface{}
}

func (e *Engine) Match(ctx *RequestContext) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	tagSet := make(map[string]bool)

	// 自动注入工具标签 $t_<toolname>
	toolNames := e.getToolNames(ctx)
	for _, name := range toolNames {
		if e.skipMCPToolTags && IsMCPTool(name) {
			continue
		}
		toolTag := GetToolTag(name)
		if toolTag != "" {
			tagSet[toolTag] = true
		}
	}

	for _, rule := range e.rules {
		if !rule.IsEnabled() {
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

	for _, rule := range e.rules {
		if !rule.IsEnabled() {
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

func (e *Engine) matchRule(rule Rule, ctx *RequestContext, currentTags map[string]bool) bool {
	for _, m := range rule.Matchers {
		if !e.matchMatcher(m, ctx, currentTags) {
			return false
		}
	}
	return true
}

func (e *Engine) matchMatcher(m Matcher, ctx *RequestContext, currentTags map[string]bool) bool {
	switch m.Type {
	case MatcherTypeHeader:
		return e.matchHeader(m, ctx.Headers)
	case MatcherTypeBodyJSON:
		return e.matchBodyJSON(m, ctx)
	case MatcherTypeTag:
		return e.matchTag(m, currentTags)
	case MatcherTypeTags:
		return e.matchTags(m, currentTags)
	case MatcherTypeTool:
		return e.matchTool(m, ctx)
	case MatcherTypeTools:
		return e.matchTools(m, ctx)
	default:
		return false
	}
}

func (e *Engine) matchHeader(m Matcher, headers http.Header) bool {
	value := headers.Get(m.Key)
	return e.matchValue(m.Match, value)
}

func (e *Engine) matchBodyJSON(m Matcher, ctx *RequestContext) bool {
	if ctx.bodyJSON == nil && len(ctx.Body) > 0 {
		var parsed map[string]interface{}
		if err := json.Unmarshal(ctx.Body, &parsed); err == nil {
			ctx.bodyJSON = parsed
		}
	}
	if ctx.bodyJSON == nil {
		return false
	}

	value := e.getJSONValue(ctx.bodyJSON, m.Key)
	return e.matchValue(m.Match, value)
}

func (e *Engine) getJSONValue(data map[string]interface{}, path string) string {
	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		default:
			return ""
		}
	}

	switch v := current.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%v", v)
	case bool:
		return fmt.Sprintf("%v", v)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (e *Engine) matchTag(m Matcher, currentTags map[string]bool) bool {
	return currentTags[NormalizeTag(m.Tag)]
}

func (e *Engine) matchTags(m Matcher, currentTags map[string]bool) bool {
	tagOp := m.TagOp
	if tagOp == "" {
		tagOp = TagMatchOpAll
	}

	switch tagOp {
	case TagMatchOpAll:
		for _, tag := range m.Tags {
			if !currentTags[NormalizeTag(tag)] {
				return false
			}
		}
		return true
	case TagMatchOpAny:
		for _, tag := range m.Tags {
			if currentTags[NormalizeTag(tag)] {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (e *Engine) getToolNames(ctx *RequestContext) []string {
	if ctx.bodyJSON == nil && len(ctx.Body) > 0 {
		var parsed map[string]interface{}
		if err := json.Unmarshal(ctx.Body, &parsed); err == nil {
			ctx.bodyJSON = parsed
		}
	}
	if ctx.bodyJSON == nil {
		return nil
	}

	tools, ok := ctx.bodyJSON["tools"].([]interface{})
	if !ok {
		return nil
	}

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if toolMap, ok := tool.(map[string]interface{}); ok {
			if name, ok := toolMap["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

func (e *Engine) matchTool(m Matcher, ctx *RequestContext) bool {
	toolNames := e.getToolNames(ctx)
	for _, name := range toolNames {
		if name == m.Tool {
			return true
		}
	}
	return false
}

func (e *Engine) matchTools(m Matcher, ctx *RequestContext) bool {
	toolNames := e.getToolNames(ctx)
	if len(toolNames) == 0 {
		return false
	}

	toolSet := make(map[string]bool)
	for _, name := range toolNames {
		toolSet[name] = true
	}

	tagOp := m.TagOp
	if tagOp == "" {
		tagOp = TagMatchOpAll
	}

	switch tagOp {
	case TagMatchOpAll:
		for _, tool := range m.Tools {
			if !toolSet[tool] {
				return false
			}
		}
		return true
	case TagMatchOpAny:
		for _, tool := range m.Tools {
			if toolSet[tool] {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (e *Engine) matchValue(vm ValueMatcher, actual string) bool {
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
		return strings.Contains(strings.ToLower(actual), strings.ToLower(vm.Value))
	case MatchOpNotContains:
		return !strings.Contains(strings.ToLower(actual), strings.ToLower(vm.Value))
	case MatchOpPrefix:
		return strings.HasPrefix(actual, vm.Value)
	case MatchOpSuffix:
		return strings.HasSuffix(actual, vm.Value)
	case MatchOpRegex:
		if re, ok := e.regexes[vm.Value]; ok {
			return re.MatchString(actual)
		}
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

func (e *Engine) UpdateRules(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = make([]Rule, 0)

	for _, rule := range BuiltinRules {
		r := rule
		enabled := true
		r.Enabled = &enabled
		e.rules = append(e.rules, r)
	}

	for _, rule := range rules {
		if rule.Builtin {
			continue
		}
		if err := ValidateRule(rule, false); err != nil {
			log.Printf("[tagging] skip invalid rule %q: %v", rule.Name, err)
			continue
		}
		e.rules = append(e.rules, rule)
	}

	sort.SliceStable(e.rules, func(i, j int) bool {
		return e.rules[i].Priority < e.rules[j].Priority
	})

	e.regexes = make(map[string]*regexp.Regexp)
	e.compileRegexes()
}

func (e *Engine) GetRules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Rule, len(e.rules))
	copy(result, e.rules)
	return result
}

func (e *Engine) TestMatch(ctx *RequestContext) map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	matchedRules := make([]string, 0)
	tagSet := make(map[string]bool)

	// 自动注入工具标签 $t_<toolname>
	toolNames := e.getToolNames(ctx)
	for _, name := range toolNames {
		if e.skipMCPToolTags && IsMCPTool(name) {
			continue
		}
		toolTag := GetToolTag(name)
		if toolTag != "" {
			tagSet[toolTag] = true
		}
	}

	for _, rule := range e.rules {
		if !rule.IsEnabled() {
			continue
		}
		if rule.HasTagMatcher() {
			continue
		}
		if e.matchRule(rule, ctx, tagSet) {
			matchedRules = append(matchedRules, rule.Name)
			for _, tag := range rule.Tags {
				tagSet[NormalizeTag(tag)] = true
			}
		}
	}

	for _, rule := range e.rules {
		if !rule.IsEnabled() {
			continue
		}
		if !rule.HasTagMatcher() {
			continue
		}
		if e.matchRule(rule, ctx, tagSet) {
			matchedRules = append(matchedRules, rule.Name)
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

	return map[string]interface{}{
		"matched_rules": matchedRules,
		"tags":          tags,
	}
}
