// Opening Wireshark on a running simulation.
//
// workbench1 got this right and it was deleted along with the rest of that
// tree; this rebuilds it from what that code actually did (MSIM-67, 68, 71),
// not from what "stream over UDP" sounds like it should mean.
//
// The engine's own StartCaptureUDP sends bare datagrams to a loopback port -
// no wrapper, nothing listening on the socket, "Wireshark sniffs the
// interface, it does not listen". The udpdump extcap is a different thing
// entirely: it speaks pcap-over-IP to whatever is on the other end of that
// port, and pointed at these datagrams it shows nothing, because that is not
// what they are. The first version of this file used udpdump anyway, on the
// strength of "it defaults to port 5555 too" - a coincidence of port number,
// not a working capture. What actually reads this stream is Wireshark
// capturing a real interface (loopback) with a display filter for the port,
// exactly the way workbench1 launched it.
//
// The dissector is two files, and both matter. meshbench.lua registers on
// udp.port 5555 and reads the pseudo-header the engine writes; it is the
// only thing that makes Wireshark look at this port at all. It then hands
// the MeshCore frame inside to meshcore_dissector.lua - Aaron Brown's
// vendored dissector, GPL-2.0-only, tools/dissector/LICENSE.meshcore_dissector
// - which knows the wire format in far more detail than anything written
// here would. Load order matters: both claim DLT_USER0 for their own radio
// layer, and meshbench.lua has to load second for its registration to be
// the one that stands. -X lua_script: is used rather than installing into
// Wireshark's plugin directory, in that order, on purpose - it is the only
// way to guarantee which one wins, and it needs no install step for a
// developer running the checkout raw.
package capture

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/session"
)

// captureUDPPort is not a choice - it is meshbench.lua's own
// MSIM_UDP_PORT, so a stream sent anywhere else is invisible to it.
const (
	captureUDPPort = "5555"
	captureUDPAddr = "127.0.0.1:" + captureUDPPort
)

// dissectorFiles finds both Lua scripts, in the order Wireshark must load
// them: the vendored MeshCore dissector first, then MeshBench's own metadata
// layer, whose DLT_USER0 registration has to be the one that stands. Beside
// the binary first, then a checkout, because a relative path in a command
// pasted elsewhere silently loads nothing and the frames arrive looking like
// bytes.
func dissectorFiles() (meshcore, meshbench string) {
	names := []string{"meshcore_dissector.lua", "meshbench.lua"}
	var roots []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		roots = append(roots, dir,
			// Where a bundle puts them, mirroring the checkout so the path in
			// the message below is the path on disk in both. The bare dir and
			// dissector/ stay for trees laid out the older ways.
			filepath.Join(dir, "tools", "dissector"),
			filepath.Join(dir, "dissector"),
			filepath.Join(dir, "..", "share", "meshbench"))
	}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Join(wd, "tools", "dissector"))
	}
	found := make([]string, len(names))
	for i, name := range names {
		for _, root := range roots {
			p := filepath.Join(root, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				found[i] = p
				break
			}
		}
	}
	return found[0], found[1]
}

// loopbackInterface is what the operating system calls the interface these
// datagrams cross, which is a different word on each of the three.
//
// It was "lo" everywhere, which is Linux's name for it. macOS calls it lo0,
// and Windows has no loopback interface at all until Npcap provides one as
// \Device\NPF_Loopback. So the capture was launched against an interface
// that does not exist on two of the three platforms we ship.
func loopbackInterface() string {
	switch runtime.GOOS {
	case "windows":
		// Npcap installs this only when its loopback support is chosen. Where
		// it is absent Wireshark says so itself, and better than a guess made
		// here would.
		return `\Device\NPF_Loopback`
	case "darwin":
		return "lo0"
	default:
		return "lo"
	}
}

// wiresharkBinary is the executable to start, or empty if there is none.
func wiresharkBinary() string {
	names := []string{"wireshark"}
	switch runtime.GOOS {
	case "darwin":
		// The .app bundle is where a Mac install actually puts it; the name
		// on PATH is often absent even when Wireshark is installed.
		names = append(names, "/Applications/Wireshark.app/Contents/MacOS/Wireshark")
	case "windows":
		// The installer does not touch PATH, so LookPath finds nothing on a
		// machine with Wireshark plainly installed. It does always land in the
		// same place, under whichever Program Files matches its build.
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if dir := os.Getenv(env); dir != "" {
				names = append(names, filepath.Join(dir, "Wireshark", "Wireshark.exe"))
			}
		}
	}
	for _, n := range names {
		if filepath.IsAbs(n) {
			// G703: the candidates are this file's own list, at most rooted at
			// a directory the operating system named. A Windows program that
			// will not look under %ProgramFiles% cannot find what a Windows
			// installer put there, and an attacker who can set that variable
			// for this process has already won.
			if st, err := os.Stat(n); err == nil && !st.IsDir() { //nolint:gosec // G703: our own path list, rooted where Windows says
				return n
			}
			continue
		}
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// wiresharkColumns is what the packet list shows: what the simulator knows -
// who sent it, who heard it, what happened, the radio numbers - rather than
// the loopback address every row would otherwise share. The IP and UDP
// layers are still in the detail pane; they genuinely are on the wire, and
// nothing here hides them, only keeps them out of the list an operator scans.
const wiresharkColumns = `gui.column.format:"Time","%t","From","%Cus:msim.from_name",` +
	`"Received by","%Cus:msim.to_name","Outcome","%Cus:msim.outcome",` +
	`"SNR","%Cus:msim.snr","Hops","%Cus:meshcore.hops",` +
	`"Hash","%Cus:meshcore.path_hash_size","Info","%i"`

// launchWireshark starts it on loopback, filtered to the capture port, with
// both dissectors and the columns that make the list legible - and does not
// wait, returning why it could not rather than an error.
//
// Not an error, because failing to open a window is not a failed capture -
// the frames are already streaming by the time this is called, and returning
// an error from the verb would report the whole thing as broken when the
// recoverable part is "start Wireshark yourself".
//
// Detached deliberately: Wireshark outlives the run it was opened on, and a
// simulation that cannot exit until somebody closes a packet viewer is a
// simulation that looks hung.
func launchWireshark(bin, meshcoreLua, meshbenchLua string) string {
	args := []string{"-k", "-i", loopbackInterface(), "-f", "udp port " + captureUDPPort}
	if meshcoreLua != "" {
		args = append(args, "-X", "lua_script:"+meshcoreLua)
	}
	if meshbenchLua != "" {
		args = append(args, "-X", "lua_script:"+meshbenchLua, "-o", wiresharkColumns)
	}
	// G204: bin is whatever wiresharkBinary resolved - a PATH lookup or one of
	// its own fixed paths - and every argument is built above from constants
	// and paths this process found. None of it reaches here from a caller.
	// Starting a named program is the entire purpose of this function.
	cmd := exec.Command(bin, args...) //nolint:gosec // G204: bin and args are resolved here, not supplied
	// Its own stdio, so a chatty GTK does not interleave with the session log.
	cmd.Stdout, cmd.Stderr = nil, nil
	// Wireshark finds dumpcap on PATH; put a runnable one first if the system
	// copy is not executable by this user - see usableDumpcap.
	if dc := usableDumpcap(); dc != "" {
		cmd.Env = append(os.Environ(),
			"PATH="+filepath.Dir(dc)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if err := cmd.Start(); err != nil {
		return err.Error()
	}
	// Reaped on its own, so the process table does not fill with zombies
	// across a session where somebody opens it repeatedly.
	go func() { _ = cmd.Wait() }()
	return ""
}

// usableDumpcap returns a dumpcap this user can actually execute, making one
// if necessary.
//
// Distributions ship it root:wireshark, mode rwxr-xr--: readable by anyone,
// executable only by the group. A user not in that group gets "Permission
// denied" from inside Wireshark, against a helper they never asked for and
// have no reason to expect - capturing loopback traffic on their own machine
// needs none of the raw-interface capability that permission model exists
// for, so a plain copy in the user's own bin directory works and needs no
// privilege to make. Adding the account to the wireshark group is the better
// fix and needs root and a fresh login; this needs neither, and a capture
// available now beats one available after logging out.
func usableDumpcap() string {
	// Not a Windows problem. Wireshark's own installer puts dumpcap.exe beside
	// Wireshark.exe, where Wireshark finds it without help, and the
	// group-execute permission this works around is a Unix packaging habit
	// with no counterpart there. Copying a file into a bin directory nobody
	// asked for would be the only effect.
	if runtime.GOOS == "windows" {
		return ""
	}
	if p, err := exec.LookPath("dumpcap"); err == nil && session.Executable(p) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	mine := filepath.Join(home, ".local", "bin", "dumpcap")
	if session.Executable(mine) {
		return mine
	}
	for _, src := range []string{"/usr/bin/dumpcap", "/usr/local/bin/dumpcap", "/opt/wireshark/bin/dumpcap"} {
		b, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if os.MkdirAll(filepath.Dir(mine), 0o755) != nil {
			continue
		}
		if os.WriteFile(mine, b, 0o755) != nil {
			continue
		}
		return mine
	}
	return ""
}

// wiresharkHint is what to run by hand when we could not do it.
//
// It has to be runnable on the platform it is printed on, which is the whole
// point of handing it over: the verb answers `launched` false and this, and
// the reference tells a caller that this is the command to run. It said
// `wireshark -k -i lo -f 'udp port 5555'` everywhere - a binary name that is
// not on PATH on Windows, an interface that exists on neither Windows nor
// macOS, and POSIX quoting that cmd would pass through as part of the filter.
//
// bin is the binary we found, or empty when we found none. Naming the one we
// found saves the reader the search we just did; naming none is honest when
// there is nothing to name.
func wiresharkHint(bin, meshcoreLua, meshbenchLua string) string {
	exe := "wireshark"
	if bin != "" {
		exe = quoteArg(bin)
	}
	args := fmt.Sprintf("%s -k -i %s -f %s", exe,
		quoteArg(loopbackInterface()), quoteArg("udp port "+captureUDPPort))
	// The whole token is quoted, not the path inside it: Wireshark wants one
	// argument reading lua_script:<path>, and a shell splitting it at the
	// space in "Program Files" hands it two. Which is where these land on a
	// real Windows install - beside the binary, under Program Files.
	if meshcoreLua != "" {
		args += " -X " + quoteArg("lua_script:"+meshcoreLua)
	}
	if meshbenchLua != "" {
		args += " -X " + quoteArg("lua_script:"+meshbenchLua)
	}
	return args
}

// quoteArg quotes one argument the way the platform's own shell reads it, and
// only where it needs quoting - a bare word in quotes is noise in something
// somebody is meant to read and retype.
func quoteArg(s string) string {
	if !strings.ContainsAny(s, " \t") {
		return s
	}
	if runtime.GOOS == "windows" {
		return "\"" + s + "\""
	}
	return "'" + s + "'"
}
