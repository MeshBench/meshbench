// The hardware capability matrix: not can this board run, but which
// capabilities does it actually demonstrate - untested distinguished from
// failed, because a blank cell reads as working and it should read as
// unknown.
package session

import (
	"context"
	"fmt"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/sim/boardcheck"
)

// defaultBoardVersion is what the matrix probes when nothing else is asked
// for, and it tracks MeshCore's latest release rather than the one a result
// was first measured on - a matrix about a superseded firmware answers a
// question nobody asked.
//
// Published board images are tagged bare - "v1.17.1" - unlike a native build
// ref such as "repeater-v1.17.1"; the "repeater-" prefix is this project's own
// convention for a build for this machine, and the board catalogue does not
// use it.
const defaultBoardVersion = "v1.17.1"

func registerBoardMatrix(st *state.Store, s *Sim) {
	st.Handle("board.matrix", func(w *state.World, p any) (any, error) {
		version, _ := stringField(p, "version")
		if version == "" {
			version = defaultBoardVersion
		}
		publishMatrix(w, version)
		return map[string]any{"version": version, "boards": len(w.BoardMatrix)}, nil
	})

	// board.probe: one board, one real emulator boot. Slow, so it goes on a
	// job, and every capability it measures overwrites that board's cached
	// row when it finishes.
	st.Handle("board.probe", func(w *state.World, p any) (any, error) {
		board, _ := stringField(p, "board")
		if board == "" {
			return nil, fmt.Errorf("board.probe needs a board")
		}
		version, _ := stringField(p, "version")
		if version == "" {
			version = defaultBoardVersion
		}
		if s.boardProbing {
			return nil, fmt.Errorf("a board probe is already running")
		}
		s.boardProbing = true
		w.Jobs = append(w.Jobs, state.Job{ID: "boardprobe", What: "probing " + board, Total: 1})
		terr := s.terrain()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			report := boardcheck.Probe(ctx, terr, board, version)
			if err := report.Save(); err != nil {
				// The measurement still happened; only the cache write failed,
				// so the result is said aloud rather than lost silently.
				_, _ = st.Do(context.Background(), "ui.said",
					"board probe: could not save its cache entry: "+err.Error())
			}
			_, _ = st.Do(context.Background(), "board.probe_finished",
				map[string]any{"board": board, "version": version})
		}()
		return map[string]any{"probing": true, "board": board, "version": version}, nil
	})

	st.Handle("board.probe_finished", func(w *state.World, p any) (any, error) {
		s.boardProbing = false
		w.Jobs = finishJob(w.Jobs, "boardprobe")
		board, _ := stringField(p, "board")
		version, _ := stringField(p, "version")
		publishMatrix(w, version)
		var passed, failed int
		for _, c := range boardcheck.Load(board, version).Results {
			switch c.State {
			case boardcheck.Passed:
				passed++
			case boardcheck.Failed:
				failed++
			}
		}
		w.Say(fmt.Sprintf("%s: %d capabilities passed, %d failed", board, passed, failed))
		return map[string]any{"board": board, "passed": passed, "failed": failed}, nil
	})
}

func publishMatrix(w *state.World, version string) {
	reports := boardcheck.MatrixReports(version)
	rows := make([]state.BoardRow, 0, len(reports))
	for _, r := range reports {
		row := state.BoardRow{Board: r.Board, Version: r.Version, Stale: r.Stale}
		if !r.MeasuredAt.IsZero() {
			row.MeasuredAt = r.MeasuredAt.Format("2006-01-02 15:04")
		}
		for _, c := range boardcheck.Capabilities {
			res := r.Results[c]
			row.Cells = append(row.Cells, state.BoardCapabilityCell{
				Capability: string(c), State: string(res.State), Detail: res.Detail,
			})
		}
		rows = append(rows, row)
	}
	w.BoardMatrix = rows
	w.BoardMatrixVersion = version
}
