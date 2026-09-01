package engine_test

import (
	"context"
	"os"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// All three roles in one mesh, and the difference between two of them.
//
// A room server is a BBS for shared posts - it stores message history and
// hands it back to clients that log in, the way an email server does. It
// looks like a repeater from every side this simulator normally sees: it runs
// on a mast, it has the same console, it takes the same admin password. The
// one place it differs is the air, and it differs in the direction that
// flatters a network - by default it does not forward. (MeshCore lets one
// repeat with `set repeat on`, which its own docs discourage; the published
// build boots with forwarding off, and that default is what a scenario has to
// assume.) A scenario that models a room server as a repeater reports reach
// the mesh does not have.
//
// The behaviour is read from the middle node's own transmissions, not from
// whether the far end heard anything. Over flat earth the 73 km sender-to-far
// path still closes directly - confirmed live: with a room server in the
// middle the far end hears the advert while the room server transmits nothing
// at all - so "did the far end hear it" measures the direct path, not the
// relay. What separates the roles is whether the middle node retransmitted:
// the repeater does (tx > 0), the room server does not (tx == 0), and that is
// what each run asserts, having first confirmed the middle actually heard the
// advert it was given the chance to forward.
//
//	MESHBENCH_LIVE=1 go test ./internal/engine/ -run TestLiveRoles -v -timeout 400s
func TestLiveRolesRepeaterForwardsAndRoomServerDoesNot(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}

	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	radio := scenario.RadioConfig{CentreHz: 869.525e6, BandwidthHz: 250e3,
		SpreadFactor: 10, CodingRate: 1}

	// Same chain both times, only the middle node's role changes. Reusing the
	// geometry is the point: a difference in the result then has one cause.
	run := func(t *testing.T, prefix string, middle scenario.Kind) (tx, rx map[string]int) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 180e9)
		defer cancel()

		e := engine.New(flat{}, engine.Config{
			FreqMHz: 869.525, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
			NoiseFigDB: 6, StepMs: 10, Seed: 4417,
		})
		defer func() { _ = e.Close() }()

		// ~37 km hops, as in the flood test: each hop is comfortably decodable
		// and the 73 km end-to-end path is far below the demodulator floor, so
		// anything at the far end arrived by a relay.
		// A native version is a release tag, and the tags are per role -
		// room-server-v1.17.0, not v1.17.0 - so a scenario mixing roles has to
		// pin each one. Getting this wrong resolves nothing and reads as
		// "no native builds published", which points at the release rather
		// than at the string.
		version := map[scenario.Kind]string{
			scenario.Companion:      "companion-v1.17.0",
			scenario.SimpleRepeater: "repeater-v1.17.0",
			scenario.RoomServer:     "room-server-v1.17.0",
		}
		for _, spec := range []struct {
			name     string
			kind     scenario.Kind
			lat, lon float64
		}{
			// The ends are repeaters so the sender has a CLI to type "advert"
			// at. A companion has no command line - it speaks the companion
			// protocol over its serial link - so typing at one silently does
			// nothing, which reads as a mesh that dropped the packet.
			{prefix + "-sender", scenario.SimpleRepeater, 56.70, -3.90},
			{prefix + "-middle", middle, 56.70, -3.30},
			{prefix + "-far", scenario.SimpleRepeater, 56.70, -2.70},
		} {
			e.Add(scenario.Node{
				Name: spec.name, Kind: spec.kind,
				Position: scenario.LatLon{Lat: spec.lat, Lon: spec.lon}, HeightAGLm: 10,
				Antenna: mast, TxPowerDBm: 10, NoiseFigureDB: 6, Radio: radio,
				Firmware: scenario.FirmwareRef{Version: version[spec.kind]},
			}, nil)
		}

		if err := e.AttachNative(ctx, 4417); err != nil {
			t.Fatal(err)
		}
		node, ok := e.NodeByName(prefix + "-sender")
		if !ok || node.Firmware == nil {
			t.Fatal("the sender has no firmware")
		}
		if err := node.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
			t.Fatal(err)
		}
		if err := e.Run(ctx, 40_000); err != nil {
			t.Fatal(err)
		}

		tx, rx = map[string]int{}, map[string]int{}
		for _, ev := range e.Events() {
			switch ev.Kind {
			case "tx":
				tx[ev.From]++
			case "rx":
				rx[ev.To]++
			}
		}
		t.Logf("%s: tx=%v rx=%v", prefix, tx, rx)
		return tx, rx
	}

	t.Run("repeater", func(t *testing.T) {
		tx, rx := run(t, "role-rep", scenario.SimpleRepeater)
		if tx["role-rep-middle"] == 0 {
			t.Error("the repeater forwarded nothing; the flood died at the first hop")
		}
		if rx["role-rep-far"] == 0 {
			t.Error("the far node heard nothing, and a relay was its only route")
		}
	})

	t.Run("room server", func(t *testing.T) {
		tx, rx := run(t, "role-room", scenario.RoomServer)
		if rx["role-room-middle"] == 0 {
			t.Error("the room server heard nothing, so this proves nothing about forwarding")
		}
		// The middle node's own transmissions, not the far end's receptions: the
		// direct path leaks over flat earth, so a room server that forwarded
		// nothing is one that transmitted nothing, whatever the far end heard.
		if tx["role-room-middle"] != 0 {
			t.Errorf("the room server transmitted %d packets; it heard the advert and "+
				"forwarded it, which is the repeater's behaviour, not a room server's",
				tx["role-room-middle"])
		}
	})
}
