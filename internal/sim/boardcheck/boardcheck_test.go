package boardcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/firmware/emulated/renode"
)

// A board never run reports "untested" in every column, never blank and
// never anything that could be mistaken for a pass.
func TestUntestedReportCoversEveryCapability(t *testing.T) {
	r := untestedReport("Generic_E22_sx1262", "repeater-v1.17.0")
	if len(r.Results) != len(Capabilities) {
		t.Fatalf("got %d results, want %d", len(r.Results), len(Capabilities))
	}
	for _, c := range Capabilities {
		res, ok := r.Results[c]
		if !ok {
			t.Fatalf("capability %q missing entirely", c)
		}
		if res.State != Untested {
			t.Errorf("capability %q: got %q, want untested", c, res.State)
		}
	}
}

// A cached report round-trips exactly, and reading a board never probed
// before is the same "untested" shape a fresh one would report - not on
// disk and never run are the same state to a reader.
func TestCacheRoundTripsAndDefaultsToUntested(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("LOCALAPPDATA", cacheHome)

	fresh := Load("Generic_E22_sx1262", "repeater-v1.17.0")
	if fresh.Results[Build].State != Untested {
		t.Fatalf("a board never probed: got %q, want untested", fresh.Results[Build].State)
	}

	r := untestedReport("Generic_E22_sx1262", "repeater-v1.17.0")
	r.set(Build, Passed, "image found")
	r.set(Boot, Passed, "attached")
	r.set(Radio, Failed, "no transmission observed")
	r.EmulatorFP = "test-fingerprint@123-456"
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}

	got := Load("Generic_E22_sx1262", "repeater-v1.17.0")
	if got.Results[Build].State != Passed || got.Results[Build].Detail != "image found" {
		t.Errorf("build did not round-trip: %+v", got.Results[Build])
	}
	if got.Results[Radio].State != Failed || got.Results[Radio].Detail != "no transmission observed" {
		t.Errorf("radio did not round-trip: %+v", got.Results[Radio])
	}
	// TX was never set on the saved report, so it must still read as
	// untested - Save must not silently invent a verdict for it.
	if got.Results[TX].State != Untested {
		t.Errorf("an unset capability read as %q, not untested", got.Results[TX].State)
	}
}

// Changing the emulator marks a cached report stale rather than keeping it
// current - the whole reason caching is safe to do at all.
func TestALoadedReportIsStaleWhenTheEmulatorFingerprintDiffers(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("LOCALAPPDATA", cacheHome)

	r := untestedReport("RAK_4631", "repeater-v1.17.0")
	r.set(Boot, Passed, "attached")
	r.EmulatorFP = "an-old-qemu-build@1-1"
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}

	got := Load("RAK_4631", "repeater-v1.17.0")
	// Whatever EmulatorFingerprint finds on this machine, it is not the
	// saved fingerprint above - so this must read as stale, not as still
	// current.
	if !got.Stale {
		t.Error("a report saved under a different emulator fingerprint did not report stale")
	}
	// Staleness does not erase the measurement - it is still readable.
	if got.Results[Boot].State != Passed {
		t.Errorf("a stale report lost its own data: %+v", got.Results[Boot])
	}
}

// A file this build cannot fully account for - missing a capability key
// entirely - is treated as untested rather than trusted partially: a
// half-understood cache entry is exactly the "guessed at" failure mode
// this package exists to avoid.
func TestAnIncompleteCacheFileIsTreatedAsUntested(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("LOCALAPPDATA", cacheHome)

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	// Hand-write a report missing every capability but "build" - as an
	// older format might have.
	partial := []byte(`{"Board":"Heltec_v2","Version":"repeater-v1.17.0",` +
		`"Results":{"build":{"Capability":"build","State":"passed"}}}`)
	if err := os.WriteFile(reportPath(dir, "Heltec_v2", "repeater-v1.17.0"), partial, 0o644); err != nil {
		t.Fatal(err)
	}

	got := Load("Heltec_v2", "repeater-v1.17.0")
	if got.Results[Boot].State != Untested {
		t.Errorf("an incomplete cache file was trusted: boot=%+v", got.Results[Boot])
	}
}

// The matrix agrees with hw.EmulatableBoards() on the can-it-run
// question: every board that function blocks reports boot-failed with its
// own reason, and every board it allows is at least representable (never
// silently dropped from the matrix).
func TestMatrixAgreesWithEmulatableBoards(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("LOCALAPPDATA", cacheHome)

	ok, blocked := hw.EmulatableBoards()
	okNames := map[string]bool{}
	for _, b := range ok {
		okNames[b.Name] = true
	}

	reports := MatrixReports("repeater-v1.17.0")
	if len(reports) != len(hw.Boards()) {
		t.Fatalf("got %d reports, want one per board (%d)", len(reports), len(hw.Boards()))
	}
	byName := map[string]BoardReport{}
	for _, r := range reports {
		byName[r.Board] = r
	}
	for name, reason := range blocked {
		if okNames[name] {
			continue // a board is never in both maps at once; nothing to check here
		}
		r, ok := byName[name]
		if !ok {
			t.Fatalf("%s is blocked but missing from the matrix entirely", name)
		}
		if r.Results[Boot].State != Failed {
			t.Errorf("%s is blocked (%s) but the matrix shows boot as %q", name, reason, r.Results[Boot].State)
		}
		if r.Results[Boot].Detail != reason {
			t.Errorf("%s: matrix reason %q does not match EmulatableBoards' own %q",
				name, r.Results[Boot].Detail, reason)
		}
	}
}

// The fingerprint names the emulator the board runs under, not whichever
// variable happens to be set: on a machine with both, an nRF52 row was stamped
// with QEMU's binary (#551). The chip library is part of it, and for Renode so
// are the peripherals beside it.
func TestTheFingerprintFollowsTheBoardsEmulator(t *testing.T) {
	dir := t.TempDir()
	file := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	qemu, renodeBin, chip := file("qemu", "q"), file("renode", "r"), file("chip", "c")
	if err := os.MkdirAll(filepath.Join(dir, "support", "peripherals"), 0o755); err != nil {
		t.Fatal(err)
	}
	periph := filepath.Join(dir, "support", "peripherals", "VirtualSX1262.cs")
	if err := os.WriteFile(periph, []byte("p"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(emulated.EnvQEMU, qemu)
	t.Setenv(renode.EnvRenode, renodeBin)
	t.Setenv(emulated.EnvRadioLib, chip)
	t.Setenv(emulated.EnvRenodeSupport, filepath.Join(dir, "support"))

	nrf := EmulatorFingerprint("RAK_4631")
	if !strings.Contains(nrf, renodeBin+"@") || strings.Contains(nrf, qemu+"@") {
		t.Errorf("an nRF52 board's fingerprint is not Renode's: %s", nrf)
	}
	esp := EmulatorFingerprint("LilyGo_TDeck")
	if !strings.Contains(esp, qemu+"@") || strings.Contains(esp, renodeBin+"@") {
		t.Errorf("an ESP32 board's fingerprint is not QEMU's: %s", esp)
	}
	for _, fp := range []string{nrf, esp} {
		if !strings.Contains(fp, chip+"@") {
			t.Errorf("the chip library is not in %s", fp)
		}
	}

	// Each part moves the rows it should and only those.
	touch := func(p, body string) {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	touch(qemu, "qemu, rebuilt")
	if EmulatorFingerprint("RAK_4631") != nrf {
		t.Error("a new QEMU retired an nRF52 row")
	}
	if EmulatorFingerprint("LilyGo_TDeck") == esp {
		t.Error("a new QEMU did not retire an ESP32 row")
	}
	touch(periph, "peripheral, changed")
	if EmulatorFingerprint("RAK_4631") == nrf {
		t.Error("a changed Renode peripheral did not retire an nRF52 row")
	}
	esp = EmulatorFingerprint("LilyGo_TDeck")
	touch(chip, "chip, new release")
	if EmulatorFingerprint("LilyGo_TDeck") == esp {
		t.Error("a new chip library did not retire an ESP32 row")
	}
	if got := EmulatorFingerprint("No_Such_Board"); got != "unknown board" {
		t.Errorf("an unknown board: %s", got)
	}
}
