package firmware_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// The dynamic linker's own sentence, which is all a dead node leaves behind
// when the machine is too old for the build. Fifty-six of these at once read
// as a firmware fault for a fortnight.
func TestALinkerRefusalIsNamedAsTheMachineNotTheFirmware(t *testing.T) {
	const said = "/home/runner/_work/_temp/native/meshcore-simple_repeater-linux-amd64: " +
		"/lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.38' not found " +
		"(required by /home/runner/_work/_temp/native/meshcore-simple_repeater-linux-amd64)"

	why := firmware.WhyTheHostCannotRunIt(said)
	if why == "" {
		t.Fatal("a linker refusal was not recognised")
	}
	for _, want := range []string{firmware.HostTooOld, "2.38", "build.sh", firmware.EnvNativeBinary} {
		if !strings.Contains(why, want) {
			t.Errorf("the explanation does not mention %q: %s", want, why)
		}
	}
}

// Everything else is left alone. This is not a general diagnoser, and a
// firmware that genuinely crashed must not be reported as a machine fault.
func TestOtherFailuresAreNotClaimedAsTheMachines(t *testing.T) {
	for _, said := range []string{
		"",
		"assertion failed at mesh.cpp:412",
		"panic: runtime error: index out of range",
		"error while loading shared libraries: libfoo.so.1: cannot open shared object file",
		// Close, but not the one: a missing library is a packaging problem,
		// not a version floor, and the advice would be wrong.
		"symbol lookup error: undefined symbol: meshcore_start",
	} {
		if why := firmware.WhyTheHostCannotRunIt(said); why != "" {
			t.Errorf("%q was claimed as a machine fault: %s", said, why)
		}
	}
}
