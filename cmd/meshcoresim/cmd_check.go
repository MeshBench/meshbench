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
	"github.com/MeshBench/meshbench/internal/regression"
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
	sf, bw, freq := regression.RadioOf(fx)
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

	if err := regression.Provision(e, fx); err != nil {
		return err
	}
	if !*quiet {
		fmt.Printf("provisioned %d nodes\n", e.FirmwareCount())
	}
	sends := fx.Sends
	if len(sends) == 0 {
		// Nothing scheduled, so the run would have no traffic in it and every
		// count would be zero. One advert each instead - but spread, not at
		// once: fifty-six nodes adverting on the same millisecond put the
		// loudest of them over 29% duty cycle, which fails a compliance
		// assertion for a reason that is an artefact of the harness rather
		// than a property of the network.
		sends = regression.AdvertSchedule(fx, 30000)
	}
	if err := regression.RunSends(ctx, e, sends, uint32(*forMs)); err != nil {
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
