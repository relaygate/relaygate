// Package confirm defines the unified typed confirmation phrases for
// sensitive Panel / CLI / API operations.
package confirm

import "strings"

const (
	// PhraseZH is the Chinese confirmation phrase (exact match).
	PhraseZH = "确认"
	// PhraseEN is the English confirmation phrase (exact match, case-sensitive).
	PhraseEN = "Confirm"
)

// Match reports whether s is a valid confirmation phrase after TrimSpace.
// Accepts PhraseZH or PhraseEN only (Confirm is case-sensitive).
func Match(s string) bool {
	switch strings.TrimSpace(s) {
	case PhraseZH, PhraseEN:
		return true
	default:
		return false
	}
}

// Hint is a short bilingual prompt for errors and CLI help.
func Hint() string {
	return PhraseZH + " / " + PhraseEN
}
