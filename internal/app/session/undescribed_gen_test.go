package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Writes the ratchet list from the tree. Run with -update-manifest alongside
// the manifest itself; it is a separate test only so the manifest one stays
// about the manifest.
func TestWriteUndescribedList(t *testing.T) {
	if !*updateManifest {
		t.Skip("set -update-manifest to rewrite docs/verbs-undescribed.txt")
	}
	st, _ := Boot(Options{NoPrefs: true, Headless: true})
	var b strings.Builder
	b.WriteString("# Verbs that do not yet say what they are or what they take.\n")
	b.WriteString("#\n")
	b.WriteString("# A ratchet, not a list of exemptions: it may only shrink. Describe a verb\n")
	b.WriteString("# at its st.HandleSpec call and delete its line from here. A verb added\n")
	b.WriteString("# today is not on this list, so it has to describe itself.\n")
	b.WriteString("#\n")
	b.WriteString("# Regenerate with:\n")
	b.WriteString("#   go test ./internal/app/session -run 'TestTheVerbManifest|TestWriteUndescribed' -update-manifest\n")
	for _, v := range st.Undescribed() {
		b.WriteString(v)
		b.WriteString("\n")
	}
	path := filepath.Join("..", "..", "..", "docs", "verbs-undescribed.txt")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s: %d verbs still to describe", path, len(st.Undescribed()))
}
