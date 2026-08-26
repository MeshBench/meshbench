// Emit the closed sets both clients need, from the one place that defines them.
//
//	go run ./tools/clientgen            write them
//	go run ./tools/clientgen -check     fail if they are out of date
//
// Boards, radio presets and node kinds are fixed lists that live in
// internal/world/scenario. A client that spelled them as free strings would
// take "LilyGo_TDek" without a word and produce a node that is not the one
// somebody asked for - and a hand-copied list is a list that is wrong the week
// somebody adds a board.
//
// So they are generated, and CI fails when the generated files disagree with
// the tree. That is the same guard the licence inventory has, and for the same
// reason: a generated file nobody regenerates is a stale file with a
// convincing header.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func main() {
	check := flag.Bool("check", false, "fail if the generated files are out of date")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fail(err)
	}
	for path, body := range map[string][]byte{
		filepath.Join(root, "clients", "go", "meshbench", "sets_gen.go"): goSets(),
		filepath.Join(root, "clients", "python", "meshbench", "sets.py"): pySets(),
	} {
		if *check {
			have, err := os.ReadFile(path) //nolint:gosec // a path this program chose
			if err != nil || !bytes.Equal(have, body) {
				fail(fmt.Errorf("%s is out of date; run go run ./tools/clientgen",
					mustRel(root, path)))
			}
			continue
		}
		if err := os.WriteFile(path, body, 0o644); err != nil { //nolint:gosec // source, not a secret
			fail(err)
		}
		fmt.Println("wrote", mustRel(root, path))
	}
	if *check {
		fmt.Println("the generated sets are current")
	}
}

// kinds is the node types, in the order somebody would meet them.
//
// Listed here rather than read from the package because Go has no way to
// enumerate the constants of a named string type, and the alternative - parsing
// the source - would be a second thing to keep correct. The compiler still
// checks every one of them, below.
var kinds = []struct {
	Go, Py, Value, Doc string
}{
	{"SimpleRepeater", "SIMPLE_REPEATER", string(scenario.SimpleRepeater),
		"forwards, and nothing else"},
	{"AdvancedRepeater", "ADVANCED_REPEATER", string(scenario.AdvancedRepeater),
		"forwards, serves clients, holds state"},
	{"Companion", "COMPANION", string(scenario.Companion),
		"a user's device - the thing a phone connects to"},
	{"RoomServer", "ROOM_SERVER", string(scenario.RoomServer),
		"holds posts for clients to collect, and does not forward: a mesh " +
			"that treats one as a repeater overstates its own reach"},
	{"SDRObserver", "SDR_OBSERVER", string(scenario.SDRObserver),
		"runs no firmware and transmits nothing; captures the summed field " +
			"at its antenna and hands back IQ"},
	{"Emitter", "EMITTER", string(scenario.Emitter),
		"interference that is not MeshCore, propagated through the same " +
			"terrain as everything else"},
}

// set is one closed set of strings both clients need.
//
// Value is a typed constant wherever the tree has one, so the compiler checks
// every entry here against the thing it is generated from. Two of these have no
// Go constant to point at - the tabs live behind the toolkit, and a codegen
// tool that imported Gio would be a codegen tool nobody could run headless -
// so those are literals with a guard test where the real list lives.
type set struct {
	Go, Py, Value, Doc string
}

// roles is the MeshCore application a node runs, which is the string every
// firmware verb is keyed on.
var roles = []set{
	{"RoleSimpleRepeater", "SIMPLE_REPEATER", string(scenario.RoleSimpleRepeater),
		"forwards; both repeater kinds run it and differ only in configuration"},
	{"RoleCompanionRadio", "COMPANION_RADIO", string(scenario.RoleCompanionRadio),
		"a user's device - the thing a phone connects to"},
	{"RoleSimpleRoomServer", "SIMPLE_ROOM_SERVER", string(scenario.RoleSimpleRoomServer),
		"holds posts for clients to collect, and does not forward"},
	{"RoleCompanionRadioUSB", "COMPANION_RADIO_USB", string(scenario.RoleCompanionRadioUSB),
		"the USB companion build; board images only, where a board publishes " +
			"both transports at one version"},
	{"RoleCompanionRadioBLE", "COMPANION_RADIO_BLE", string(scenario.RoleCompanionRadioBLE),
		"the Bluetooth companion build; board images only"},
}

// classes is what happened to an event.
var classes = []set{
	{"ClassSent", "SENT", string(engine.ClassSent), "this node transmitted it"},
	{"ClassReceived", "RECEIVED", string(engine.ClassReceived),
		"this node decoded it, for the first time"},
	{"ClassHalfDuplex", "HALF_DUPLEX", string(engine.ClassHalfDuplex),
		"missed because this node's own transmitter was keyed; LoRa is half duplex"},
	{"ClassInterference", "INTERFERENCE", string(engine.ClassInterference),
		"would have decoded, but a stronger signal took it"},
	{"ClassFloor", "FLOOR", string(engine.ClassFloor),
		"too quiet: under the demodulator's threshold for its spreading factor"},
}

// tabs is the panes a node window can be opened on.
//
// Literal, and internal/ui/workbench has a test that fails when this and the
// window's own list disagree. The alternative is importing the toolkit into a
// code generator.
var tabs = []set{
	{"TabConsole", "CONSOLE", "Console", "the firmware's text console, which only a repeater has"},
	{"TabCompanion", "COMPANION", "Companion", "channels, contacts and the companion command line"},
	{"TabSDR", "SDR", "SDR", "an observer's antenna: serve it, read the address"},
	{"TabSettings", "SETTINGS", "Settings", "what this node is: identity, radio, regions, firmware"},
	{"TabRadio", "RADIO", "Radio", "what the chip is really doing"},
	{"TabStats", "STATS", "Stats", "what it has cost and what it has carried"},
	{"TabActivity", "ACTIVITY", "Activity", "what it has heard and sent, in order"},
	{"TabConnect", "CONNECT", "Connect", "hand this companion to a real client"},
	{"TabHardware", "HARDWARE", "Hardware",
		"the board drawn as itself - its screen, its lamps, the buttons " +
			"somebody can press; only a board that declares any of that grows it"},
	{"TabOutput", "OUTPUT", "Output",
		"what the node printed: its serial port, the emulator running it, " +
			"or the radio model beside it"},
}

// strategies is how a fetched deployment meets the scenario already loaded.
var strategies = []set{
	{"Replace", "REPLACE", "replace-all",
		"throw away what is loaded and take the import; what the shipped " +
			"fixtures were built with"},
	{"Add", "ADD", "add", "keep what is loaded and add the names it has not got"},
}

// transports is how a served companion is reached.
var transports = []set{
	{"OverTCP", "TCP", "tcp",
		"a socket on every interface, on a port the system picks; the one to " +
			"point a phone or another machine at"},
	{"OverSerial", "SERIAL", "serial", "a pseudo-terminal, for a client that wants a serial port"},
}

func goSets() []byte {
	var b strings.Builder
	b.WriteString(`// Code generated by tools/clientgen. DO NOT EDIT.
//
// The closed sets, from internal/world/scenario. Constants rather than free
// strings so a typo is a compile error here instead of a verb refusing at run
// time - and generated so the list cannot be wrong the week somebody adds a
// board.

package meshbench

// Kind is what a node is.
type Kind string

// The node kinds.
const (
`)
	for _, k := range kinds {
		b.WriteString(goComment("\t", k.Go+" "+k.Doc+"."))
		fmt.Fprintf(&b, "\t%s Kind = %q\n", k.Go, k.Value)
	}
	b.WriteString(")\n\n// Board is a hardware profile this build knows about.\ntype Board string\n\n")
	b.WriteString("// The boards. A node's board decides its transmit ceiling, its receive\n")
	b.WriteString("// chain's noise figure and the battery the energy model uses, so naming one\n")
	b.WriteString("// that does not exist is refused rather than defaulted.\nconst (\n")
	for _, bd := range scenario.Boards() {
		fmt.Fprintf(&b, "\t// %s: %s, %s, by %s.\n",
			bd.Name, bd.MCU, bd.Radio, bd.Vendor)
		fmt.Fprintf(&b, "\tBoard%s Board = %q\n", goIdent(bd.Name), bd.Name)
	}
	b.WriteString(")\n\n// Boards is every one, for a caller offering a choice.\nvar Boards = []Board{\n")
	for _, bd := range scenario.Boards() {
		fmt.Fprintf(&b, "\tBoard%s,\n", goIdent(bd.Name))
	}
	b.WriteString("}\n\n// Preset is a named set of LoRa parameters for a territory.\ntype Preset string\n\n")
	b.WriteString("// The community presets. This is an agreement between operators rather than\n")
	b.WriteString("// a configuration, which is why the list is baked in rather than fetched.\nconst (\n")
	for _, p := range scenario.RadioPresets {
		fmt.Fprintf(&b, "\t// %s: %.3f MHz, %.1f kHz, SF%d, CR 4/%d.\n",
			p.Label, p.FreqMHz, p.BwKHz, p.SF, p.CR)
		fmt.Fprintf(&b, "\tPreset%s Preset = %q\n", goIdent(p.Label), p.Label)
	}
	b.WriteString(")\n\n// Presets is every one, and DefaultPreset is what a fresh scenario uses.\nvar Presets = []Preset{\n")
	for _, p := range scenario.RadioPresets {
		fmt.Fprintf(&b, "\tPreset%s,\n", goIdent(p.Label))
	}
	fmt.Fprintf(&b, "}\n\nconst DefaultPreset Preset = %q\n", scenario.DefaultPreset)

	goEnum(&b, "Role", "Role is the MeshCore application a node runs, named as upstream "+
		"names its example directory.\n\nThe string every firmware verb is keyed on. The published catalogue "+
		"spells some of the same things differently - \"repeater\", \"room-server\" - and those "+
		"belong to the release assets; typing one at a verb pins nothing and the run refuses "+
		"to start with no clue as to why.", roles, true)
	goEnum(&b, "Class", "Class is what happened to an event.", classes, true)
	goEnum(&b, "Tab", "Tab is a pane of a node's own window.", tabs, true)
	goEnum(&b, "Strategy", "Strategy is how an imported deployment meets what is already loaded.",
		strategies, false)
	goEnum(&b, "Transport", "Transport is how a served companion is reached.", transports, false)
	return []byte(b.String())
}

// goEnum writes one named string type, its constants, and - when a caller
// might reasonably offer the choice - the slice of all of them.
func goEnum(b *strings.Builder, name, doc string, items []set, all bool) {
	b.WriteString("\n" + goComment("", doc) + "type " + name + " string\n\nconst (\n")
	for _, it := range items {
		b.WriteString(goComment("\t", it.Go+" is "+it.Doc+"."))
		fmt.Fprintf(b, "\t%s %s = %q\n", it.Go, name, it.Value)
	}
	b.WriteString(")\n")
	if !all {
		return
	}
	fmt.Fprintf(b, "\n// %ss is every one, for a caller offering a choice.\nvar %ss = []%s{\n",
		name, name, name)
	for _, it := range items {
		fmt.Fprintf(b, "\t%s,\n", it.Go)
	}
	b.WriteString("}\n")
}

func pySets() []byte {
	var b strings.Builder
	b.WriteString(`"""Generated by tools/clientgen. DO NOT EDIT.

The closed sets, from internal/world/scenario. Enums rather than free strings
so an editor can complete them and a typo fails where it was written, not as a
verb refusing three calls later - and generated so the list cannot be wrong the
week somebody adds a board.

Every one subclasses str, so it goes on the wire as itself and a plain string
is still accepted anywhere one is asked for.
"""

from __future__ import annotations

from enum import Enum


class _Set(str, Enum):
    """A closed set of strings, which is its value wherever a string is wanted.

    The __str__ line is spelled out because a plain (str, Enum) does not do
    this: an f-string of Board.LILYGO_TDECK renders as "Board.LILYGO_TDECK",
    which then reaches a verb as a board name nothing matches. Comparison and
    JSON already used the value and only printing disagreed, which is the worst
    combination of the two.
    """

    __str__ = str.__str__

    def __repr__(self) -> str:
        return type(self).__name__ + "." + self.name


class Kind(_Set):
    """What a node is."""

`)
	for _, k := range kinds {
		fmt.Fprintf(&b, "    %s = %q\n%s\n", k.Py, k.Value, pyDoc("    ", k.Doc))
	}
	b.WriteString(`
class Board(_Set):
    """A hardware profile this build knows about.

    A node's board decides its transmit ceiling, its receive chain's noise
    figure and the battery the energy model uses, so naming one that does not
    exist is refused rather than defaulted.
    """

`)
	for _, bd := range scenario.Boards() {
		fmt.Fprintf(&b, "    %s = %q\n%s\n", pyIdent(bd.Name), bd.Name,
			pyDoc("    ", fmt.Sprintf("%s, %s, by %s.", bd.MCU, bd.Radio, bd.Vendor)))
	}
	b.WriteString(`
class Preset(_Set):
    """A named set of LoRa parameters for a territory.

    An agreement between operators rather than a configuration, which is why
    the list is baked in rather than fetched.
    """

`)
	seen := map[string]bool{}
	for _, p := range scenario.RadioPresets {
		id := pyIdent(p.Label)
		if seen[id] {
			continue
		}
		seen[id] = true
		fmt.Fprintf(&b, "    %s = %q\n%s\n", id, p.Label,
			pyDoc("    ", fmt.Sprintf("%.3f MHz, %.1f kHz, SF%d, CR 4/%d.",
				p.FreqMHz, p.BwKHz, p.SF, p.CR)))
	}
	fmt.Fprintf(&b, "\nDEFAULT_PRESET = Preset(%q)\n\n", scenario.DefaultPreset)

	pyEnum(&b, "Role", `The MeshCore application a node runs, named as upstream names
    its example directory.

    The string every firmware verb is keyed on. The published catalogue spells
    some of the same things differently - "repeater", "room-server" - and those
    belong to the release assets; typing one at a verb pins nothing, and the run
    then refuses to start with no clue as to why.`, roles)
	pyEnum(&b, "Class", "What happened to an event.", classes)
	pyEnum(&b, "Tab", "A pane of a node's own window.", tabs)
	pyEnum(&b, "Strategy",
		"How an imported deployment meets what is already loaded.", strategies)
	pyEnum(&b, "Transport", "How a served companion is reached.", transports)
	// Each entry leaves a blank line after it, which at the end of the file is
	// one newline too many for ruff.
	return []byte(strings.TrimRight(b.String(), "\n") + "\n")
}

func pyEnum(b *strings.Builder, name, doc string, items []set) {
	fmt.Fprintf(b, "\nclass %s(_Set):\n    \"\"\"%s\"\"\"\n\n", name, doc)
	for _, it := range items {
		fmt.Fprintf(b, "    %s = %q\n%s\n", it.Py, it.Value, pyDoc("    ", it.Doc))
	}
}

// goComment wraps prose as a Go comment, because a generated line that runs
// past the margin is a lint finding nobody can fix without editing a
// generator - which is where it belongs, so it is fixed here.
func goComment(indent, text string) string {
	var out strings.Builder
	for _, line := range wrap(text, 72-len(indent)) {
		fmt.Fprintf(&out, "%s// %s\n", indent, line)
	}
	return out.String()
}

// pyDoc is the same for a Python docstring.
func pyDoc(indent, text string) string {
	lines := wrap(text, 84-len(indent))
	if len(lines) == 1 {
		return fmt.Sprintf("%s\"\"\"%s\"\"\"\n", indent, lines[0])
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%s\"\"\"%s\n", indent, lines[0])
	for _, l := range lines[1:] {
		fmt.Fprintf(&out, "%s%s\n", indent, l)
	}
	fmt.Fprintf(&out, "%s\"\"\"\n", indent)
	return out.String()
}

// wrap breaks prose at a width, on spaces.
func wrap(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = w
			continue
		}
		line += " " + w
	}
	return append(lines, line)
}

// goIdent turns a board or preset name into an exported Go identifier.
func goIdent(s string) string {
	var out []rune
	up := true
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if up {
				out = append(out, unicode.ToUpper(r))
				up = false
				continue
			}
			out = append(out, r)
		default:
			up = true
		}
	}
	return string(out)
}

// pyIdent turns one into a Python enum member.
func pyIdent(s string) string {
	var out []rune
	prev := '_'
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			out = append(out, unicode.ToUpper(r))
			prev = r
		default:
			if prev != '_' {
				out = append(out, '_')
				prev = '_'
			}
		}
	}
	name := strings.Trim(string(out), "_")
	// An identifier cannot begin with a digit, and one preset does.
	if name != "" && unicode.IsDigit(rune(name[0])) {
		name = "P" + name
	}
	return name
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("clientgen: no go.mod above %s", dir)
		}
		dir = parent
	}
}

func mustRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "clientgen:", err)
	os.Exit(1)
}
