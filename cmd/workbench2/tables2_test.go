package main

import "testing"

// The Latest swatch goes to the newest release, never to a study build or a
// branch - and v1.9 sits below v1.17, because versions are numbers.
func TestLatestIsTheNewestRelease(t *testing.T) {
	if _, ok := releaseVersion("txcheck-22"); ok {
		t.Error("a branch name counted as a release")
	}
	if _, ok := releaseVersion("study-01"); ok {
		t.Error("a study build counted as a release")
	}
	n, ok := releaseVersion("repeater-v1.17.0")
	if !ok || n != "1.17.0" {
		t.Errorf("repeater-v1.17.0 parsed as %q, %v", n, ok)
	}
	if !laxVersionLess("1.9.0", "1.17.0") {
		t.Error("v1.9 did not sit below v1.17")
	}
	if laxVersionLess("1.17.0", "1.9.0") {
		t.Error("v1.17 sat below v1.9")
	}
}
