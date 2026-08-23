// The firmware library in a shape the socket can carry.
//
// firmware.library answered with how many builds there were and put the builds
// themselves in the snapshot, where only a panel can reach them. From outside
// the window "which builds does this machine hold" was therefore unanswerable,
// and every client that tried got an integer where it expected a list - which
// is the same mistake nodes.stats made, and is fixed the same way.
package session

import "github.com/MeshBench/meshbench/internal/app/state"

func libraryRows(rows []state.FirmwareRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, b := range rows {
		out = append(out, map[string]any{
			"role": b.Role, "version": b.Version, "board": b.Board,
			"bytes": b.Bytes, "on_disk": b.OnDisk, "path": b.Path,
			// What a delete would break, and why a row nobody can honour is
			// not simply hidden.
			"in_use": b.InUse, "unavailable": b.Unavailable,
		})
	}
	return out
}
