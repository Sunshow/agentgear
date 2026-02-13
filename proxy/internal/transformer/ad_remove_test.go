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
				Keywords:       []string{"900100200"},
				PrefixBoundary: sep,
			},
			input: "Official group: 900100200 - promo ongoing - Hi! How can I help?",
			want:  "Hi! How can I help?",
		},
		{
			name: "prefix ad with multiple separators inside ad",
			config: &AdRemoveConfig{
				Keywords:       []string{"900100200", "ADGROUP"},
				PrefixBoundary: sep,
			},
			input: "ADGROUP：900100200 - seasonal promo ongoing - Hi! How can I help?",
			want:  "Hi! How can I help?",
		},
		{
			name: "prefix ad with zero-width chars and no separator",
			config: &AdRemoveConfig{
				Keywords:       []string{"900100200", "advendor"},
				PrefixBoundary: sep,
			},
			input: "\u200dPromo, group: 9\u200d00100200, https://advendor.comHi! How can I help?",
			want:  "",
		},
		{
			name: "prefix ad with pipe separator",
			config: &AdRemoveConfig{
				Keywords:       []string{"advendor"},
				PrefixBoundary: sep,
			},
			input: "Seasonal promo ongoing｜https://advendor.com｜Hi! How can I help?",
			want:  "Hi! How can I help?",
		},
		{
			name: "suffix ad with newline separator",
			config: &AdRemoveConfig{
				Keywords:       []string{"advendor", "900100200"},
				SuffixBoundary: sep,
			},
			input: "Hi! How can I help?\n\nADTEST | https://advendor.com, group: 900100200",
			want:  "Hi! How can I help?",
		},
		{
			name: "suffix ad with zero-width chars",
			config: &AdRemoveConfig{
				Keywords:       []string{"advendor"},
				SuffixBoundary: sep,
			},
			input: "Hi! How can I help?\n\nA\u200dDTEST | https://advendor.com",
			want:  "Hi! How can I help?",
		},
		{
			name: "suffix ad with dash separator",
			config: &AdRemoveConfig{
				Keywords:       []string{"advendor"},
				SuffixBoundary: sep,
			},
			input: "Hello world - advendor.com promo",
			want:  "Hello world",
		},
		{
			name: "no keyword match passes through",
			config: &AdRemoveConfig{
				Keywords:       []string{"adtest", "900100200"},
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
				Keywords:       []string{"advendor", "900100200"},
				PrefixBoundary: sep,
				SuffixBoundary: sep,
			},
			input: "Hi! How can I help today?\n\nADVENDOR | https://advendor.com, group: 900100200",
			want:  "Hi! How can I help today?",
		},
		{
			name: "both boundaries configured prefix wins",
			config: &AdRemoveConfig{
				Keywords:       []string{"advendor", "900100200"},
				PrefixBoundary: sep,
				SuffixBoundary: sep,
			},
			input: "ADGROUP：900100200 - seasonal promo ongoing - Hi! How can I help?",
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

func TestAdRemover_ProcessMultiDelta(t *testing.T) {
	logger := zap.NewNop()
	sep := `(?:\n\n|\s-\s|[|｜])`

	t.Run("ad in last delta", func(t *testing.T) {
		def := &TransformerDef{
			Name: "test_ad_remove",
			AdRemove: &AdRemoveConfig{
				Keywords:       []string{"advendor", "900100200"},
				SuffixBoundary: sep,
			},
		}
		ar := NewAdRemover(def, logger)

		got := ar.Process("Hi")
		if got != "Hi" {
			t.Errorf("delta 1: got %q, want %q", got, "Hi")
		}
		got = ar.Process("! How can I help you today?")
		if got != "! How can I help you today?" {
			t.Errorf("delta 2: got %q, want %q", got, "! How can I help you today?")
		}
		got = ar.Process("\n\nADGROUP：900100200 | advendor.com | seasonal promo")
		if got != "" {
			t.Errorf("delta 3 (ad): got %q, want empty", got)
		}
	})

	t.Run("ad-only delta (whole block is ad)", func(t *testing.T) {
		def := &TransformerDef{
			Name: "test_ad_remove",
			AdRemove: &AdRemoveConfig{
				Keywords:       []string{"900100200"},
				PrefixBoundary: sep,
				SuffixBoundary: sep,
			},
		}
		ar := NewAdRemover(def, logger)

		got := ar.Process("Seasonal promo ongoing,")
		if got != "Seasonal promo ongoing," {
			t.Errorf("no keyword delta: got %q, want passthrough", got)
		}

		got = ar.Process("ADGROUP：900100200, seasonal promo ongoing")
		if got != "" {
			t.Errorf("ad-only delta no sep: got %q, want empty", got)
		}
	})

	t.Run("ad in first delta normal in later", func(t *testing.T) {
		def := &TransformerDef{
			Name: "test_ad_remove",
			AdRemove: &AdRemoveConfig{
				Keywords:       []string{"900100200"},
				PrefixBoundary: sep,
			},
		}
		ar := NewAdRemover(def, logger)

		got := ar.Process("ADGROUP：900100200 - seasonal promo ongoing - Hi! How can I help?")
		if got != "Hi! How can I help?" {
			t.Errorf("delta 1 (ad): got %q, want %q", got, "Hi! How can I help?")
		}
		got = ar.Process(" More content here")
		if got != " More content here" {
			t.Errorf("delta 2: got %q, want passthrough", got)
		}
	})

	t.Run("each delta checked independently", func(t *testing.T) {
		def := &TransformerDef{
			Name: "test_ad_remove",
			AdRemove: &AdRemoveConfig{
				Keywords:       []string{"adtest"},
				PrefixBoundary: sep,
				SuffixBoundary: sep,
			},
		}
		ar := NewAdRemover(def, logger)

		got := ar.Process("Hello world")
		if got != "Hello world" {
			t.Errorf("delta 1: got %q, want passthrough", got)
		}
		got = ar.Process("adtest promo content")
		if got != "" {
			t.Errorf("delta 2: got %q, want empty", got)
		}
		got = ar.Process("another adtest ad")
		if got != "" {
			t.Errorf("delta 3: got %q, want empty", got)
		}
		got = ar.Process("clean content")
		if got != "clean content" {
			t.Errorf("delta 4: got %q, want passthrough", got)
		}
	})
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
