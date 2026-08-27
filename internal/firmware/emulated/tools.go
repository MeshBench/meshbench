// Where the emulator, the radio model and the sockets between them are found.
//
// Kept apart from the node itself because none of it is about a node: it is
// about this machine, and what is installed on it.
package emulated

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	return filepath.Join(dir, "meshbench", "tools")
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

// PadImageKeeping is PadImage, except that a flash the node has already been
// running is left exactly as it is.
//
// A board keeps what it was told between runs, and this is where that had
// stopped being true. The flash was rewritten from the pristine image on every
// start, so an emulated node's NVS and its filesystem were blanked each time -
// its identity, its preferences, its contacts, its region. Two places in the
// tree describe the opposite behaviour and are right about native nodes: a
// repeater keeping its identity between sessions is how hardware behaves, and
// firmware.wipe exists precisely because it does.
//
// What decides is the source, recorded beside the flash. A node pinned to a
// different build gets a fresh chip, because that is what reflashing a board
// is; a node started again on the same build gets the chip it left behind.
// Same reasoning as the card image, which has always been kept.
func PadImageKeeping(src, dst string) (int, error) {
	stamp := dst + ".src"
	want, err := imageStamp(src)
	if err != nil {
		return 0, err
	}
	if have, err := os.ReadFile(stamp); err == nil && string(have) == want {
		if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
			return int(st.Size() >> 20), nil
		}
	}
	mb, err := PadImage(src, dst)
	if err != nil {
		return 0, err
	}
	// Written after the flash, so an interrupted write leaves a stamp that
	// does not match and the next run rebuilds rather than booting half a
	// chip. Best effort: a stamp that cannot be written costs a re-flash on
	// the next start, which is what happened every time before this.
	_ = os.WriteFile(stamp, []byte(want), 0o644)
	return mb, nil
}

// imageStamp identifies the build a flash was made from.
//
// The digest rather than the path or the modification time: a build imported
// twice under two labels is two paths and one chip, and an image rebuilt in
// place by a compiler keeps both its path and, often enough, its size.
func imageStamp(src string) (string, error) {
	// The image this node was told to run, from the cache or from an import.
	f, err := os.Open(src) //nolint:gosec // the build the caller asked for
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// PadImage copies a flash image padded to a size QEMU will accept, always
// rewriting the destination.
//
// Two traps in one function. QEMU takes only 2, 4, 8 or 16 MB images; and the
// size must match what the image header asks for, or ESP-IDF asserts in
// do_core_init with a message naming both sizes.
//
// Callers starting a node want PadImageKeeping instead: this one blanks
// whatever the board had written to its flash.
func PadImage(src, dst string) (int, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return 0, err
	}
	mb, err := ClassifyESPImage(data)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", src, err)
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

// ClassifyESPImage reads an ESP32 flash image's header and answers the flash
// size in megabytes it was built for, or says why it is not one.
//
// Split out of PadImage so the same question can be asked at import, where it
// can still be answered by refusing. Asked only at play, an application-only
// build imported cleanly, listed cleanly, could be pinned to a node - and then
// failed minutes later, in a message about a flash image, to somebody who
// thought they were starting a board.
func ClassifyESPImage(data []byte) (flashMB int, err error) {
	if len(data) < 0x1004 {
		return 0, fmt.Errorf("firmware: too small to be a merged image")
	}
	// Where the header lives differs by part, so it is looked for rather than
	// assumed: an ESP32 boots its bootloader from 0x1000 and a merged image
	// for one starts with padding, while an ESP32-S3 boots from zero. The byte
	// is 0xE9 either way.
	hdr := -1
	switch {
	case data[0] == 0xE9:
		hdr = 0 // ESP32-S3, and the other parts that boot from zero
	case data[0x1000] == 0xE9:
		hdr = 0x1000 // ESP32
	}
	if hdr < 0 {
		return 0, fmt.Errorf("firmware: no image header at 0x0 or 0x1000; " +
			"it is probably an application-only build rather than a merged one")
	}
	sizes := map[byte]int{0: 1, 1: 2, 2: 4, 3: 8, 4: 16}
	mb, ok := sizes[data[hdr+3]>>4]
	if !ok {
		return 0, fmt.Errorf("firmware: the image header declares an unknown flash size")
	}
	if mb == 1 {
		mb = 2 // QEMU's smallest
	}
	return mb, nil
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

// cardBytes is how big a card a node gets. Small as cards go, because nothing
// here fills one and a file per node is a file per node.
const cardBytes = 64 << 20

// MakeCard is the file behind a board's card slot, made once and kept.
//
// Made rather than downloaded, and kept rather than remade, because what is on
// it is what the node wrote last time: a card an operator can look inside
// afterwards is most of the reason for having one at all. Sparse, so an empty
// card costs nothing on disk.
func MakeCard(path string) error {
	if st, err := os.Stat(path); err == nil && st.Size() == cardBytes {
		return nil
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Truncate(cardBytes)
}
