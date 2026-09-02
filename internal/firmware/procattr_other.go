//go:build !linux && !windows

package firmware

import "syscall"

// ChildProcAttr on platforms without PDEATHSIG.
//
// macOS has no parent-death signal. The fallback is the socket: a node whose
// bridge closes exits its own read loop, which covers everything except a
// parent dying while a node is wedged — rarer, and the price of the platform.
func ChildProcAttr() *syscall.SysProcAttr { return nil }
