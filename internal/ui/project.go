package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/A13xB0/meshcoresim/internal/boundary"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/fixture"
)

// The workbench's saved setup is the shipped fixture format, under the names
// this file has always used. One definition, in internal/fixture, because the
// headless test runner reads the same files and a second copy of the struct
// would drift until a fixture loaded in one program and not the other.
type (
	project        = fixture.Fixture
	savedArea      = fixture.Area
	savedSend      = fixture.Send
	savedAssertion = fixture.Assertion
)

func projectDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "projects"
	}
	return filepath.Join(base, "meshcoresim", "projects")
}

// saveProject writes everything needed to resume.
func (a *App) saveProject(name string) error {
	if name = strings.TrimSpace(name); name == "" {
		name = fmt.Sprintf("%d-nodes-%s", len(a.Nodes), time.Now().Format("2006-01-02"))
	}
	p := project{
		Name: name, Saved: time.Now(), Nodes: a.Nodes,
		Seed: a.runSeed(), FreqMHz: a.freqMHz,
		MarginKm: float64(a.bnd.marginKm),
		Lat:      a.view.CentreLat, Lon: a.view.CentreLon,
		MetresPerPixel: a.view.MetresPerPixel,
	}
	for _, f := range a.bnd.chosen {
		p.Areas = append(p.Areas, savedArea{Name: f.Name, Boundaries: f.Boundaries})
	}
	for _, s := range a.sched.sends {
		p.Sends = append(p.Sends, savedSend{
			Node: s.node, AtMs: s.atMs, EveryMs: s.everyMs, Command: s.command})
	}
	for _, as := range a.sched.asserts {
		p.Assertions = append(p.Assertions, savedAssertion{
			Kind: string(as.Kind), Node: as.Node, WithinMs: as.WithinMs,
			AtLeast: as.AtLeast, AtMost: as.AtMost, MaxPct: as.MaxPct})
	}

	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(projectDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(projectDir(), name+".json"), b, 0o644)
}

// openProject restores a saved setup.
func (a *App) openProject(name string) error {
	b, err := os.ReadFile(filepath.Join(projectDir(), name+".json"))
	if err != nil {
		return err
	}
	var p project
	if err := json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("%s is not a project: %w", name, err)
	}

	a.Nodes = p.Nodes
	if p.Seed != 0 {
		a.seed = p.Seed
	}
	if p.FreqMHz > 0 {
		a.freqMHz = p.FreqMHz
	}
	a.bnd.chosen = a.bnd.chosen[:0]
	for _, area := range p.Areas {
		a.bnd.chosen = append(a.bnd.chosen, boundary.Found{
			Name: area.Name, DisplayName: area.Name, Boundaries: area.Boundaries,
		})
	}
	if p.MarginKm > 0 {
		a.bnd.marginKm = float32(p.MarginKm)
	}

	a.sched.sends = a.sched.sends[:0]
	for _, s := range p.Sends {
		a.sched.sends = append(a.sched.sends, send{
			node: s.Node, atMs: s.AtMs, everyMs: s.EveryMs, command: s.Command})
	}
	a.sched.asserts = a.sched.asserts[:0]
	for _, as := range p.Assertions {
		a.sched.asserts = append(a.sched.asserts, engine.Assertion{
			Kind: engine.AssertKind(as.Kind), Node: as.Node, WithinMs: as.WithinMs,
			AtLeast: as.AtLeast, AtMost: as.AtMost, MaxPct: as.MaxPct,
		})
	}

	a.selected, a.linkTo = -1, -1
	if p.MetresPerPixel > 0 {
		a.view.CentreLat, a.view.CentreLon = p.Lat, p.Lon
		a.view.MetresPerPixel = p.MetresPerPixel
	} else {
		a.view.MetresPerPixel = 0 // refit on the next frame
	}
	a.terrainDirty = true
	a.buildEngine()
	a.selectFirstLink()
	return nil
}

// listProjects is the saved projects, newest first.
func listProjects() []savedNet {
	entries, err := os.ReadDir(projectDir())
	if err != nil {
		return nil
	}
	var out []savedNet
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		n := savedNet{name: strings.TrimSuffix(e.Name(), ".json")}
		if info, err := e.Info(); err == nil {
			n.saved = info.ModTime()
		}
		if b, err := os.ReadFile(filepath.Join(projectDir(), e.Name())); err == nil {
			var p project
			if json.Unmarshal(b, &p) == nil {
				n.nodes = len(p.Nodes)
			}
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].saved.After(out[j].saved) })
	return out
}
