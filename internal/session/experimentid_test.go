package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func baseInputs() experimentInputs {
	return experimentInputs{
		ManifestVersion: experimentManifestVersion,
		Fixture:         "fixture-scotland-strict.json",
		GeometryFP:      "a41c9f0e6b2d5583",
		FreqMHz:         869.618,
		Firmware:        []string{"repeater-v1.17.0"},
		Arms:            []ExpArm{{Label: "baseline"}},
		Seeds:           []uint64{4417, 9001},
		Senders:         []string{"comp"},
		RunForMs:        90_000,
		SendAtMs:        30_000,
		Provisioning:    DefaultProvisioning(),
		MeshBench:       "a824a02",
		Dirty:           false,
	}
}

// The same sweep defined twice gives the same ID, and perturbing any single
// input changes it - work item 1's own acceptance test.
func TestSameInputsGiveSameIDEachPerturbedInputGivesADifferentOne(t *testing.T) {
	base := baseInputs()
	id1, err := experimentID(base)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := experimentID(baseInputs())
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("the same inputs hashed to %q and %q", id1, id2)
	}

	perturb := map[string]func(*experimentInputs){
		"fixture":  func(in *experimentInputs) { in.Fixture = "some-other-fixture.json" },
		"geometry": func(in *experimentInputs) { in.GeometryFP = "0000000000000000" },
		"freq":     func(in *experimentInputs) { in.FreqMHz = 869.525 },
		"firmware": func(in *experimentInputs) { in.Firmware = []string{"repeater-v1.16.0"} },
		"arms":     func(in *experimentInputs) { in.Arms = []ExpArm{{Label: "other"}} },
		"seeds":    func(in *experimentInputs) { in.Seeds = []uint64{1, 2, 3} },
		"senders":  func(in *experimentInputs) { in.Senders = []string{"other-comp"} },
		"run_for":  func(in *experimentInputs) { in.RunForMs = 60_000 },
		"send_at":  func(in *experimentInputs) { in.SendAtMs = 15_000 },
		"prov": func(in *experimentInputs) {
			in.Provisioning.StaggerMs = in.Provisioning.StaggerMs + 1
		},
		"meshbench": func(in *experimentInputs) { in.MeshBench = "deadbee" },
		"dirty":     func(in *experimentInputs) { in.Dirty = true },
	}
	for name, mutate := range perturb {
		t.Run(name, func(t *testing.T) {
			mutated := baseInputs()
			mutate(&mutated)
			mid, err := experimentID(mutated)
			if err != nil {
				t.Fatal(err)
			}
			if mid == id1 {
				t.Fatalf("perturbing %s did not change the ID (still %s)", name, id1)
			}
		})
	}
}

// A dirty tree produces an ID that differs from the clean one and says so -
// an ID that silently means "plus whatever was uncommitted" is worse than no
// ID at all.
func TestADirtyTreeChangesTheID(t *testing.T) {
	clean := baseInputs()
	dirty := baseInputs()
	dirty.Dirty = true
	cleanID, err := experimentID(clean)
	if err != nil {
		t.Fatal(err)
	}
	dirtyID, err := experimentID(dirty)
	if err != nil {
		t.Fatal(err)
	}
	if cleanID == dirtyID {
		t.Fatal("a dirty tree hashed to the same ID as a clean one")
	}
}

// An older manifest missing its version field loads with a clear complaint
// rather than a wrong ID: this build cannot promise the hash covers
// everything that decided the run, so it refuses to pretend it can.
func TestAnOlderManifestMissingItsVersionFieldComplains(t *testing.T) {
	old := `{"id":"abc123","inputs":{"fixture":"x.json","seeds":[1,2]}}`
	_, err := parseExperimentManifest([]byte(old))
	if err == nil {
		t.Fatal("a manifest with no version field loaded as though it were complete")
	}
	if !strings.Contains(err.Error(), "no version") && !strings.Contains(err.Error(), "version field") {
		t.Fatalf("the complaint does not say what is wrong: %v", err)
	}
}

// A manifest from a newer build than this one is refused for the same
// reason: this build does not know what the newer version's format hashes.
func TestANewerManifestVersionComplains(t *testing.T) {
	b, err := json.Marshal(ExperimentManifest{
		ID: "abc123",
		Inputs: experimentInputs{
			ManifestVersion: experimentManifestVersion + 1,
			Fixture:         "x.json",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseExperimentManifest(b)
	if err == nil {
		t.Fatal("a manifest from a newer build loaded without complaint")
	}
}

// A well-formed manifest round-trips: the same inputs come back byte for
// byte, so replaying it defines the same experiment it was saved from.
func TestAWellFormedManifestRoundTrips(t *testing.T) {
	in := baseInputs()
	id, err := experimentID(in)
	if err != nil {
		t.Fatal(err)
	}
	m := ExperimentManifest{ID: id, Inputs: in}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseExperimentManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id {
		t.Fatalf("id: got %q want %q", got.ID, id)
	}
	if got.Inputs.Fixture != in.Fixture || len(got.Inputs.Seeds) != len(in.Seeds) {
		t.Fatalf("inputs did not round-trip: got %+v", got.Inputs)
	}
}

// The full path: define a sweep, export it (which writes the manifest),
// then replay its ID into a fresh, unstarted experiment on a different Sim -
// "a manifest round-trips into a defined, unstarted experiment."
func TestAManifestRoundTripsIntoADefinedUnstartedExperiment(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	st, s := register(t)
	s.persist = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "experiment.vary", map[string]any{
		"parameter": "repeater_version",
		"values":    []any{"repeater-v1.17.0", "repeater-v1.16.0"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(ctx, "experiment.seeds", map[string]any{
		"seeds": []any{1.0, 2.0, 3.0},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(ctx, "experiment.senders", map[string]any{
		"senders": []any{"comp"},
	}); err != nil {
		t.Fatal(err)
	}

	res, err := st.Do(ctx, "experiment.export", nil)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("experiment.export did not return a map: %#v", res)
	}
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatal("experiment.export did not stamp an id")
	}

	// A fresh session, as though the ID had been handed to somebody else.
	st2, s2 := register(t)
	s2.persist = true
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go st2.Run(ctx2)

	res2, err := st2.Do(ctx2, "experiment.replay", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("replaying %s: %v", id, err)
	}
	rm, ok := res2.(map[string]any)
	if !ok {
		t.Fatalf("experiment.replay did not return a map: %#v", res2)
	}
	if started, _ := rm["started"].(bool); started {
		t.Fatal("replay started the experiment; it must only define it")
	}
	if arms, _ := rm["arms"].(int); arms != 2 {
		t.Fatalf("arms: got %d want 2", arms)
	}
	if seeds, _ := rm["seeds"].(int); seeds != 3 {
		t.Fatalf("seeds: got %d want 3", seeds)
	}
	e2 := s2.experiment()
	if len(e2.Senders) != 1 || e2.Senders[0] != "comp" {
		t.Fatalf("senders did not restore: %v", e2.Senders)
	}
}
