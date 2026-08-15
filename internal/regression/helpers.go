// Provisioning, sending and scheduling: the plain subset a headless run
// needs, moved here from cmd/meshcoresim so both the CLI's single-fixture
// `test` command and a directory of regression cases run a fixture exactly
// the same way. Two copies of "how a fixture boots" is the drift this
// package exists to prevent, the same reasoning fixture.RegionCommands
// itself is built on.
package regression

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/fixture"
)

// RadioOf takes the radio settings from the fixture's own nodes rather than
// assuming a default - a fixture built on one preset and run on another is a
// different network wearing the same node list.
func RadioOf(fx *fixture.Fixture) (sf int, bwHz, freqMHz float64) {
	freqMHz = fx.FreqMHz
	for _, n := range fx.Nodes {
		if n.Radio.SpreadFactor > 0 && n.Radio.BandwidthHz > 0 {
			if freqMHz == 0 {
				freqMHz = n.Radio.CentreHz / 1e6
			}
			return n.Radio.SpreadFactor, n.Radio.BandwidthHz, freqMHz
		}
	}
	return 10, 250e3, freqMHz
}

// Provision tells each node what it is before the run starts. A node that
// boots unprovisioned advertises the firmware's built-in name, has no
// position, believes the time is zero, and holds no regions - so it neither
// relays scoped traffic nor gives anybody a reason to send it any.
func Provision(e *engine.Engine, fx *fixture.Fixture) error {
	// One clock for the whole mesh, from the fixture rather than the wall,
	// because MeshCore judges freshness by timestamps and a run has to be
	// reproducible. A fixed epoch: 1 January 2026.
	const epoch = 1767225600
	for _, spec := range fx.Nodes {
		n, ok := e.NodeByName(spec.Name)
		if !ok || n.Firmware == nil {
			continue
		}
		cmds := []string{
			"set name " + spec.Name,
			fmt.Sprintf("time %d", epoch),
		}
		if spec.Kind.Transmits() {
			cmds = append(cmds,
				fmt.Sprintf("set lat %.6f", spec.Position.Lat),
				fmt.Sprintf("set lon %.6f", spec.Position.Lon))
		}
		cmds = append(cmds, fixture.RegionCommands(spec)...)
		for _, c := range cmds {
			if err := n.Firmware.Bridge.Type([]byte(c + "\r\n")); err != nil {
				return fmt.Errorf("provisioning %s: %w", spec.Name, err)
			}
		}
	}
	return nil
}

// RunSends plays a fixture's traffic schedule and then lets the run finish.
func RunSends(ctx context.Context, e *engine.Engine, sends []fixture.Send, forMs uint32) error {
	type pending struct {
		fixture.Send
		next uint32
	}
	var queue []pending
	for _, s := range sends {
		queue = append(queue, pending{s, s.AtMs})
	}
	for {
		now := e.NowMs()
		if now >= forMs {
			return nil
		}
		until := forMs
		for i := range queue {
			if queue[i].next > now && queue[i].next < until {
				until = queue[i].next
			}
		}
		for i := range queue {
			q := &queue[i]
			if q.next > now || q.Command == "" {
				continue
			}
			n, ok := e.NodeByName(q.Node)
			if !ok || n.Firmware == nil {
				return fmt.Errorf("send at %d ms: %s runs no firmware", q.AtMs, q.Node)
			}
			if err := n.Firmware.Bridge.Type([]byte(q.Command + "\r\n")); err != nil {
				return err
			}
			if q.EveryMs > 0 {
				q.next = now + q.EveryMs
			} else {
				q.next = forMs + 1
			}
		}
		if err := e.Run(ctx, until); err != nil {
			return err
		}
		if until == forMs {
			return nil
		}
	}
}

// AdvertSchedule spreads one advert per transmitting node across a window -
// what a fixture with no traffic of its own gets, so the run has something
// in it. Spread rather than simultaneous: nodes adverting on the same
// millisecond put the loudest of them over a duty-cycle assertion for a
// reason that is an artefact of the harness, not a property of the network.
func AdvertSchedule(fx *fixture.Fixture, windowMs uint32) []fixture.Send {
	var tx []string
	for _, n := range fx.Nodes {
		if n.Kind.Transmits() {
			tx = append(tx, n.Name)
		}
	}
	if len(tx) == 0 {
		return nil
	}
	out := make([]fixture.Send, 0, len(tx))
	for i, name := range tx {
		out = append(out, fixture.Send{
			Node: name, AtMs: uint32(i) * windowMs / uint32(len(tx)), Command: "advert"})
	}
	return out
}
