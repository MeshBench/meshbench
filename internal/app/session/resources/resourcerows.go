// The resource inventory in a shape the socket can carry.
//
// resource.list answered with how many rows there were and put the rows
// themselves in the snapshot, where only a panel can reach them. From outside
// the window "what does this machine hold, and what could it fetch" was
// therefore unanswerable - the same mistake nodes.stats, firmware.library, the
// study area and console.read each made, and it is fixed the same way.
package resources

import "github.com/MeshBench/meshbench/internal/app/state"

func resourceRows(rows []state.ResourceRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"kind": r.Kind, "name": r.Name, "version": r.Version,
			"state": r.State, "bytes": r.Bytes, "estimated": r.Estimated,
			"path": r.Path,
			// Why carries the reason for anything that is not a plain on-disk
			// row, which for a tool this platform has no build for is the
			// whole content of the answer.
			"why": r.Why,
			// Where to get it when it cannot be got from here. A script
			// reading fetchable:false learns only that this is not the door.
			"howto": r.HowTo, "howto_panel": r.HowToPanel,
			// What a caller may do about it, so a script does not have to try
			// a fetch to find out that there is nothing to ask for.
			"fetchable": r.Fetchable, "licensed": r.Licensed, "auto": r.Auto,
		})
	}
	return out
}
