// Command meshbench-mcp exposes the engine to an AI client over MCP.
//
// Spawned by the client over stdio. Nothing listens on a port and there is
// nothing to deploy: MeshcoreSim is an application, not a service.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/MeshBench/meshbench/internal/app/mcp"
	"github.com/MeshBench/meshbench/internal/app/version"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

func main() {
	defaultCache, _ := os.UserCacheDir()
	cacheDir := flag.String("terrain-cache", filepath.Join(defaultCache, "meshbench", "terrain"),
		"where downloaded elevation tiles live")
	offline := flag.Bool("offline", false,
		"never download terrain; answer only from the cache and fail loudly otherwise")
	flag.Parse()

	store, err := terrain.NewTileStore(*cacheDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The operator decides whether tiles may be downloaded, not the assistant.
	store.Offline = *offline

	s := mcp.NewServer("meshbench", version.String())
	if err := mcp.RegisterEngineTools(s, store); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The live-session tools, which drive a running workbench rather than this
	// process's own engine. Registered whether or not one is up: the socket
	// comes and goes with the window, and a tool list that changed underneath a
	// client mid-conversation is worse than a tool that says nothing is running.
	if err := mcp.RegisterSessionTools(s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Diagnostics go to stderr: stdout is the protocol, and one stray line on
	// it desynchronises the client for the rest of the session.
	fmt.Fprintf(os.Stderr, "meshbench-mcp %s, terrain cache %s (offline=%v)\n", version.String(), *cacheDir, *offline)

	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
