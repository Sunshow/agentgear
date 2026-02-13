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

	// Find first keyword position and last keyword end position
	firstKeywordPos := -1
	lastKeywordEnd := -1
	matchedKeyword := ""
	for _, kw := range ar.config.Keywords {
		kwLower := strings.ToLower(kw)
		idx := strings.Index(cleanedLower, kwLower)
		if idx >= 0 {
			if matchedKeyword == "" {
				matchedKeyword = kw
			}
			if firstKeywordPos < 0 || idx < firstKeywordPos {
				firstKeywordPos = idx
			}
			end := idx + len(kwLower)
			if end > lastKeywordEnd {
				lastKeywordEnd = end
			}
		}
	}

	if firstKeywordPos < 0 {
		return text
	}

	ar.done = true
	ar.logger.Info("ad_remove detected ad",
		zap.String("transformer", ar.name),
		zap.String("keyword", matchedKeyword))

	// Try suffix ad: find the FIRST separator BEFORE the first keyword
	// (earliest boundary between normal content and ad)
	if ar.suffixBoundary != nil {
		allMatches := ar.suffixBoundary.FindAllStringIndex(cleaned, -1)
		for i := 0; i < len(allMatches); i++ {
			if allMatches[i][0] < firstKeywordPos {
				origIdx := mapping[allMatches[i][0]]
				ar.logger.Info("ad_remove suffix removed",
					zap.String("transformer", ar.name))
				return text[:origIdx] + ar.config.ReplaceWith
			}
		}
	}

	// Try prefix ad: find the LAST separator AFTER the last keyword
	// (latest boundary between ad and normal content)
	if ar.prefixBoundary != nil {
		allMatches := ar.prefixBoundary.FindAllStringIndex(cleaned, -1)
		for i := len(allMatches) - 1; i >= 0; i-- {
			if allMatches[i][0] >= lastKeywordEnd {
				origIdx := mapping[allMatches[i][1]]
				ar.logger.Info("ad_remove prefix removed",
					zap.String("transformer", ar.name))
				return ar.config.ReplaceWith + text[origIdx:]
			}
		}
	}

	// No boundary matched, remove entire delta
	ar.logger.Info("ad_remove full removal (no boundary matched)",
		zap.String("transformer", ar.name))
	return ar.config.ReplaceWith
}

// Reset resets state for a new text content block.
func (ar *AdRemover) Reset() {
	ar.done = false
}
