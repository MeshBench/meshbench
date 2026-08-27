package workbench

import (
	"testing"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/ui/theme"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

func aBuildSnapshot(notes string, coproc bool) *state.Snapshot {
	s := auditSnapshot()
	s.Library = []state.FirmwareRow{{
		Role: "companion_radio_usb", Version: "mesh-rs", Board: "LilyGo_TDeck",
		OnDisk: true, Bytes: 3 << 20, InUse: 2,
		Path:     "/cache/board/LilyGo_TDeck/companion_radio_usb@mesh-rs.bin",
		Facts:    firmware.ImageFacts{Kind: "whole flash image", Bootable: true, FlashMB: 16},
		Settings: firmware.BuildSettings{CoprocAtReset: coproc, Notes: notes},
	}}
	return s
}

// aFirmwareWindow is a window drawn against one snapshot, with what its
// controls asked for.
//
// One harness for the whole test, because Gio delivers a click against the
// previous frame's layout: a button pressed in a second harness was never laid
// out in the first, and the press lands nowhere.
func aFirmwareWindow(t *testing.T, snap *state.Snapshot) (*firmwareWindowPanel,
	*panelHarness, *[]call) {
	t.Helper()
	var got []call
	p := &firmwareWindowPanel{
		role: "companion_radio_usb", version: "mesh-rs", board: "LilyGo_TDeck",
	}
	p.OnDo = func(verb string, params any) {
		got = append(got, call{verb: verb, params: params})
	}
	h := newPanelHarness(func(th *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
		return p.auditDraw(th, gtx, s)
	}, snap)
	h.frame()
	h.frame()
	return p, h, &got
}

type call struct {
	verb   string
	params any
}

// The window fills itself from the build it was opened on, rather than from
// whatever the last one showed.
func TestTheWindowShowsTheBuildItWasOpenedOn(t *testing.T) {
	p, _, _ := aFirmwareWindow(t, aBuildSnapshot("traps in its own vector", true))
	if got := fieldText(&p.name); got != "mesh-rs" {
		t.Errorf("the name box says %q", got)
	}
	if got := fieldText(&p.notes); got != "traps in its own vector" {
		t.Errorf("the notes box says %q", got)
	}
	if !p.coproc.Bool.Value {
		t.Error("the setting is off in a window whose build has it on")
	}
	// And it is not offering to save what it has just read.
	r, _ := p.row(aBuildSnapshot("traps in its own vector", true))
	if p.changed(r) {
		t.Error("a freshly opened window already thinks it has unsaved changes")
	}
}

// Applying sends one call carrying everything, so a rename and a setting
// changed together are one move rather than two that can half-fail.
func TestApplyingSendsTheWholeChange(t *testing.T) {
	snap := aBuildSnapshot("", false)
	p, h, got := aFirmwareWindow(t, snap)
	p.name.Editor.SetText("wadamesh 1.2")
	p.notes.Editor.SetText("boots to its own panic handler")
	p.coproc.Bool.Value = true
	p.roleWant = "simple_repeater"
	// A frame with the change in it, so the apply button exists to be pressed:
	// it is only drawn when there is something to apply.
	h.frame()

	p.apply.Click.Click()
	h.frame()
	h.frame()

	if len(*got) != 1 {
		t.Fatalf("apply sent %d calls, want 1: %+v", len(*got), *got)
	}
	c := (*got)[0]
	if c.verb != "firmware.update" {
		t.Fatalf("apply reached %s", c.verb)
	}
	m := c.params.(map[string]any)
	// The build it means, by all three of its current names: a label alone
	// can carry more than one build and the wrong one is somebody else's
	// image.
	if m["version"] != "mesh-rs" || m["role"] != "companion_radio_usb" ||
		m["board"] != "LilyGo_TDeck" {
		t.Errorf("it did not say which build it meant: %+v", m)
	}
	if m["label"] != "wadamesh 1.2" || m["new_role"] != "simple_repeater" {
		t.Errorf("the rename did not travel: %+v", m)
	}
	if m["coproc_at_reset"] != true || m["notes"] != "boots to its own panic handler" {
		t.Errorf("the settings did not travel: %+v", m)
	}
}

// Once the build is renamed, the window is about the renamed build - not
// about a name nothing answers to.
func TestTheWindowFollowsARenamedBuild(t *testing.T) {
	snap := aBuildSnapshot("", false)
	p, h, _ := aFirmwareWindow(t, snap)
	p.name.Editor.SetText("wadamesh 1.2")
	h.frame()
	p.apply.Click.Click()
	h.frame()
	h.frame()

	if p.version != "wadamesh 1.2" {
		t.Fatalf("the window is still about %q", p.version)
	}
	// And when the library catches up, it reads the new build rather than
	// still offering to save the rename it already made.
	renamed := aBuildSnapshot("", false)
	renamed.Library[0].Version = "wadamesh 1.2"
	h.snap = renamed
	h.frame()
	h.frame()
	r, found := p.row(renamed)
	if !found {
		t.Fatal("the window cannot find the build it renamed")
	}
	if p.changed(r) {
		t.Error("the window still thinks the rename is unsaved")
	}
}

// A build deleted from under the window says so, rather than drawing an empty
// page or guessing at another build.
func TestAWindowWhoseBuildHasGoneSaysSo(t *testing.T) {
	p, h, got := aFirmwareWindow(t, aBuildSnapshot("", false))
	empty := auditSnapshot()
	empty.Library = nil
	h.snap = empty
	h.frame()
	h.frame()
	if len(*got) != 0 {
		t.Errorf("a window with no build ran verbs anyway: %+v", *got)
	}
	if _, found := p.row(empty); found {
		t.Error("it found a build in an empty library")
	}
}

// Picking a different role is a draft, not a move.
//
// The role chips wrote straight into the field the window finds its build by,
// so choosing one made the window lose the build it was about - and with it
// the apply button that would have carried the choice anywhere.
func TestChoosingARoleDoesNotLoseTheBuild(t *testing.T) {
	snap := aBuildSnapshot("", false)
	p, h, got := aFirmwareWindow(t, snap)

	p.roleChips["simple_repeater"].Click.Click()
	h.frame()
	h.frame()

	if p.roleWant != "simple_repeater" {
		t.Fatalf("the chip did not take: %q", p.roleWant)
	}
	r, found := p.row(snap)
	if !found {
		t.Fatal("the window lost its build the moment a role was picked")
	}
	if !p.changed(r) {
		t.Error("picking a role left nothing to apply")
	}
	if len(*got) != 0 {
		t.Errorf("picking a role ran a verb by itself: %+v", *got)
	}
}
