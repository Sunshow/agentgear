package tagging

import (
	"regexp"
	"strings"
)

// openCodeVariantRegex extracts the variant identifier from opencode User-Agent.
// e.g. "opencode/1.2.6.c0nr+72b210f" → captures "c0nr"
var openCodeVariantRegex = regexp.MustCompile(`^opencode/\d+\.\d+\.\d+\.([a-zA-Z0-9]+)`)

// extractOpenCodeTags generates dynamic variant tags from the opencode User-Agent.
// Returns ["$a_opencode_<variant>"] if a variant suffix exists, nil otherwise.
func extractOpenCodeTags(ctx *RequestContext) []string {
	ua := ctx.Headers.Get("User-Agent")
	matches := openCodeVariantRegex.FindStringSubmatch(ua)
	if len(matches) > 1 {
		variant := strings.ToLower(matches[1])
		return []string{"$a_opencode_" + variant}
	}
	return nil
}

var BuiltinRules = []Rule{
	{
		Name:     "$A_Droid",
		Priority: -1000,
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
		Name:     "$A_OpenCode",
		Priority: -1000,
		Builtin:  true,
		Matchers: []Matcher{
			{
				Type: MatcherTypeHeader,
				Key:  "User-Agent",
				Match: ValueMatcher{
					Op:    MatchOpRegex,
					Value: `^opencode/\d+\.\d+\.\d+`,
				},
			},
		},
		Tags:            []string{"$a_opencode"},
		DynamicTagsFunc: extractOpenCodeTags,
	},
	{
		Name:     "$P_Anthropic",
		Priority: -1000,
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
