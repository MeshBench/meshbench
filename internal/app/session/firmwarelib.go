// The firmware library: what is on this machine, and what can be.
//
// The old workbench had a window for this - filters, downloads with progress,
// import, delete, use-for-role, wipe - and the Gio build had none of it. What
// is actually in the cache is the only thing that decides what a node can run,
// and a build that failed to download looks identical to one in daily use from
// outside that directory.
//
// The three verbs here are the ones that only read. Changing the cache is in
// firmwarecache.go and changing a node is in firmwarenodes.go, registered from
// here so the store is still wired up by one call.
package session

import (
	"sort"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

func registerFirmwareLibrary(st *state.Store, s *Sim) {
	registerFirmwareCache(st, s)
	registerFirmwareNodes(st, s)

	st.HandleSpec("firmware.installed", state.Spec{
		What: "read what is actually in the firmware cache on this machine, " +
			"which is the only thing that decides what a node can start today",
		Returns: []string{"cache", "installed"},
		Answers: "`installed` is a row per build on disk - `version`, `role`, " +
			"`board`, `native`, `bytes`, `path` - and is empty on a machine that " +
			"has downloaded nothing. It says nothing about what is published: " +
			"`firmware.library` is the list that holds both.",
		Example: &state.Example{
			Params: map[string]any{}, What: "see what this machine can run offline",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		cache := firmware.DefaultCacheDir()
		in := firmware.ListInstalled(cache)
		out := make([]map[string]any, 0, len(in))
		builds := make([]state.Build, 0, len(in))
		for _, b := range in {
			out = append(out, map[string]any{
				"version": b.Version, "role": b.Role, "board": b.Board,
				"native": b.Native, "bytes": b.Bytes, "path": b.Path,
			})
			builds = append(builds, state.Build{
				Version: b.Version, Role: b.Role, Board: b.Board,
				Native: b.Native, Bytes: b.Bytes, Path: b.Path,
			})
		}
		w.Builds = builds
		return map[string]any{"cache": cache, "installed": out}, nil
	})

	// Published and on disk in one list rather than two, because a build
	// imported from a branch is in no catalogue and is exactly the kind of
	// thing worth testing, while a published one nobody has fetched still has
	// to be offerable. The old workbench answers this question with a table
	// and the Gio build answered it with a form asking for a version somebody
	// was expected to already know.
	st.HandleSpec("firmware.library", state.Spec{
		What: "list every build there is, on disk and published together, so a " +
			"build nobody has fetched can still be offered and one imported " +
			"from a branch still appears",
		Returns: []string{"builds", "count"},
		Answers: "The catalogue is asked over the network in the background and " +
			"its answer lands seconds later, so the first call in a session " +
			"answers from disk alone and is worth making again. Each row in " +
			"`builds` carries `role`, `version`, `board`, `bytes`, `on_disk`, " +
			"`path`, `in_use` and `unavailable`.",
		Example: &state.Example{
			Params: map[string]any{}, What: "list what could be run, fetched or not",
		},
	}, func(w *state.World, _ any) (any, error) {
		// Disk first, immediately; the network once, afterwards. The library
		// read only the catalogue's cache, and everything in the cache is by
		// definition already downloaded - so the one thing a library is for,
		// showing what could be fetched, never appeared on it.
		s.startPublishedFetch(st)
		s.fillLibrary(w)
		return map[string]any{
			"builds": libraryRows(w.Library), "count": len(w.Library),
		}, nil
	})

	// "no firmware for 34 of 58 nodes" is a diagnosis, not a way out. A run
	// that cannot start should be able to ask what these nodes ought to run,
	// and by role rather than by node, because pinning fifty-eight of them one
	// at a time is not a question anybody answers.
	st.HandleSpec("firmware.needed", state.Spec{
		What: "ask what a mesh that will not start is missing, by role rather " +
			"than by node, and what is already installed to fill each gap",
		Returns: []string{"roles"},
		Answers: "`roles` is a list of `{role, nodes, choices}`: how many nodes " +
			"running that role have no build this machine holds, and the " +
			"versions installed for it, which may be none. An empty list means " +
			"every node that runs firmware has one it can start.",
		Example: &state.Example{
			Params: map[string]any{}, What: "find out what the run is short of",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		installed := firmware.ListInstalled(firmware.DefaultCacheDir())
		have := map[string]bool{}
		for _, b := range installed {
			have[b.Role+"@"+b.Version] = true
			have[b.Version] = true
		}
		var order []string
		counts := map[string]int{}
		for _, n := range s.nodes {
			if !n.Kind.RunsFirmware() {
				continue
			}
			role := nodeRole(n)
			if v := n.Firmware.Version; v != "" && (have[role+"@"+v] || have[v]) {
				continue
			}
			if counts[role] == 0 {
				order = append(order, role)
			}
			counts[role]++
		}
		roles := make([]any, 0, len(order))
		for _, role := range order {
			seen := map[string]bool{}
			var choices []string
			for _, b := range installed {
				if b.Role == role && !seen[b.Version] {
					seen[b.Version] = true
					choices = append(choices, b.Version)
				}
			}
			sort.Strings(choices)
			roles = append(roles, map[string]any{
				"role": role, "nodes": counts[role], "choices": choices,
			})
		}
		return map[string]any{"roles": roles}, nil
	})
}
