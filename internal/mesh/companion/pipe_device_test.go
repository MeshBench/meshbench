package companion_test

import "os"

// openDevice opens the pty slave the way client software would.
func openDevice(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR, 0)
}
