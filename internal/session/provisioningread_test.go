package session

import (
	"strings"
	"testing"
)

func TestSplitTranscriptAttributesReplyToItsEcho(t *testing.T) {
	// Both the echo of "get name" and the firmware's own reply to it start
	// with "> " once the timestamp is stripped - CommonCLI.cpp writes get
	// replies as "> %s" itself. Only matching against the exact commands
	// sent tells them apart.
	lines := []string{
		"   0.000  > get name",
		"   0.041  > Abernethy Repeater",
		"   0.041  > get path.hash.mode",
		"   0.078  > 0",
	}
	entries := splitTranscript(lines, []string{"get name", "get path.hash.mode"})
	if len(entries) != 2 {
		t.Fatalf("got %d entries, wanted 2: %+v", len(entries), entries)
	}
	if entries[0].Command != "get name" || len(entries[0].Reply) != 1 ||
		entries[0].Reply[0] != "> Abernethy Repeater" {
		t.Errorf("first entry: %+v", entries[0])
	}
	if entries[1].Command != "get path.hash.mode" || len(entries[1].Reply) != 1 ||
		entries[1].Reply[0] != "> 0" {
		t.Errorf("second entry: %+v", entries[1])
	}
}

func TestSplitTranscriptDropsOutputBeforeTheFirstEcho(t *testing.T) {
	lines := []string{"   0.000  stray boot chatter", "   0.010  > get name", "   0.020  > x"}
	entries := splitTranscript(lines, []string{"get name"})
	if len(entries) != 1 || entries[0].Command != "get name" {
		t.Fatalf("got %+v", entries)
	}
}

func TestSplitTranscriptCollectsMultilineReplies(t *testing.T) {
	lines := []string{
		"   0.000  > region",
		"   0.010  * F",
		"   0.010   sco F",
		"   0.010  > region default",
		"   0.020  default scope is sco",
	}
	entries := splitTranscript(lines, []string{"region", "region default"})
	if len(entries) != 2 {
		t.Fatalf("got %d entries: %+v", len(entries), entries)
	}
	if len(entries[0].Reply) != 2 {
		t.Errorf("region's reply should be both tree lines, got %+v", entries[0].Reply)
	}
}

func TestIsRefusalMatchesEveryFirmwareRefusalFormat(t *testing.T) {
	for _, reply := range []string{
		"Error, must be 0,1, or 2",
		"Error, must be: off, minimal, moderate, or strict",
		"(ERR: clock cannot go backwards)",
		"Err - unknown region",
		"Err - unknown parent",
		"Unknown command",
	} {
		if !isRefusal(reply) {
			t.Errorf("%q should read as a refusal", reply)
		}
	}
}

func TestIsRefusalAcceptsTheOKFormats(t *testing.T) {
	for _, reply := range []string{
		"OK", "OK - (flood allowed)", "OK - clock set: 14:00 - 30/12/2026 UTC",
		"> minimal", " default scope is now sco", "",
	} {
		if isRefusal(reply) {
			t.Errorf("%q should not read as a refusal", reply)
		}
	}
}

func TestSettleStepsScalesWithTheLongestScriptNotTheWholeNetwork(t *testing.T) {
	// The bound is per-node work, not network size - every firmware process
	// drains its own queue in parallel.
	if a, b := settleSteps(1), settleSteps(50); a >= b {
		t.Errorf("a one-line script (%d) should settle in fewer steps than a "+
			"fifty-line one (%d)", a, b)
	}
	if got := settleSteps(1); got < 60 {
		t.Errorf("even a trivial script gets at least the old default budget, got %d", got)
	}
	if got := settleSteps(10000); got > 900 {
		t.Errorf("settle steps must stay bounded, got %d", got)
	}
}

func TestSectionLineDoesNotLookLikeFirmwareOutput(t *testing.T) {
	got := sectionLine("8 sent, 8 accepted")
	if got == "" {
		t.Fatal("empty section line")
	}
	// No leading digits: a real console line always starts with a timestamp,
	// and this must be visually distinct from one.
	if got[0] >= '0' && got[0] <= '9' {
		t.Errorf("a section line must not start like a timestamp: %q", got)
	}
	if !strings.Contains(got, "8 sent, 8 accepted") {
		t.Errorf("wanted the summary text in %q", got)
	}
}
