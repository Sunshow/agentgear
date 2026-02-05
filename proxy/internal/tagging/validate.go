package tagging

import (
	"fmt"
	"regexp"
	"strings"
)

var userTagRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

const ToolTagPrefix = "$t_"

func IsSystemTag(tag string) bool {
	return strings.HasPrefix(tag, "$")
}

func GetToolTag(toolName string) string {
	if toolName == "" {
		return ""
	}
	return ToolTagPrefix + strings.ToLower(toolName)
}

func IsMCPTool(toolName string) bool {
	return strings.Contains(toolName, "___")
}

func IsValidUserTag(tag string) bool {
	if IsSystemTag(tag) {
		return false
	}
	return userTagRegex.MatchString(tag)
}

func NormalizeTag(tag string) string {
	return strings.ToLower(tag)
}

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

func ValidateRulePriority(priority int, isBuiltin bool) error {
	if !isBuiltin && priority < 0 {
		return fmt.Errorf("user rule priority must be >= 0, got: %d", priority)
	}
	return nil
}

func ValidateRule(rule Rule, isBuiltin bool) error {
	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}
	if err := ValidateRulePriority(rule.Priority, isBuiltin); err != nil {
		return err
	}
	if err := ValidateRuleTags(rule.Tags, isBuiltin); err != nil {
		return err
	}
	for i, m := range rule.Matchers {
		if err := ValidateMatcher(m); err != nil {
			return fmt.Errorf("matcher[%d]: %w", i, err)
		}
	}
	return nil
}

func ValidateMatcher(m Matcher) error {
	switch m.Type {
	case MatcherTypeHeader:
		if m.Key == "" {
			return fmt.Errorf("header matcher requires key")
		}
	case MatcherTypeBodyJSON:
		if m.Key == "" {
			return fmt.Errorf("body_json matcher requires key")
		}
	case MatcherTypeTag:
		if m.Tag == "" {
			return fmt.Errorf("tag matcher requires tag")
		}
	case MatcherTypeTags:
		if len(m.Tags) == 0 {
			return fmt.Errorf("tags matcher requires tags")
		}
	case MatcherTypeTool:
		if m.Tool == "" {
			return fmt.Errorf("tool matcher requires tool")
		}
	case MatcherTypeTools:
		if len(m.Tools) == 0 {
			return fmt.Errorf("tools matcher requires tools")
		}
	default:
		return fmt.Errorf("unknown matcher type: %s", m.Type)
	}
	return nil
}
