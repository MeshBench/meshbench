package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Where a verb is registered and what the published surface says about it are
// two statements of one decision, so they are held together here: a verb the
// workbench calls itself has no client façade, and a verb with no client façade
// is either one of those or one of the few a window alone can act on.
//
// Both directions, because each has already gone wrong on its own: facade.json
// named a client call for five verbs no client could reach, and the journal
// kept its own hand-written copy of the same list.
func TestTheInternalVerbsAndTheFacadeAgree(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "tools", "verbdoc", "facade.json"))
	if err != nil {
		t.Skip("no facade.json:", err)
	}
	var facade struct {
		Facade   map[string]string `json:"facade"`
		NoFacade map[string]string `json:"no_facade"`
	}
	if err := json.Unmarshal(b, &facade); err != nil {
		t.Fatal(err)
	}

	st, _ := Boot(Options{NoPrefs: true, Headless: true})
	internal := map[string]bool{}
	for _, v := range st.InternalVerbs() {
		internal[v] = true
		if call := facade.Facade[v]; call != "" {
			t.Errorf("%s is the workbench's own callback and the socket refuses "+
				"it, but facade.json offers clients %s", v, call)
		}
		if facade.NoFacade[v] == "" {
			t.Errorf("%s is internal but facade.json gives no reason for having "+
				"no client call; add one to no_facade", v)
		}
	}
	// A verb with no façade that is not internal is a verb only an attached
	// interface can act on. Naming them keeps the exception a short list rather
	// than a shrug.
	interfaceOnly := map[string]bool{"resource.licence.hide": true}
	for v := range facade.NoFacade {
		if !internal[v] && !interfaceOnly[v] {
			t.Errorf("%s has no client façade but is registered as a public "+
				"verb; register it with st.HandleInternal or give it a call", v)
		}
	}
}

// Every callback is kept out of the journal, from the registration rather than
// from a list beside it: the journal is a history of what drove the world, and
// a worker reporting its own progress drove nothing.
func TestTheJournalSkipsEveryInternalVerb(t *testing.T) {
	st, _ := Boot(Options{NoPrefs: true, Headless: true})
	for _, v := range st.InternalVerbs() {
		if !st.SkipsJournal(v) {
			t.Errorf("%s is internal but would be journalled", v)
		}
	}
}
