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
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// project is a whole setup: the network, where it is, and how it is being run.
//
// A saved network is the nodes alone, which loses the study around them — the
// boundary, the seed, the traffic schedule, the assertions. Reopening a piece
// of work should reopen the work, not the nodes it happened to contain.
type project struct {
	Name    string          `json:"name"`
	Saved   time.Time       `json:"saved"`
	Nodes   []scenario.Node `json:"nodes"`
	Seed    uint64          `json:"seed"`
	FreqMHz float64         `json:"freq_mhz"`

	// Areas are the boundary's chosen regions, by name, with their polygons —
	// carried rather than re-fetched, so a project opens offline and gives the
	// same answer months later even if OSM's outline has moved.
	Areas    []savedArea `json:"areas"`
	MarginKm float64     `json:"margin_km"`

	Sends      []savedSend      `json:"sends"`
	Assertions []savedAssertion `json:"assertions"`

	// View is where the map was looking, because reopening a study half a
	// country from where it was left is a small thing that feels wrong.
	Lat            float64 `json:"lat,omitempty"`
	Lon            float64 `json:"lon,omitempty"`
	MetresPerPixel float64 `json:"metres_per_pixel,omitempty"`
}

type savedArea struct {
	Name       string              `json:"name"`
	Boundaries []scenario.Boundary `json:"boundaries"`
}

type savedSend struct {
	Node    string `json:"node"`
	AtMs    uint32 `json:"at_ms"`
	EveryMs uint32 `json:"every_ms"`
	Command string `json:"command"`
}

type savedAssertion struct {
	Kind     string  `json:"kind"`
	Node     string  `json:"node,omitempty"`
	WithinMs uint32  `json:"within_ms,omitempty"`
	AtLeast  int     `json:"at_least,omitempty"`
	AtMost   int     `json:"at_most,omitempty"`
	MaxPct   float64 `json:"max_pct,omitempty"`
}

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
