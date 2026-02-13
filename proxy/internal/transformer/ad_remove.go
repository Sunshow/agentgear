package transformer

import (
	"regexp"
	"strings"

	"go.uber.org/zap"
)

// AdRemover detects and removes injected ad content from streaming text deltas.
// It works per-delta (ads appear in a single delta) and only processes the first
// delta that matches any configured keyword.
type AdRemover struct {
	config         *AdRemoveConfig
	logger         *zap.Logger
	name           string
	prefixBoundary *regexp.Regexp
	suffixBoundary *regexp.Regexp
	done           bool
}

func NewAdRemover(def *TransformerDef, logger *zap.Logger) *AdRemover {
	ar := &AdRemover{
		config: def.AdRemove,
		logger: logger,
		name:   def.Name,
	}
	if def.AdRemove.PrefixBoundary != "" {
		ar.prefixBoundary = regexp.MustCompile(def.AdRemove.PrefixBoundary)
	}
	if def.AdRemove.SuffixBoundary != "" {
		ar.suffixBoundary = regexp.MustCompile(def.AdRemove.SuffixBoundary)
	}
	return ar
}

// Process checks a text delta for ad content and removes it if found.
// Returns the cleaned text. Empty string means suppress the delta.
func (ar *AdRemover) Process(text string) string {
	if ar.done {
		return text
	}

	cleaned, mapping := stripZeroWidthChars(text)
	cleanedLower := strings.ToLower(cleaned)

	matchedKeyword := ""
	keywordPos := -1
	for _, kw := range ar.config.Keywords {
		idx := strings.Index(cleanedLower, strings.ToLower(kw))
		if idx >= 0 {
			matchedKeyword = kw
			keywordPos = idx
			break
		}
	}

	if keywordPos < 0 {
		return text
	}

	ar.done = true
	ar.logger.Info("ad_remove detected ad",
		zap.String("transformer", ar.name),
		zap.String("keyword", matchedKeyword))

	// Try suffix boundary first: if keyword appears after the boundary match, it's a suffix ad
	if ar.suffixBoundary != nil {
		loc := ar.suffixBoundary.FindStringIndex(cleaned)
		if loc != nil && keywordPos >= loc[0] {
			return ar.removeSuffix(cleaned, mapping, text, loc)
		}
	}

	// Try prefix boundary: if keyword appears before the boundary match, it's a prefix ad
	if ar.prefixBoundary != nil {
		loc := ar.prefixBoundary.FindStringIndex(cleaned)
		if loc != nil && keywordPos < loc[0] {
			return ar.removePrefix(cleaned, mapping, text, loc)
		}
	}

	// No boundary matched, remove entire delta
	ar.logger.Info("ad_remove full removal (no boundary matched)",
		zap.String("transformer", ar.name))
	return ar.config.ReplaceWith
}

// removePrefix removes ad content from the beginning of text up to the boundary.
func (ar *AdRemover) removePrefix(cleaned string, mapping []int, original string, boundaryLoc []int) string {
	origIdx := mapping[boundaryLoc[0]]
	result := original[origIdx:]
	ar.logger.Info("ad_remove prefix removed",
		zap.String("transformer", ar.name))
	return ar.config.ReplaceWith + result
}

// removeSuffix removes ad content from the boundary to the end of text.
func (ar *AdRemover) removeSuffix(cleaned string, mapping []int, original string, boundaryLoc []int) string {
	origIdx := mapping[boundaryLoc[0]]
	result := original[:origIdx]
	ar.logger.Info("ad_remove suffix removed",
		zap.String("transformer", ar.name))
	return result + ar.config.ReplaceWith
}

// Reset resets state for a new text content block.
func (ar *AdRemover) Reset() {
	ar.done = false
}
