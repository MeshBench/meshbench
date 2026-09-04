package resource

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Every published asset says enough about itself to be checked.
//
// The catalogue is the whole safety of this: a URL with no digest is a
// download nothing can refuse, and an archive with no declared binary unpacks
// to a tree the lookup cannot find its way into. Both are silent faults, so
// they are asserted rather than reviewed.
func TestEveryToolAssetCanBeVerified(t *testing.T) {
	for _, r := range toolReleases {
		if r.Name == "" || r.Version == "" || r.Why == "" || r.Terms == "" {
			t.Errorf("%q is missing a name, version, purpose or terms", r.Name)
		}
		for plat, a := range r.Assets {
			if !strings.HasPrefix(a.URL, "https://") {
				t.Errorf("%s/%s: %q is not an https URL", r.Name, plat, a.URL)
			}
			if len(a.SHA256) != 64 {
				t.Errorf("%s/%s has no sha256 to check the download against", r.Name, plat)
			}
			if a.Bytes <= 0 {
				t.Errorf("%s/%s has no size, so its row cannot say what it costs",
					r.Name, plat)
			}
			if a.Magic == "" {
				t.Errorf("%s/%s says nothing about what it must be, so a build for "+
					"the wrong architecture would install cleanly", r.Name, plat)
			}
			if a.Kind != plainFile && (a.Root == "" || a.Binary == "") {
				t.Errorf("%s/%s is an archive with no root or no binary in it",
					r.Name, plat)
			}
			if _, ok := r.Unsupported[plat]; ok {
				t.Errorf("%s/%s is both published and declared unsupported", r.Name, plat)
			}
		}
	}
}

// A tool with no build here says so, and says which kind of absence it is.
func TestAnUnavailableToolExplainsItself(t *testing.T) {
	tc := &Toolchain{Dir: t.TempDir()}
	rows, err := tc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(toolReleases) {
		t.Fatalf("listed %d rows, want %d", len(rows), len(toolReleases))
	}
	for _, r := range rows {
		if r.State == Unavailable {
			if r.Fetchable {
				t.Errorf("%s is unavailable and still offers a fetch", r.Name)
			}
			if r.Why == "" {
				t.Errorf("%s is unavailable and does not say why", r.Name)
			}
			continue
		}
		if !r.Fetchable {
			t.Errorf("%s is available on %s/%s and offers no fetch",
				r.Name, runtime.GOOS, runtime.GOARCH)
		}
	}
}

// The terms are readable before the download, which is the whole point of
// asking about a licence at all.
func TestTermsAreReadableBeforeAnythingIsFetched(t *testing.T) {
	tc := &Toolchain{Dir: t.TempDir()}
	for _, r := range toolReleases {
		if got := tc.Licence(r.Name, r.Version); got == "" {
			t.Errorf("%s has no terms to read before it is fetched", r.Name)
		}
	}
	if tc.Licence("something-else", "") != "" {
		t.Error("terms were produced for a tool that does not exist")
	}
}

// Which board needs which tool. Every emulated node needs the chip whatever
// its MCU; only an ESP32 needs QEMU and only an nRF52 needs Renode, and a row
// that claims otherwise sends somebody to fetch 61 MB they will never run.
func TestToolsForMatchesTheMCU(t *testing.T) {
	for _, c := range []struct {
		mcu  string
		want []string
	}{
		{"ESP32-S3", []string{"virtual-sx1262", "qemu-system-xtensa"}},
		{"ESP32", []string{"virtual-sx1262", "qemu-system-xtensa"}},
		{"nRF52840", []string{"virtual-sx1262", "renode"}},
		{"RP2040", []string{"virtual-sx1262"}},
	} {
		got := ToolsFor(c.mcu)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("ToolsFor(%q) = %v, want %v", c.mcu, got, c.want)
		}
	}
}

// A row is on disk only when the name the lookup asks for is there.
//
// An unpacked tree whose binary is missing is not an installation, and a row
// calling it one is how somebody comes to debug an emulator that was never
// there.
func TestAToolIsOnDiskOnlyWhenTheLookupWouldFindIt(t *testing.T) {
	dir := t.TempDir()
	tc := &Toolchain{Dir: dir}
	rel, ok := releaseNamed("virtual-sx1262")
	if !ok {
		t.Fatal("virtual-sx1262 is not in the catalogue")
	}
	a, ok := rel.asset()
	if !ok {
		t.Skipf("no chip model build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if got := tc.row(rel); got.State == OnDisk {
		t.Fatal("an empty tools directory reported a tool on disk")
	}
	// An installation is what the fetcher leaves behind, which for an archive
	// is the unpacked tree as well as the link the lookup finds. A link alone
	// is the case this test exists for: it is what a half-removed install looks
	// like, and a row calling it on disk sends somebody to debug an emulator
	// that is not there.
	if a.Binary != "" {
		writeFake(t, filepath.Join(dir, a.Binary), a.Magic, 4096)
	}
	writeFake(t, filepath.Join(dir, rel.Name), a.Magic, 4096)
	got := tc.row(rel)
	if got.State != OnDisk {
		t.Fatalf("state is %q with the tool in place, want %q", got.State, OnDisk)
	}
	if got.Estimated {
		t.Error("a tool on disk was still reporting an estimated size")
	}
	// A plain file measures itself; an archive measures the tree it unpacked
	// to, which holds the same one file here.
	if got.Bytes != 4096 {
		t.Errorf("measured %d bytes, want 4096", got.Bytes)
	}
	if err := tc.Remove(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	if tc.row(rel).State == OnDisk {
		t.Error("the tool is still on disk after being removed")
	}
}
