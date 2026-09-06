package capture

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The port is not a free choice - it is meshbench.lua's own MSIM_UDP_PORT -
// so this fails loudly if somebody "tidies" it back to an arbitrary value.
func TestWeStreamWhereMeshbenchLuaListens(t *testing.T) {
	if !strings.HasSuffix(captureUDPAddr, ":5555") {
		t.Fatalf("streaming to %s, which is not where meshbench.lua registers", captureUDPAddr)
	}
}

// The whole bug this file exists to fix: udpdump and a bare UDP stream are
// two different protocols that happen to share a default port number.
// Whatever capture.wireshark launches has to point Wireshark at a real
// interface with a display filter, never at the extcap.
func TestLaunchNeverUsesTheUdpdumpExtcap(t *testing.T) {
	// There is no live wireshark binary to launch in a test, so this checks
	// the hint - the same words a human would be told to run by hand - which
	// launchWireshark's own exec.Command is built from the same way.
	got := wiresharkHint("", "", "")
	if strings.Contains(got, "udpdump") {
		t.Fatalf("the hint still mentions udpdump: %q", got)
	}
	if !strings.Contains(got, "-i "+loopbackInterface()) {
		t.Fatalf("wanted a real interface with a display filter, got %q", got)
	}
	if !strings.Contains(got, "udp port "+captureUDPPort) {
		t.Fatalf("wanted the capture filtered to the port meshbench.lua listens on, got %q", got)
	}
}

func TestTheHintCarriesBothDissectorsInLoadOrder(t *testing.T) {
	got := wiresharkHint("", "/path/meshcore_dissector.lua", "/path/meshbench.lua")
	meshcoreAt := strings.Index(got, "meshcore_dissector.lua")
	meshbenchAt := strings.Index(got, "meshbench.lua")
	if meshcoreAt < 0 || meshbenchAt < 0 {
		t.Fatalf("both dissectors should appear, got %q", got)
	}
	if meshcoreAt > meshbenchAt {
		t.Fatalf("meshcore_dissector.lua must load before meshbench.lua - its "+
			"DLT_USER0 registration has to be the one that stands - got: %q", got)
	}
}

func TestTheHintOmitsAMissingDissectorRatherThanAnEmptyFlag(t *testing.T) {
	got := wiresharkHint("", "", "/path/meshbench.lua")
	if strings.Contains(got, "lua_script:  ") || strings.Contains(got, "lua_script:-X") {
		t.Errorf("a missing path should not leave a bare -X, got %q", got)
	}
	if !strings.Contains(got, "meshbench.lua") {
		t.Errorf("the one that was found should still be there, got %q", got)
	}
}

// dissectorFiles has to look in the same places for both scripts and keep
// them paired - a mismatched pair (one found beside the binary, one only in
// a checkout) is exactly the kind of thing that looks like it works and
// silently does not.
func TestDissectorFilesFindsBothInACheckout(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(wd))
	if _, err := os.Stat(filepath.Join(root, "tools", "dissector", "meshbench.lua")); err != nil {
		t.Skip("not running from a checkout")
	}
	t.Chdir(root)
	meshcore, meshbench := dissectorFiles()
	if meshcore == "" {
		t.Error("meshcore_dissector.lua was not found in the checkout")
	}
	if meshbench == "" {
		t.Error("meshbench.lua was not found in the checkout")
	}
}

func TestDissectorFilesReturnsEmptyRatherThanGuessing(t *testing.T) {
	t.Chdir(t.TempDir())
	meshcore, meshbench := dissectorFiles()
	if meshcore != "" || meshbench != "" {
		t.Errorf("nowhere to find them, wanted empty paths, got %q %q", meshcore, meshbench)
	}
}

// The loopback interface is named differently on each platform, and naming it
// wrong is not a degraded capture: Wireshark opens on an interface that does
// not exist, or on somebody else's traffic. "lo" everywhere was wrong on two
// of the three we ship.
func TestTheLoopbackInterfaceIsTheOneThisPlatformHas(t *testing.T) {
	want := map[string]string{
		"windows": `\Device\NPF_Loopback`,
		"darwin":  "lo0",
		"linux":   "lo",
	}[runtime.GOOS]
	if want == "" {
		t.Skipf("no expectation recorded for %s", runtime.GOOS)
	}
	if got := loopbackInterface(); got != want {
		t.Errorf("on %s the loopback interface is %q, got %q", runtime.GOOS, want, got)
	}
}

// The hint is handed to somebody to run, and the reference says so, so it has
// to be runnable where it is printed. Two ways it was not, on Windows: an
// interface that does not exist there, and POSIX quoting that cmd passes
// through as part of the filter.
func TestTheHintIsRunnableOnThisPlatform(t *testing.T) {
	got := wiresharkHint("", "", "")
	if !strings.Contains(got, "-i "+loopbackInterface()) {
		t.Errorf("the hint names an interface this platform has not got: %q", got)
	}
	quoted := quoteArg("udp port " + captureUDPPort)
	if !strings.Contains(got, "-f "+quoted) {
		t.Errorf("the filter is not quoted for this platform's shell: %q", got)
	}
	if runtime.GOOS == "windows" && strings.Contains(got, "'") {
		t.Errorf("single quotes reach cmd as part of the argument: %q", got)
	}
}

// A binary we already found is worth naming: the reader is being asked to run
// it by hand, and on Windows the name alone does not resolve.
func TestTheHintNamesTheBinaryWhenThereIsOne(t *testing.T) {
	got := wiresharkHint("/opt/wireshark/bin/wireshark", "", "")
	if !strings.HasPrefix(got, "/opt/wireshark/bin/wireshark ") {
		t.Errorf("wanted the binary we found at the front, got %q", got)
	}
	spaced := wiresharkHint(`C:\Program Files\Wireshark\Wireshark.exe`, "", "")
	if !strings.Contains(spaced, quoteArg(`C:\Program Files\Wireshark\Wireshark.exe`)) {
		t.Errorf("a path with a space has to survive being retyped, got %q", spaced)
	}
}

// Windows installs Wireshark under Program Files and does not touch PATH, so
// looking only on PATH finds nothing on a machine that plainly has it.
func TestWindowsFindsWiresharkWhereTheInstallerPutsIt(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the Program Files search is Windows-only")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "Wireshark", "Wireshark.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ProgramFiles", dir)
	t.Setenv("ProgramFiles(x86)", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if got := wiresharkBinary(); got != exe {
		t.Errorf("wanted %q from the Program Files search, got %q", exe, got)
	}
}

// The dumpcap permission dance is a Unix packaging habit. On Windows there is
// nothing to work around and a copy would land in a directory nobody asked for.
func TestDumpcapIsLeftAloneOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows opts out")
	}
	if got := usableDumpcap(); got != "" {
		t.Errorf("nothing should be copied on Windows, got %q", got)
	}
}

// Where these actually land on Windows is beside the binary, under Program
// Files - so the path has a space in it, and an unquoted lua_script token is
// two arguments by the time Wireshark sees it.
func TestTheHintQuotesADissectorPathWithASpace(t *testing.T) {
	p := filepath.Join(`C:\Program Files\MeshBench`, "tools", "dissector", "meshbench.lua")
	got := wiresharkHint("", "", p)
	if !strings.Contains(got, quoteArg("lua_script:"+p)) {
		t.Errorf("the lua_script token has to survive a space in the path, got %q", got)
	}
	if strings.Contains(got, `-X lua_script:C:\Program Files`) {
		t.Errorf("unquoted, this splits at the space before Wireshark sees it: %q", got)
	}
}

// The bundle ships them to tools/dissector beside the binary, which is what
// the refusal names and what the checkout looks like. They were shipped there
// before this root existed, so they were shipped somewhere nothing looked.
func TestDissectorFilesFindsWhatTheBundleShips(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path on this platform")
	}
	dir := filepath.Join(filepath.Dir(exe), "tools", "dissector")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skipf("cannot write beside the test binary: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Join(filepath.Dir(exe), "tools")) })
	for _, n := range []string{"meshbench.lua", "meshcore_dissector.lua"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("-- x"), 0o644); err != nil {
			t.Skipf("cannot write beside the test binary: %v", err)
		}
	}
	t.Chdir(t.TempDir()) // so only the beside-the-binary root can answer
	meshcore, meshbench := dissectorFiles()
	if meshcore == "" || meshbench == "" {
		t.Errorf("a bundle layout was not found: meshcore=%q meshbench=%q", meshcore, meshbench)
	}
}
