package updates

import (
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// Handing something to the desktop: a folder to look in, or a page to read.
//
// Two things here rather than one, and both are the application admitting where
// it stops. Release notes are prose that will outgrow any panel, and a file
// manager is a thing every desktop already has and does better. What is refused
// is anything that is neither: the target arrives from a release feed, which is
// somebody else's data even when the somebody is us.

// openExternal asks the desktop to open a folder or a page, and returns why it
// could not - empty when it did.
//
// Not an error when the launcher is missing: failing to open a window is not a
// failed download, and the path has already been said out loud.
func openExternal(target string) string {
	if why := openable(target); why != "" {
		return why
	}
	bin, args := openCommand(runtime.GOOS, target)
	path, err := exec.LookPath(bin)
	if err != nil {
		return bin + " is not on this machine, so nothing here can open it for you"
	}
	// No context on purpose: a file manager or a browser outlives the click
	// that opened it, and one tied to this session's context would be killed
	// when the session ended.
	//nolint:gosec,noctx // the target is checked above and passed as an argument, not a shell string
	cmd := exec.Command(path, args...)
	if err := cmd.Start(); err != nil {
		return err.Error()
	}
	// Reaped on its own goroutine: a file manager outlives the click that
	// opened it, and a simulation that cannot exit until somebody closes a
	// folder is a simulation that looks hung.
	go func() { _ = cmd.Wait() }()
	return ""
}

// openable refuses anything that is not a local path or a web page.
func openable(target string) string {
	if target == "" {
		return "there is nothing to open"
	}
	if strings.HasPrefix(target, "/") || filepathish(target) {
		return ""
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return target + " is not a folder or a web page"
	}
	return ""
}

// filepathish is the Windows half of "this is a local path": a drive letter,
// which url.Parse would otherwise read as a scheme.
func filepathish(target string) bool {
	return len(target) > 2 && target[1] == ':' &&
		(target[2] == '\\' || target[2] == '/')
}

// openCommand is what each desktop calls its opener.
func openCommand(goos, target string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{target}
	case "windows":
		// rundll32 rather than `start`, which is a shell builtin and would
		// need a shell to run it.
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		return "xdg-open", []string{target}
	}
}
