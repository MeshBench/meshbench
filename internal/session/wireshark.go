// Opening Wireshark on a running simulation.
//
// The frames have been streamable for a while - StartCaptureUDP puts every
// one on a UDP socket - but nothing in the application opened Wireshark on
// them, and nothing in the menus reached the verb, so in practice the feature
// workbench 1 had was gone: you had to know the port, know that udpdump is
// the extcap that reads it, and start Wireshark yourself.
//
// Three things are needed for it to just work, and the previous version had
// only the first: the stream, on the port Wireshark's own extcap listens on;
// the dissector, where Wireshark looks for plugins; and Wireshark, started.
package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// udpdumpAddr is where Wireshark's udpdump extcap listens.
//
// 5555 is not a choice - it is the extcap's own default, so streaming there
// means "wireshark -k -i udpdump" works with nothing configured. The previous
// default of 19000 meant a correct stream that Wireshark would never look at,
// which is the most confusing kind of working.
const udpdumpAddr = "127.0.0.1:5555"

// dissectorPluginDir is where Wireshark loads personal Lua plugins from.
func dissectorPluginDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, ".local", "lib", "wireshark", "plugins"), nil
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Wireshark", "plugins"), nil
	}
	return filepath.Join(home, ".local", "lib", "wireshark", "plugins"), nil
}

// installDissector copies the Lua dissector where Wireshark will find it.
//
// Copied rather than symlinked: a link into a source tree breaks the moment
// the tree moves, and this has to keep working for somebody running an
// installed binary with no checkout at all. Rewritten every time, so a
// dissector that has gained fields is not shadowed by an older copy somebody
// installed by hand months ago.
func installDissector(source string) (string, error) {
	dir, err := dissectorPluginDir()
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("reading the dissector: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, filepath.Base(source))
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

// dissectorSource is where the Lua plugin lives, looked for beside the binary
// and then in a checkout.
//
// An installed copy carries it next to the executable; a developer running
// from the tree has it under tools. Neither is guaranteed, and a missing
// dissector is not fatal - Wireshark still shows the frames, just as bytes -
// so this reports absence rather than failing.
func dissectorSource() string {
	var roots []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		roots = append(roots,
			filepath.Join(dir, "meshcore_dissector.lua"),
			filepath.Join(dir, "dissector", "meshcore_dissector.lua"),
			filepath.Join(dir, "..", "share", "meshbench", "meshcore_dissector.lua"))
	}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots,
			filepath.Join(wd, "tools", "dissector", "meshcore_dissector.lua"))
	}
	for _, p := range roots {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// wiresharkBinary is the executable to start, or empty if there is none.
func wiresharkBinary() string {
	names := []string{"wireshark"}
	if runtime.GOOS == "darwin" {
		// The .app bundle is where a Mac install actually puts it; the name
		// on PATH is often absent even when Wireshark is installed.
		names = append(names, "/Applications/Wireshark.app/Contents/MacOS/Wireshark")
	}
	for _, n := range names {
		if filepath.IsAbs(n) {
			if st, err := os.Stat(n); err == nil && !st.IsDir() {
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

// launchWireshark starts it on the udpdump interface and does not wait,
// returning why it could not rather than an error.
//
// Not an error, because failing to open a window is not a failed capture -
// the frames are already streaming by the time this is called, and returning
// an error from the verb would report the whole thing as broken when the
// recoverable part is "start Wireshark yourself".
//
// Detached deliberately: Wireshark outlives the run it was opened on, and a
// simulation that cannot exit until somebody closes a packet viewer is a
// simulation that looks hung.
func launchWireshark(bin string) string {
	cmd := exec.Command(bin, "-k", "-i", "udpdump")
	// Its own stdio, so a chatty GTK does not interleave with the session log.
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return err.Error()
	}
	// Reaped on its own, so the process table does not fill with zombies
	// across a session where somebody opens it repeatedly.
	go func() { _ = cmd.Wait() }()
	return ""
}

// wiresharkHint is what to run by hand when we could not do it.
func wiresharkHint(addr string) string {
	port := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		port = addr[i+1:]
	}
	if port == "5555" {
		return "wireshark -k -i udpdump"
	}
	return fmt.Sprintf("wireshark -k -i udpdump  (with udpdump's port set to %s)", port)
}
