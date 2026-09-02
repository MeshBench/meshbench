//go:build !linux

// The modem's serial port on everything else.
//
// goldencap talks to real silicon over a termios port, which is Linux here.
// This exists so the tree compiles everywhere rather than so the tool runs
// everywhere: a build that cannot say why it will not work is worse than one
// that will not build, and go build ./... failing on Windows cost a week of
// nobody noticing internal/app/session had stopped compiling there.
package main

import (
	"fmt"
	"runtime"
)

func openKISS(path string) (*kissPort, error) {
	return nil, fmt.Errorf(
		"goldencap drives the modem over a termios serial port, which is "+
			"implemented for linux and not for %s; run it from a machine with "+
			"the modem plugged in", runtime.GOOS)
}

func (k *kissPort) readRaw(buf []byte) (int, error) {
	return 0, fmt.Errorf("goldencap: no serial port on %s", runtime.GOOS)
}
