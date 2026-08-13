package session

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// Control socket parity, measured rather than remembered.
//
// The two workbenches are meant to be comparable - that is the whole reason
// the old one is still here - and the socket is how anything automated talks
// to either. A verb the old one answers and this one does not is a script that
// silently cannot be pointed at the new build.
//
// This does not demand parity today; it stops it getting worse. Every verb
// still missing is named below, so the number is a fact in the repository
// rather than a figure in somebody's head, and closing one means deleting a
// line here.

// stillMissing are the old socket's verbs this build does not answer yet.
var stillMissing = map[string]string{}

var oldVerb = regexp.MustCompile(`case "([a-z_]+\.[a-z_]+)"`)

// oldVerbs is what internal/ui answers, read from its dispatch rather than
// from a list somebody maintains.
//
// Only control.go, which is the dispatch. Reading every file counted
// "loop.detect" as a verb because a sweep parameter switch happens to use the
// same shape, and a denominator with a phantom in it makes the last verb
// impossible to close.
func oldVerbs(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "ui", "control.go"))
	if err != nil {
		t.Skipf("no old workbench to compare against: %v", err)
	}
	seen := map[string]bool{}
	for _, m := range oldVerb.FindAllSubmatch(b, -1) {
		seen[string(m[1])] = true
	}
	var out []string
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func TestSocketParityDoesNotGetWorse(t *testing.T) {
	st := state.New(10)
	Register(st, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	var missing, closed []string
	for _, v := range oldVerbs(t) {
		_, err := st.Do(ctx, v, nil)
		answered := err == nil || !strings.Contains(err.Error(), "unknown verb")
		_, known := stillMissing[v]
		switch {
		case !answered && !known:
			missing = append(missing, v)
		case answered && known:
			closed = append(closed, v)
		}
	}
	if len(missing) > 0 {
		t.Errorf("these verbs were answered before and are not now: %v", missing)
	}
	// Closing one is good news, and the list has to be edited so the count
	// stays true. Otherwise this test slowly stops meaning anything.
	if len(closed) > 0 {
		t.Errorf("these are implemented now - delete them from stillMissing: %v", closed)
	}
	t.Logf("socket parity: %d of %d old verbs answered",
		len(oldVerbs(t))-len(stillMissing), len(oldVerbs(t)))
}
