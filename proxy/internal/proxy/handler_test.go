package proxy

import (
	"testing"

	"github.com/sunshow/agentgear/proxy/internal/transformer"
)

func TestMatchModelPattern(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		pattern  string
		expected bool
	}{
		{"exact match", "claude-opus-4-6", "claude-opus-4-6", true},
		{"wildcard match 1", "claude-opus-4-6", "claude-opus-4-6*", true},
		{"wildcard match 2", "claude-opus-4-6-20260101", "claude-opus-4-6*", true},
		{"wildcard match 3", "claude-opus-4.6", "claude-opus-4.6*", true},
		{"wildcard match 4", "claude-opus-4.6-20260101", "claude-opus-4.6*", true},
		{"no match 1", "claude-sonnet-4", "claude-opus-4-6*", false},
		{"no match 2", "claude-opus-3-5", "claude-opus-4-6*", false},
		{"empty pattern", "claude-opus-4-6", "", false},
		{"empty model", "", "claude-opus-4-6*", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchModelPattern(tt.model, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchModelPattern(%q, %q) = %v, want %v", tt.model, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestGetContextTokenLimit(t *testing.T) {
	handler := &transformer.TransformerDef{
		ContextTokenLimit: 200000,
		ModelContextLimits: []transformer.ModelContextLimit{
			{ModelPattern: "claude-opus-4-6*", TokenLimit: 1000000},
			{ModelPattern: "claude-opus-4.6*", TokenLimit: 1000000},
		},
	}

	tests := []struct {
		name     string
		model    string
		expected int
	}{
		{"opus 4-6", "claude-opus-4-6", 1000000},
		{"opus 4-6 with date", "claude-opus-4-6-20260101", 1000000},
		{"opus 4.6", "claude-opus-4.6", 1000000},
		{"opus 4.6 with date", "claude-opus-4.6-20260201", 1000000},
		{"sonnet 4 (default)", "claude-sonnet-4", 200000},
		{"opus 3.5 (default)", "claude-opus-3-5", 200000},
		{"empty model (default)", "", 200000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getContextTokenLimit(handler, tt.model)
			if result != tt.expected {
				t.Errorf("getContextTokenLimit(%q) = %d, want %d", tt.model, result, tt.expected)
			}
		})
	}
}
