package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The log is the only thing a Windows user has when the application was
// started from the Start menu, so where it goes and what lands in it are worth
// holding still. Both were found by a report that said only "it starts and
// then disappears".

func TestErrorLogPathIsUnderTheCache(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	got, err := errorLogPath()
	if err != nil {
		t.Fatalf("errorLogPath: %v", err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	want := filepath.Join(cache, "meshbench", "meshbench-error.log")
	if got != want {
		t.Errorf("errorLogPath = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Dir(got)); err != nil {
		t.Errorf("the directory was not made: %v", err)
	}
}

func TestReportFatalWritesTheMessageAndNamesTheFile(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	path := reportFatal("no fixture called \"nope\"")
	if path == "" {
		t.Fatal("reportFatal returned no path, so nothing could name the file")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the log back: %v", err)
	}
	if !strings.Contains(string(b), `no fixture called "nope"`) {
		t.Errorf("the log does not carry the message: %q", b)
	}
	// Appended rather than replaced: a second failure must not erase the
	// first, which is how a run that failed twice loses the interesting half.
	reportFatal("and then a second one")
	b, _ = os.ReadFile(path)
	if !strings.Contains(string(b), "and then a second one") ||
		!strings.Contains(string(b), `no fixture called "nope"`) {
		t.Errorf("the second message replaced the first: %q", b)
	}
}
