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
// is whenever somebody ran this from a terminal, we adopt the handles it did
// not give us and the output appears where they expected it. If there is not,
// which is the Start menu and Explorer, everything written to stderr goes to a
// file instead - a refusal because reportFatal puts it there, and a panic
// because the error handle itself points at it.

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
	procGetStdHandle  = kernel32.NewProc("GetStdHandle")
	procSetStdHandle  = kernel32.NewProc("SetStdHandle")
)

const (
	attachParentProcess = ^uintptr(0)  // (DWORD)-1
	stdOutputHandle     = ^uintptr(10) // (DWORD)-11
	stdErrorHandle      = ^uintptr(11) // (DWORD)-12
)

// errorGoesSomewhere says a person can read what is written to stderr, so
// recordCrashes has nothing to do. Set by adoptConsole, which runs first.
var errorGoesSomewhere bool

// stdHandleMissing reports whether Windows has nothing behind one of the
// standard handles.
//
// This is the whole question, because a GUI-subsystem binary is not always
// handed nothing. A parent that redirected our output - "meshbench.exe
// -version > build.txt", a pipe into findstr, a script capturing the message a
// failure writes - passes real handles through STARTUPINFO, and those are the
// ones the user asked for. Replacing them sends the output to the screen and
// leaves the file empty, which is the same silence the other way round.
func stdHandleMissing(which uintptr) bool {
	h, _, _ := procGetStdHandle.Call(which)
	return h == 0 || h == uintptr(syscall.InvalidHandle)
}

// adoptConsole gives the standard handles the console that started this
// process, for the ones that have nothing behind them.
func adoptConsole() {
	// Asked before attaching, and that order is the point: AttachConsole
	// installs console handles of its own, so afterwards there is no way to
	// tell what the parent actually gave us - and what the parent gave us is
	// exactly what must be left alone.
	outMissing := stdHandleMissing(stdOutputHandle)
	errMissing := stdHandleMissing(stdErrorHandle)
	errorGoesSomewhere = !errMissing
	if r, _, _ := procAttachConsole.Call(attachParentProcess); r == 0 {
		return
	}
	// Opened rather than redirected: under -H windowsgui the handles were nil
	// when the runtime built os.Stdout, so reopening the console device is
	// what gives it something real to write to. AttachConsole has already
	// pointed the process's own handles at the console, which is what a panic
	// is printed to.
	var con *os.File
	console := func() *os.File {
		if con == nil {
			con, _ = os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		}
		return con
	}
	if outMissing {
		if f := console(); f != nil {
			os.Stdout = f
		}
	}
	if errMissing {
		if f := console(); f != nil {
			os.Stderr = f
			errorGoesSomewhere = true
		}
	}
}

// recordCrashes points the process's error handle at the log, so a panic says
// where it happened instead of vanishing.
//
// This is the failure the rest of this file could not reach. A refusal travels
// through reportFatal, but the runtime prints a panic straight to the handle
// Windows reports for stderr and then exits, never through os.Stderr - so from
// the Start menu, with no console and no handle, a crash leaves nothing
// anywhere and the application has "started and disappeared".
func recordCrashes() {
	if errorGoesSomewhere {
		return
	}
	path, err := errorLogPath()
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	// Deliberately never closed: it has to still be open when the process
	// dies, because dying is when the write this exists for happens.
	fmt.Fprintf(f, "%s  %s started with no console; anything below this "+
		"line is what it wrote to stderr\n",
		time.Now().Format(time.RFC3339), invoked())
	os.Stderr = f
	procSetStdHandle.Call(stdErrorHandle, f.Fd())
	errorGoesSomewhere = true
}

// errorLogPath is where a message goes when there is no console to put it on.
func errorLogPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "meshbench")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "meshbench-error.log"), nil
}

// reportFatal writes somewhere a person can find when there is no console.
//
// Returns the path so the caller can name it, because a log nobody is told
// about is the same silence one step removed.
func reportFatal(msg string) string {
	path, err := errorLogPath()
	if err != nil {
		return ""
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	fmt.Fprintf(f, "%s  %s\n", time.Now().Format(time.RFC3339), msg)
	return path
}
