package id

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Standard JAV ID patterns, ordered by specificity.
// Each pattern captures the ID in group 1 and uses lookahead/lookbehind or
// non-word-char anchors to avoid matching mid-word, while also handling
// underscore suffixes like "_HD".
var patterns = []*regexp.Regexp{
	// FC2-PPV-XXXXXXX  (must come before generic pattern)
	regexp.MustCompile(`(?i)(?:^|[^A-Z0-9])(FC2-PPV-\d{5,8})(?:[^A-Z0-9]|$)`),
	// Standard: ABC-123 (2-5 letters, hyphen, 2-5 digits)
	regexp.MustCompile(`(?i)(?:^|[^A-Z0-9])([A-Z]{2,5}-\d{2,5})(?:[^A-Z0-9]|$)`),
}

// Extract returns the normalized JAV ID (uppercase) from a filename or path.
// Returns ("", false) if no recognizable ID is found.
func Extract(nameOrPath string) (string, bool) {
	base := filepath.Base(nameOrPath)
	// Remove extension
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	for _, re := range patterns {
		if sub := re.FindStringSubmatch(name); sub != nil {
			return strings.ToUpper(sub[1]), true
		}
	}
	return "", false
}
