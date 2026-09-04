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
package firmwarelib

import (
	"sort"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

func registerFirmwareLibrary(st *state.Store, s *session.Sim) {
	registerFirmwareCache(st, s)
	registerFirmwareNodes(st, s)

	st.Handle("firmware.installed", func(w *state.World, _ any) (any, error) {
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
	st.Handle("firmware.library", func(w *state.World, _ any) (any, error) {
		// Disk first, immediately; the network once, afterwards. The library
		// read only the catalogue's cache, and everything in the cache is by
		// definition already downloaded - so the one thing a library is for,
		// showing what could be fetched, never appeared on it.
		startPublishedFetch(s, st)
		fillLibrary(s, w)
		return map[string]any{
			"builds": session.LibraryRows(w.Library), "count": len(w.Library),
		}, nil
	})

	// "no firmware for 34 of 58 nodes" is a diagnosis, not a way out. A run
	// that cannot start should be able to ask what these nodes ought to run,
	// and by role rather than by node, because pinning fifty-eight of them one
	// at a time is not a question anybody answers.
	st.Handle("firmware.needed", func(w *state.World, _ any) (any, error) {
		installed := firmware.ListInstalled(firmware.DefaultCacheDir())
		have := map[string]bool{}
		for _, b := range installed {
			have[b.Role+"@"+b.Version] = true
			have[b.Version] = true
		}
		var order []string
		counts := map[string]int{}
		for _, n := range s.Nodes() {
			if !n.Kind.RunsFirmware() {
				continue
			}
			role := session.NodeRole(n)
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
