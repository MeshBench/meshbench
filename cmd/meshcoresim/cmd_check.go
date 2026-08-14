package main

import (
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/fixture"
)

// runTest is the regression harness: load a fixture, run it on real firmware,
// and decide whether it passed.
//
// It is the piece that lets a firmware change be judged by something other than
// a person reading two event logs and forming an opinion. An exit code and a
// JUnit file are what a pipeline can act on; the printed summary is for whoever
// has to work out why it went red.
//
// Native firmware only, and deliberately. Emulated nodes run on wall time, so
// two runs of one seed do not agree, and a gate that flickers is worse than no
// gate.
func runTest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	store := terrainFlags(fs)
	path := fs.String("fixture", "", "fixture JSON to run")
	forMs := fs.Uint("for", 120000, "how long to simulate, ms")
	seed := fs.Uint64("seed", 0, "override the fixture's seed")
	junit := fs.String("junit", "", "write a JUnit XML report here")
	endpoint := fs.String("endpoint", "",
		"serve a companion node to a real client: \"tcp:<node>\" or \"serial:<node>\"")
	quiet := fs.Bool("quiet", false, "only print the verdict")
	if err := parse(fs, args, "run a fixture and check its assertions"); err != nil {
		return err
	}
	if *path == "" {
		return fmt.Errorf("give a fixture with -fixture; the shipped ones are in fixtures/")
	}

	fx, err := fixture.Load(*path)
	if err != nil {
		return err
	}
	t, err := store()
	if err != nil {
		return err
	}
	if len(fx.Assertions) == 0 {
		// Not an error: a fixture with no assertions is still worth running for
		// its summary. It cannot fail, though, and saying so beats a green tick
		// that means nothing.
		fmt.Fprintln(os.Stderr, "meshcoresim: this fixture carries no assertions, "+
			"so it can report but not pass or fail")
	}

	runSeed := fx.Seed
	if *seed != 0 {
		runSeed = *seed
	}
	sf, bw, freq := radioOf(fx)
	e := engine.New(t, engine.Config{
		FreqMHz: freq, SF: sf, BandwidthHz: bw, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: runSeed,
	})
	defer func() { _ = e.Close() }()
	for _, n := range fx.Nodes {
		e.Add(n, nil)
	}

	started := time.Now()
	if err := e.AttachNative(ctx, runSeed); err != nil {
		return fmt.Errorf("%w\n\nBuild one with meshcore-native/build.sh, or check that the "+
			"fixture's firmware versions are published: a bare v1.17.0 resolves nothing, "+
			"because MeshCore tags one role at a time", err)
	}

	on, tx := fx.Permissive()
	fmt.Printf("%s: %d nodes, %d running firmware, SF%d at %.0f kHz, seed %d\n",
		fx.Name, len(fx.Nodes), e.FirmwareCount(), sf, bw/1000, runSeed)
	if on > 0 {
		// Loudly, every time. A permissive fixture answers a reach question more
		// generously than the real network, and a report that does not say so is
		// the flattering-but-wrong answer this simulator exists to avoid.
		fmt.Printf("PERMISSIVE: %d of %d transmitting nodes forward flood traffic for any "+
			"region. This is more permissive than the real network.\n", on, tx)
	}

	if *endpoint != "" {
		link, err := serveEndpoint(e, *endpoint)
		if err != nil {
			return err
		}
		fmt.Printf("endpoint: %s %s (node %s)\n", link.Kind, link.Addr, link.Node)
	}

	if err := provision(e, fx, *quiet); err != nil {
		return err
	}
	sends := fx.Sends
	if len(sends) == 0 {
		// Nothing scheduled, so the run would have no traffic in it and every
		// count would be zero. One advert each instead - but spread, not at
		// once: fifty-six nodes adverting on the same millisecond put the
		// loudest of them over 29% duty cycle, which fails a compliance
		// assertion for a reason that is an artefact of the harness rather
		// than a property of the network.
		sends = advertSchedule(fx, 30000)
	}
	if err := runSends(ctx, e, sends, uint32(*forMs), *quiet); err != nil {
		return err
	}

	results := e.Check(fx.Checks())
	failed := report(results, *quiet)
	if *junit != "" {
		if err := writeJUnit(*junit, fx.Name, results, time.Since(started)); err != nil {
			return err
		}
		fmt.Printf("JUnit written to %s\n", *junit)
	}
	if failed > 0 {
		// An error, so main exits 1: a pipeline reads the code, not the prose.
		return fmt.Errorf("%d of %d assertions failed", failed, len(results))
	}
	fmt.Printf("PASS: %d assertions, %v\n", len(results), time.Since(started).Round(time.Second))
	return nil
}

// radioOf takes the radio settings from the fixture's own nodes rather than
// from flags. A fixture built on the EU narrow preset and run at a default
// SF10/250 kHz is a different network wearing the same node list.
func radioOf(fx *fixture.Fixture) (sf int, bwHz, freqMHz float64) {
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

// serveEndpoint hands one node's companion protocol to a real client, so an
// application developer can point their app at a whole simulated mesh.
func serveEndpoint(e *engine.Engine, spec string) (*engine.CompanionLink, error) {
	kind, node, ok := strings.Cut(spec, ":")
	if !ok || node == "" {
		return nil, fmt.Errorf("endpoint %q: write it as tcp:<node> or serial:<node>", spec)
	}
	switch kind {
	case "tcp":
		// Port 0: the operating system picks a free one and the link reports
		// it. A fixed default would collide with the last run that has not
		// finished dying.
		return e.ServeCompanionTCP(node, "127.0.0.1:0")
	case "serial":
		return e.ServeCompanionSerial(node)
	}
	return nil, fmt.Errorf("endpoint %q: kind must be tcp or serial", kind)
}

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
func provision(e *engine.Engine, fx *fixture.Fixture, quiet bool) error {
	// One clock for the whole mesh, from the fixture rather than the wall,
	// because MeshCore judges freshness by timestamps and a run has to be
	// reproducible. A fixed epoch: 1 January 2026.
	const epoch = 1767225600
	lines := 0
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
			lines++
		}
	}
	if !quiet {
		fmt.Printf("provisioned %d nodes with %d lines\n", e.FirmwareCount(), lines)
	}
	return nil
}

// runSends plays the fixture's traffic schedule and then lets the run finish.
func runSends(ctx context.Context, e *engine.Engine, sends []fixture.Send, forMs uint32, quiet bool) error {
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
		// The next thing due, or the end of the run - whichever comes first.
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
			if !quiet {
				fmt.Printf("  %6.1f s  %s: %s\n", float64(now)/1000, q.Node, q.Command)
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

// advertSchedule spreads one advert per transmitting node across a window.
func advertSchedule(fx *fixture.Fixture, windowMs uint32) []fixture.Send {
	var tx []string
	for _, n := range fx.Nodes {
		if n.Kind.Transmits() {
			tx = append(tx, n.Name)
		}
	}
	out := make([]fixture.Send, 0, len(tx))
	for i, name := range tx {
		out = append(out, fixture.Send{
			Node: name, AtMs: uint32(i) * windowMs / uint32(len(tx)), Command: "advert"})
	}
	return out
}

func report(results []engine.Result, quiet bool) int {
	failed := 0
	for _, r := range results {
		if !r.Passed {
			failed++
		}
		if quiet && r.Passed {
			continue
		}
		mark := "ok  "
		if !r.Passed {
			mark = "FAIL"
		}
		fmt.Printf("%s  %s\n        %s\n", mark, r.Assertion.String(), r.Detail)
	}
	return failed
}

// JUnit, because it is what every pipeline already reads. The shape is the
// small subset every consumer agrees on: one suite, one case per assertion, and
// the detail in the failure message rather than only in the log.
type junitSuite struct {
	XMLName  xml.Name    `xml:"testsuite"`
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Time     float64     `xml:"time,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func writeJUnit(path, name string, results []engine.Result, took time.Duration) error {
	suite := junitSuite{Name: name, Tests: len(results), Time: took.Seconds()}
	for _, r := range results {
		c := junitCase{Name: r.Assertion.String(), ClassName: name}
		if !r.Passed {
			suite.Failures++
			c.Failure = &junitFailure{Message: r.Detail, Text: r.Detail}
		}
		suite.Cases = append(suite.Cases, c)
	}
	b, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), b...), 0o644)
}
