// The hardware capability matrix: not can this board run, but which
// capabilities does it actually demonstrate - untested distinguished from
// failed, because a blank cell reads as working and it should read as
// unknown.
package board

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/session"
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

func registerBoardMatrix(st *state.Store, s *session.Sim) {
	st.HandleSpec("board.matrix", state.Spec{
		What: "publish what every board in the catalogue was last measured to " +
			"demonstrate, for one firmware release, without booting anything",
		Params: []state.Param{
			{Name: "version", Type: state.ParamString, Primary: true,
				What: "the MeshCore release the rows are read for; absent or " +
					"empty takes the release the matrix defaults to, which " +
					"tracks the latest rather than whatever a result was first " +
					"measured on"},
		},
		Returns: []string{"version", "boards"},
		Answers: "`boards` is a count: the rows themselves go into the snapshot " +
			"as the board matrix, where a panel or `ui.state` reads them. " +
			"Nothing is measured here, so a board with no cached result for that " +
			"release reads as untested, and one that cannot be emulated at all " +
			"reads as boot-failed with the reason.",
		Example: &state.Example{
			Params: map[string]any{}, What: "read the matrix as it stands",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		version, _ := session.StringField(p, "version")
		if version == "" {
			version = defaultBoardVersion
		}
		publishMatrix(w, version)
		return map[string]any{"version": version, "boards": len(w.BoardMatrix)}, nil
	})

	st.HandleSpec("board.probe", state.Spec{
		What: "boot one board in the emulator for real and measure what it " +
			"demonstrates, overwriting that board's cached row",
		Params: []state.Param{
			{Name: "board", Type: state.ParamString, Required: true, Primary: true,
				What: "the board to boot, by its catalogue name; absent is refused"},
			{Name: "version", Type: state.ParamString,
				What: "the MeshCore release to probe, which has to be named to be " +
					"meant; absent takes the release the matrix defaults to"},
		},
		Returns: []string{"probing", "board", "version"},
		Answers: "It answers as soon as the job is started, not when the board " +
			"has been measured: the boot runs on its own goroutine for as long " +
			"as the probe budget allows, and the result arrives through " +
			"`board.probe_finished`, which republishes the matrix. Poll " +
			"`job.list` for `boardprobe`. A second call while one is running is " +
			"refused, because a board is probed one at a time.",
		Example: &state.Example{
			Params: map[string]any{"board": "Heltec_WSL3"},
			What:   "measure one board's capabilities",
		},
	}, func(w *state.World, p any) (any, error) {
		board, _ := session.StringField(p, "board")
		if board == "" {
			return nil, fmt.Errorf("board.probe needs a board")
		}
		version, _ := session.NamedField(p, "version")
		if version == "" {
			version = defaultBoardVersion
		}
		if s.BoardProbing() {
			return nil, fmt.Errorf("a board probe is already running")
		}
		s.SetBoardProbing(true)
		w.Jobs = append(w.Jobs, state.Job{ID: "boardprobe", What: "probing " + board, Total: 1})
		terr := s.Terrain()
		go func() {
			// Long enough for every phase Probe can honestly spend its full
			// budget on, not a flat guess - a board that only failed here
			// because the caller's timeout was too short reported "failed"
			// for a limit nobody told the operator about.
			ctx, cancel := context.WithTimeout(context.Background(), boardcheck.ProbeBudget())
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

	st.HandleInternalSpec("board.probe_finished", state.Spec{
		What: "take a finished probe back onto the store's goroutine: clear the " +
			"job, republish the matrix over the row the probe has just written, " +
			"and say how the board did",
		Params: []state.Param{
			{Name: "board", Type: state.ParamString, Required: true, Primary: true,
				What: "the board that was probed"},
			{Name: "version", Type: state.ParamString,
				What: "the release it was probed at; an empty one republishes the " +
					"matrix for an empty version, which holds no rows"},
		},
		Returns: []string{"board", "passed", "failed"},
		Answers: "`passed` and `failed` are counted off the cached row the probe " +
			"saved, so capabilities it never reached are in neither total.",
	}, func(w *state.World, p any) (any, error) {
		s.SetBoardProbing(false)
		w.Jobs = session.FinishJob(w.Jobs, "boardprobe")
		board, _ := session.StringField(p, "board")
		version, _ := session.NamedField(p, "version")
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
