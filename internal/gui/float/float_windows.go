package float

import (
	"syscall"

	"gioui.org/app"
)

var (
	user32       = syscall.NewLazyDLL("user32.dll")
	setWindowPos = user32.NewProc("SetWindowPos")
)

const (
	hwndTopmost   = ^uintptr(0) // (HWND)-1
	swpNoMove     = 0x0002
	swpNoSize     = 0x0001
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040
)

func keep(e app.ViewEvent) bool {
	v, ok := e.(app.Win32ViewEvent)
	if !ok || v.HWND == 0 {
		return false
	}
	r, _, _ := setWindowPos.Call(v.HWND, hwndTopmost, 0, 0, 0, 0,
		uintptr(swpNoMove|swpNoSize|swpNoActivate|swpShowWindow))
	return r != 0
}
