package firmware_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// The cache decides what a node can run, so the manager has to describe it
// accurately: a build left behind by a rename and a build in daily use look
// identical from outside the directory.
func TestListsBothKindsOfBuild(t *testing.T) {
	cache := t.TempDir()
	write(t, cache, "native/repeater-v1.17.0/meshcore-simple_repeater-linux-amd64", "elf")
	write(t, cache, "native/companion-v1.17.0/meshcore-companion_radio-linux-amd64", "elf")
	write(t, cache, "board/Generic_E22_sx1262/simple_repeater-v1.17.0.bin", "image")
	// A local compile leaves this behind; it is not a firmware.
	write(t, cache, "native/repeater-v1.17.0/obj/thing.o", "junk")

	got := firmware.ListInstalled(cache)
	if len(got) != 3 {
		for _, g := range got {
			t.Logf("  %s", g.Label())
		}
		t.Fatalf("listed %d builds, want 3", len(got))
	}

	var native, board int
	for _, g := range got {
		if g.Native {
			native++
			if g.Board != "" {
				t.Errorf("%s is native and yet claims board %q", g.Label(), g.Board)
			}
		} else {
			board++
			if g.Board == "" {
				t.Errorf("%s is a board image with no board, which cannot be run", g.Label())
			}
		}
		if g.Role == "" {
			t.Errorf("%s has no role", g.Path)
		}
		if g.Bytes == 0 {
			t.Errorf("%s reports zero bytes", g.Path)
		}
	}
	if native != 2 || board != 1 {
		t.Errorf("got %d native and %d board builds, want 2 and 1", native, board)
	}
}

// The role is carried in the filename, and a role may contain hyphens, so it
// cannot be recovered by splitting on them.
func TestRoleSurvivesTheFilename(t *testing.T) {
	cache := t.TempDir()
	write(t, cache, "native/main/meshcore-companion_radio-linux-amd64", "elf")
	write(t, cache, "board/Heltec_v3/simple_repeater-v1.16.0.bin", "image")

	roles := map[string]bool{}
	for _, g := range firmware.ListInstalled(cache) {
		roles[g.Role] = true
	}
	for _, want := range []string{"companion_radio", "simple_repeater"} {
		if !roles[want] {
			t.Errorf("role %q was lost reading the cache; got %v", want, roles)
		}
	}
}

// A downloaded build's digest keeps the build's own name, so it parses to the
// same role and the same version. The library keys its rows on exactly that,
// and a directory read hands the sidecar over second, so it replaced the build
// it describes and every row in the window read as 65 bytes of hex.
func TestTheDigestBesideABuildIsNotABuild(t *testing.T) {
	cache := t.TempDir()
	bin := "native/repeater-v1.17.1/meshcore-simple_repeater-linux-amd64"
	write(t, cache, bin, "elf")
	write(t, cache, bin+".sha256", "a9f1c0de\n")
	// Windows names the build with a suffix of its own, and the digest goes
	// after it rather than in place of it.
	win := "native/companion-v1.17.1/meshcore-companion_radio-windows-amd64.exe"
	write(t, cache, win, "pe")
	write(t, cache, win+".sha256", "b7e2d1af\n")

	got := firmware.ListInstalled(cache)
	if len(got) != 2 {
		for _, g := range got {
			t.Logf("  %s  %s", g.Label(), g.Path)
		}
		t.Fatalf("listed %d builds, want 2", len(got))
	}
	for _, g := range got {
		if strings.HasSuffix(g.Path, ".sha256") {
			t.Errorf("%s points at a digest, not a build", g.Label())
		}
		if g.Bytes == 0 {
			t.Errorf("%s reports no size, so it is not the file that was written", g.Label())
		}
	}
}

// A delete button driven by a path is exactly the shape of thing that removes
// somebody's home directory when a later caller builds the path differently.
func TestRemoveRefusesAnythingOutsideTheCache(t *testing.T) {
	cache := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "precious")
	if err := os.WriteFile(elsewhere, []byte("do not delete"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := firmware.Remove(cache, firmware.Installed{Path: elsewhere})
	if err == nil {
		t.Fatal("removing a file outside the cache was allowed")
	}
	if !strings.Contains(err.Error(), "outside the cache") {
		t.Errorf("error does not say why: %v", err)
	}
	if _, err := os.Stat(elsewhere); err != nil {
		t.Error("the file outside the cache was deleted anyway")
	}
}

// Removing the last build of a version should take the empty directory with it,
// or a cleared cache reads as a list of versions that are all empty.
func TestRemoveTidiesTheEmptyVersion(t *testing.T) {
	cache := t.TempDir()
	write(t, cache, "native/repeater-v1.16.0/meshcore-simple_repeater-linux-amd64", "elf")

	all := firmware.ListInstalled(cache)
	if len(all) != 1 {
		t.Fatalf("expected one build, got %d", len(all))
	}
	if err := firmware.Remove(cache, all[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cache, "native", "repeater-v1.16.0")); !os.IsNotExist(err) {
		t.Error("the emptied version directory was left behind")
	}
	if got := firmware.ListInstalled(cache); len(got) != 0 {
		t.Errorf("cache still lists %d builds after removing the only one", len(got))
	}
}

// Importing is wanted for everything that was never released, which is most of
// what is worth testing.
func TestImportLandsWhereTheRunnerLooks(t *testing.T) {
	cache := t.TempDir()
	src := filepath.Join(t.TempDir(), "my-build")
	if err := os.WriteFile(src, []byte("a local build"), 0o644); err != nil {
		t.Fatal(err)
	}

	in, err := firmware.Import(cache, src, "v9.9.9-mine", "simple_repeater", "")
	if err != nil {
		t.Fatal(err)
	}
	if !in.Native {
		t.Error("an import with no board should be native")
	}
	// Downloads arrive without the execute bit, and a native build that cannot
	// be executed fails with a permission error naming the file, not the cause.
	st, err := os.Stat(in.Path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no executable bit - a file is executable there by its
	// extension - so asking for one fails on a platform where nothing is
	// wrong.
	if runtime.GOOS != "windows" && st.Mode()&0o111 == 0 {
		t.Error("imported native build is not executable")
	}
	if len(firmware.ListInstalled(cache)) != 1 {
		t.Error("the imported build does not appear in the cache listing")
	}
}

func TestImportNeedsEnoughToIdentifyIt(t *testing.T) {
	cache := t.TempDir()
	src := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := firmware.Import(cache, src, "", "simple_repeater", ""); err == nil {
		t.Error("import without a version was accepted; the cache would be unreadable")
	}
	if _, err := firmware.Import(cache, src, "v1", "", ""); err == nil {
		t.Error("import without a role was accepted")
	}
}

// The dev-loop path this reproduces: firmware.Build imports a fresh build
// into the cache, and cmd_dev then hands that same cache path back to the
// firmware.import verb, which computes the identical destination and tries to
// import it again. Reproduced here through the two exported entry points a
// real run goes through - Build's own Import call, then Catalogue.Import,
// which is what the firmware.import verb calls - rather than through
// cmd_dev itself, since a real meshcore-native build is too heavy for a unit
// test.
func TestReimportingABuildAtItsOwnCachePathDoesNotEmptyIt(t *testing.T) {
	cache := t.TempDir()
	staged := filepath.Join(t.TempDir(), "meshcore-simple_repeater-linux-amd64")
	want := []byte("what meshcore-native's build.sh produced")
	if err := os.WriteFile(staged, want, 0o644); err != nil {
		t.Fatal(err)
	}

	// firmware.Build's own import: the freshly built binary into the cache.
	built, err := firmware.Import(cache, staged, "local-mybranch", "simple_repeater", "")
	if err != nil {
		t.Fatal(err)
	}
	if built.Bytes != int64(len(want)) {
		t.Fatalf("build landed %d bytes, want %d", built.Bytes, len(want))
	}

	// cmd_dev then hands built.Path to firmware.import, which for a native
	// build (no board) is Catalogue.Import.
	cat := &firmware.Catalogue{CacheDir: cache}
	img, err := cat.Import(built.Path, "", "simple_repeater", "local-mybranch")
	if err != nil {
		t.Fatal(err)
	}

	if img.Size != int64(len(want)) {
		t.Errorf("reimport reports %d bytes, want %d: the message would claim "+
			"success over a truncated file", img.Size, len(want))
	}
	got, err := os.ReadFile(built.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("the cached build after reimport: got %d bytes, want %d (%q)",
			len(got), len(want), got)
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
