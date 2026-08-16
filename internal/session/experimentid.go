// Experiment IDs and replay.
//
// A sweep's result lives in the operator's session and nowhere else. Every
// input that decides it is recorded somewhere - seeds, firmware refs, the
// geometry fingerprint, resolved provisioning - but nothing gathers them into
// something citable. This hashes the resolved input set into one short
// string, writes a manifest carrying it, and can restore a defined-but-not-
// started experiment from that string alone: a result can be handed to
// somebody else and reproduced exactly, or shown to be impossible to
// reproduce because something about it has changed.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// experimentManifestVersion is stamped on every manifest this build writes.
// A manifest without it, or with a higher one, is a format this build cannot
// promise to have hashed completely - replay says so rather than silently
// computing an ID that looks right and is not.
const experimentManifestVersion = 1

// experimentInputs is everything that decides a sweep's numbers. Field names
// are part of the hash - JSON, not Go's field order, so a struct tag rename
// changes every ID already handed out and a field reorder does not.
type experimentInputs struct {
	ManifestVersion int          `json:"manifest_version"`
	Fixture         string       `json:"fixture"`
	GeometryFP      string       `json:"geometry_fp"`
	FreqMHz         float64      `json:"freq_mhz"`
	Firmware        []string     `json:"firmware"`
	Arms            []ExpArm     `json:"arms"`
	Seeds           []uint64     `json:"seeds"`
	Senders         []string     `json:"senders"`
	RunForMs        uint32       `json:"run_for_ms"`
	SendAtMs        uint32       `json:"send_at_ms"`
	Provisioning    Provisioning `json:"provisioning"`
	// MeshBench is the commit this binary was built from, and Dirty is
	// whether the tree had uncommitted changes at build time. An ID that
	// lies is worse than none: a dirty tree is stamped as such rather than
	// silently meaning "plus whatever was uncommitted".
	MeshBench string `json:"meshbench"`
	Dirty     bool   `json:"dirty"`
}

// ExperimentManifest is what lands on disk: the inputs, the ID they hash to,
// and - once the sweep has run - its results. A manifest written before the
// run has no results; experiment.replay does not need them, only the inputs.
type ExperimentManifest struct {
	ID      string           `json:"id"`
	Inputs  experimentInputs `json:"inputs"`
	Results []ExpResult      `json:"results,omitempty"`
	Summary []map[string]any `json:"summary,omitempty"`
}

// meshbenchVCS reads this binary's own build stamp: the commit it was built
// from, and whether the tree was clean. go build stamps this automatically
// from git when it is run inside a checkout, which every developer build and
// every source archive from the release pipeline both are.
func meshbenchVCS() (commit string, dirty, ok bool) {
	info, has := debug.ReadBuildInfo()
	if !has {
		return "", false, false
	}
	for _, set := range info.Settings {
		switch set.Key {
		case "vcs.revision":
			commit = set.Value
		case "vcs.modified":
			dirty = set.Value == "true"
		}
	}
	if commit == "" {
		return "", false, false
	}
	if len(commit) > 9 {
		commit = commit[:9]
	}
	return commit, dirty, true
}

// firmwareRefs is every distinct firmware version in play, sorted - the same
// set BuildOf shows, kept as a slice so it hashes as data rather than as the
// formatting of a joined string.
func firmwareRefs(s *Sim) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range s.nodes {
		v := n.Firmware.Version
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// inputsFor gathers the resolved input set for the experiment as currently
// defined against the currently loaded network - the set two operators need
// to agree on before their IDs can match. Resolved provisioning, not the
// panel's settings: what actually gets typed at a node, after defaults.
func (e *experiment) inputsFor(s *Sim) experimentInputs {
	e.mu.Lock()
	arms := append([]ExpArm(nil), e.Arms...)
	seeds := append([]uint64(nil), e.Seeds...)
	senders := append([]string(nil), e.Senders...)
	runFor, sendAt := e.RunForMs, e.SendAtMs
	e.mu.Unlock()

	senders = append([]string(nil), senders...)
	sort.Strings(senders)

	commit, dirty, _ := meshbenchVCS()
	prov := DefaultProvisioning()
	if s.prov != nil {
		prov = *s.prov
	}
	return experimentInputs{
		ManifestVersion: experimentManifestVersion,
		Fixture:         s.fixturePath,
		GeometryFP:      fmt.Sprintf("%016x", s.geomFP),
		FreqMHz:         s.freqMHz,
		Firmware:        firmwareRefs(s),
		Arms:            arms,
		Seeds:           seeds,
		Senders:         senders,
		RunForMs:        runFor,
		SendAtMs:        sendAt,
		Provisioning:    prov,
		MeshBench:       commit,
		Dirty:           dirty,
	}
}

// experimentID hashes the input set into a short string. Sixteen hex
// characters of a SHA-256 digest: short enough to read aloud and paste into
// a ticket, and at sixty-four bits, no two different input sets a study
// actually produces are going to collide by chance.
func experimentID(in experimentInputs) (string, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16], nil
}

// experimentsDir is where manifests live, or empty when persistence is off -
// in tests, where writing to the developer's cache would make runs depend on
// each other. The same convention matrixDir uses, and for the same reason.
func (s *Sim) experimentsDir() string {
	if !s.persist {
		return ""
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(cache, "meshcoresim", "experiments")
	if os.MkdirAll(dir, 0o755) != nil {
		return ""
	}
	return dir
}

// saveManifest writes the manifest for the current definition, with whatever
// results have come back so far - none, if called at definition time; the
// finished set, if called when the sweep completes. Same inputs, same ID, so
// the file at the end of a run is the same file a mid-run export would have
// named, with results filled in. A no-op, not an error, when persistence is
// off: the ID itself is still computed and stamped regardless.
func (e *experiment) saveManifest(s *Sim) (ExperimentManifest, error) {
	in := e.inputsFor(s)
	id, err := experimentID(in)
	if err != nil {
		return ExperimentManifest{}, err
	}
	e.mu.Lock()
	results := append([]ExpResult(nil), e.results...)
	e.mu.Unlock()
	m := ExperimentManifest{ID: id, Inputs: in, Results: results, Summary: e.summarise()}
	dir := s.experimentsDir()
	if dir == "" {
		return m, nil
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, err
	}
	return m, os.WriteFile(filepath.Join(dir, id+".json"), b, 0o644)
}

// loadExperimentManifest reads a manifest by ID and refuses one this build
// cannot promise to have hashed completely, rather than replaying it as
// though it were.
func (s *Sim) loadExperimentManifest(id string) (ExperimentManifest, error) {
	dir := s.experimentsDir()
	if dir == "" {
		return ExperimentManifest{}, fmt.Errorf("no manifest for %q: nothing is saved", id)
	}
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return ExperimentManifest{}, fmt.Errorf("no manifest for %q: %w", id, err)
	}
	return parseExperimentManifest(b)
}

func parseExperimentManifest(b []byte) (ExperimentManifest, error) {
	var m ExperimentManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return ExperimentManifest{}, err
	}
	if m.Inputs.ManifestVersion == 0 {
		return ExperimentManifest{}, fmt.Errorf(
			"this manifest carries no version field - it was written before " +
				"replay existed, so this build cannot promise the hash covers " +
				"everything that decides the run; its numbers are provenance, " +
				"not something to reproduce")
	}
	if m.Inputs.ManifestVersion > experimentManifestVersion {
		return ExperimentManifest{}, fmt.Errorf(
			"this manifest is version %d; this build only understands version %d "+
				"and older - replaying it here could silently drop an input the "+
				"newer version hashed", m.Inputs.ManifestVersion, experimentManifestVersion)
	}
	return m, nil
}

func registerExperimentID(st *state.Store, s *Sim) {
	// experiment.manifest: the ID for what is currently defined, without
	// writing anything - "the same sweep defined twice gives the same ID",
	// checkable before comparing notes with somebody else.
	st.Handle("experiment.manifest", func(w *state.World, _ any) (any, error) {
		e := s.experiment()
		id := stampExperimentID(w, s, e)
		return manifestDescribe(id, e.inputsFor(s)), nil
	})

	// experiment.replay: takes an ID, restores the definition it hashes to,
	// and stops there - the operator presses run.
	st.Handle("experiment.replay", func(w *state.World, p any) (any, error) {
		id, _ := stringField(p, "id")
		if id == "" {
			id = soleString(p)
		}
		if id == "" {
			return nil, fmt.Errorf("experiment.replay needs an id")
		}
		m, err := s.loadExperimentManifest(id)
		if err != nil {
			return nil, err
		}
		e := s.experiment()
		e.mu.Lock()
		e.Arms = append([]ExpArm(nil), m.Inputs.Arms...)
		e.Seeds = append([]uint64(nil), m.Inputs.Seeds...)
		e.Senders = append([]string(nil), m.Inputs.Senders...)
		e.RunForMs, e.SendAtMs = m.Inputs.RunForMs, m.Inputs.SendAtMs
		e.results = nil
		e.mu.Unlock()
		pr := m.Inputs.Provisioning
		s.prov = &pr

		warn := ""
		switch {
		case m.Inputs.Fixture != "" && m.Inputs.Fixture != s.fixturePath:
			warn = fmt.Sprintf("this manifest was defined against %s; %s is loaded - "+
				"open the right fixture before running, or the numbers will not be "+
				"about the same network", m.Inputs.Fixture, orNone(s.fixturePath))
		case m.Inputs.GeometryFP != fmt.Sprintf("%016x", s.geomFP):
			warn = "the loaded network's geometry no longer matches this manifest's " +
				"fingerprint - positions, height or tx power have changed since it was recorded"
		}
		id = stampExperimentID(w, s, e)
		w.Say(fmt.Sprintf("restored experiment %s (%d arms, %d seeds); not started",
			id, len(e.Arms), len(e.Seeds)))
		out := map[string]any{"id": id, "restored": true, "started": false,
			"arms": len(e.Arms), "seeds": len(e.Seeds)}
		if warn != "" {
			out["warning"] = warn
			w.Say(warn)
		}
		return out, nil
	})
}

// stampExperimentID computes the ID for the experiment as currently defined
// and writes it onto the world, so the Sweep panel shows it every frame
// without needing its own poll.
func stampExperimentID(w *state.World, s *Sim, e *experiment) string {
	in := e.inputsFor(s)
	id, err := experimentID(in)
	if err != nil {
		return ""
	}
	w.ExperimentID = id
	w.ExperimentIdentity = state.ExperimentIdentity{
		Fixture: in.Fixture, GeometryFP: in.GeometryFP,
		Firmware: in.Firmware, MeshBench: in.MeshBench, Dirty: in.Dirty,
	}
	return id
}

func orNone(s string) string {
	if s == "" {
		return "nothing"
	}
	return s
}

func manifestDescribe(id string, in experimentInputs) map[string]any {
	return map[string]any{
		"id": id, "fixture": in.Fixture, "geometry_fp": in.GeometryFP,
		"firmware": in.Firmware, "arms": len(in.Arms), "seeds": in.Seeds,
		"senders": in.Senders, "run_for_ms": in.RunForMs, "send_at_ms": in.SendAtMs,
		"meshbench": in.MeshBench, "dirty": in.Dirty,
	}
}
