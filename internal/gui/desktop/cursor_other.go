//go:build !(linux || freebsd || openbsd || netbsd)

package desktop

// MatchSystemCursor does nothing off the free desktops.
//
// Windows and macOS hand an application the system cursor at the size the
// system chose; there is nothing to reconcile and nothing to get wrong.
func MatchSystemCursor() (string, int) { return "", 0 }
