package firmware

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
