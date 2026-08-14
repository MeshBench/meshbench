//go:build !windows

package engine

import "golang.org/x/sys/unix"

// mkfifo makes the named pipe Wireshark reads live captures from.
func mkfifo(path string) error { return unix.Mkfifo(path, 0o600) }
