package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The worker-callback exclusions must stay in step with the verbs facade.json
// marks as having no client call - the same set for the same reason. A new one
// added there must be excluded here, or it becomes journal noise.
func TestJournalSkipsEveryWorkerCallback(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "tools", "verbdoc", "facade.json"))
	if err != nil {
		t.Skip("no facade.json:", err)
	}
	var facade struct {
		NoFacade map[string]string `json:"no_facade"`
	}
	if err := json.Unmarshal(b, &facade); err != nil {
		t.Fatal(err)
	}
	excluded := map[string]bool{}
	for _, v := range journalWorkerCallbacks {
		excluded[v] = true
	}
	for v := range facade.NoFacade {
		if !excluded[v] {
			t.Errorf("%s has no client façade (a worker callback) but is not "+
				"excluded from the journal; add it to journalWorkerCallbacks", v)
		}
	}
}
