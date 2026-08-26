// Where an emulated node's output goes, and what the emulator was asked to
// say about itself.
//
// Four files per node rather than one, because they answer four questions and
// a merged log answers none of them well: what the board printed, what the ROM
// printed before the board's own console existed, what the emulator said about
// running it, and what the radio model logged. They shared a file until the
// emulator's own complaints started matching the patterns the board probe
// looks for in the board's output.
package firmware

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvQEMUDebug turns on the emulator's own instruction and access tracing, as
// a comma-separated list of QEMU log items - "unimp,guest_errors" being the
// pair worth reaching for first, because between them they name every register
// the machine does not model and every access it rejected.
//
// Off by default and deliberately not a setting: the output is megabytes a
// second. One `-d unimp` run once filled a 16 GB tmpfs, and the space stayed
// allocated until the emulator holding the file was killed.
const EnvQEMUDebug = "MESHBENCH_QEMU_DEBUG"

// EnvCoprocAtReset brings the emulated coprocessors up enabled, which the part
// does not do.
//
// For one firmware only, and it is a lie the machine tells: CPENABLE resets to
// zero on this architecture and a firmware decides which of its tasks may use
// the floating point unit. A firmware whose exception handler saves floating
// point state before anything has enabled it takes a CoprocessorDisabled trap
// inside an exception vector, which is fatal and loops for ever - and hides
// everything behind it, so the board looks like it simply stopped.
//
// Off by default, because a firmware that genuinely mismanages that register
// would be flattered by it. On, it is a way to see what happens next.
const EnvCoprocAtReset = "MESHBENCH_QEMU_COPROC_AT_RESET"

// CoprocAtReset reports whether that has been asked for.
func CoprocAtReset() bool {
	v := strings.TrimSpace(os.Getenv(EnvCoprocAtReset))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// The three files a node's output is read from. Named here because the reader
// is in another layer: the application shows them and only this package knows
// where the emulator was told to put them.
const (
	consoleLogName  = "console.log"
	emulatorLogName = "emulator.log"
	radioLogName    = "radio.log"
	// romLogName is UART0 on a board whose application talks over USB: the
	// boot chain up to the point the firmware takes over, and nothing after.
	romLogName   = "rom.log"
	traceLogName = "emulator-trace.log"
)

// ConsoleLogName, EmulatorLogName, RadioLogName and TraceLogName are those
// files' names, for a caller that has a node's directory and wants one of them.
func ConsoleLogName() string  { return consoleLogName }
func EmulatorLogName() string { return emulatorLogName }
func RadioLogName() string    { return radioLogName }

// ROMLogName is UART0's log on a board whose console is on the USB port.
func ROMLogName() string   { return romLogName }
func TraceLogName() string { return traceLogName }

// qemuDebugArgs is the tracing the environment asked for, and nothing when it
// asked for none.
func qemuDebugArgs(dir string) []string {
	items := strings.TrimSpace(os.Getenv(EnvQEMUDebug))
	if items == "" {
		return nil
	}
	return []string{"-d", items, "-D", filepath.Join(dir, traceLogName)}
}

// QEMUTracing reports what the emulator was asked to trace, so a pane showing
// the output can say why it is enormous.
func QEMUTracing() string { return strings.TrimSpace(os.Getenv(EnvQEMUDebug)) }

// ConsoleLog is everything the node has said on its serial port: the whole
// boot chain, ROM through application, and the replies to anything typed at it.
func (e *EmulatedNode) ConsoleLog() ([]byte, error) {
	return os.ReadFile(e.ConsolePath())
}

// ConsolePath is where this node's boot output is written.
func (e *EmulatedNode) ConsolePath() string {
	return filepath.Join(e.Dir, consoleLogName)
}

// EmulatorLog is what the emulator itself said: the properties it refused, the
// peripherals it does not implement, the reason it exited.
//
// A different question from ConsoleLog, and they used to share a file. A board
// that says nothing and an emulator that would not start look identical in a
// merged log, and the patterns the probe matches for a boot loop could be
// satisfied by the emulator complaining rather than by the board booting.
func (e *EmulatedNode) EmulatorLog() ([]byte, error) {
	return os.ReadFile(e.EmulatorPath())
}

// EmulatorPath is where the emulator's own output is written.
func (e *EmulatedNode) EmulatorPath() string {
	return filepath.Join(e.Dir, emulatorLogName)
}
