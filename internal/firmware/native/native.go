// The native firmware backend: MeshCore compiled for this host, run as a child
// process. The peer of the emulated backend - same Backend interface, a real
// binary instead of an emulated MCU.
package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// Native runs MeshCore compiled for this host, as a child process.
type Native struct {
	// Path is the binary. Empty means resolve it with firmware.FindNative.
	Path string

	// Role is the MeshCore application this node runs — a directory name under
	// MeshCore's examples/. Empty means firmware.DefaultRole.
	//
	// This is the only thing that decides what kind of node this is. There is no
	// enum of node types here on purpose: when upstream ships something new, it
	// is a string that already works rather than a case to add.
	Role string

	// Seed makes key generation and any firmware-side randomness reproducible.
	// Without it a native node picks a different identity every run and no two
	// runs of the same scenario are comparable.
	Seed uint64

	// SF, BandwidthKHz and CodingRate are the node's LoRa settings. Zero values
	// leave the binary's own defaults in place.
	SF           int
	BandwidthKHz float64
	CodingRate   int

	// WorkDir is the node's filesystem: the directory its identity, prefs and
	// ACL land in. Every node needs its own. The repeater application persists
	// its identity to "flash" on first boot, and three hundred processes
	// sharing a working directory all loaded the first one's key — a mesh of
	// clones, in which every node drops every packet as its own echo and
	// nothing is ever relayed.
	WorkDir string

	// Log receives the node's stderr. Nil discards it. The firmware's own
	// diagnostics are the only window into a native node, so a scenario that
	// misbehaves is much easier to explain with this attached.
	Log io.Writer

	mu  sync.Mutex
	cmd *exec.Cmd
	// exited closes once cmd.Wait has returned, whichever of Stop or a crash
	// caused it - the one signal both need, so Wait is only ever called
	// once. cmd.Wait called a second time is undefined.
	exited chan struct{}
}

func (n *Native) Kind() string { return "native" }

// The shim built into the native firmware implements the console frames.
func (n *Native) HasConsole() bool { return true }

// Native nodes answer console frames on the bridge, so there is no separate
// port to hand back.
func (n *Native) ConsoleIn() io.Writer { return nil }

// PID is the operating system process, or zero if it is not running.
//
// Exposed so an interface can say what a node costs. With 154 of these on one
// machine, "which node is using the memory" is a question somebody asks well
// before they ask anything about radio - from a panel on the frame thread,
// while the engine is starting and stopping nodes on goroutines of its own.
// Under the lock for that reason: both writers hold it, and a read that did
// not was a race the detector reports and a torn read reports as a process
// belonging to a node that has none.
func (n *Native) PID() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.cmd == nil || n.cmd.Process == nil {
		return 0
	}
	return n.cmd.Process.Pid
}

func (n *Native) Start(ctx context.Context, bridgeAddr string) error {
	path, err := firmware.FindNative(n.Path, n.Role)
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
	if n.WorkDir != "" {
		if err := os.MkdirAll(n.WorkDir, 0o755); err != nil {
			return fmt.Errorf("firmware: node filesystem: %w", err)
		}
		cmd.Dir = n.WorkDir
	}
	// The kernel kills the child if this process dies — any way it dies. The
	// graceful path still closes the bridge and waits; this is for the paths
	// that never reach it, which left three hundred orphans running after a
	// killed workbench.
	cmd.SysProcAttr = firmware.ChildProcAttr()
	cmd.Stderr = n.Log
	if cmd.Stderr == nil {
		cmd.Stderr = io.Discard
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("firmware: launch %s: %w", path, err)
	}
	n.cmd = cmd
	// Waited on from here, not from Stop - a crash needs reaping exactly as
	// much as a graceful exit does, and does not wait for anyone to ask.
	// Left to Stop alone, a process that died on its own during a run stayed
	// a zombie until the engine closed, which on a scenario nobody stops for
	// a while is indistinguishable from a leak - and cmd.Wait must only be
	// called once per process, so this is the only place that ever does.
	exited := make(chan struct{})
	n.exited = exited
	go func() { _ = cmd.Wait(); close(exited) }()
	return nil
}

func (n *Native) Stop() error {
	n.mu.Lock()
	cmd, exited := n.cmd, n.exited
	n.cmd = nil
	n.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// The bridge has already been closed, so the node should be on its way out
	// under its own steam; give it long enough to write its closing line before
	// taking that away from it.
	select {
	case <-exited:
		return nil
	case <-time.After(gracePeriod):
	}
	_ = cmd.Process.Kill()
	// Bounded, like the grace period before it.
	//
	// Killing a process is not the end of waiting for one. cmd.Wait waits for
	// the goroutine os/exec uses to copy the node's output as well as for the
	// process, and that writer is the caller's: the engine hands every native
	// node a file on whatever filesystem the operator keeps their cache on, and
	// a stalled mount is a write that never returns. So the wait after the kill
	// outlived the process it was waiting for, with no deadline at all, and
	// took the whole teardown with it.
	//
	// Reported rather than absorbed. A node this backend can no longer account
	// for is somebody's next question, and returning nil for it says the
	// opposite.
	select {
	case <-exited:
		return nil
	case <-time.After(reapPeriod):
		return fmt.Errorf("firmware: native node %d has not been reaped %v after being killed; "+
			"either the process or the writer taking its output is stuck", cmd.Process.Pid, reapPeriod)
	}
}

// Long enough for a node to notice a closed socket and flush, short enough that
// a hung node does not stall a scenario tearing down a hundred of them.
const gracePeriod = 2 * time.Second

// reapPeriod is how long the kill is given to take effect. It exists only so
// that a teardown ends, and a node that is behaving never reaches it.
const reapPeriod = 5 * time.Second
