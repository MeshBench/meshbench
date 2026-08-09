// Command meshcoresim-mcp exposes the engine to an AI client over MCP.
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

	"github.com/A13xB0/meshcoresim/internal/mcp"
	"github.com/A13xB0/meshcoresim/internal/terrain"
)

const version = "0.1.0"

func main() {
	defaultCache, _ := os.UserCacheDir()
	cacheDir := flag.String("terrain-cache", filepath.Join(defaultCache, "meshcoresim", "terrain"),
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

	s := mcp.NewServer("meshcoresim", version)
	if err := mcp.RegisterEngineTools(s, store); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Diagnostics go to stderr: stdout is the protocol, and one stray line on
	// it desynchronises the client for the rest of the session.
	fmt.Fprintf(os.Stderr, "meshcoresim-mcp %s, terrain cache %s (offline=%v)\n", version, *cacheDir, *offline)

	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
