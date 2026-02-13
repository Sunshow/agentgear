package transformer

import (
	"strings"
	"unicode"

	"go.uber.org/zap"
)

// isZeroWidthRune returns true if the rune is a zero-width or invisible Unicode character.
func isZeroWidthRune(r rune) bool {
	switch {
	case r == 0x00AD: // soft hyphen
		return true
	case r == 0x034F: // combining grapheme joiner
		return true
	case r == 0x061C: // Arabic letter mark
		return true
	case r == 0x200B: // zero-width space
		return true
	case r == 0x200C: // zero-width non-joiner
		return true
	case r == 0x200D: // zero-width joiner
		return true
	case r == 0xFEFF: // BOM / zero-width no-break space
		return true
	case r >= 0x2060 && r <= 0x2064: // word joiner, invisible operators
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
		return true
	case r == 0x115F || r == 0x1160: // Hangul Choseong/Jungseong fillers
		return true
	case unicode.Is(unicode.Cf, r): // general format characters
		return true
	}
	return false
}

// stripZeroWidthChars removes all zero-width Unicode characters from text.
// Returns the cleaned string and a mapping from cleaned-string byte offsets
// to original-string byte offsets.
func stripZeroWidthChars(text string) (string, []int) {
	var buf strings.Builder
	buf.Grow(len(text))
	mapping := make([]int, 0, len(text))
	for i, r := range text {
		if !isZeroWidthRune(r) {
			runeStr := text[i : i+len(string(r))]
			buf.WriteString(runeStr)
			for j := range len(runeStr) {
				mapping = append(mapping, i+j)
			}
		}
	}
	// sentinel: map end-of-cleaned to end-of-original
	mapping = append(mapping, len(text))
	return buf.String(), mapping
}

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
		if p.StripZeroWidth {
			text = cr.applyPatternStripped(text, p)
		} else {
			text = cr.applyPatternExact(text, p)
		}
	}
	return text
}

func (cr *ContentReplacer) applyPatternExact(text string, p ContentReplacePattern) string {
	idx := strings.Index(text, p.Match)
	if idx < 0 {
		return text
	}

	endIdx := idx + len(p.Match)
	endIdx = cr.extendTrimAfter(text, endIdx, p.TrimAfter)

	cr.logger.Info("content_replace applied",
		zap.String("transformer", cr.name),
		zap.String("match", p.Match))

	return text[:idx] + p.ReplaceWith + text[endIdx:]
}

func (cr *ContentReplacer) applyPatternStripped(text string, p ContentReplacePattern) string {
	cleaned, mapping := stripZeroWidthChars(text)

	idx := strings.Index(cleaned, p.Match)
	if idx < 0 {
		return text
	}

	// Map cleaned offsets back to original offsets
	origStart := mapping[idx]
	cleanedEnd := idx + len(p.Match)
	origEnd := mapping[cleanedEnd]

	// For TrimAfter, work on the original text from origEnd onward
	origEnd = cr.extendTrimAfter(text, origEnd, p.TrimAfter)

	cr.logger.Info("content_replace applied (strip_zero_width)",
		zap.String("transformer", cr.name),
		zap.String("match", p.Match))

	return text[:origStart] + p.ReplaceWith + text[origEnd:]
}

func (cr *ContentReplacer) extendTrimAfter(text string, endIdx int, trimAfter string) int {
	if trimAfter == "" {
		return endIdx
	}

	afterMatch := text[endIdx:]
	sepIdx := strings.Index(afterMatch, trimAfter)
	if sepIdx >= 0 {
		endIdx += sepIdx + len(trimAfter)
		remaining := text[endIdx:]
		trimmed := strings.TrimLeft(remaining, "\n\r ")
		endIdx += len(remaining) - len(trimmed)
	} else {
		endIdx = len(text)
	}
	return endIdx
}

// ProcessNonStreaming applies replacement patterns to a complete text string (for non-streaming responses).
func (cr *ContentReplacer) ProcessNonStreaming(text string) string {
	return cr.applyPatterns(text)
}
