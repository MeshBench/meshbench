package resources_test

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/resources"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func listResources(t *testing.T) map[string]any {
	t.Helper()
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go store.Run(ctx)

	got, err := store.Do(ctx, "resource.list", nil)
	if err != nil {
		t.Fatalf("resource.list: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("resource.list returned %T, want an object", got)
	}
	return m
}

// The resource verbs moved out of session; resource.list still registers and
// answers (even an empty cache is rows, not an error), and resource.remove
// still refuses something the inventory does not hold. Driven through the
// store, as a client would.
func TestResourceListAndRemove(t *testing.T) {
	if _, ok := listResources(t)["rows"]; !ok {
		t.Error("resource.list returned no row count")
	}

	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	if _, err := store.Do(ctx, "resource.remove", map[string]any{
		"kind": "nothing", "name": "nowhere",
	}); err == nil {
		t.Error("resource.remove accepted a resource the inventory does not hold")
	}
}

// The rows themselves, not a count of them.
//
// It answered {"rows": 5} and left the rows in the snapshot, where only a
// panel could reach them, so from outside the window there was no way to ask
// what this machine holds - which is the same fault nodes.stats,
// firmware.library, the study area and console.read each had.
func TestResourceListReturnsTheRowsAndNotJustACount(t *testing.T) {
	m := listResources(t)
	rows, ok := m["resources"].([]map[string]any)
	if !ok {
		t.Fatalf("resource.list answered %v, with no rows in it", m)
	}
	if len(rows) == 0 {
		t.Fatal("resource.list returned an empty row list")
	}
	if n, ok := m["rows"].(int); !ok || n != len(rows) {
		t.Errorf("the count says %v and there are %d rows", m["rows"], len(rows))
	}
	for _, r := range rows {
		for _, key := range []string{"kind", "name", "state", "why"} {
			if _, ok := r[key]; !ok {
				t.Errorf("a row is missing %q: %v", key, r)
			}
		}
		if r["name"] == "" || r["state"] == "" {
			t.Errorf("a row has no name or no state: %v", r)
		}
		// What a caller may do about it, which is the question a script asks
		// straight after "what is there".
		if _, ok := r["fetchable"].(bool); !ok {
			t.Errorf("a row does not say whether it can be fetched: %v", r)
		}
	}
}

// The emulator toolchain is on the page, which it was not: the five providers
// were the SoftDevice and four caches, so there was no path to the chip model,
// QEMU or Renode from inside the application at all.
func TestTheEmulatorToolchainIsListed(t *testing.T) {
	rows, _ := listResources(t)["resources"].([]map[string]any)
	want := map[string]bool{
		"virtual-sx1262": false, "qemu-system-xtensa": false, "renode": false,
	}
	for _, r := range rows {
		name, _ := r["name"].(string)
		if _, ok := want[name]; !ok {
			continue
		}
		want[name] = true
		if r["kind"] != string(resource.ToolchainKind) {
			t.Errorf("%s is listed under kind %v", name, r["kind"])
		}
		// Fetchable or explained. A row that is neither is a page telling
		// somebody nothing, which is what this replaced.
		if fetchable, _ := r["fetchable"].(bool); !fetchable {
			if why, _ := r["why"].(string); why == "" {
				t.Errorf("%s cannot be fetched and does not say why", name)
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("%s is not on the resources page", name)
		}
	}
}

// A row that cannot be fetched from this page still has to say where the thing
// comes from.
//
// Building footprints sat here at nothing, Fetch disabled, with "fills itself
// as the map is used" beside it: true of terrain and false of them. They are
// pulled from Configuration > Environ, and the page that listed them was the
// one place that did not say so.
func TestARowThatCannotBeFetchedHereSaysWhereItComesFrom(t *testing.T) {
	rows, _ := listResources(t)["resources"].([]map[string]any)
	buildings := false
	for _, r := range rows {
		name, _ := r["name"].(string)
		fetchable, _ := r["fetchable"].(bool)
		howto, _ := r["howto"].(string)
		why, _ := r["why"].(string)
		if !fetchable && howto == "" && why == "" {
			t.Errorf("%s cannot be fetched here and says nothing about where "+
				"it can be: %v", name, r)
		}
		if name != "building footprints" {
			continue
		}
		buildings = true
		if !strings.Contains(howto, "Configuration > Environ") ||
			!strings.Contains(howto, "environ.fetch") {
			t.Errorf("the buildings row does not name the page or the verb "+
				"that fetches them: %q", howto)
		}
		if strings.Contains(howto, "fills itself") {
			t.Errorf("the buildings row still claims to fill itself: %q", howto)
		}
		// Nothing pulls them on anybody's behalf, so the row must not say the
		// application will see to it.
		if auto, _ := r["auto"].(bool); auto {
			t.Error("the buildings row is marked as fetched automatically")
		}
	}
	if !buildings {
		t.Fatal("building footprints are not on the resources page at all")
	}
}

// And the refusal a script gets says the same thing the panel does, rather than
// the one sentence that used to be said about every cache.
func TestFetchingBuildingsSaysWhereTheyAreFetchedFrom(t *testing.T) {
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	_, err := store.Do(ctx, "resource.fetch", map[string]any{
		"kind": string(resource.Buildings), "name": "building footprints"})
	if err == nil {
		t.Fatal("resource.fetch accepted building footprints, which it cannot fetch")
	}
	if !strings.Contains(err.Error(), "Configuration > Environ") {
		t.Errorf("the refusal does not say where they come from: %v", err)
	}
}
