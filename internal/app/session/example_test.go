package session

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// A described verb carries a call somebody can copy.
//
// The rule for anything that describes itself at all, rather than something to
// work towards: whoever writes a description has the handler open in front of
// them, and that is the only moment the example is cheap.
//
// Internal callbacks are exempt because the socket refuses them: an example of
// a call no caller may make would be a lie in the shape of a fact.
func TestEveryDescribedPublicVerbCarriesAnExample(t *testing.T) {
	st, _ := Boot(Options{NoPrefs: true, Headless: true})

	var missing, offered []string
	for name, sp := range describedVerbs(t) {
		if st.IsInternal(name) {
			if sp.Example != nil {
				offered = append(offered, name)
			}
			continue
		}
		if sp.Example == nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("%d described verbs have no example:\n  %s\n"+
			"add an example to their entry in the .verbs.json beside the code",
			len(missing), strings.Join(missing, "\n  "))
	}
	if len(offered) > 0 {
		sort.Strings(offered)
		t.Errorf("%d internal callbacks carry an example of a call the socket "+
			"refuses:\n  %s", len(offered), strings.Join(offered, "\n  "))
	}
}

// An example that names a parameter the verb does not declare, or leaves out
// one it insists on, is worse than no example: it is a call that will be
// refused, printed in the reference as the way to make it.
func TestExamplesUseTheParametersTheVerbDeclares(t *testing.T) {
	for name, sp := range describedVerbs(t) {
		if sp.Example == nil {
			continue
		}
		t.Run(name, func(t *testing.T) { checkExample(t, name, sp) })
	}
}

func checkExample(t *testing.T, name string, sp state.Spec) {
	t.Helper()
	declared := map[string]state.Param{}
	var primary string
	for _, p := range sp.Params {
		declared[p.Name] = p
		if p.Primary {
			primary = p.Name
		}
	}
	given := map[string]bool{}
	switch v := sp.Example.Params.(type) {
	case nil:
	case map[string]any:
		for k := range v {
			if _, ok := declared[k]; !ok {
				t.Errorf("the example passes %q, which %s does not declare", k, name)
			}
			given[k] = true
		}
	default:
		// A bare value is the primary parameter said the short way, so a verb
		// with no primary has no short way to say anything.
		if primary == "" {
			t.Fatalf("the example passes a bare %T, but %s declares no primary parameter", v, name)
		}
		given[primary] = true
	}
	for _, p := range sp.Params {
		if p.Required && !given[p.Name] {
			t.Errorf("%s requires %q and the example omits it", name, p.Name)
		}
	}
	// The reference prints the example as a socket line, so it has to survive
	// being one.
	if _, err := json.Marshal(map[string]any{
		"id": 1, "method": name, "params": sp.Example.Params,
	}); err != nil {
		t.Errorf("the example does not marshal as a request: %v", err)
	}
}

// The examples marked runnable are run.
//
// This is what separates an example from a sentence about one. Each goes to a
// fresh session holding two nodes, because an example that only works after
// the one before it is not an example of anything a reader can do.
func TestRunnableExamplesAreAnswered(t *testing.T) {
	specs := describedVerbs(t)

	names := make([]string, 0, len(specs))
	for name, sp := range specs {
		if sp.Example != nil && sp.Example.Runnable {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("no example is marked runnable, so nothing here is checked")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			fresh, _ := aNetwork(t)
			if _, err := fresh.Do(t.Context(), name, specs[name].Example.Params); err != nil {
				t.Errorf("the example was refused: %v", err)
			}
		})
	}
	t.Logf("%d of %d described verbs have an example that runs", len(names), len(specs))
}
