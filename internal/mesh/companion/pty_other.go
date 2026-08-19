//go:build !linux && !darwin

package companion

import "errors"

// Windows has no pty a companion client could open as a serial port, so this
// says so rather than failing somewhere further in. The TCP transport is the
// one to use there, and it is the same Pipe underneath - a companion served
// over TCP behaves identically, it is only the address that differs.
func OpenSerial(Serial) (*PTYLink, error) {
	return nil, errors.New(
		"companion: a virtual serial port needs a pty, which this platform has none of - serve the companion over TCP instead")
}
