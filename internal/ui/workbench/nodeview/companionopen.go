package nodeview

import "strings"

// OpenOnCompanionMode is which way in a companion's tab shows when it is
// first drawn: Client, except when a capture is being taken of one of the
// others. The CLI is where meshcore-cli's answers land, and a mode that only
// a click can reach is one no screenshot or check can show.
var OpenOnCompanionMode companionMode

// CompanionModeByName is a mode named the way its switch is labelled, ignoring
// case; an empty name is the default.
func CompanionModeByName(name string) (companionMode, bool) {
	if strings.TrimSpace(name) == "" {
		return modeClient, true
	}
	for _, m := range companionModes {
		if strings.EqualFold(m.Label, name) {
			return m.Mode, true
		}
	}
	return modeClient, false
}

// CompanionModeNames is every mode, for saying what was possible when a name
// matched none.
func CompanionModeNames() []string {
	out := make([]string, 0, len(companionModes))
	for _, m := range companionModes {
		out = append(out, m.Label)
	}
	return out
}
