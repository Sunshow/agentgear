package transformer

import (
	"testing"

	"go.uber.org/zap"
)

func TestContentReplacer_Process(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name     string
		patterns []ContentReplacePattern
		deltas   []string
		want     []string
	}{
		{
			name: "remove ad with trim_after",
			patterns: []ContentReplacePattern{
				{Match: "【ADTEST】", TrimAfter: "\n\n", ReplaceWith: ""},
			},
			deltas: []string{
				"【ADTEST】https://example.com promo text\n\n\n\nHi",
				"! How",
				" can I help you today?",
			},
			want: []string{
				"Hi",
				"! How",
				" can I help you today?",
			},
		},
		{
			name: "no match passes through",
			patterns: []ContentReplacePattern{
				{Match: "【ADTEST】", TrimAfter: "\n\n", ReplaceWith: ""},
			},
			deltas: []string{
				"Hello world",
				" how are you?",
			},
			want: []string{
				"Hello world",
				" how are you?",
			},
		},
		{
			name: "match without trim_after",
			patterns: []ContentReplacePattern{
				{Match: "[AD]", ReplaceWith: ""},
			},
			deltas: []string{
				"[AD]Hello",
				" world",
			},
			want: []string{
				"Hello",
				" world",
			},
		},
		{
			name: "match with replacement text",
			patterns: []ContentReplacePattern{
				{Match: "【广告】", TrimAfter: "\n\n", ReplaceWith: "[removed]"},
			},
			deltas: []string{
				"【广告】some ad content\n\nreal content",
			},
			want: []string{
				"[removed]real content",
			},
		},
		{
			name: "ad only in first delta, match at beginning",
			patterns: []ContentReplacePattern{
				{Match: "【ADTEST】", TrimAfter: "\n\n", ReplaceWith: ""},
			},
			deltas: []string{
				"【ADTEST】ad text\n\n",
				"actual content",
			},
			want: []string{
				"",
				"actual content",
			},
		},
		{
			name: "match with content before marker",
			patterns: []ContentReplacePattern{
				{Match: "【ADTEST】", TrimAfter: "\n\n", ReplaceWith: ""},
			},
			deltas: []string{
				"prefix【ADTEST】ad text\n\nsuffix",
			},
			want: []string{
				"prefixsuffix",
			},
		},
		{
			name: "strip_zero_width removes obfuscated ad",
			patterns: []ContentReplacePattern{
				{Match: "【ADTEST】", TrimAfter: "\n\n", StripZeroWidth: true},
			},
			deltas: []string{
				"【\u200CADTE\uFEFFST\u2060】https://example.com promo\n\n\n\nHi there",
			},
			want: []string{
				"Hi there",
			},
		},
		{
			name: "strip_zero_width no match passes through",
			patterns: []ContentReplacePattern{
				{Match: "【ADTEST】", TrimAfter: "\n\n", StripZeroWidth: true},
			},
			deltas: []string{
				"Hello world",
			},
			want: []string{
				"Hello world",
			},
		},
		{
			name: "strip_zero_width with content before marker",
			patterns: []ContentReplacePattern{
				{Match: "【ADTEST】", TrimAfter: "\n\n", StripZeroWidth: true},
			},
			deltas: []string{
				"prefix【\u200CADTE\uFEFFST\u2060】ad\n\nsuffix",
			},
			want: []string{
				"prefixsuffix",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &TransformerDef{
				Name:            "test",
				ContentPatterns: tt.patterns,
			}
			cr := NewContentReplacer(def, logger)

			for i, delta := range tt.deltas {
				got := cr.Process(delta)
				if got != tt.want[i] {
					t.Errorf("delta[%d]: got %q, want %q", i, got, tt.want[i])
				}
			}
		})
	}
}

func TestContentReplacer_Reset(t *testing.T) {
	logger := zap.NewNop()
	def := &TransformerDef{
		Name: "test",
		ContentPatterns: []ContentReplacePattern{
			{Match: "【AD】", TrimAfter: "\n\n", ReplaceWith: ""},
		},
	}
	cr := NewContentReplacer(def, logger)

	// First block
	got := cr.Process("【AD】ad\n\nHi")
	if got != "Hi" {
		t.Errorf("first block: got %q, want %q", got, "Hi")
	}

	// Reset for second block
	cr.Reset()

	got = cr.Process("【AD】another ad\n\nHello")
	if got != "Hello" {
		t.Errorf("second block after reset: got %q, want %q", got, "Hello")
	}
}

func TestContentReplacer_ProcessNonStreaming(t *testing.T) {
	logger := zap.NewNop()
	def := &TransformerDef{
		Name: "test",
		ContentPatterns: []ContentReplacePattern{
			{Match: "【ADTEST】", TrimAfter: "\n\n", ReplaceWith: ""},
		},
	}
	cr := NewContentReplacer(def, logger)

	text := "【ADTEST】https://example.com ad\n\n\n\nHello world"
	got := cr.ProcessNonStreaming(text)
	want := "Hello world"
	if got != want {
		t.Errorf("ProcessNonStreaming: got %q, want %q", got, want)
	}
}
