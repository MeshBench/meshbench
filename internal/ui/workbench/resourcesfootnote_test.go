package workbench

import (
	"strings"
	"testing"
)

// The Resources page does not claim that nothing arrives unasked.
//
// It did, in its last line, while its own "Filled itself" card three rows
// higher counted 64.0 MB that had. On a fresh install that is most of what the
// page is accounting for - the map tiles and the ground under the study both
// fill themselves - so the one sentence on the page whose job is to be true
// about every row above it was the one that was not.
func TestTheResourcesFootnoteDoesNotDenyWhatThePageCounts(t *testing.T) {
	if strings.Contains(resourcesFootnote, "without being asked") {
		t.Errorf("the footnote still denies unasked-for downloads, which the "+
			"page's own Filled itself card contradicts:\n%s", resourcesFootnote)
	}
	// And it uses the rows' own vocabulary, so a reader can match the sentence
	// to the rows it is about rather than learning a second wording for the
	// same split.
	if !strings.Contains(resourcesFootnote, "only when asked") {
		t.Errorf("the footnote should name the rows it is about in their own "+
			"words:\n%s", resourcesFootnote)
	}
}
