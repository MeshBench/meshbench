//go:build linux || darwin

package companion

import "golang.org/x/sys/unix"

// rawTermios is the flag work itself, which is identical on Linux and Darwin
// even though the ioctls that fetch and store the struct are not. Kept in one
// place so the two platforms cannot drift into subtly different definitions
// of "raw" - which would show up as a framing bug on one of them and nowhere
// else.
func rawTermios(t *unix.Termios) {
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8
}
