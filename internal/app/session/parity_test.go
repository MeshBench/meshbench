package session

import (
	"context"
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/control"
)

// The same run, watched and unwatched.
//
// ADR-0019 asks for this by name: "every verb needs to behave identically in
// both modes, or the harness tests something users never run. A shared test
// that drives the same verbs both ways is the cheapest guard."
//
// Most of the guard is structural rather than tested - both entry points build
// their session through Boot, so there is one registration and it cannot
// diverge - and what is left for a test is the part Boot cannot enforce: that
// attaching an interface does not change what the simulation does.

// The sequence. Deliberately the shape of a real scripted run rather than a
// list of every verb: place, measure, step, and ask what happened.
var paritySteps = []struct {
	verb   string
	params any
}{
	{"project.new", map[string]any{}},
	{"nodes.place", map[string]any{
		"name": "Alpha", "kind": "simple-repeater", "lat": 56.30, "lon": -3.30}},
	{"nodes.place", map[string]any{
		"name": "Bravo", "kind": "simple-repeater", "lat": 56.20, "lon": -3.28}},
	{"nodes.place", map[string]any{
		"name": "Charlie", "kind": "companion", "lat": 56.25, "lon": -3.10}},
	{"nodes.select", "Bravo"},
	{"nodes.regions", map[string]any{"node": "Bravo", "regions": []any{"Fife"}}},
	{"sim.seed", map[string]any{"seed": 4242.0}},
	{"sim.step", nil},
	{"sim.step", nil},
	{"nodes.delete", map[string]any{"node": "Charlie"}},
	{"nodes.stats", nil},
	{"session.describe", nil},
}

func runParity(t *testing.T, ui UI) map[string]any {
	t.Helper()
	st, sm := Boot(Options{NoPrefs: true, Headless: ui == nil})
	if ui != nil {
		sm.SetUI(ui)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	for i, s := range paritySteps {
		if _, err := st.Do(ctx, s.verb, s.params); err != nil {
			t.Fatalf("step %d, %s: %v", i, s.verb, err)
		}
	}
	// The snapshot summary rather than the verb returns: it is what a client
	// reads, and it covers the world rather than one answer.
	return snapshotSummary(st.Snapshot())
}

func TestAHeadlessRunAndAWatchedOneAgree(t *testing.T) {
	headless := runParity(t, nil)
	watched := runParity(t, &stubUI{})

	// Not the whole summary: seq counts publishes, and a session with an
	// interface attached publishes when the interface asks it to. What has to
	// match is the simulation.
	for _, k := range []string{
		"nodes", "links", "seed", "now_ms", "playing", "selected",
		"step_ms", "areas", "events", "node_stats", "real_firmware",
	} {
		if fmt.Sprint(headless[k]) != fmt.Sprint(watched[k]) {
			t.Errorf("%s: headless %v, watched %v", k, headless[k], watched[k])
		}
	}
}

// And the difference that is supposed to exist, does: a window verb works with
// a window and refuses without one, by code rather than by prose.
func TestWindowVerbsRefuseHeadlessAndWorkWatched(t *testing.T) {
	windowVerbs := []struct {
		verb   string
		params any
	}{
		{"workspace.set", map[string]any{"view": "Run"}},
		{"panels.list", nil},
		{"panel.open", map[string]any{"name": "Map"}},
		{"map.fit", nil},
		{"ui.state", nil},
		{"view.list", nil},
	}

	stH, _ := Boot(Options{NoPrefs: true, Headless: true})
	stW, smW := Boot(Options{NoPrefs: true})
	smW.SetUI(&stubUI{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stH.Run(ctx)
	go stW.Run(ctx)

	for _, v := range windowVerbs {
		_, err := stH.Do(ctx, v.verb, v.params)
		if err == nil {
			t.Errorf("%s answered in a session with no interface", v.verb)
			continue
		}
		if got := control.CodeOf(err); got != control.Unavailable {
			t.Errorf("%s refused with %q, want %q (%v)",
				v.verb, got, control.Unavailable, err)
		}
		if _, err := stW.Do(ctx, v.verb, v.params); err != nil {
			t.Errorf("%s refused with an interface attached: %v", v.verb, err)
		}
	}
}

// Boot is what makes the two modes one construction; this holds it to that.
func TestBootRegistersTheSameVerbsBothWays(t *testing.T) {
	h, _ := Boot(Options{NoPrefs: true, Headless: true})
	w, _ := Boot(Options{NoPrefs: true})
	hv, wv := sortedVerbs(h.Verbs()), sortedVerbs(w.Verbs())
	if len(hv) != len(wv) {
		t.Fatalf("headless registers %d verbs, watched %d", len(hv), len(wv))
	}
	for i := range hv {
		if hv[i] != wv[i] {
			t.Fatalf("verb %d differs: headless %q, watched %q", i, hv[i], wv[i])
		}
	}
	if Mode != "workbench" {
		t.Errorf("Mode is %q after booting a watched session", Mode)
	}
}

// stubUI is an interface that does nothing, which is all a parity test needs:
// the question is whether the verbs behave the same, not what a window draws.
type stubUI struct {
	views  []string
	layers map[string]bool
}

func (u *stubUI) ShowView(string) error       { return nil }
func (u *stubUI) PanelNames() []string        { return []string{"Map", "Nodes running"} }
func (u *stubUI) Quit()                       {}
func (u *stubUI) CentreMap(_, _, _ float64)   {}
func (u *stubUI) FitMap()                     {}
func (u *stubUI) OpenNodeWindow(string)       {}
func (u *stubUI) OpenPanel(_, _ string) error { return nil }
func (u *stubUI) ClosePanel(string) error     { return nil }
func (u *stubUI) ResetLayout()                {}
func (u *stubUI) CloseWindow(string) error    { return nil }
func (u *stubUI) Scale() float64              { return 1 }
func (u *stubUI) SetScale(float64)            {}
func (u *stubUI) SaveView(n string) error     { u.views = append(u.views, n); return nil }
func (u *stubUI) LoadView(string) error       { return nil }
func (u *stubUI) ListViews() []string         { return u.views }
func (u *stubUI) DeleteView(string) error     { return nil }
func (u *stubUI) ZoomMap(float64)             {}
func (u *stubUI) FilterMap(string)            {}
func (u *stubUI) SetTool(string) error        { return nil }

func (u *stubUI) SetLayer(name string, on bool) error {
	if u.layers == nil {
		u.layers = map[string]bool{}
	}
	u.layers[name] = on
	return nil
}

func (u *stubUI) Layers() map[string]bool { return u.layers }
func (u *stubUI) State() map[string]any   { return map[string]any{"view": "Plan"} }

var _ UI = (*stubUI)(nil)
