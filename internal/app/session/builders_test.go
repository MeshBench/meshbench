package session

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// unsetBy is the fields of a snapshot type that a builder never names.
//
// Read out of the composite literal rather than exercised, because the fault
// this catches is a field nothing writes: a builder that leaves one out
// produces a perfectly valid value, publishes a zero, and is indistinguishable
// from a network where the answer really is empty.
func unsetBy(t *testing.T, file, fn string, typ reflect.Type) []string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(src), "func "+fn+"(")
	if i < 0 {
		t.Fatalf("%s has moved out of %s; this test reads it by name", fn, file)
	}
	body := string(src[i:])
	if j := strings.Index(body, "\nfunc "); j > 0 {
		body = body[:j]
	}
	set := map[string]bool{}
	for _, m := range fieldKey.FindAllStringSubmatch(body, -1) {
		set[m[1]] = true
	}
	var missing []string
	for i := 0; i < typ.NumField(); i++ {
		if name := typ.Field(i).Name; !set[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

var fieldKey = regexp.MustCompile(`([A-Z][A-Za-z0-9]*):`)

// Does the whole of a node survive the one builder?
//
// The check that makes the always-empty fields impossible to reintroduce rather
// than merely fixed. There were two builders, they set different subsets, and
// nothing compared them: one left Hardware and the five card fields out and was
// the path every project.open took, the other left TrueRF out and was the path
// every import took. There is one now, and this holds it to every field the
// snapshot type declares, so a field added to state.Node and not to the builder
// fails on the commit that adds it rather than reading as an answer over the
// socket for however long it takes somebody to notice.
func TestTheNodeBuilderSetsEveryFieldANodeHas(t *testing.T) {
	missing := unsetBy(t, "importcommit.go", "stateNodes", reflect.TypeOf(state.Node{}))
	if len(missing) > 0 {
		t.Errorf("stateNodes leaves %s unset; every field of state.Node is "+
			"published over the socket, and one nothing writes reads as an "+
			"answer rather than as a gap", strings.Join(missing, ", "))
	}
}

// And the same of a line of conversation.
//
// state.CompanionMessage carried a Receipt and a Failed that messageView never
// set and nothing else wrote, while docs/scripting-types.md described the
// receipt as the thing the simulator can answer and a real phone cannot. Two of
// them are gone; this is what stops a third being added to the type and left
// unwritten.
func TestTheMessageBuilderSetsEveryFieldAMessageHas(t *testing.T) {
	missing := unsetBy(t, "companionview.go", "messageView",
		reflect.TypeOf(state.CompanionMessage{}))
	if len(missing) > 0 {
		t.Errorf("messageView leaves %s unset; a field of a published type that "+
			"nothing writes is a promise the tree does not keep",
			strings.Join(missing, ", "))
	}
}
