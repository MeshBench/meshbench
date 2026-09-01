package resources_test

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/resources"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func check(t *testing.T) map[string]any {
	t.Helper()
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go store.Run(ctx)

	got, err := store.Do(ctx, "setup.check", nil)
	if err != nil {
		t.Fatalf("setup.check: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("setup.check returned %T, want an object", got)
	}
	return m
}

func groupsOf(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	g, ok := m["groups"].([]map[string]any)
	if !ok {
		t.Fatalf("setup.check answered %v, with no groups in it", m)
	}
	return g
}

// The whole point of the check: every dependency in one answer, over the socket
// as well as in the window. A machine that could only be surveyed by opening
// four panels is the machine this was written for.
func TestTheCheckNamesEveryDependencyInOnePlace(t *testing.T) {
	groups := groupsOf(t, check(t))
	want := map[string]bool{
		"This build": false, "Firmware": false, "Terrain": false,
		"Emulator toolchain": false,
	}
	rows := 0
	for _, g := range groups {
		name, _ := g["name"].(string)
		if _, ok := want[name]; ok {
			want[name] = true
		}
		list, _ := g["rows"].([]map[string]any)
		rows += len(list)
	}
	for name, found := range want {
		if !found {
			t.Errorf("the check has no %q group", name)
		}
	}
	if rows == 0 {
		t.Fatal("the check reported no rows at all")
	}
}

// Every row has to be actionable in words, whether or not it is actionable by a
// button. The rows that matter most - an emulator with no build for this
// platform - are exactly the ones no button can fix, and those used to be
// discoverable only by starting a node and reading the failure.
func TestEveryRowSaysWhatToDoAboutIt(t *testing.T) {
	for _, g := range groupsOf(t, check(t)) {
		rows, _ := g["rows"].([]map[string]any)
		for _, r := range rows {
			name, _ := r["name"].(string)
			if s, _ := r["state"].(string); s == "" {
				t.Errorf("%s has no state", name)
			}
			if do, _ := r["do"].(string); strings.TrimSpace(do) == "" {
				t.Errorf("%s says nothing about what to do", name)
			}
			// A verb without parameters is a button that would refuse, and a
			// refusal a script has to try before it can read.
			if v, _ := r["verb"].(string); v != "" {
				if p, ok := r["params"].(map[string]any); !ok || len(p) == 0 {
					t.Errorf("%s offers %s with no parameters", name, v)
				}
			}
		}
	}
}

// The counts are the answer a script actually wants, and they have to agree
// with the rows they were counted from.
func TestTheCountsAgreeWithTheRows(t *testing.T) {
	m := check(t)
	total := 0
	for _, key := range []string{"ready", "needed", "undecided", "blocked", "missing"} {
		n, ok := m[key].(int)
		if !ok {
			t.Fatalf("setup.check gave no %s count", key)
		}
		total += n
	}
	rows := 0
	for _, g := range groupsOf(t, m) {
		list, _ := g["rows"].([]map[string]any)
		rows += len(list)
	}
	if total != rows {
		t.Errorf("the counts add up to %d and there are %d rows", total, rows)
	}
}

// A session with no settings file has been told nothing and asks nothing, so
// terrain reads as allowed rather than as a question - the check must not
// invent an "undecided" for a machine that has nowhere to keep an answer.
func TestTerrainIsNotAskedWhereNoAnswerCouldBeKept(t *testing.T) {
	for _, g := range groupsOf(t, check(t)) {
		if g["name"] != "Terrain" {
			continue
		}
		rows, _ := g["rows"].([]map[string]any)
		if len(rows) != 1 {
			t.Fatalf("the terrain group has %d rows, want 1", len(rows))
		}
		if s, _ := rows[0]["state"].(string); s != string(state.SetupReady) {
			t.Errorf("terrain reads %q on a session with no settings file", s)
		}
	}
}
