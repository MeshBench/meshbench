// Where the emulator, the radio model and the sockets between them are found.
//
// Kept apart from the node itself because none of it is about a node: it is
// about this machine, and what is installed on it.
package firmware

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ToolsDir is where the emulator and the radio model are kept.
//
// The same shape as the native build cache, and for the same reason: a desktop
// application is not launched from a shell, so it does not inherit one's PATH.
// Requiring an environment variable meant emulation worked from a terminal and
// failed from the desktop, with an error that read as a missing package.
func ToolsDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "tools"
	}
	return filepath.Join(dir, "meshcoresim", "tools")
}

// EnvRenodeSupport names a directory holding Renode's platform descriptions
// and peripheral models, for anyone running from a source tree.
const EnvRenodeSupport = "MESHCORESIM_RENODE_SUPPORT"

// SupportDir is where Renode's .repl platform descriptions and .cs peripheral
// models are read from at run time.
//
// Beside the binary before the cache, because the cache is the one place
// nothing fills. The emulators arrive by download and unpack themselves; these
// files are ours, they ship in the bundle, and until now nothing copied them
// anywhere - which made every emulated nRF52 board depend on a directory a
// user's machine has no way to acquire.
func SupportDir() string {
	if p := os.Getenv(EnvRenodeSupport); p != "" {
		return p
	}
	if self, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(self), "renode-support")
		if dirExists(filepath.Join(beside, "peripherals")) {
			return beside
		}
	}
	return ToolsDir()
}

// CheckSupport refuses early, and says where it looked.
//
// Without these files Renode starts, fails to compile a peripheral, and boots
// a machine with holes in it - and the only sign is a compiler error buried in
// a console log nobody reads. The board then reports "untested" for a reason
// that has nothing to do with the board.
func CheckSupport(dir string, needed ...string) error {
	var missing []string
	for _, n := range needed {
		if !fileExists(filepath.Join(dir, n)) {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("firmware: %s is missing from %s (%d of %d support files). "+
		"They ship in the bundle as renode-support beside the binary; from a source "+
		"tree, point %s at tools/renode",
		strings.Join(missing, ", "), dir, len(missing), len(needed), EnvRenodeSupport)
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// lookupTool finds a binary: the environment variable, then beside the
// simulator, then the tools directory, then PATH.
//
// The message names all of them, because "qemu-system-xtensa not found" sends
// people to their package manager for a build that will not do: ours carries an
// SX1262 and a GPIO implementation upstream has not got.
func lookupTool(env, name string) (string, error) {
	if p := os.Getenv(env); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("firmware: %s points at %s, which is not there", env, p)
	}
	// The names a tool might be found under in a directory. Windows names its
	// executables, and a zip cannot carry the symlink the Linux tarball and
	// the macOS bundle use - so the emulator's own unpacked layout is
	// searched too, rather than requiring a link nobody could ship.
	candidates := []string{name}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, name+".exe")
	}
	subdirs := []string{"", "qemu/bin", "qemu-meshbench/bin"}
	if self, err := os.Executable(); err == nil {
		dir := filepath.Dir(self)
		// Renode unpacks into a directory carrying its version, so the name
		// changes with every release and cannot be listed above. Globbing for
		// the shape is what the Linux tarball's symlink step already does;
		// this is the same rule on the side that has to find it.
		if matches, err := filepath.Glob(filepath.Join(dir, "renode*-portable")); err == nil {
			for _, m := range matches {
				subdirs = append(subdirs, filepath.Base(m))
			}
		}
		for _, sub := range subdirs {
			for _, cand := range candidates {
				if p := filepath.Join(dir, sub, cand); fileExists(p) {
					return p, nil
				}
			}
		}
	}
	for _, cand := range candidates {
		if p := filepath.Join(ToolsDir(), cand); fileExists(p) {
			return p, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("firmware: %s not found - looked beside the simulator, "+
		"in %s, and on PATH. Put it in that directory or set %s. A distribution "+
		"build will not do: ours carries the SX1262 device",
		name, ToolsDir(), env)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// PadImage copies a flash image padded to a size QEMU will accept.
//
// Two traps in one function. QEMU takes only 2, 4, 8 or 16 MB images; and the
// size must match what the image header asks for, or ESP-IDF asserts in
// do_core_init with a message naming both sizes.
//
// Where the header lives is the part that differs by part. An ESP32 boots its
// bootloader from 0x1000, so a merged image starts with padding and reading a
// header from zero gives 0xff and a nonsense answer. An ESP32-S3 boots from
// zero, so the header is the first thing in the file. Looked for rather than
// assumed: the byte is 0xE9 either way, and a merged image for one part has
// erased flash where the other keeps its header.
func PadImage(src, dst string) (int, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return 0, err
	}
	if len(data) < 0x1004 {
		return 0, fmt.Errorf("firmware: %s is too small to be a merged image", src)
	}
	hdr := -1
	switch {
	case data[0] == 0xE9:
		hdr = 0 // ESP32-S3, and the other parts that boot from zero
	case data[0x1000] == 0xE9:
		hdr = 0x1000 // ESP32
	}
	if hdr < 0 {
		return 0, fmt.Errorf("firmware: %s has no image header at 0x0 or 0x1000; "+
			"it is probably an application-only build rather than a merged one", src)
	}
	sizes := map[byte]int{0: 1, 1: 2, 2: 4, 3: 8, 4: 16}
	mb, ok := sizes[data[hdr+3]>>4]
	if !ok {
		return 0, fmt.Errorf("firmware: %s declares an unknown flash size", src)
	}
	if mb == 1 {
		mb = 2 // QEMU's smallest
	}
	want := mb << 20
	if len(data) > want {
		return 0, fmt.Errorf("firmware: %s is larger than the %d MB its header declares",
			src, mb)
	}
	out := make([]byte, want)
	copy(out, data)
	for i := len(data); i < want; i++ {
		out[i] = 0xFF // erased flash
	}
	return mb, os.WriteFile(dst, out, 0o644)
}

// waitForSocket blocks until the radio model is listening, or the context ends.
//
// Polled rather than assumed: the device connects to this socket as QEMU
// realizes it, and a race there fails the whole boot with a message about the
// radio being unreachable, which points at configuration rather than at timing.
func waitForSocket(ctx context.Context, path string) error {
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return fmt.Errorf("firmware: the radio model never opened %s", path)
}
