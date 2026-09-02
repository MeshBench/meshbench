// Is this machine ready, and if not, what would fix it.
//
// Every fact here was already reachable and none of them were in one place. A
// fresh install downloaded half a gigabyte of terrain it never asked about,
// left the firmware cache empty, and had no tools directory at all; the first
// news of any of that was a node refusing to start. The pieces have since grown
// consent gates and fetchers of their own, so what was left missing was the
// path through them: one answer that names every dependency, says what it costs
// before it is spent, and says what to do about the ones the application cannot
// fetch itself.
//
// Read-only and offline, like the listing it is built on. A check that started
// a download would be the same mistake in a new place.
package resources

import (
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/session/updates"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

func registerSetup(st *state.Store, s *session.Sim) {
	st.Handle("setup.check", func(w *state.World, _ any) (any, error) {
		// The cache listing first, because two of the four groups are read off
		// it. Nothing here touches the network.
		if _, err := relistResources(s, w); err != nil {
			return nil, err
		}
		w.Setup = setupGroups(s, w)
		tally := setupTally(w.Setup)
		return map[string]any{
			"groups": setupWire(w.Setup),
			// The counts as well as the rows, because a script's question is
			// usually "is anything wrong" and reading that off a nested list is
			// work every caller would otherwise do the same way.
			"ready":     tally[state.SetupReady],
			"needed":    tally[state.SetupNeeded],
			"undecided": tally[state.SetupUndecided],
			"blocked":   tally[state.SetupBlocked],
			"missing":   tally[state.SetupMissing],
		}, nil
	})
}

// setupGroups is the whole check, in the order somebody meets the problems:
// what they installed, what a node runs, what a link is measured over, and what
// an emulated board needs on top of all three.
func setupGroups(s *session.Sim, w *state.World) []state.SetupGroup {
	found, rows := toolsFound(), w.Resources
	allowed, asked := s.UpdateConsent()
	return []state.SetupGroup{
		buildGroup(found, updates.SetupRow(w.Update, allowed, asked)),
		firmwareGroup(s),
		terrainGroup(s, rows),
		toolchainGroup(rows, found),
	}
}

// setupTally counts the states, so the page and a script agree on what "ready"
// means without either counting for itself.
func setupTally(groups []state.SetupGroup) map[state.SetupState]int {
	out := map[state.SetupState]int{}
	for _, g := range groups {
		for _, r := range g.Rows {
			out[state.SetupState(r.State)]++
		}
	}
	return out
}

// setupWire is the check in a shape the socket carries. The same lesson as the
// resource listing: a count where the rows were the question is an answer only
// a panel can use.
func setupWire(groups []state.SetupGroup) []map[string]any {
	out := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		rows := make([]map[string]any, 0, len(g.Rows))
		for _, r := range g.Rows {
			rows = append(rows, map[string]any{
				"name": r.Name, "state": r.State, "what": r.What,
				"cost": r.Cost, "where": r.Where, "do": r.Do,
				"verb": r.Verb, "params": r.Params,
			})
		}
		out = append(out, map[string]any{
			"name": g.Name, "note": g.Note, "rows": rows,
		})
	}
	return out
}

// firmwareGroup is what a node would actually run.
//
// Two rows, not one. A native build is an executable for this machine and a
// board image is bytes for a chip, and a machine holding one of them is not
// halfway to holding the other.
func firmwareGroup(s *session.Sim) state.SetupGroup {
	cache := firmware.DefaultCacheDir()
	native, board := 0, 0
	for _, b := range firmware.ListInstalled(cache) {
		if b.Native {
			native++
			continue
		}
		board++
	}
	wantNative, wantBoard := nodesByBackend(s)
	return state.SetupGroup{
		Name: "Firmware",
		Note: "Builds are downloaded on demand and cached in " + cache +
			". Over the socket, firmware.download takes a per-role release tag " +
			"- repeater-v1.17.0, companion-v1.17.0 - because that is how the " +
			"assets are published; a bare v1.17.0 resolves nothing.",
		Rows: []state.SetupRow{
			firmwareRow("native builds", "native", native, wantNative,
				"MeshCore compiled for this machine, which is what a node runs "+
					"unless it is pinned to a board",
				"a few megabytes a role", filepath.Join(cache, "native")),
			firmwareRow("board images", "board", board, wantBoard,
				"the published .bin and .uf2 a real board would be flashed with, "+
					"for emulated nodes",
				"one to two megabytes an image",
				filepath.Join(cache, firmware.BoardDir)),
		},
	}
}

// firmwareRow is one half of the library, and the action it offers is to open
// the library rather than to download anything: which build to take has a role
// and a board in it, and a button that chose one would choose wrong most of the
// time.
func firmwareRow(name, kind string, have, want int, what, cost, where string) state.SetupRow {
	row := state.SetupRow{
		Name: name, What: what, Cost: cost, Where: where,
		State: string(firmwareState(have, want)),
		Do:    firmwareDo(kind, have, want),
	}
	if have == 0 {
		row.Verb, row.Params = "panel.open", map[string]any{"name": "Firmware"}
	}
	return row
}

// nodesByBackend counts what this session would need of each kind, so an empty
// cache reads as blocking only where something is actually waiting on it.
func nodesByBackend(s *session.Sim) (native, board int) {
	for _, n := range s.Nodes() {
		if n.Firmware.Emulated() {
			board++
			continue
		}
		native++
	}
	return native, board
}

func firmwareState(have, want int) state.SetupState {
	switch {
	case have > 0:
		return state.SetupReady
	case want > 0:
		return state.SetupNeeded
	default:
		return state.SetupMissing
	}
}

func firmwareDo(kind string, have, want int) string {
	switch {
	case have > 0:
		return "nothing to do: the library holds " + kind + " builds already, " +
			"and Firmware can fetch more."
	case want == 0:
		return "nothing here is waiting on one. Firmware lists what is " +
			"published and downloads one when something is."
	default:
		return "open Firmware and download one for each role in use. Nodes " +
			"waiting on a build will not start until one is there."
	}
}

// terrainGroup is the one large download the application makes on its own
// initiative, and therefore the only one that has to ask first.
func terrainGroup(s *session.Sim, rows []state.ResourceRow) state.SetupGroup {
	allowed, asked := s.TerrainConsent()
	return state.SetupGroup{
		Name: "Terrain",
		Note: "The basemap and the map tiles under the view fill themselves as " +
			"the map is used, and are small. Terrain heights are neither: the " +
			"ground under a national network is several hundred megabytes, " +
			"which is why it is the one thing this machine is asked about. " +
			"Resources says what any of it has cost the disk.",
		Rows: []state.SetupRow{
			terrainRow(allowed, asked, resourceByName(rows, "terrain tiles")),
		},
	}
}

func terrainRow(allowed, asked bool, onDisk state.ResourceRow) state.SetupRow {
	row := state.SetupRow{
		Name: "terrain heights",
		What: "the ground every link budget is measured over; without it the " +
			"model falls back to free space, which flatters every link a hill " +
			"would have blocked",
		// Short enough to read across from the name. The shape of the cost
		// belongs on the row; the exact figure belongs to the study that is
		// about to spend it, and that one is measured rather than guessed.
		Cost:  "a few hundred MB for a country",
		Where: onDisk.Path,
		Verb:  "terrain.allow",
	}
	switch {
	case !asked:
		row.State = string(state.SetupUndecided)
		row.Do = "nothing has been downloaded, and nothing will be until this " +
			"is answered. Allowed, a study fetches only the tiles its own links " +
			"cross, tens of kilobytes each, quoting the total before it starts; " +
			"they are then cached for ever."
		row.Params = map[string]any{"on": true}
	case !allowed:
		row.State = string(state.SetupMissing)
		row.Do = "downloads are off. Links are measured over whatever ground " +
			"is already cached, and over ground that is not there the answer " +
			"is free space - the most optimistic one there is."
		row.Params = map[string]any{"on": true}
	default:
		row.State = string(state.SetupReady)
		row.Do = "downloads are on. A study fetches the tiles it needs and " +
			"says how many first; the switch is also in Configuration > System."
		row.Params = map[string]any{"on": false}
	}
	return row
}

// resourceByName finds one listed row, or the zero value. Names are unique
// across the providers this reads, which is not true of kinds.
func resourceByName(rows []state.ResourceRow, name string) state.ResourceRow {
	for _, r := range rows {
		if r.Name == name {
			return r
		}
	}
	return state.ResourceRow{}
}
