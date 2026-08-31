// Package fakenative is a stand-in for the native MeshCore child process.
//
// What the native backend does around that process - start it, refuse to start
// it twice, notice it died, give up on one that will not be reaped - is
// ordinary Go, and it was untestable only because it was entangled with
// launching a real binary. That binary is built in another repository and is
// absent from most machines and every CI runner, so five of the seven tests
// beside it skipped and the airtime agreement CLAUDE.md states as a rule was
// never once checked by the pipeline.
//
// So a test re-enters its own test binary instead. It is a real process on the
// real operating system, with real pipes, real signals and a real exit status,
// and it is present wherever the test is. Nothing here models MeshCore: it
// speaks just enough of the bridge to be attached to, and its point is the
// ways a child process can misbehave.
package fakenative

import (
	"os"
	"path/filepath"
	"strconv"
)

// EnvMode carries the behaviour a re-entered test binary should take on.
// Empty means this process is the test itself, not a stand-in for a node.
const EnvMode = "MESHBENCH_FAKE_NATIVE"

// EnvTxAtMs is when a ModeAdvert child puts a frame on the air, counted in the
// simulated milliseconds the bridge ticks it to rather than in real ones - the
// engine supplies the clock, which is the whole point of lockstep.
const EnvTxAtMs = "MESHBENCH_FAKE_NATIVE_TX_MS"

// What a stand-in can be asked to be.
const (
	// ModeAttach is a node that behaves: it connects, acknowledges every tick,
	// answers the console, and exits cleanly when the bridge goes away.
	ModeAttach = "attach"

	// ModeAdvert is ModeAttach plus one frame on the air, once the simulated
	// clock reaches EnvTxAtMs.
	ModeAdvert = "advert"

	// ModeExit starts successfully and is gone before anything can connect to
	// it. A launch that worked and a node that is running are not the same
	// fact, and the backend reports the first as if it were the second.
	ModeExit = "exit"

	// ModeCrash is ModeExit with a failing status, which is the only thing
	// that tells a node that fell over from one that was asked to stop.
	ModeCrash = "crash"

	// ModeStuck never connects, never exits, and is deaf to every signal a
	// graceful stop can send - so a test reaches the path that kills it.
	// SIGKILL is not among the ones it ignores and cannot be.
	ModeStuck = "stuck"
)

// CrashStatus is what ModeCrash exits with. Any non-zero value would do; a
// specific one lets a test assert that the status survived to the caller
// rather than being flattened into "it stopped".
const CrashStatus = 3

// Path is the binary to point a native backend at: this test binary, which
// re-enters as a stand-in when EnvMode is set in its environment.
//
// Absolute, because the backend runs a node in a working directory of its own
// and a relative argv[0] would not resolve from there.
func Path() string {
	if abs, err := filepath.Abs(os.Args[0]); err == nil {
		return abs
	}
	return os.Args[0]
}

// Mode is the behaviour this process was re-entered with, or empty for a
// process that is not a stand-in at all. A test binary's TestMain asks this
// before running any test.
func Mode() string { return os.Getenv(EnvMode) }

// txAtMs is when a ModeAdvert child transmits, defaulting early enough that a
// test does not have to simulate long to see it.
func txAtMs() uint32 {
	if v, err := strconv.ParseUint(os.Getenv(EnvTxAtMs), 10, 32); err == nil {
		return uint32(v)
	}
	return 1_000
}
