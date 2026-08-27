package capture

import (
	"os"
	"path/filepath"
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
	got := wiresharkHint("", "")
	if strings.Contains(got, "udpdump") {
		t.Fatalf("the hint still mentions udpdump: %q", got)
	}
	if !strings.Contains(got, "-i lo") {
		t.Fatalf("wanted a real interface with a display filter, got %q", got)
	}
	if !strings.Contains(got, "udp port "+captureUDPPort) {
		t.Fatalf("wanted the capture filtered to the port meshbench.lua listens on, got %q", got)
	}
}

func TestTheHintCarriesBothDissectorsInLoadOrder(t *testing.T) {
	got := wiresharkHint("/path/meshcore_dissector.lua", "/path/meshbench.lua")
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
	got := wiresharkHint("", "/path/meshbench.lua")
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
