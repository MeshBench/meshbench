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
	"strings"
	"sync"
	"time"
)

// EnvNativeBinary overrides where the native node binary is found.
const EnvNativeBinary = "MESHBENCH_NATIVE"

// DefaultRole is the application a node runs when nothing says otherwise.
const DefaultRole = "simple_repeater"

// NativeBinaryName is what the published build of one role is called.
//
// Role, then platform. Both are in the filename rather than in a directory
// because these are downloaded, not built here: MeshCore is MIT and our host
// builds of it are published from a separate repository under that same licence
// (ADR-0020), one file per role per platform out of a release.
//
// The role is a MeshCore example directory name, passed through rather than
// mapped: a node is a node, and what makes one a repeater instead of a companion
// is only which application was linked. Anything upstream ships is a legal value
// here, including one that did not exist when this was written.
func NativeBinaryName(role string) string {
	if role == "" {
		role = DefaultRole
	}
	name := fmt.Sprintf("meshcore-%s-%s-%s", role, runtime.GOOS, runtime.GOARCH)
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
func FindNative(explicit, role string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("%w at %s: %w", ErrNativeMissing, explicit, err)
		}
		return explicit, nil
	}
	if p := os.Getenv(EnvNativeBinary); p != "" {
		st, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("%w at %s (from %s): %w", ErrNativeMissing, p, EnvNativeBinary, err)
		}
		// A directory holds one build per role, which is what a scenario mixing
		// roles needs. Pointing at a single binary overrides every node
		// regardless of role, so a mesh of repeaters and room servers quietly
		// became a mesh of whichever one was named.
		if st.IsDir() {
			q := filepath.Join(p, NativeBinaryName(role))
			if _, err := os.Stat(q); err != nil {
				return "", fmt.Errorf("%w: %s is a directory but holds no %s (from %s)",
					ErrNativeMissing, p, NativeBinaryName(role), EnvNativeBinary)
			}
			return q, nil
		}
		return p, nil
	}
	name := NativeBinaryName(role)
	if self, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%w: looked for %s beside the simulator and on PATH. "+
		"Download one with `msim firmware get %s`, or set %s to a build of your own",
		ErrNativeMissing, name, role, EnvNativeBinary)
}

// NodeWorkDir is where a named node's persistent files live. Stable across
// runs on purpose: a repeater keeping its identity between sessions is how
// hardware behaves, and the seed regenerates the same identity anyway.
func NodeWorkDir(name string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, name)
	return filepath.Join(NodeFSRoot(), safe)
}

// EnvNodeFS moves every node's persistent storage somewhere else.
//
// A node keeps its identity and its preferences between runs, on purpose:
// that is how hardware behaves. It is also a quiet trap when comparing two
// firmware builds, because **saved preferences beat a compiled default**. A
// node that has run before loads its old value and never reaches the changed
// one, so both arms of an A/B return identical numbers and the change looks
// like it did nothing - silently, and in both arms, which is the worst way for
// a comparison to fail.
//
// Pointing this at a directory per arm gives each one nodes that have never
// run. It is equally useful to a person who wants an experiment not to inherit
// last week's state.
const EnvNodeFS = "MESHBENCH_NODEFS"

// NodeFSRoot is where per-node storage lives.
func NodeFSRoot() string {
	if p := os.Getenv(EnvNodeFS); p != "" {
		return p
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "nodefs"
	}
	return filepath.Join(base, "meshbench", "nodefs")
}

// WipeNodeStorage deletes every node's persistent files — identities, prefs,
// regions, ACLs. The next attach boots every node factory-fresh.
//
// This is the fix for a poisoned flash: preference and region files written
// during a bad run (most memorably, three hundred processes sharing one
// working directory) gate real behaviour — a repeater whose regions file says
// deny-flood silently repeats nothing, which looks exactly like an RF problem.
// Identities regenerate deterministically from the run seed, so a wipe costs
// nothing but the next boot.
func WipeNodeStorage() error {
	// Through NodeFSRoot, so a run with MESHBENCH_NODEFS set wipes the
	// storage it is actually using. Wiping a different directory from the one
	// in use is worse than not wiping at all: it reports success and changes
	// nothing.
	return os.RemoveAll(NodeFSRoot())
}

// Native runs MeshCore compiled for this host, as a child process.
type Native struct {
	// Path is the binary. Empty means resolve it with FindNative.
	Path string

	// Role is the MeshCore application this node runs — a directory name under
	// MeshCore's examples/. Empty means DefaultRole.
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
// before they ask anything about radio.
func (n *Native) PID() int {
	if n.cmd == nil || n.cmd.Process == nil {
		return 0
	}
	return n.cmd.Process.Pid
}

func (n *Native) Start(ctx context.Context, bridgeAddr string) error {
	path, err := FindNative(n.Path, n.Role)
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
	cmd.SysProcAttr = ChildProcAttr()
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
	case <-time.After(gracePeriod):
		_ = cmd.Process.Kill()
		<-exited
	}
	return nil
}

// Long enough for a node to notice a closed socket and flush, short enough that
// a hung node does not stall a scenario tearing down a hundred of them.
const gracePeriod = 2 * time.Second
