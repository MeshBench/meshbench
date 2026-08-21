//go:build !windows

package session

import "golang.org/x/sys/unix"

// executable reports whether this process could run the file at p.
//
// unix.Access rather than reading the mode bits, because the question is
// whether *this* user may execute it - which depends on the process's own uid
// and groups, not on the file's permissions alone. A dumpcap that is
// root:wireshark and mode rwxr-xr-- is executable to a member of that group
// and not to anybody else, and the mode bits alone cannot tell them apart.
func executable(p string) bool { return unix.Access(p, unix.X_OK) == nil }
