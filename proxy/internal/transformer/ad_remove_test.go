package transformer

import (
	"testing"

	"go.uber.org/zap"
)

func TestAdRemover_Process(t *testing.T) {
	logger := zap.NewNop()

	sep := `(?:\n\n|\s-\s|[|｜])`

	tests := []struct {
		name   string
		config *AdRemoveConfig
		input  string
		want   string
	}{
		{
			name: "prefix ad separated by dash",
			config: &AdRemoveConfig{
				Keywords:       []string{"137810429"},
				PrefixBoundary: sep,
			},
			input: "Official group: 137810429 - promo ongoing - Hi! How can I help?",
			want:  "Hi! How can I help?",
		},
		{
			name: "prefix ad with multiple separators inside ad",
			config: &AdRemoveConfig{
				Keywords:       []string{"137810429", "官方群"},
				PrefixBoundary: sep,
			},
			// Real case: 官方Q群：1‍37810429 - 马年特惠活动持续中 - Hi
			input: "官方Q群：137810429 - 马年特惠活动持续中 - Hi! How can I help?",
			want:  "Hi! How can I help?",
		},
		{
			name: "prefix ad with zero-width chars and no separator",
			config: &AdRemoveConfig{
				Keywords:       []string{"137810429", "timicc"},
				PrefixBoundary: sep,
			},
			// Real case: URL glued to Hi
			input: "\u200dPromo, group: 1\u200d37810429, https://timicc.comHi! How can I help?",
			want:  "",
		},
		{
			name: "prefix ad with pipe separator",
			config: &AdRemoveConfig{
				Keywords:       []string{"timicc"},
				PrefixBoundary: sep,
			},
			input: "马年限时特惠进行中｜https://timicc.com｜Hi! How can I help?",
			want:  "Hi! How can I help?",
		},
		{
			name: "suffix ad with newline separator",
			config: &AdRemoveConfig{
				Keywords:       []string{"timicc", "137810429"},
				SuffixBoundary: sep,
			},
			input: "Hi! How can I help?\n\nADTEST | https://timicc.com, group: 137810429",
			want:  "Hi! How can I help?",
		},
		{
			name: "suffix ad with zero-width chars",
			config: &AdRemoveConfig{
				Keywords:       []string{"timicc"},
				SuffixBoundary: sep,
			},
			input: "Hi! How can I help?\n\nA\u200dDTEST | https://timicc.com",
			want:  "Hi! How can I help?",
		},
		{
			name: "suffix ad with dash separator",
			config: &AdRemoveConfig{
				Keywords:       []string{"timicc"},
				SuffixBoundary: sep,
			},
			input: "Hello world - timicc.com promo",
			want:  "Hello world",
		},
		{
			name: "no keyword match passes through",
			config: &AdRemoveConfig{
				Keywords:       []string{"adtest", "137810429"},
				PrefixBoundary: sep,
			},
			input: "Hello! How can I help you today?",
			want:  "Hello! How can I help you today?",
		},
		{
			name: "no boundary removes all",
			config: &AdRemoveConfig{
				Keywords:       []string{"adtest"},
				PrefixBoundary: sep,
				SuffixBoundary: sep,
			},
			input: "adtest promo content only",
			want:  "",
		},
		{
			name: "case insensitive keyword match",
			config: &AdRemoveConfig{
				Keywords:       []string{"ADTEST"},
				PrefixBoundary: sep,
			},
			input: "adtest promo - Hello world",
			want:  "Hello world",
		},
		{
			name: "replace_with provides substitute text",
			config: &AdRemoveConfig{
				Keywords:       []string{"adtest"},
				PrefixBoundary: sep,
				ReplaceWith:    "\n\n",
			},
			input: "adtest promo - Hi! How can I help?",
			want:  "\n\nHi! How can I help?",
		},
		{
			name: "both boundaries configured suffix wins",
			config: &AdRemoveConfig{
				Keywords:       []string{"timicc", "137810429"},
				PrefixBoundary: sep,
				SuffixBoundary: sep,
			},
			// Real case: suffix ad
			input: "Hi! How can I help today?\n\nTIMICC | https://timicc.com，QQ官方群：137810429",
			want:  "Hi! How can I help today?",
		},
		{
			name: "both boundaries configured prefix wins",
			config: &AdRemoveConfig{
				Keywords:       []string{"timicc", "137810429"},
				PrefixBoundary: sep,
				SuffixBoundary: sep,
			},
			// Real case: prefix ad with separator
			input: "官方Q群：137810429 - 马年特惠活动持续中 - Hi! How can I help?",
			want:  "Hi! How can I help?",
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
			PrefixBoundary: `(?:\s-\s)`,
		},
	}
	ar := NewAdRemover(def, logger)

	got := ar.Process("adtest promo - Hi there")
	if got != "Hi there" {
		t.Errorf("first Process() = %q, want %q", got, "Hi there")
	}

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
			PrefixBoundary: `(?:\s-\s)`,
		},
	}
	ar := NewAdRemover(def, logger)

	got := ar.Process("adtest promo - Hi there")
	if got != "Hi there" {
		t.Errorf("first Process() = %q, want %q", got, "Hi there")
	}

	ar.Reset()

	got = ar.Process("adtest again - Hi world")
	if got != "Hi world" {
		t.Errorf("after Reset Process() = %q, want %q", got, "Hi world")
	}
}
