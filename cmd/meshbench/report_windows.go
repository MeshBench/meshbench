package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// A Windows release is linked with -H windowsgui so the workbench does not
// drag a console window behind it. The cost is that the binary starts with no
// standard handles at all: everything written to stderr goes nowhere, and a
// command that refuses simply vanishes. "It starts and then disappears" is
// what that looks like from the outside, and it is indistinguishable from a
// crash.
//
// So two things happen at startup. If there is a parent console, which there
// is whenever somebody ran this from a terminal, we attach to it and the
// output appears where they expected it. If there is not, which is the Start
// menu and Explorer, the message goes to a file and the exit code carries.

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
)

const attachParentProcess = ^uintptr(0) // (DWORD)-1

// adoptConsole reattaches the standard handles to the console that started
// this process, when there is one.
func adoptConsole() {
	r, _, _ := procAttachConsole.Call(attachParentProcess)
	if r == 0 {
		return
	}
	// The handles are nil under -H windowsgui, so they are opened rather than
	// redirected: reopening the console device is what gives Go's os.Stdout
	// something real to write to.
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = f
		os.Stderr = f
	}
}

// reportFatal writes somewhere a person can find when there is no console.
//
// Returns the path so the caller can name it, because a log nobody is told
// about is the same silence one step removed.
func reportFatal(msg string) string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(base, "meshbench")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(dir, "meshbench-error.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	fmt.Fprintf(f, "%s  %s\n", time.Now().Format(time.RFC3339), msg)
	return path
}
