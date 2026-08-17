package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The port is the whole reason this was broken, so it is worth a test that
// fails loudly if somebody "tidies" it back to an arbitrary one.
func TestWeStreamWhereUdpdumpListens(t *testing.T) {
	if !strings.HasSuffix(udpdumpAddr, ":5555") {
		t.Fatalf("streaming to %s, which is not where udpdump listens", udpdumpAddr)
	}
}

func TestTheHintMatchesThePort(t *testing.T) {
	if got := wiresharkHint(udpdumpAddr); got != "wireshark -k -i udpdump" {
		t.Errorf("on the default port the hint should need no configuring, got %q", got)
	}
	if got := wiresharkHint("127.0.0.1:19000"); !strings.Contains(got, "19000") {
		t.Errorf("off the default port the hint must say which port, got %q", got)
	}
}

func TestInstallingTheDissectorOverwritesAnOlderCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)

	src := filepath.Join(t.TempDir(), "meshcore_dissector.lua")
	if err := os.WriteFile(src, []byte("-- new"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, err := dissectorPluginDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "meshcore_dissector.lua")
	if err := os.WriteFile(stale, []byte("-- old"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest, err := installDissector(src)
	if err != nil {
		t.Fatal(err)
	}
	if dest != stale {
		t.Errorf("installed to %s, wanted %s", dest, stale)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "-- new" {
		t.Errorf("an older copy shadowed the new one: %q", body)
	}
}

func TestAMissingDissectorIsReportedNotFatal(t *testing.T) {
	if _, err := installDissector(filepath.Join(t.TempDir(), "absent.lua")); err == nil {
		t.Fatal("installing a dissector that is not there should say so")
	}
}

// The checkout copy has to be findable, or a developer run gets raw bytes.
func TestTheCheckoutDissectorIsFound(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(wd))
	if _, err := os.Stat(filepath.Join(root, "tools", "dissector", "meshcore_dissector.lua")); err != nil {
		t.Skip("not running from a checkout")
	}
	t.Chdir(root)
	if got := dissectorSource(); got == "" {
		t.Error("the dissector in the tree was not found")
	}
}
