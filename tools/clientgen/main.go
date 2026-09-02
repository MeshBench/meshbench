// Emit the closed sets all three clients need, from the one place that defines
// them.
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
//
// The Node client is generated too, though it has no compiler to spend the
// safety on. What it gets instead is completion and a name to grep for: an
// editor offers Board.LILYGO_TDECK from the frozen object, where a free string
// offers nothing, and a member that has gone is undefined at the call rather
// than a board name the workbench has never heard of arriving on the wire.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		filepath.Join(root, "pkg", "client-go", "meshbench", "sets_gen.go"): goSets(),
		filepath.Join(root, "pkg", "client-python", "meshbench", "sets.py"): pySets(),
		filepath.Join(root, "pkg", "client-js", "lib", "sets.mjs"):          jsSets(),
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
var kinds = []set{
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

// set is one closed set of strings every client needs.
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
	{"ClassCollision", "COLLISION", string(engine.ClassCollision),
		"decoded its header, then a collision destroyed more symbols than " +
			"the coding rate could repair"},
	{"ClassReceiverBusy", "RECEIVER_BUSY", string(engine.ClassReceiverBusy),
		"arrived at a demodulator already locked to another packet; a LoRa " +
			"receiver decodes one at a time"},
	{"ClassFloor", "FLOOR", string(engine.ClassFloor),
		"too quiet: under the demodulator's threshold for its spreading factor"},
	{"ClassUnclassified", "UNCLASSIFIED", string(engine.ClassUnclassified),
		"a miss whose cause the engine did not establish; never assume it " +
			"was a weak signal"},
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
	{"TabAntenna", "ANTENNA", "Antenna",
		"what this node stands under and which way it points"},
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
