package transformer

import (
	"testing"

	"go.uber.org/zap"
)

func TestAdRemover_Process(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name   string
		config *AdRemoveConfig
		input  string
		want   string
	}{
		{
			name: "prefix ad with boundary match",
			config: &AdRemoveConfig{
				Keywords:       []string{"adtest", "137810429"},
				PrefixBoundary: "(Hi|Hello|Sure)",
			},
			input: "Official group: 137810429 - promo ongoing - Hi! How can I help?",
			want:  "Hi! How can I help?",
		},
		{
			name: "prefix ad with zero-width chars",
			config: &AdRemoveConfig{
				Keywords:       []string{"adtest", "137810429"},
				PrefixBoundary: "(Hi|Hello)",
			},
			input: "\u200dPromo ongoing, group: 1\u200d37810429, https://example.comHi! How can I help?",
			want:  "Hi! How can I help?",
		},
		{
			name: "suffix ad with boundary match",
			config: &AdRemoveConfig{
				Keywords:       []string{"adtest", "137810429"},
				SuffixBoundary: `\n\n`,
			},
			input: "Hi! How can I help?\n\nADTEST | https://example.com, group: 137810429",
			want:  "Hi! How can I help?",
		},
		{
			name: "suffix ad with zero-width chars",
			config: &AdRemoveConfig{
				Keywords:       []string{"adtest"},
				SuffixBoundary: `\n\n`,
			},
			input: "Hi! How can I help?\n\nA\u200dDTEST | https://example.com",
			want:  "Hi! How can I help?",
		},
		{
			name: "no keyword match passes through",
			config: &AdRemoveConfig{
				Keywords:       []string{"adtest", "137810429"},
				PrefixBoundary: "(Hi|Hello)",
			},
			input: "Hello! How can I help you today?",
			want:  "Hello! How can I help you today?",
		},
		{
			name: "prefix ad no boundary removes all",
			config: &AdRemoveConfig{
				Keywords: []string{"adtest"},
			},
			input: "adtest promo content only",
			want:  "",
		},
		{
			name: "suffix ad no boundary removes all",
			config: &AdRemoveConfig{
				Keywords: []string{"adtest"},
			},
			input: "some content followed by adtest promo",
			want:  "",
		},
		{
			name: "case insensitive keyword match",
			config: &AdRemoveConfig{
				Keywords:       []string{"ADTEST"},
				PrefixBoundary: "(Hi|Hello)",
			},
			input: "adtest promo Hello world",
			want:  "Hello world",
		},
		{
			name: "replace_with provides substitute text",
			config: &AdRemoveConfig{
				Keywords:       []string{"adtest"},
				PrefixBoundary: "(Hi|Hello)",
				ReplaceWith:    "\n\n",
			},
			input: "adtest promo Hi! How can I help?",
			want:  "\n\nHi! How can I help?",
		},
		{
			name: "multiple keywords first match wins",
			config: &AdRemoveConfig{
				Keywords:       []string{"keyword1", "keyword2", "137810429"},
				PrefixBoundary: "(Hi)",
			},
			input: "137810429 promo Hi there",
			want:  "Hi there",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &TransformerDef{
				Name:     "test_ad_remove",
				AdRemove: tt.config,
			}
			ar := NewAdRemover(def, logger)
			got := ar.Process(tt.input)
			if got != tt.want {
				t.Errorf("Process() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAdRemover_ProcessOnlyOnce(t *testing.T) {
	logger := zap.NewNop()
	def := &TransformerDef{
		Name: "test_ad_remove",
		AdRemove: &AdRemoveConfig{
			Keywords:       []string{"adtest"},
			PrefixBoundary: "(Hi)",
		},
	}
	ar := NewAdRemover(def, logger)

	// First delta has ad
	got := ar.Process("adtest promo Hi there")
	if got != "Hi there" {
		t.Errorf("first Process() = %q, want %q", got, "Hi there")
	}

	// Second delta should pass through even if it contains keyword
	got = ar.Process("adtest another delta")
	if got != "adtest another delta" {
		t.Errorf("second Process() = %q, want passthrough", got)
	}
}

func TestAdRemover_Reset(t *testing.T) {
	logger := zap.NewNop()
	def := &TransformerDef{
		Name: "test_ad_remove",
		AdRemove: &AdRemoveConfig{
			Keywords:       []string{"adtest"},
			PrefixBoundary: "(Hi)",
		},
	}
	ar := NewAdRemover(def, logger)

	got := ar.Process("adtest promo Hi there")
	if got != "Hi there" {
		t.Errorf("first Process() = %q, want %q", got, "Hi there")
	}

	ar.Reset()

	got = ar.Process("adtest again Hi world")
	if got != "Hi world" {
		t.Errorf("after Reset Process() = %q, want %q", got, "Hi world")
	}
}
