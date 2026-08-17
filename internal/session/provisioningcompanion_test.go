package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/companion/proto"
	"github.com/MeshBench/meshbench/internal/provision"
)

func TestCompanionFramesTranslateName(t *testing.T) {
	frames := companionFrames([]provision.ResolvedCommand{
		{Command: "set name Deeside Companion"},
	})
	if len(frames) != 1 {
		t.Fatalf("got %d frames: %v", len(frames), frames)
	}
	want := proto.SetAdvertName("Deeside Companion")
	if string(frames[0]) != string(want) {
		t.Errorf("got % x, wanted % x", frames[0], want)
	}
}

// A repeater sends lat and lon as two separate `set` commands; the companion
// protocol has one call for both, so the translation must wait for the
// second half before emitting anything.
func TestCompanionFramesCombineLatAndLon(t *testing.T) {
	frames := companionFrames([]provision.ResolvedCommand{
		{Command: "set lat 57.183400"},
		{Command: "set lon -3.641200"},
	})
	if len(frames) != 1 {
		t.Fatalf("got %d frames, wanted one combined SetAdvertLatLon: %v", len(frames), frames)
	}
	want := proto.SetAdvertLatLon(57.1834, -3.6412)
	if string(frames[0]) != string(want) {
		t.Errorf("got % x, wanted % x", frames[0], want)
	}
}

func TestCompanionFramesTranslatePathHashModeUnchanged(t *testing.T) {
	// resolveScalars already converts bytes to the mode 0..2 wire value
	// before this ever sees it - path.hash.mode 0..2 passes straight through
	// to proto.SetPathHashMode, which takes the same range.
	frames := companionFrames([]provision.ResolvedCommand{
		{Command: "set path.hash.mode 2"},
	})
	want := proto.SetPathHashMode(2)
	if len(frames) != 1 || string(frames[0]) != string(want) {
		t.Fatalf("got %v, wanted % x", frames, want)
	}
}

func TestCompanionFramesTranslateClock(t *testing.T) {
	frames := companionFrames([]provision.ResolvedCommand{{Command: "time 1788220800"}})
	want := proto.SetDeviceTime(1788220800)
	if len(frames) != 1 || string(frames[0]) != string(want) {
		t.Fatalf("got %v, wanted % x", frames, want)
	}
}

func TestCompanionFramesTranslateDefaultScope(t *testing.T) {
	frames := companionFrames([]provision.ResolvedCommand{{Command: "region default sco"}})
	if len(frames) != 1 || proto.Command(frames[0][0]) != proto.CmdSetDefaultFloodScope {
		t.Fatalf("got %v", frames)
	}
}

func TestCompanionFramesClearDefaultScope(t *testing.T) {
	frames := companionFrames([]provision.ResolvedCommand{{Command: "region default <null>"}})
	want := proto.ClearDefaultScope()
	if len(frames) != 1 || string(frames[0]) != string(want) {
		t.Fatalf("got %v, wanted % x", frames, want)
	}
}

// region put/allowf/save and the un-scoped flood switch have no companion
// equivalent - a companion holds one scope, not a region table - and must be
// silently dropped rather than misdirected at something that will refuse
// them or, worse, be misread as something else.
func TestCompanionFramesDropCommandsWithNoEquivalent(t *testing.T) {
	frames := companionFrames([]provision.ResolvedCommand{
		{Command: "region put fif"},
		{Command: "region allowf fif"},
		{Command: "region save"},
		{Command: "region allowf *"},
		{Command: "set loop.detect minimal"},
	})
	if len(frames) != 0 {
		t.Fatalf("wanted nothing translated, got %d frames: %v", len(frames), frames)
	}
}

func TestCompanionFramesIgnoreUnparsableValues(t *testing.T) {
	frames := companionFrames([]provision.ResolvedCommand{
		{Command: "set lat not-a-number"},
		{Command: "set path.hash.mode not-a-number"},
	})
	if len(frames) != 0 {
		t.Fatalf("got %v, wanted nothing - lat with no matching lon, and an unparsable mode", frames)
	}
}
