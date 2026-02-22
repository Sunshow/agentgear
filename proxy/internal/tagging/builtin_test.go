package tagging

import (
	"net/http"
	"testing"
)

func TestExtractOpenCodeTags(t *testing.T) {
	tests := []struct {
		name     string
		ua       string
		expected []string
	}{
		{
			name:     "plain semver, no variant",
			ua:       "opencode/1.2.6",
			expected: nil,
		},
		{
			name:     "variant c0nr with build metadata",
			ua:       "opencode/1.2.6.c0nr+72b210f",
			expected: []string{"$a_opencode_c0nr"},
		},
		{
			name:     "variant beta without build metadata",
			ua:       "opencode/2.0.0.beta",
			expected: []string{"$a_opencode_beta"},
		},
		{
			name:     "variant with uppercase normalized to lowercase",
			ua:       "opencode/1.0.0.FooBar",
			expected: []string{"$a_opencode_foobar"},
		},
		{
			name:     "variant stops at non-alphanumeric",
			ua:       "opencode/1.2.6.c0nr-extra",
			expected: []string{"$a_opencode_c0nr"},
		},
		{
			name:     "not opencode agent",
			ua:       "factory-cli/1.0.0",
			expected: nil,
		},
		{
			name:     "empty user agent",
			ua:       "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &RequestContext{
				Headers: http.Header{},
			}
			ctx.Headers.Set("User-Agent", tt.ua)

			result := extractOpenCodeTags(ctx)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
				return
			}
			for i, tag := range result {
				if tag != tt.expected[i] {
					t.Errorf("expected tag[%d]=%q, got %q", i, tt.expected[i], tag)
				}
			}
		})
	}
}

func TestEngineMatchOpenCodeVariant(t *testing.T) {
	engine := NewEngine(Config{})

	tests := []struct {
		name         string
		ua           string
		expectTags   []string
		unexpectTags []string
	}{
		{
			name:         "plain opencode generates only base tag",
			ua:           "opencode/1.2.6",
			expectTags:   []string{"$a_opencode"},
			unexpectTags: []string{"$a_opencode_c0nr"},
		},
		{
			name:       "opencode with variant generates both tags",
			ua:         "opencode/1.2.6.c0nr+72b210f",
			expectTags: []string{"$a_opencode", "$a_opencode_c0nr"},
		},
		{
			name:         "droid agent unaffected",
			ua:           "factory-cli/1.0.0",
			expectTags:   []string{"$a_droid"},
			unexpectTags: []string{"$a_opencode"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &RequestContext{
				Headers: http.Header{},
			}
			ctx.Headers.Set("User-Agent", tt.ua)

			tags := engine.Match(ctx)
			tagSet := make(map[string]bool)
			for _, tag := range tags {
				tagSet[tag] = true
			}

			for _, expected := range tt.expectTags {
				if !tagSet[expected] {
					t.Errorf("expected tag %q not found in %v", expected, tags)
				}
			}
			for _, unexpected := range tt.unexpectTags {
				if tagSet[unexpected] {
					t.Errorf("unexpected tag %q found in %v", unexpected, tags)
				}
			}
		})
	}
}
