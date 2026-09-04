package experiment

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
)

// Varying twice used to leave the second parameter's arms and throw the first
// away, so a matrix of two parameters was unreachable and nothing said so.
func TestVaryingTwiceCrossesTheArms(t *testing.T) {
	var arms []session.ExpArm
	cross := func(param string, values ...string) {
		base := arms
		if len(base) == 0 || (len(base) == 1 && !base[0].Names()) {
			base = []session.ExpArm{{}}
		}
		var out []session.ExpArm
		for _, b := range base {
			for _, v := range values {
				a, seg, err := varied(b, param, v)
				if err != nil {
					t.Fatalf("varying %s by %q: %v", param, v, err)
				}
				a.Label = joinLabel(b.Label, seg)
				out = append(out, a)
			}
		}
		arms = out
	}

	cross("repeater_version", "repeater-v1.17.0", "repeater-v1.17.1")
	if len(arms) != 2 {
		t.Fatalf("two firmware versions gave %d arms", len(arms))
	}
	// The first cross onto a fresh experiment must not carry a placeholder
	// segment: these are "1.17.0", not "· 1.17.0".
	if arms[0].Label != "1.17.0" {
		t.Fatalf("first arm is labelled %q", arms[0].Label)
	}

	cross("path_hash_mode", "0", "1", "2")
	if len(arms) != 6 {
		t.Fatalf("two versions by three hash modes gave %d arms, want 6", len(arms))
	}
	if arms[0].Label != "1.17.0 · 1-byte" {
		t.Fatalf("crossed arm is labelled %q", arms[0].Label)
	}
	// Mode 0 is one byte per hop and is also the zero value, which is why the
	// field is a pointer: an arm that set it must be distinguishable from one
	// that did not.
	if arms[0].PathHashMode == nil || *arms[0].PathHashMode != 0 {
		t.Fatalf("mode 0 did not survive the cross: %v", arms[0].PathHashMode)
	}
	// Every arm keeps the firmware it was crossed from.
	for _, a := range arms {
		if a.RepeaterVersion == "" {
			t.Fatalf("arm %q lost its firmware in the cross", a.Label)
		}
	}
}

// An arm writes only what it names, over the session's settings. Writing every
// field would reset the ones it is silent about, and for path hash mode the
// zero value is a real setting.
func TestAnArmWritesOnlyWhatItNames(t *testing.T) {
	base := session.DefaultProvisioning()
	base.LoopDetect = "strict"
	base.CadMode = "on"

	mode := 2
	arm := session.ExpArm{PathHashMode: &mode}
	got := base
	arm.ApplyOver(&got)

	if got.CompPathHashMode != 2 {
		t.Fatalf("companion path hash is %d, not what the arm named", got.CompPathHashMode)
	}
	if got.LoopDetect != "strict" || got.CadMode != "on" {
		t.Fatalf("the arm reset settings it was silent about: loop=%q cad=%q",
			got.LoopDetect, got.CadMode)
	}
	if got.PathHashMode != base.PathHashMode {
		t.Fatalf("a companion setting moved the repeaters' own to %d", got.PathHashMode)
	}
}

// Any firmware setting can be an arm, not only the ones with a field.
//
// The AGC reset interval is the case that matters: it is off by default, it is
// what makes MeshCore 1.17.1's gain fault reachable at all, and nothing in this
// codebase has or should have a struct field for it.
func TestAnyFirmwareSettingCanBeAnArm(t *testing.T) {
	on, seg, err := varied(session.ExpArm{}, "set:agc.reset.interval", "4")
	if err != nil {
		t.Fatalf("varying a plain setting: %v", err)
	}
	if on.Set["agc.reset.interval"] != "4" {
		t.Fatalf("arm carries %v", on.Set)
	}
	if seg != "agc.reset.interval 4" {
		t.Fatalf("label segment is %q", seg)
	}

	// Crossed onto, the sibling arms must not share one map.
	off, _, _ := varied(on, "set:radio.rxgain", "off")
	if _, ok := on.Set["radio.rxgain"]; ok {
		t.Fatal("crossing wrote back into the arm it was crossed from")
	}
	if off.Set["agc.reset.interval"] != "4" || off.Set["radio.rxgain"] != "off" {
		t.Fatalf("crossed arm carries %v", off.Set)
	}

	// And it has to reach the node, which means the provisioning script.
	prov := session.DefaultProvisioning()
	off.ApplyOver(&prov)
	if !strings.Contains(prov.Extra, "set agc.reset.interval 4") ||
		!strings.Contains(prov.Extra, "set radio.rxgain off") {
		t.Fatalf("the arm's settings never reached provisioning: %q", prov.Extra)
	}
}

// Every parameter the Bench offers has to be one the verb accepts, or the
// failure arrives after the arms have been built.
func TestEveryOfferedParameterCanBeVaried(t *testing.T) {
	for _, p := range session.VaryParams {
		v := "0"
		switch p.Name {
		case "loop_detect":
			v = "off"
		case "cad":
			v = "on"
		case "repeater_version", "companion_version":
			v = "repeater-v1.17.0"
		}
		if _, _, err := varied(session.ExpArm{}, p.Name, v); err != nil {
			t.Errorf("the Bench offers %q but the verb refuses it: %v", p.Label, err)
		}
	}
}
