package tagging

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
		Tags: []string{"$a_opencode"},
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
