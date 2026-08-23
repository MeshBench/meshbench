package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// runHeadless is ADR-0019: the verbs, the socket, and no window.
//
// Not "the workbench without a picture". The point is that a firmware
// developer changes a constant in MeshCore, opens a pull request, and CI tells
// them whether the mesh still delivers - on a runner with no display, no GPU
// and no toolkit.
//
// The ADR's central argument was that control verbs were serviced on the frame
// thread, which made a harness hostage to the renderer. That is no longer true
// of the store: state.Store.Run owns its own goroutine and its own ticker and
// advances simulated time whether or not anything is drawn, and the socket is
// pumped from a worker. So this is the same session the window builds -
// literally, through session.Boot - with nothing attached to look at it.
func runHeadless(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("headless", flag.ExitOnError)
	fixture := fs.String("fixture", "", "open this fixture or project at startup")
	seed := fs.Uint64("seed", 0, "override the scenario's seed")
	socket := fs.String("control-socket", "",
		"where to answer: a path for a unix socket, or \"tcp\" for loopback "+
			"with a token (the default on Windows). Two runs on one machine "+
			"need two addresses")
	forDur := fs.Duration("for", 0,
		"exit after this long; the default is to run until interrupted")
	play := fs.Bool("play", false, "start the run immediately")
	unwatched := fs.Bool("unverified-wiring", false,
		"run boards whose wiring nobody has watched boot")
	quiet := fs.Bool("quiet", false, "do not echo status lines to stderr")
	if err := parse(fs, args, "run the verbs with no window, for scripts and CI"); err != nil {
		return err
	}

	// The linter wants this run's context threaded into Boot, because a warm
	// started by a verb registered here builds its own. That is deliberate: a
	// warm outlives the verb that asked for it and is cancelled by the next
	// warm, so its lifetime is the session's and not any one caller's. Same
	// finding as the workbench's, which is already in the baseline for the
	// same reason.
	//nolint:contextcheck // a warm's lifetime is the session's, not a request's
	st, _ := session.Boot(session.Options{
		Headless: true, UnverifiedWiring: *unwatched,
	})

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go st.Run(ctx)

	srv, err := session.ServeControlAt(ctx, st, *socket)
	if err != nil {
		// Fatal here, unlike in the workbench. A window with no socket is
		// still a workbench somebody can use; a headless run with no socket is
		// a process nothing can reach, and reporting that as a warning and
		// then waiting is the shape of a hang.
		return err
	}
	defer func() { _ = srv.Close() }()

	if !*quiet {
		go echoStatus(ctx, st)
	}
	fmt.Fprintln(os.Stderr, "meshbench", version.Detail(), "headless")

	if *seed != 0 {
		if _, err := st.Do(ctx, "sim.seed", map[string]any{"seed": float64(*seed)}); err != nil {
			return err
		}
	}
	if *fixture != "" {
		// Inline, not on a worker. The workbench defers this because a
		// national fixture takes a moment and an application that has not
		// appeared is indistinguishable from one that has crashed; here there
		// is nothing to appear, and a script that connects before the network
		// is loaded would find an empty one.
		if _, err := st.Do(ctx, "project.open", *fixture); err != nil {
			return fmt.Errorf("loading %s: %w", *fixture, err)
		}
	}
	if *play {
		if _, err := st.Do(ctx, "sim.play", nil); err != nil {
			return err
		}
	}

	// Now wait. Either for a length of time, or for whoever started this to
	// stop it - the signal handling is main's, and cancelling the context is
	// how it arrives here.
	if *forDur > 0 {
		t := time.NewTimer(*forDur)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
		}
		return nil
	}
	<-ctx.Done()
	return nil
}

// echoStatus prints what the session says about itself.
//
// The window has a status line and a log panel; a headless run has stderr, and
// without this a job that refused something says nothing to whoever is
// watching it. Read off the snapshot rather than hooked into the store,
// because the snapshot never blocks a verb - and a log that could delay the
// thing it is logging is worse than no log.
//
// By the sentence, not by the sequence number. Status is the last line said
// and stays said; the sequence moves on every publish, which for a playing run
// is every tick. Keying on it printed "running to 2.0 s" seventeen times for
// one command.
func echoStatus(ctx context.Context, st *state.Store) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	last := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s := st.Snapshot()
			if s == nil || s.Status == "" || s.Status == last {
				continue
			}
			last = s.Status
			fmt.Fprintf(os.Stderr, "%8.2fs  %s\n", float64(s.NowMs)/1000, s.Status)
		}
	}
}
