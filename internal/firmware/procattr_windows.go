package firmware

import "syscall"

// ChildProcAttr keeps a child's console off the screen.
//
// Every firmware node is a console program, and Windows gives a console
// program its own window when a GUI application starts it. A national network
// is several hundred of those, so a run that works perfectly buries the
// workbench under a wall of empty black windows.
//
// CREATE_NO_WINDOW rather than HideWindow: HideWindow asks the child to start
// its window minimised and hidden, which a console host may still flash, while
// this one is never created at all. The child keeps its stdout and stderr,
// which is what the bridge and the console log read.
//
// There is no parent-death signal here. The fallback is the same one macOS
// relies on: a node whose bridge closes leaves its own read loop.
func ChildProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x08000000}
}
