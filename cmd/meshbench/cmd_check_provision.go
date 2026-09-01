package main

import (
	"context"
	"fmt"
	"time"

	"github.com/MeshBench/meshbench/internal/app/fixture"
	"github.com/MeshBench/meshbench/internal/mesh/companion"
	"github.com/MeshBench/meshbench/internal/mesh/proto"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// One clock for the whole mesh, from the fixture rather than the wall, because
// MeshCore judges freshness by timestamps and a run has to be reproducible. A
// fixed epoch: 1 January 2026.
const scenarioEpoch = 1767225600

// provision tells each node what it is before the run starts.
//
// A node that boots unprovisioned advertises the firmware's built-in name, has
// no position, believes the time is zero, and holds no regions - so it neither
// relays scoped traffic nor gives anybody a reason to send it any. The first
// version of this command skipped it and reported zero deliveries on a healthy
// mesh, which reads as a broken simulator and is a missing step.
//
// The region half comes from internal/fixture, the same function the workbench
// uses, because that is the part with a trap in it. The rest is deliberately
// the plain subset: a headless gate does not need the operator's provisioning
// preferences, it needs the fixture to behave as its author saw it behave.
//
// Companions and room servers go the other way, over the protocol they speak.
// Typing at one writes bytes nothing reads, so every line here reported itself
// sent while the node stayed nameless, positionless and unscoped for the whole
// run - and unable to say anything when a schedule asked it to.
func provision(ctx context.Context, e *engine.Engine, fx *fixture.Fixture, quiet bool) error {
	lines := 0
	var framed []scenario.Node
	for _, spec := range fx.Nodes {
		n, ok := e.NodeByName(spec.Name)
		if !ok || n.Firmware == nil {
			continue
		}
		if fixture.SpeaksCompanion(spec.Kind) {
			framed = append(framed, spec)
			continue
		}
		for _, c := range typedProvisioning(spec) {
			if err := n.Firmware.Bridge.Type([]byte(c + "\r\n")); err != nil {
				return fmt.Errorf("provisioning %s: %w", spec.Name, err)
			}
			lines++
		}
	}
	sent, err := provisionCompanions(ctx, e, framed)
	if err != nil {
		return err
	}
	if !quiet {
		fmt.Printf("provisioned %d nodes with %d lines and %d frames\n",
			e.FirmwareCount(), lines, sent)
	}
	return nil
}

// typedProvisioning is the console script for a node that has a console.
func typedProvisioning(spec scenario.Node) []string {
	cmds := []string{
		"set name " + spec.Name,
		fmt.Sprintf("time %d", scenarioEpoch),
	}
	if spec.Kind.Transmits() {
		cmds = append(cmds,
			fmt.Sprintf("set lat %.6f", spec.Position.Lat),
			fmt.Sprintf("set lon %.6f", spec.Position.Lon))
	}
	return append(cmds, fixture.RegionCommands(spec)...)
}

// provisionCompanions tells the framed-protocol nodes what they are.
//
// Two passes with the clock moved between them, which is the part that cannot
// be shortened. Everything a companion is told waits on its reply to AppStart,
// and that reply is written by firmware, which only runs when simulated time
// moves; sent in one breath the configuration lands in front of a device that
// has not finished starting and it leaves the bridge.
//
// Nothing here reads the replies. The harness has no panel to draw them in and
// no decision that depends on them, and a claim on the port would take it from
// the -endpoint client this command can also serve.
func provisionCompanions(ctx context.Context, e *engine.Engine, specs []scenario.Node) (int, error) {
	if len(specs) == 0 {
		return 0, nil
	}
	sent := 0
	for _, spec := range specs {
		n, ok := e.NodeByName(spec.Name)
		if !ok || n.Firmware == nil {
			continue
		}
		if err := frame(n, proto.AppStart("meshbench")); err != nil {
			return sent, fmt.Errorf("app start at %s: %w", spec.Name, err)
		}
		sent++
	}
	if err := settle(ctx, e, 500); err != nil {
		return sent, fmt.Errorf("companion handshake: %w", err)
	}
	for _, spec := range specs {
		n, ok := e.NodeByName(spec.Name)
		if !ok || n.Firmware == nil {
			continue
		}
		for _, payload := range fixture.CompanionProvisioning(spec, scenarioEpoch) {
			if err := frame(n, payload); err != nil {
				return sent, fmt.Errorf("provisioning %s: %w", spec.Name, err)
			}
			sent++
		}
	}
	if err := settle(ctx, e, 2000); err != nil {
		return sent, fmt.Errorf("configuring companions: %w", err)
	}
	return sent, nil
}

// say runs one line of a schedule at whichever console the node has.
func say(n *engine.Node, line string, nowMs uint32) error {
	b, err := scheduledLine(n.Spec().Kind, line, nowMs)
	if err != nil {
		return err
	}
	return n.Firmware.Bridge.Type(b)
}

// scheduledLine is the bytes one line of a schedule becomes at a node of this
// kind: typed at a console, or framed at a companion.
//
// A function of its own with nothing running behind it, because the choice is
// the whole bug this file was written to fix and it is invisible when it goes
// wrong. Writing at a port succeeds whether or not anything reads it, so text
// sent at a companion is reported as sent, does nothing, and reads as broken RF
// three layers away.
func scheduledLine(k scenario.Kind, line string, nowMs uint32) ([]byte, error) {
	if !fixture.SpeaksCompanion(k) {
		return []byte(line + "\r\n"), nil
	}
	// Simulated time, not the wall clock: a message stamped with the hour the
	// run happened to start at is a run that cannot be compared with another,
	// and determinism is the property this whole command exists to offer.
	at := time.Unix(scenarioEpoch+int64(nowMs)/1000, 0).UTC()
	payload, err := fixture.CompanionCommand(line, at)
	if err != nil {
		return nil, err
	}
	return companion.Frame(payload), nil
}

// frame puts the envelope on a payload and writes it at the node's port.
func frame(n *engine.Node, payload []byte) error {
	return n.Firmware.Bridge.Type(companion.Frame(payload))
}

// settle lets the firmware answer, which it can only do while the clock moves.
func settle(ctx context.Context, e *engine.Engine, ms uint32) error {
	return e.Run(ctx, e.NowMs()+ms)
}
