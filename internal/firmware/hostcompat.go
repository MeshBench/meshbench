package firmware

import (
	"fmt"
	"regexp"
)

// Why a build that is perfectly good will not start on this machine.
//
// A node that dies in the dynamic linker exits 1 with nothing MeshBench wrote,
// and fifty-six of them doing it at once reads as a firmware fault. It is not
// one: the build is fine and the machine is too old for it. MeshCore's
// published Linux builds are made on a newer base than MeshBench's own glibc
// floor, so this reaches anyone on Ubuntu 22.04 or Debian 12 running a
// downloaded build, and it reaches CI on the one lab runner that pins that
// floor deliberately.
//
// Recognising it here rather than leaving the linker's own sentence to be read
// is the difference between "no node started" and a fault nobody can act on.

// HostTooOld is the phrase such a message carries.
//
// Exported because this failure crosses a boundary that turns errors into
// strings - an experiment cell reports its Err as text - so a caller that has
// to tell "this machine cannot run it" from "the firmware misbehaved" has
// nothing else left to match on.
const HostTooOld = "this machine's C library is older than the build needs"

// glibcWanted is what the dynamic linker says when it will not load a binary:
//
//	/lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.38' not found (required by ...)
var glibcWanted = regexp.MustCompile("version `GLIBC_([0-9]+\\.[0-9]+)' not found")

// WhyTheHostCannotRunIt reads a dead child's last words and returns a sentence
// when the cause is the machine rather than the firmware. It returns empty for
// everything else, which is most things: this is not a general diagnoser.
func WhyTheHostCannotRunIt(stderr string) string {
	m := glibcWanted.FindStringSubmatch(stderr)
	if m == nil {
		return ""
	}
	return fmt.Sprintf("%s: it wants glibc %s. The build is not at fault and "+
		"restarting will not help - MeshCore's published Linux builds are made "+
		"on a newer base than this machine has. Build one with meshcore-native's "+
		"build.sh, or point %s at a build for this machine",
		HostTooOld, m[1], EnvNativeBinary)
}
