package firmware

import "syscall"

// ChildProcAttr ties a firmware process's life to the simulator's.
//
// PDEATHSIG delivers SIGKILL to the child the moment its parent dies — however
// the parent dies, including SIGKILL and crashes, which no amount of cleanup
// code in the parent can handle by definition. Without it, killing a workbench
// with three hundred attached repeaters left three hundred MeshCore processes
// running for someone to discover in htop.
//
// The graceful path is unchanged: Stop still closes the bridge and waits, and a
// node still exits on its own when its socket closes. This is the backstop for
// every path that never reaches Stop.
func ChildProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
