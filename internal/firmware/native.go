package firmware

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// EnvNativeBinary overrides where the native node binary is found.
const EnvNativeBinary = "MESHCORESIM_NATIVE"

// NativeBinaryName is what the per-architecture release is called.
//
// The architecture is in the filename rather than in a directory because these
// are downloaded, not built here: MeshCore is MIT and our host build of it is
// distributed from a separate repository under that same licence (ADR-0020), so
// what lands on a user's machine is one file per platform out of a release.
func NativeBinaryName() string {
	name := fmt.Sprintf("meshcore-node-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// ErrNativeMissing means no native node binary could be found.
var ErrNativeMissing = errors.New("firmware: native node binary not found")

// FindNative locates the native node binary.
//
// Explicit path, then environment, then next to the simulator, then PATH. The
// simulator's own directory is checked before PATH because the binary is
// downloaded into it, and a stale copy on someone's PATH from a different
// MeshCore version is a very quiet way to get wrong answers.
func FindNative(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("%w at %s: %w", ErrNativeMissing, explicit, err)
		}
		return explicit, nil
	}
	if p := os.Getenv(EnvNativeBinary); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%w at %s (from %s): %w", ErrNativeMissing, p, EnvNativeBinary, err)
		}
		return p, nil
	}
	name := NativeBinaryName()
	if self, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%w: looked for %s beside the simulator and on PATH; set %s to override",
		ErrNativeMissing, name, EnvNativeBinary)
}

// Native runs MeshCore compiled for this host, as a child process.
type Native struct {
	// Path is the binary. Empty means resolve it with FindNative.
	Path string

	// Seed makes key generation and any firmware-side randomness reproducible.
	// Without it a native node picks a different identity every run and no two
	// runs of the same scenario are comparable.
	Seed uint64

	// SF, BandwidthKHz and CodingRate are the node's LoRa settings. Zero values
	// leave the binary's own defaults in place.
	SF           int
	BandwidthKHz float64
	CodingRate   int

	// Log receives the node's stderr. Nil discards it. The firmware's own
	// diagnostics are the only window into a native node, so a scenario that
	// misbehaves is much easier to explain with this attached.
	Log io.Writer

	mu  sync.Mutex
	cmd *exec.Cmd
}

func (n *Native) Kind() string { return "native" }

func (n *Native) Start(ctx context.Context, bridgeAddr string) error {
	path, err := FindNative(n.Path)
	if err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.cmd != nil {
		return errors.New("firmware: native node already started")
	}
	args := []string{"--bridge", bridgeAddr, "--seed", fmt.Sprint(n.Seed)}
	if n.SF != 0 {
		args = append(args, "--sf", fmt.Sprint(n.SF))
	}
	if n.BandwidthKHz != 0 {
		args = append(args, "--bw-khz", strconv.FormatFloat(n.BandwidthKHz, 'f', -1, 64))
	}
	if n.CodingRate != 0 {
		args = append(args, "--cr", fmt.Sprint(n.CodingRate))
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stderr = n.Log
	if cmd.Stderr == nil {
		cmd.Stderr = io.Discard
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("firmware: launch %s: %w", path, err)
	}
	n.cmd = cmd
	return nil
}

func (n *Native) Stop() error {
	n.mu.Lock()
	cmd := n.cmd
	n.cmd = nil
	n.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// The bridge has already been closed, so the node should be on its way out
	// under its own steam; give it long enough to write its closing line before
	// taking that away from it. The wait itself is not optional either — without
	// it the child stays a zombie, and a scenario that cycles nodes leaks one
	// per node.
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(gracePeriod):
		_ = cmd.Process.Kill()
		<-done
	}
	return nil
}

// Long enough for a node to notice a closed socket and flush, short enough that
// a hung node does not stall a scenario tearing down a hundred of them.
const gracePeriod = 2 * time.Second
