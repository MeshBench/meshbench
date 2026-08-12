package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/fixture"
)

// runServe gives an application developer a mesh and an address, and nothing
// else to think about.
//
//	meshbench serve
//
// It loads a network, starts real MeshCore firmware on every node, and exposes
// one companion over TCP or a virtual serial port. Point your client at what it
// prints. No project to configure, no scenario to build, no code to change.
func runServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	store := terrainFlags(fs)
	path := fs.String("fixture", "", "network to run; the smallest shipped one by default")
	node := fs.String("node", "", "which companion to expose; the first one by default")
	serial := fs.Bool("serial", false, "expose a virtual serial device instead of TCP")
	addr := fs.String("addr", "127.0.0.1:0", "address to listen on; port 0 picks a free one")
	quiet := fs.Bool("quiet", false, "print the endpoint and nothing else")
	if err := parse(fs, args, "run a mesh and expose a companion to your app"); err != nil {
		return err
	}

	p := *path
	if p == "" {
		var err error
		if p, err = defaultFixture(); err != nil {
			return err
		}
	}
	fx, err := fixture.Load(p)
	if err != nil {
		return err
	}
	t, err := store()
	if err != nil {
		return err
	}

	sf, bw, freq := radioOf(fx)
	e := engine.New(t, engine.Config{
		FreqMHz: freq, SF: sf, BandwidthHz: bw, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: fx.Seed,
	})
	defer func() { _ = e.Close() }()
	for _, n := range fx.Nodes {
		e.Add(n, nil)
	}
	if !*quiet {
		fmt.Printf("%s: %d nodes, starting firmware\n", fx.Name, len(fx.Nodes))
	}
	if err := e.AttachNative(ctx, fx.Seed); err != nil {
		return err
	}
	if err := provision(e, fx, true); err != nil {
		return err
	}

	target := *node
	if target == "" {
		for _, n := range fx.Nodes {
			if n.Kind.Application() == "companion_radio" {
				target = n.Name
				break
			}
		}
	}
	if target == "" {
		return fmt.Errorf("%s has no companion to expose", fx.Name)
	}

	var link *engine.CompanionLink
	if *serial {
		link, err = e.ServeCompanionSerial(target)
	} else {
		link, err = e.ServeCompanionTCP(target, *addr)
	}
	if err != nil {
		return err
	}

	if *quiet {
		fmt.Println(link.Addr)
	} else {
		fmt.Printf("\n  %s  %s\n  node %s, %d nodes running real MeshCore firmware\n\n"+
			"  Point your client at that. Ctrl-C to stop.\n\n",
			link.Kind, link.Addr, link.Node, e.FirmwareCount())
	}

	// The mesh has to keep running for the client to have anything to talk to,
	// so time advances until interrupted. Everything the client sends crosses a
	// simulated radio to real firmware on every other node.
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := e.Run(ctx, e.NowMs()+1000); err != nil {
			return err
		}
		if !*quiet && e.NowMs()%30000 == 0 {
			fmt.Printf("  %.0f s simulated, client %s\n", float64(e.NowMs())/1000,
				map[bool]string{true: "attached", false: "not attached yet"}[link.Attached()])
		}
		time.Sleep(time.Millisecond)
	}
}

// defaultFixture finds a shipped network without the caller naming one.
func defaultFixture() (string, error) {
	var roots []string
	if self, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Join(filepath.Dir(self), "fixtures"))
	}
	roots = append(roots, "fixtures")
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, ".cache", "meshcoresim", "fixtures"))
	}
	for _, r := range roots {
		p := filepath.Join(r, "fixture-fife-strict.json")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no shipped network found; pass one with -fixture")
}
