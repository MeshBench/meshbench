// The same intent, on the wire a companion actually speaks.
//
// A companion has no CLI - companion_radio is not built on CommonCLI.cpp, and
// answers only the framed binary app protocol. Every command runProvisioning
// sends elsewhere goes in as text over Bridge.Type; doing that to a companion
// would write garbage into a protocol that frames on byte length, not
// newlines, which is a way to corrupt the connection rather than merely fail
// it. So a companion never enters the CLI read/decide/send pipeline at all -
// it gets this one instead, translating the same resolved effects into the
// handful of proto.Set* frames that exist for it.
//
// Loop detection has no equivalent here, and is silently absent from a
// companion's resolved commands for exactly that reason - see
// provision.Key.CompanionField and resolveScalars' own check.
package session

import (
	"strconv"
	"strings"

	"github.com/MeshBench/meshbench/internal/companion/proto"
	"github.com/MeshBench/meshbench/internal/provider"
	"github.com/MeshBench/meshbench/internal/provision"
)

// companionFrames translates a node's resolved CLI-shaped commands into the
// binary frames a companion answers instead. Lat and lon arrive as two
// separate commands - one repeater `set` per axis - and are combined here,
// since the companion protocol sets a position in one call.
func companionFrames(cmds []provision.ResolvedCommand) [][]byte {
	var out [][]byte
	var lat, lon *float64
	for _, c := range cmds {
		switch {
		case strings.HasPrefix(c.Command, "set name "):
			out = append(out, proto.SetAdvertName(strings.TrimPrefix(c.Command, "set name ")))
		case strings.HasPrefix(c.Command, "set lat "):
			if v, err := strconv.ParseFloat(strings.TrimPrefix(c.Command, "set lat "), 64); err == nil {
				lat = &v
			}
		case strings.HasPrefix(c.Command, "set lon "):
			if v, err := strconv.ParseFloat(strings.TrimPrefix(c.Command, "set lon "), 64); err == nil {
				lon = &v
			}
		case strings.HasPrefix(c.Command, "set path.hash.mode "):
			if v, err := strconv.Atoi(strings.TrimPrefix(c.Command, "set path.hash.mode ")); err == nil {
				out = append(out, proto.SetPathHashMode(uint8(v)))
			}
		case strings.HasPrefix(c.Command, "time "):
			if v, err := strconv.ParseUint(strings.TrimPrefix(c.Command, "time "), 10, 32); err == nil {
				out = append(out, proto.SetDeviceTime(uint32(v)))
			}
		case strings.HasPrefix(c.Command, "region default "):
			scope := strings.TrimPrefix(c.Command, "region default ")
			if scope == "<null>" {
				out = append(out, proto.ClearDefaultScope())
			} else {
				out = append(out, proto.SetDefaultScope(scope, provider.RegionKey(scope)))
			}
			// region put/allowf/save and un-scoped flood have no companion
			// equivalent at all - a companion does not hold a region table,
			// only the one scope it originates under - so they are silently
			// not translated rather than attempted and failed.
		}
	}
	if lat != nil && lon != nil {
		out = append(out, proto.SetAdvertLatLon(*lat, *lon))
	}
	return out
}
