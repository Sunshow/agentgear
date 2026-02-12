package transformer

import (
	"strings"

	"go.uber.org/zap"
)

// ContentReplacer handles text content replacement in streaming responses.
// It buffers the first delta to detect and remove ad-like prefixes,
// then passes through subsequent deltas directly.
type ContentReplacer struct {
	patterns []ContentReplacePattern
	logger   *zap.Logger
	name     string

	// per-block state
	firstDelta bool
	done       bool // true after first delta processed, skip further checks
}

func NewContentReplacer(def *TransformerDef, logger *zap.Logger) *ContentReplacer {
	return &ContentReplacer{
		patterns:   def.ContentPatterns,
		logger:     logger,
		name:       def.Name,
		firstDelta: true,
	}
}

// Reset resets state for a new text content block
func (cr *ContentReplacer) Reset() {
	cr.firstDelta = true
	cr.done = false
}

// Process applies replacement patterns to text delta content.
// Returns the processed text. Empty string means the delta should be suppressed.
func (cr *ContentReplacer) Process(text string) string {
	if cr.done {
		return text
	}

	// Only check the first delta of each text block
	if cr.firstDelta {
		cr.firstDelta = false
		result := cr.applyPatterns(text)
		cr.done = true
		return result
	}

	cr.done = true
	return text
}

func (cr *ContentReplacer) applyPatterns(text string) string {
	for _, p := range cr.patterns {
		idx := strings.Index(text, p.Match)
		if idx < 0 {
			continue
		}

		// Find the end of the ad segment
		endIdx := idx + len(p.Match)

		if p.TrimAfter != "" {
			// Find the trim_after separator after the match marker
			afterMatch := text[endIdx:]
			sepIdx := strings.Index(afterMatch, p.TrimAfter)
			if sepIdx >= 0 {
				// Move past the separator
				endIdx += sepIdx + len(p.TrimAfter)
				// Continue consuming additional matching separators (e.g. extra \n)
				remaining := text[endIdx:]
				trimmed := strings.TrimLeft(remaining, "\n\r ")
				endIdx += len(remaining) - len(trimmed)
			} else {
				// No separator found, remove from match to end
				endIdx = len(text)
			}
		}

		before := text[:idx]
		after := text[endIdx:]
		text = before + p.ReplaceWith + after

		cr.logger.Info("content_replace applied",
			zap.String("transformer", cr.name),
			zap.String("match", p.Match))
	}

	return text
}

// ProcessNonStreaming applies replacement patterns to a complete text string (for non-streaming responses).
func (cr *ContentReplacer) ProcessNonStreaming(text string) string {
	return cr.applyPatterns(text)
}
