// Going away tidily.
//
// A workbench holds one operating-system process per node. If it exits without
// closing them, those processes keep running with nobody attached: 366 of them
// accumulated on this machine from three sessions, because the workbench
// ignored SIGTERM and a polite kill left every node behind.
//
// So: catch the signals a person or a script actually sends, close the engine,
// and only then exit. The engine's own Close drops each bridge, which is how a
// node is told to stop - it reports its final counters and exits, rather than
// being killed with those counters unsaid.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/A13xB0/meshcoresim/internal/session"
)

// onSignal closes the session when the process is asked to stop.
//
// Bounded, because a shutdown that hangs is indistinguishable from one that
// ignored the signal, and the second kill is usually SIGKILL - which leaves
// exactly the processes this exists to clean up.
func onSignal(ctx context.Context, cancel context.CancelFunc, s *session.Sim) {
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		got := <-sig
		fmt.Fprintln(os.Stderr, "\nclosing:", got)
		done := make(chan struct{})
		go func() {
			s.Close()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			fmt.Fprintln(os.Stderr, "firmware did not close in 20s; exiting anyway")
		}
		cancel()
		os.Exit(0)
	}()
}
