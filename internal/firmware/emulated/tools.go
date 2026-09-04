// Where the emulator, the chip model and Renode's support files are found.
//
// Kept apart from the node itself because none of it is about a node: it is
// about this machine, and what is installed on it.
package emulated

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/MeshBench/meshbench/internal/firmware/emulated/renode"
)

// ToolsDir is where the emulator and the chip model are kept.
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
func lookupTool(name string) (string, error) {
	tool, ok := emulatorTools[name]
	if !ok {
		return "", fmt.Errorf("firmware: no emulator tool called %s", name)
	}
	env := tool.env
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
	// A tool with a file name of its own is looked for under that first, and
	// under its own name after: the fetcher finishes by linking what it
	// unpacked to the tool's name, so both spellings are real on a machine
	// that downloaded it.
	candidates = append(tool.files, candidates...)
	if self, err := os.Executable(); err == nil {
		if p, ok := findIn(filepath.Dir(self), candidates); ok {
			return p, nil
		}
	}
	// The tools directory is searched the same way, subdirectories included.
	// It used to be searched flat, which is why the fetcher had to leave a
	// symlink pointing into the tree it unpacked - and a symlink is a
	// privileged operation on Windows, granted only to an elevated process or
	// a machine in developer mode. Looking in the same places on both sides
	// removes the need for one.
	if p, ok := findIn(ToolsDir(), candidates); ok {
		return p, nil
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", tool.missing(name)
}

// SupportDir is where Renode's platform descriptions and our C# peripherals
// are, which is not the same question as where a binary is.
//
// Looked for the way a tool is: an override, then beside the simulator, then
// the tools directory. "renode-support" and not "renode", because the emulator
// binary is called that and a directory sharing its name is a trap for whoever
// reads the bundle next. A release bundle carries them beside the binary, because
// they are ours and they are text - four kilobytes of .repl and the peripherals
// Renode has not got - and a bundle that shipped both emulators and neither of
// these could start an ESP32 board and not an nRF52 one, which reads as a
// broken emulator rather than as a missing file.
func SupportDir() string {
	if p := os.Getenv(EnvRenodeSupport); p != "" {
		return p
	}
	// It counts only when it actually holds the files: an empty directory of
	// that name beside the binary would otherwise win over a populated tools
	// directory and take every nRF52 board down with it.
	if self, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(self), "renode-support")
		if fileExists(filepath.Join(beside, "peripherals", "VirtualSX1262.cs")) {
			return beside
		}
	}
	return ToolsDir()
}

// EnvRenodeSupport points at that directory, for a checkout driving a build
// that is not beside its own tree.
const EnvRenodeSupport = "MESHBENCH_RENODE_SUPPORT"

// emulatorTool is what somebody has to be told when one of these is not on the
// machine, and it differs per tool.
//
// One message for all three said "ours carries the SX1262 device", which is
// QEMU's reason. Renode's is the SEVONPEND fix, and the chip model has no
// distribution build to be mistaken for at all, so a person missing Renode was
// sent to think about a device that had nothing to do with their problem.
type emulatorTool struct {
	env string
	// need is which boards stop without it, said as a clause: a machine with
	// only ESP32 nodes on it does not care that Renode is absent.
	need string
	// ours is why a build from anywhere else will not do, empty where there is
	// no other build to confuse this one with.
	ours string
	// files are the names to look for on disk, where they are not the tool's
	// own name. A shared library is named by its platform rather than by what
	// it is, so the chip model is libvirtualsx1262.so, .dylib or .dll and the
	// name here has to stay the one a person and the catalogue both use.
	files []string
}

// missing is the error a boot fails with, and the only place that says how to
// get out of it.
//
// It names the fetch first because that is now the answer for almost everybody:
// these three are resource.fetch-able into the tools directory the lookup
// already searches, and the message predated that by long enough to still be
// sending people out to find the file themselves. The environment variable and
// the directory stay at the end, because somebody who built their own still
// needs them and nothing else tells them where it goes.
func (t emulatorTool) missing(name string) error {
	msg := fmt.Sprintf("firmware: %s is not on this machine, and %s. "+
		"Fetch it in the workbench under Help > Setup, or with resource.fetch "+
		"over the control socket: it lands in %s, which is where a node looks.",
		name, t.need, ToolsDir())
	if t.ours != "" {
		msg += " " + t.ours + "."
	}
	return fmt.Errorf("%s A build of your own goes in that directory instead, "+
		"or %s points at it", msg, t.env)
}

// findIn looks for any of these names in a directory and in the layouts the
// emulators actually unpack into.
//
// A zip cannot carry the symlink the Linux tarball and the macOS bundle use,
// and Renode's directory carries its own version, so neither can be assumed nor
// listed: the shape is globbed for instead.
func findIn(dir string, candidates []string) (string, bool) {
	subdirs := []string{"", "qemu/bin", "qemu-meshbench/bin"}
	if matches, err := filepath.Glob(filepath.Join(dir, "renode*-portable")); err == nil {
		for _, m := range matches {
			subdirs = append(subdirs, filepath.Base(m))
		}
	}
	for _, sub := range subdirs {
		for _, cand := range candidates {
			if p := filepath.Join(dir, sub, cand); fileExists(p) {
				return p, true
			}
		}
	}
	return "", false
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// radioLibName is what the chip model is called on this platform, and the name
// under which a released bundle unpacks it.
const radioLibName = "virtual-sx1262"

// radioLibFiles is what that library is actually called on disk here. A shared
// library is named by its platform rather than by what it is.
var radioLibFiles = func() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"libvirtualsx1262.dll"}
	case "darwin":
		return []string{"libvirtualsx1262.dylib"}
	default:
		return []string{"libvirtualsx1262.so"}
	}
}()

// emulatorTools is the three things an emulated node needs on this machine, and
// what a person is owed when one of them is absent.
var emulatorTools = map[string]emulatorTool{
	radioLibName: {
		env:   EnvRadioLib,
		files: radioLibFiles,
		need:  "no emulated board can start without it, ESP32 or nRF52",
		ours: "There is no distribution build to go looking for: the chip is " +
			"MeshBench/virtual-sx1262, which the emulator loads",
	},
	"qemu-system-xtensa": {
		env:  EnvQEMU,
		need: "the ESP32 boards cannot start without it",
		ours: "A distribution QEMU will not do: ours carries the SX1262 device " +
			"and the GPIO implementation upstream has not got",
	},
	"renode": {
		env:  renode.EnvRenode,
		need: "the nRF52 boards cannot start without it",
		ours: "A stock Renode will not do: it starts, loads and runs as far as " +
			"the sleep the SEVONPEND fix exists for, and stops there",
	},
}

// ToolEnv names the three binaries an emulated node needs and the environment
// variable each can be pointed at.
//
// Exported because the tools are asked about from outside as well as started
// from inside: a release tarball carries them beside the binary, a fetch puts
// them in the tools directory, and only this package knows that both count.
// Derived from the table the errors are written from, so a tool cannot be
// listed in one and missing from the other.
var ToolEnv = func() map[string]string {
	m := make(map[string]string, len(emulatorTools))
	for name, t := range emulatorTools {
		m[name] = t.env
	}
	return m
}()

// FindTool answers where a node starting now would find a tool, or the error it
// would fail with.
//
// The same lookup a boot does, deliberately. Measuring the tools directory
// alone - which is all a cache listing can honestly do - reports a tarball
// install as having nothing, because its emulators sit beside the binary and
// were never downloaded. A readiness check that disagrees with the thing it is
// predicting is worse than no check.
func FindTool(name string) (string, error) { return lookupTool(name) }

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
