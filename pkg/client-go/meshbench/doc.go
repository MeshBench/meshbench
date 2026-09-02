// Package meshbench drives a MeshBench workbench from Go.
//
//	import "github.com/MeshBench/meshbench/pkg/client-go/meshbench"
//
// Under pkg/ beside the Python one, and named the same, because the two
// are peers: neither is the real interface and neither is a wrapper around the
// other. Both speak the control socket, and anything either can do the other
// can.
//
// Not internal/, deliberately: it exists to be imported, and the repository is
// public, so that means anybody rather than only us.
//
// # Two layers
//
// Call is the whole API, and everything else is a shape over it:
//
//	raw, err := wb.Call(ctx, "nodes.place", map[string]any{
//		"name": "Alpha", "kind": "simple-repeater", "lat": 56.3, "lon": -3.3})
//
// Above it sit objects for the verbs a script actually uses - nodes, the
// clock, firmware, the console, events. That layer is hand-written and is
// where readability lives; the escape hatch stays public so a verb the façade
// has not reached is one line away rather than a blocker.
//
// # Waiting
//
// Every wait is a method - WaitRunning, WaitSettled, Job.Wait - never a sleep
// in a caller. They poll today and will subscribe when the socket learns to
// push, and no caller changes when that happens. That is the whole reason
// events come after the clients rather than before.
//
// A wait measured in simulated time is not a wait measured in yours: Run(five
// minutes) is five minutes of the mesh's own clock, and on 155 emulated nodes
// that is a great deal longer than five of yours.
//
// # Honesty
//
// Anything that is a measurement carries a Provenance: the RF mode, the
// realism switches, whether the excess loss was fitted or defaulted, whether
// the fixture is permissive, and the seed. A scripted number gets pasted into
// a report with the caveats stripped, so the caveats travel in the value.
package meshbench
