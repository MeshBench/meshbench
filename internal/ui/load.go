package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/scenario"
)

// scenarioDir is where saved networks live.
func scenarioDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "scenarios"
	}
	return filepath.Join(base, "meshcoresim", "scenarios")
}

// savedNet is one file in the scenarios directory, described well enough to
// recognise. A bare name in a combo was how a network got lost the day after
// it was saved; a row that says "613 nodes, yesterday evening" is one nobody
// has to remember.
type savedNet struct {
	name  string
	nodes int
	saved time.Time
}

// savedNetworks lists the scenarios directory, newest first, cached briefly.
//
// The node count means opening every file, so the scan is cached and refreshed
// on a timer and after every save or delete — not per frame.
func (a *App) savedNetworks() []savedNet {
	if time.Since(a.savedScanAt) < 5*time.Second && !a.savedDirty {
		return a.savedCache
	}
	a.savedScanAt, a.savedDirty = time.Now(), false
	entries, err := os.ReadDir(scenarioDir())
	if err != nil {
		a.savedCache = nil
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
		if b, err := os.ReadFile(filepath.Join(scenarioDir(), e.Name())); err == nil {
			var nodes []scenario.Node
			if json.Unmarshal(b, &nodes) == nil {
				n.nodes = len(nodes)
			}
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].saved.After(out[j].saved) })
	a.savedCache = out
	return out
}

// age says when in words a person uses.
func age(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return t.Format("2 Jan 15:04")
	}
}

func defaultSaveName(nodes int) string {
	return fmt.Sprintf("%d-nodes-%s", nodes, time.Now().Format("2006-01-02"))
}

// loadSavedNet loads a named saved network, replacing or adding.
//
// Replace and add are two menu items rather than a checkbox somewhere else:
// what happens is decided where the click happens.
func (a *App) loadSavedNet(name string, replace bool) {
	b, err := os.ReadFile(filepath.Join(scenarioDir(), name+".json"))
	if err != nil {
		a.status = err.Error()
		return
	}
	var nodes []scenario.Node
	if err := json.Unmarshal(b, &nodes); err != nil {
		a.status = fmt.Sprintf("%s is not a saved network: %v", name, err)
		return
	}
	region, err := a.regionOrNil()
	if err != nil {
		a.status = err.Error()
		return
	}
	prefix := ""
	if region != nil {
		kept := nodes[:0]
		dropped := 0
		for _, n := range nodes {
			if region.Participates(n.Position) {
				kept = append(kept, n)
			} else {
				dropped++
			}
		}
		nodes = kept
		if dropped > 0 {
			prefix = fmt.Sprintf("%d outside the boundary left out; ", dropped)
		}
	}
	a.installNodes(nodes, replace)
	a.status = prefix + fmt.Sprintf("%d nodes from %s", len(nodes), name)
}

// saveNetwork writes the current nodes as a scenario file.
//
// The point is the reload: a provider import takes ages and repeats its
// assumptions every time, while a saved network comes back in milliseconds
// exactly as it was — including every setting made since the import.
func (a *App) saveNetwork() {
	if len(a.Nodes) == 0 {
		a.status = "nothing to save"
		return
	}
	name := strings.TrimSpace(a.saveName)
	if name == "" {
		name = defaultSaveName(len(a.Nodes))
	}
	if err := os.MkdirAll(scenarioDir(), 0o755); err != nil {
		a.status = err.Error()
		return
	}
	b, err := json.MarshalIndent(a.Nodes, "", "  ")
	if err != nil {
		a.status = err.Error()
		return
	}
	path := filepath.Join(scenarioDir(), name+".json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		a.status = err.Error()
		return
	}
	a.status = fmt.Sprintf("%d nodes saved as %q", len(a.Nodes), name)
	a.saveName = ""
	a.savedDirty = true
}

// regionOrNil is the active boundary, if any.
func (a *App) regionOrNil() (*scenario.Region, error) {
	// The Boundary window's chosen areas first: they are visible, named, and
	// drawn on the map, where a file path is none of those.
	if r := a.chosenRegion(); r != nil {
		return r, nil
	}
	if a.boundaryPath == "" {
		return nil, nil
	}
	return scenario.RegionFromGeoJSONFile(a.boundaryPath)
}

// installNodes replaces or extends the scenario with loaded nodes.
func (a *App) installNodes(nodes []scenario.Node, replace bool) {
	if replace {
		a.Nodes = nil
	}
	a.Nodes = append(a.Nodes, nodes...)
	a.msel = nil
	a.view.FitTo(a.Nodes, a.view.Width, a.view.Height)
	a.terrainDirty = true
	a.buildEngine()
	a.selectFirstLink()
}

// pruneOutside deletes every loaded node not inside the boundary.
//
// "Delete everything not in Scotland" as one action. Participates, not
// Contains: a repeater within RF range of the border still shapes what happens
// inside, and silently removing it makes the mesh behave better than reality.
// Anyone who wants a hard cut can set the margin to zero in the file.
func (a *App) pruneOutside() {
	region, err := a.regionOrNil()
	if err != nil {
		a.status = err.Error()
		return
	}
	if region == nil {
		return
	}
	kept := a.Nodes[:0]
	dropped := 0
	for _, n := range a.Nodes {
		if region.Participates(n.Position) {
			kept = append(kept, n)
		} else {
			dropped++
		}
	}
	a.Nodes = kept
	a.selected, a.linkTo, a.msel = -1, -1, nil
	a.status = fmt.Sprintf("%d nodes outside the boundary deleted, %d kept", dropped, len(a.Nodes))
	a.view.FitTo(a.Nodes, a.view.Width, a.view.Height)
	a.terrainDirty = true
	a.buildEngine()
	a.selectFirstLink()
}
