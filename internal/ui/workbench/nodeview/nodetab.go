// Which pane of a node's window is showing.
//
// Its own file because the set is what a node *is*: an observer runs no
// firmware and grows no console, a repeater has no contacts and grows no
// Companion tab, and a board with nothing to draw grows no Hardware tab. A
// tab that is always there and always empty teaches people to ignore tabs.
package nodeview

// Tab is which pane of the window is showing, in the mock's order.
type Tab int

const (
	// TabConsole is the firmware's text console, which only a repeater has.
	// A companion speaks the framed protocol instead, and its command line -
	// meshcore-cli's vocabulary - lives inside the Companion tab; a second
	// tab for the same thing taught people the two might differ.
	TabConsole Tab = iota
	// TabCompanion and TabConnect only exist for a node that speaks the
	// companion protocol. A repeater has no channels and no contacts, and a
	// tab that is always there and always empty teaches people to ignore
	// tabs.
	TabCompanion
	// TabSDR is the observer's front pane - serve the antenna, read the
	// address - and only an observer's window grows it.
	TabSDR
	TabSettings
	TabRadio
	// TabAntenna is what this node stands under and where it points. Beside
	// Radio because the two are the same signal path, and apart from it
	// because Radio reports what the chip says and this decides something.
	TabAntenna
	TabStats
	TabActivity
	TabConnect
	// TabHardware is the board drawn as itself - its screen, its lamps, the
	// buttons somebody can press. Only a node whose board declares any of
	// that grows it, because a tab that is always there and always empty
	// teaches people to ignore tabs.
	TabHardware
	// TabOutput is what the node itself printed, from each of the three
	// things that can print something about it: its serial port, the emulator
	// running it, and the radio model beside it. Every node that runs firmware
	// grows it - a native node writes to standard error where an emulated one
	// writes to a serial port, and the question being asked is the same.
	TabOutput
	numNodeTabs
)

func (n Tab) String() string {
	switch n {
	case TabCompanion:
		return "Companion"
	case TabSDR:
		return "SDR"
	case TabSettings:
		return "Settings"
	case TabRadio:
		return "Radio"
	case TabAntenna:
		return "Antenna"
	case TabStats:
		return "Stats"
	case TabActivity:
		return "Activity"
	case TabConnect:
		return "Connect"
	case TabHardware:
		return "Hardware"
	case TabOutput:
		return "Output"
	}
	return "Console"
}
