package update_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/update"
)

// Which bundle this is decides what an update can honestly do, so it is
// deduced from evidence rather than assumed, and every branch of that
// deduction is something a real install looks like.
func TestTheBundleIsDeducedFromWhereTheBinaryIs(t *testing.T) {
	cases := []struct {
		name, goos, exe, appImage string
		want                      update.Artefact
	}{
		{"an AppImage names itself in the environment", "linux",
			"/tmp/.mount_xyz/usr/bin/meshbench",
			"/home/someone/Downloads/meshbench-0.2.0-x86_64.AppImage", update.AppImage},
		{"a macOS bundle is a path shape", "darwin",
			"/Applications/MeshBench.app/Contents/MacOS/meshbench", "", update.Bundle},
		{"an unpacked zip on Windows has no installer's note beside it", "windows",
			`C:\Users\someone\meshbench\meshbench.exe`, "", update.Zip},
		{"a package manager put it under /usr", "linux",
			"/usr/bin/meshbench", "", update.Deb},
		{"anything else is a build somebody made or unpacked", "linux",
			"/home/someone/src/meshbench/meshbench", "", update.Loose},
	}
	for _, c := range cases {
		if got := update.Detect(c.goos, c.exe, c.appImage); got != c.want {
			t.Errorf("%s: Detect = %q, want %q", c.name, got, c.want)
		}
	}
}

// The two Windows bundles are the same files in the same layout once they are
// unpacked, and they are updated differently, so the one thing that tells them
// apart is the note the installer lays down. It has to be looked for on disk
// beside the binary rather than assumed from a path, because the installer
// takes a location from whoever runs it.
func TestAnInstalledWindowsBuildIsToldFromAnUnpackedOne(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "meshbench.exe")
	if got := update.Detect("windows", exe, ""); got != update.Zip {
		t.Errorf("with no note beside it, Detect = %q, want %q", got, update.Zip)
	}
	note := filepath.Join(dir, "installed-by-msi.txt")
	if err := os.WriteFile(note, []byte("put here by the installer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := update.Detect("windows", exe, ""); got != update.Msi {
		t.Errorf("with the installer's note beside it, Detect = %q, want %q", got, update.Msi)
	}
}

// released is the asset list a real release publishes, names and all.
var released = update.Release{
	Tag: "v0.2.0",
	Assets: []update.Asset{
		{Name: "meshbench-linux-x86_64.tar.gz", Bytes: 44_000_000},
		{Name: "meshbench-0.2.0-x86_64.AppImage", Bytes: 26_000_000},
		{Name: "meshbench_0.2.0_amd64.deb", Bytes: 26_000_000},
		{Name: "MeshBench-0.2.0-arm64.dmg", Bytes: 30_000_000},
		{Name: "meshbench-0.2.0-windows-x86_64.zip", Bytes: 40_000_000},
		{Name: "meshbench-0.2.0-windows-x86_64.msi", Bytes: 40_000_000},
		{Name: "meshbench-0.2.0-source.tar.gz", Bytes: 3_000_000},
		{Name: update.SumsAsset, Bytes: 400},
	},
}

func TestEachBundleTakesItsOwnArtefact(t *testing.T) {
	cases := []struct {
		art          update.Artefact
		goos, goarch string
		want         string
	}{
		{update.Tarball, "linux", "amd64", "meshbench-linux-x86_64.tar.gz"},
		{update.Loose, "linux", "amd64", "meshbench-linux-x86_64.tar.gz"},
		{update.AppImage, "linux", "amd64", "meshbench-0.2.0-x86_64.AppImage"},
		{update.Bundle, "darwin", "arm64", "MeshBench-0.2.0-arm64.dmg"},
		{update.Zip, "windows", "amd64", "meshbench-0.2.0-windows-x86_64.zip"},
		{update.Msi, "windows", "amd64", "meshbench-0.2.0-windows-x86_64.msi"},
	}
	for _, c := range cases {
		got, why := update.AssetFor(released, c.art, c.goos, c.goarch)
		if why != "" {
			t.Errorf("%s on %s/%s: refused with %q", c.art, c.goos, c.goarch, why)
			continue
		}
		if got.Name != c.want {
			t.Errorf("%s on %s/%s took %q, want %q",
				c.art, c.goos, c.goarch, got.Name, c.want)
		}
	}
}

// The source archive is also a .tar.gz and is the one thing on the page that is
// not a build. Taking it would leave somebody unpacking a source tree over
// their installation.
func TestTheSourceArchiveIsNeverTakenAsABuild(t *testing.T) {
	only := update.Release{Tag: "v0.2.0", Assets: []update.Asset{
		{Name: "meshbench-0.2.0-source.tar.gz"}}}
	if _, why := update.AssetFor(only, update.Tarball, "linux", "amd64"); why == "" {
		t.Error("the source archive was offered as a Linux build")
	}
}

// A package manager's copy is not ours to replace, and the refusal says so
// rather than offering a download that would fight apt.
func TestAPackagedBuildIsHandedBackToItsPackageManager(t *testing.T) {
	_, why := update.AssetFor(released, update.Deb, "linux", "amd64")
	if !strings.Contains(why, "package manager") {
		t.Errorf("the .deb refusal is %q, want it to name the package manager", why)
	}
}

// A platform nothing is published for gets a sentence, not a button that would
// fail.
func TestAPlatformWithNoBuildSaysSoRatherThanOfferingOne(t *testing.T) {
	if _, why := update.AssetFor(released, update.Loose, "linux", "riscv64"); why == "" {
		t.Error("riscv64 was offered a download that does not exist")
	}
	if _, why := update.AssetFor(released, update.Bundle, "darwin", "amd64"); why == "" {
		t.Error("an Intel Mac was offered the arm64 disk image")
	}
}

// Every bundle's instruction is different, and each has to name the thing that
// actually trips people on that platform.
func TestTheInstructionIsTheOneForThisPlatform(t *testing.T) {
	cases := []struct {
		art  update.Artefact
		want string
	}{
		{update.Deb, "apt"},
		{update.Zip, "cannot be replaced while it is running"},
		{update.Msi, "replaces this installation in place"},
		{update.Bundle, "quarantined"},
		{update.AppImage, "rename over a running binary"},
		{update.Tarball, "emulators"},
	}
	for _, c := range cases {
		got := update.Swap(c.art, "/cache/updates/v0.2.0/file", "/opt/meshbench/meshbench")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s says %q, want it to mention %q", c.art, got, c.want)
		}
	}
}

// What can be swapped in place and what cannot is a fact about the platform,
// and the interface has to be able to say which - a "restart to finish" that
// silently did nothing would be worse than the instruction.
func TestWindowsAndThePackageManagerCannotBeSwappedInPlace(t *testing.T) {
	// The installed Windows build is in this list too: the .msi can replace
	// it, and MeshBench still cannot replace its own running binary, which is
	// the question this answers.
	for _, a := range []update.Artefact{update.Zip, update.Msi, update.Deb} {
		if update.CanSwapItself(a) {
			t.Errorf("%s claims a binary swap it cannot do", a)
		}
	}
	for _, a := range []update.Artefact{update.AppImage, update.Tarball, update.Bundle} {
		if !update.CanSwapItself(a) {
			t.Errorf("%s could be swapped by a rename and says it cannot", a)
		}
	}
}
