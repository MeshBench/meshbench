// Which pane of a node's window is showing.
//
// Its own file because the set is what a node *is*: an observer runs no
// firmware and grows no console, a repeater has no contacts and grows no
// Companion tab, and a board with nothing to draw grows no Hardware tab. A
// tab that is always there and always empty teaches people to ignore tabs.
package workbench

// nodeTab is which pane of the window is showing, in the mock's order.
type nodeTab int

const (
	// tabConsole is the firmware's text console, which only a repeater has.
	// A companion speaks the framed protocol instead, and its command line -
	// meshcore-cli's vocabulary - lives inside the Companion tab; a second
	// tab for the same thing taught people the two might differ.
	tabConsole nodeTab = iota
	// tabCompanion and tabConnect only exist for a node that speaks the
	// companion protocol. A repeater has no channels and no contacts, and a
	// tab that is always there and always empty teaches people to ignore
	// tabs.
	tabCompanion
	// tabSDR is the observer's front pane - serve the antenna, read the
	// address - and only an observer's window grows it.
	tabSDR
	tabSettings
	tabRadio
	tabStats
	tabActivity
	tabConnect
	// tabHardware is the board drawn as itself - its screen, its lamps, the
	// buttons somebody can press. Only a node whose board declares any of
	// that grows it, because a tab that is always there and always empty
	// teaches people to ignore tabs.
	tabHardware
	// tabOutput is what the node itself printed, from each of the three
	// things that can print something about it: its serial port, the emulator
	// running it, and the radio model beside it. Every node that runs firmware
	// grows it - a native node writes to standard error where an emulated one
	// writes to a serial port, and the question being asked is the same.
	tabOutput
	numNodeTabs
)

func (n nodeTab) String() string {
	switch n {
	case tabCompanion:
		return "Companion"
	case tabSDR:
		return "SDR"
	case tabSettings:
		return "Settings"
	case tabRadio:
		return "Radio"
	case tabStats:
		return "Stats"
	case tabActivity:
		return "Activity"
	case tabConnect:
		return "Connect"
	case tabHardware:
		return "Hardware"
	case tabOutput:
		return "Output"
	}
	return "Console"
}
