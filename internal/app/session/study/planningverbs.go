// Planning, coverage, and saving what you have.
//
// The old workbench answered three planning questions - bridge two areas,
// cover a gap, survive a failure - and the Gio build had the panel with the
// results table and no way to ask any of them.
package study

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/study/coverage"
)

func registerPlanningVerbs(st *state.Store, s *session.Sim) {
	st.Handle("coverage.start", func(w *state.World, p any) (any, error) {
		mode, _ := session.StringField(p, "mode")
		if mode == "" {
			mode = "best"
		}
		switch mode {
		case "best", "best-server", "gaps", "redundancy":
		case "node":
			for _, n := range w.Nodes {
				if n.Selected {
					return st.Do(context.Background(), "coverage.compute", n.Name)
				}
			}
			return nil, fmt.Errorf("select a node first")
		default:
			return nil, fmt.Errorf("no coverage mode %q; there is best, gaps, redundancy and node", mode)
		}
		// The network-wide rasters are the same computation from every node,
		// combined differently. Rasterising 311 of them takes minutes, so this
		// runs as a job and reports rather than blocking the store.
		id := "coverage-" + mode
		w.Jobs = append(w.Jobs, state.Job{
			ID: id, What: "coverage: " + mode, Total: len(w.Nodes)})
		names := make([]string, 0, len(w.Nodes))
		for _, n := range w.Nodes {
			names = append(names, n.Name)
		}
		// One shared grid for every node: rasters over per-node boxes do not
		// share ground, and Combine rightly refuses them - which is why this
		// job could never finish on a spread-out network before.
		south, north, west, east, gw, gh, boxErr := mapBox(s.Nodes(), coverageCells(s))
		if boxErr != nil {
			return nil, boxErr
		}
		byName := make(map[string]int, len(s.Nodes()))
		for i := range s.Nodes() {
			byName[s.Nodes()[i].Name] = i
		}
		go func() {
			rasters := make([]*coverage.Raster, 0, len(names))
			for i, name := range names {
				ni, ok := byName[name]
				if !ok {
					continue
				}
				r, err := rasterOnBox(s, context.Background(), s.Nodes()[ni],
					south, north, west, east, gw, gh)
				if err == nil && r != nil {
					rasters = append(rasters, r)
				}
				if i%10 == 0 {
					_, _ = st.Do(context.Background(), "job.progress", state.Job{
						ID: id, What: "coverage: " + mode, Done: i, Total: len(names)})
				}
			}
			combined, err := coverage.Combine(rasters)
			if err != nil {
				_, _ = st.Do(context.Background(), "coverage.failed", err.Error())
				return
			}
			_, _ = st.Do(context.Background(), "coverage.combined",
				map[string]any{"mode": mode, "combined": combined})
		}()
		return map[string]any{"mode": mode, "nodes": len(w.Nodes), "started": true}, nil
	})

	st.Handle("project.save", func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "name")
		if name == "" {
			return nil, fmt.Errorf("project.save needs a name")
		}
		dir, err := projectsDir()
		if err != nil {
			return nil, err
		}
		// A name, not a path. It is joined onto the projects directory and then
		// written to, so "../../.bashrc" was a write anywhere the workbench can
		// reach, spelled as saving a project - and the caller who typed a path
		// by mistake would have been told it saved.
		if !usableAsProjectName(name) {
			return nil, session.BadParams(
				"project.save: %q is a path, not a project name - projects are "+
					"saved into %s and are named without separators", name, dir)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		path := filepath.Join(dir, name+".json")
		f := struct {
			Nodes    []any   `json:"nodes"`
			MarginKm float64 `json:"margin_km"`
		}{MarginKm: float64(w.MarginKm)}
		for _, n := range s.Nodes() {
			f.Nodes = append(f.Nodes, n)
		}
		b, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return nil, err
		}
		w.Say("saved " + name)
		return map[string]any{"saved": name, "path": path, "nodes": len(s.Nodes())}, nil
	})

	st.Handle("project.list", func(_ *state.World, _ any) (any, error) {
		dir, err := projectsDir()
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"projects": []string{}}, nil
			}
			return nil, err
		}
		var names []string
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				names = append(names, strings.TrimSuffix(e.Name(), ".json"))
			}
		}
		sort.Strings(names)
		return map[string]any{"projects": names, "dir": dir}, nil
	})
}

// coverage.combined lands the network-wide answer.
func registerCoverageCombined(st *state.Store, s *session.Sim) {
	st.HandleInternal("coverage.combined", func(w *state.World, p any) (any, error) {
		m, ok := p.(map[string]any)
		if !ok {
			return nil, session.WrongCallback("coverage.combined")
		}
		c, _ := m["combined"].(*coverage.Combined)
		mode, _ := m["mode"].(string)
		if c == nil {
			return nil, fmt.Errorf("no combined coverage")
		}
		gaps, known := c.GapCells()
		out := map[string]any{
			"mode": mode, "gap_cells": gaps, "known_cells": known,
			"redundancy":              c.Redundancy(),
			"single_point_of_failure": c.SinglePointOfFailure(),
		}
		w.Jobs = session.FinishJob(w.Jobs, "coverage-"+mode)
		switch mode {
		case "gaps":
			w.Say(fmt.Sprintf("%d of %d cells reach nobody", gaps, known))
		case "redundancy":
			w.Say(fmt.Sprintf("%.1f servers per covered cell; %d cells depend on one",
				c.Redundancy(), c.SinglePointOfFailure()))
		default:
			w.Say(fmt.Sprintf("coverage combined over %d cells", known))
		}
		return out, nil
	})
}

// usableAsProjectName reports whether a name stays inside the projects
// directory when it is joined onto it.
//
// Both separators are refused, not only this platform's: a fixture written on
// Windows travels, and a backslash that means nothing here is a directory
// there.
func usableAsProjectName(name string) bool {
	if strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return false
	}
	return name == filepath.Base(name)
}

func projectsDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "meshbench", "projects"), nil
}
