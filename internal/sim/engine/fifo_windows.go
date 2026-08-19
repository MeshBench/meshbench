package engine

import "errors"

// Windows has named pipes, but not ones a program can create with a path on
// disk and hand to Wireshark the way `wireshark -k -i /path/to/fifo` expects
// - its pipes live in \\.\pipe\ and are a different API on both ends. Live
// capture therefore says so here, and capturing to a file, which is the same
// pcapng stream, works on every platform.
func mkfifo(string) error {
	return errors.New("live capture to a pipe needs a FIFO, which Windows has none of - capture to a file instead")
}
