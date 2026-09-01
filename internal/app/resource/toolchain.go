// The emulator toolchain, fetched from our own forks at runtime.
//
// Three binaries stand between a source checkout and an emulated board:
// radioserver, which every emulated node needs whatever its MCU; QEMU for the
// ESP32 family; and Renode for the nRF52s. A release tarball carries them
// beside the binary and its users never meet this, but the AppImage and the
// .deb carry only radioserver, and a development checkout has had no path to
// any of them - the tools directory the lookup already searches was described
// as "where the installer puts anything it downloads after the fact", and for
// a source build no installer ever does.
//
// So they arrive the way everything else here arrives: fetched from where they
// are published, verified, and cached. The destination is the tools directory
// itself, which is already the third step of the lookup, so a fetched tool is
// found with nothing else configured.
package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ToolchainKind is its row group.
const ToolchainKind Kind = "toolchain"

// archiveKind is how an asset arrives.
type archiveKind string

const (
	plainFile archiveKind = ""
	tarGzip   archiveKind = "tar.gz"
)

// toolAsset is one published build, for one platform.
//
// SHA256 is not decoration. These are 40 KB to 61 MB over a link that may be
// slow or lossy, and a truncated emulator does not announce itself: it
// unpacks, it starts, and the board does not come up. The digest is checked
// before anything is written where the lookup would find it.
type toolAsset struct {
	URL    string
	SHA256 string
	Bytes  int64
	Kind   archiveKind
	// Binary is the path within the tools directory the archive puts the
	// executable at, relative to it. For a plain file that is the tool's own
	// name; for an archive it is whatever prefix the archive carries.
	Binary string
	// Root is the top-level directory the archive unpacks to, so a removal has
	// something to delete and an unpack has something to replace.
	Root string
	// Magic is what the executable must begin with once unpacked, so a build
	// for the wrong platform is refused here rather than by the kernel three
	// screens later.
	Magic execFormat
}

// toolRelease is one tool, its published builds, and why anybody wants it.
type toolRelease struct {
	// Name is what lookupTool asks for, so the fetched file has to land under
	// exactly this name. Anything else is a download that changes nothing.
	Name    string
	Version string
	Why     string
	Terms   string
	// MCU is the board family this tool is needed for, matched by prefix, or
	// empty where every emulated node needs it.
	MCU string
	// Assets, by "GOOS/GOARCH". A platform absent from the map has no build,
	// which the row says rather than offering a button that would fail.
	Assets map[string]toolAsset
	// Unsupported explains a platform we deliberately do not offer, as opposed
	// to one nobody has built for yet.
	Unsupported map[string]string
}

// ToolsFor names the tools a board with this MCU cannot boot without.
//
// Exported because the caller that knows how many nodes are waiting is the one
// that holds the scenario, and it should not have to know that every emulated
// node needs radioserver while only an nRF52 needs Renode. Matched on the MCU
// prefix, which is the board catalogue's own field: "nRF52840" and "ESP32-S3"
// are what a profile actually says.
func ToolsFor(mcu string) []string {
	var out []string
	for _, r := range toolReleases {
		if r.MCU == "" || strings.HasPrefix(mcu, r.MCU) {
			out = append(out, r.Name)
		}
	}
	return out
}

// Toolchain provides the emulator toolchain rows.
type Toolchain struct {
	// Dir is the tools directory: where the emulator lookup already searches,
	// and therefore the only place a fetch is worth putting anything.
	Dir string
	// HTTP is the client, so a test answers without a network.
	HTTP httpDoer
	// Needed counts the nodes in this scenario that cannot boot without each
	// tool, keyed by the tool's name, so a missing row reads as blocking
	// rather than optional.
	Needed map[string]int
}

func (t *Toolchain) Kind() Kind { return ToolchainKind }

// platform is the key an asset map is read by.
func platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// asset is the build for this machine, and whether there is one.
func (r toolRelease) asset() (toolAsset, bool) {
	a, ok := r.Assets[platform()]
	return a, ok
}

// installedAt is where a tool ends up, and dirs is everything a removal has to
// take with it: the unpacked tree as well as the name the lookup finds.
func (t *Toolchain) installedAt(r toolRelease) string {
	return filepath.Join(t.Dir, r.Name)
}

func (t *Toolchain) List(_ context.Context) ([]Row, error) {
	out := make([]Row, 0, len(toolReleases))
	for _, r := range toolReleases {
		out = append(out, t.row(r))
	}
	return out, nil
}

// row is one tool's state on this machine, which is three different answers:
// present, fetchable, or not a thing this platform can have.
func (t *Toolchain) row(r toolRelease) Row {
	row := Row{
		Kind: ToolchainKind, Name: r.Name, Version: r.Version,
		Why: r.Why, State: Available,
		// Never automatic. Two of these are 60 MB or more of somebody else's
		// licensed software, and both of those facts are ones a person should
		// meet before the download rather than after it.
		Auto: false,
		// The terms are readable before the bytes arrive, deliberately. A
		// licence you can only read once you have taken the copy is not a
		// choice you were offered.
		Licensed: true,
	}
	a, ok := r.asset()
	if !ok {
		row.State, row.Fetchable = Unavailable, false
		row.Why = r.unavailableBecause()
		return row
	}
	row.Fetchable, row.Bytes, row.Estimated = true, a.Bytes, true
	// Measured once it is here, because the packed size an asset advertises is
	// not what the disk gave up: Renode unpacks to several times its download.
	if n, err := treeBytes(t.installedAt(r), a, t.Dir); err == nil && n > 0 {
		row.State, row.Path, row.Bytes, row.Estimated = OnDisk, t.installedAt(r), n, false
		return row
	}
	if t.Needed[r.Name] > 0 {
		row.State = Needed
		row.Why = fmt.Sprintf("%d node(s) here cannot boot without it: %s",
			t.Needed[r.Name], r.Why)
	}
	return row
}

// unavailableBecause says which kind of absence this is. "No build published"
// and "the emulator path has never worked here" are different answers, and an
// operator can act on only one of them.
func (r toolRelease) unavailableBecause() string {
	if why, ok := r.Unsupported[platform()]; ok {
		return why
	}
	return fmt.Sprintf("no %s build is published for %s", r.Name, platform())
}

// treeBytes measures an installed tool, and reports nothing at all when the
// name the lookup would find is missing. An unpacked tree with no usable
// binary in it is not an installation, and a row calling it one sends somebody
// to debug an emulator that was never there.
func treeBytes(link string, a toolAsset, dir string) (int64, error) {
	if !fileExists(link) {
		return 0, os.ErrNotExist
	}
	root := dir
	if a.Root != "" {
		root = filepath.Join(dir, a.Root)
	}
	if a.Kind == plainFile {
		st, err := os.Stat(link)
		if err != nil {
			return 0, err
		}
		return st.Size(), nil
	}
	return dirBytes(root)
}

func (t *Toolchain) Remove(_ context.Context, row Row) error {
	for _, r := range toolReleases {
		if r.Name != row.Name {
			continue
		}
		if err := os.Remove(t.installedAt(r)); err != nil && !os.IsNotExist(err) {
			return err
		}
		if a, ok := r.asset(); ok && a.Root != "" {
			return os.RemoveAll(filepath.Join(t.Dir, a.Root))
		}
		return nil
	}
	return fmt.Errorf("resource: no emulator tool called %s", row.Name)
}

// Licence is the terms this tool arrives under, answerable before it is
// fetched. The full texts are in the application's own Licences window; this
// is what has to be read before pressing Fetch.
func (t *Toolchain) Licence(name, _ string) string {
	for _, r := range toolReleases {
		if r.Name == name {
			return r.Terms
		}
	}
	return ""
}
