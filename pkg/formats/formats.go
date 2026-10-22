package formats

import (
	"regexp"
	"strings"

	"dep_service/pkg/depmap"
)

var reBracketedTokenInText = regexp.MustCompile(`\[[\p{L}\p{N}_]+\]`)

// RestoreCellString checks if val can be restored via dm.
// 1. Exact match: dm.Get(val) or dm.Get(trimmed)
// 2. Substring match: finds bracketed tokens [Type_XXXX] in val and replaces them with their original values if found in dm.
func RestoreCellString(val string, dm *depmap.DepMap) (string, bool) {
	if dm == nil || len(dm.Mapping) == 0 || val == "" {
		return val, false
	}

	// 1. Exact match
	if orig, ok := dm.Get(val); ok {
		return orig, true
	}
	trimmed := strings.TrimSpace(strings.TrimPrefix(val, "\ufeff"))
	if trimmed != val {
		if orig, ok := dm.Get(trimmed); ok {
			return orig, true
		}
	}

	// 2. Substring token replacement for bracketed tokens [Type_XXXX]
	matches := reBracketedTokenInText.FindAllString(val, -1)
	if len(matches) == 0 {
		return val, false
	}

	restored := val
	modified := false
	for _, m := range matches {
		if orig, ok := dm.Get(m); ok {
			restored = strings.ReplaceAll(restored, m, orig)
			modified = true
		}
	}
	return restored, modified
}
