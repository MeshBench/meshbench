// Regression scenarios: exporting one from a sweep, and running a directory
// of them.
//
// "Will this stay fixed?" is post-release QA, not pre-release development -
// the closest fit to how MeshBench is actually used. Everything a case
// needs already exists in pieces: a fixture, an experiment definition, and
// - once #67 lands - an experiment ID. This is the one portable file and
// the runner that takes a directory of them.
package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MeshBench/meshbench/internal/fixture"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/regression"
)

func regressionsDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cfg, "meshcoresim", "regressions")
	return dir, os.MkdirAll(dir, 0o755)
}

func registerRegression(st *state.Store, s *Sim) {
	// regression.export: the current sweep's baseline arm, as a case - "a
	// tripped invariant becomes a saved case in one gesture."
	//
	// A sweep's sender has to be a companion - the engine refuses anything
	// else - and a companion has no console, so its send cannot be replayed
	// through the fixture.Send mechanism this runner checks a case with. A
	// case exported straight from the sweep's own numbers would therefore be
	// calibrated against a stimulus it can never reproduce, and every run of
	// it would fail for a reason that is an artefact of the export, not a
	// regression. So this measures instead: it runs the case once, on the
	// same advert-driven stimulus regression.RunCase will actually use, and
	// calibrates the assertion against what *that* run produces. Slower than
	// a save, but "the case reproduces its own number" is the one thing a
	// regression case has to be true.
	st.Handle("regression.export", func(w *state.World, p any) (any, error) {
		if s.fixturePath == "" {
			return nil, fmt.Errorf("no fixture is loaded")
		}
		if s.regRunning {
			return nil, fmt.Errorf("a regression run is already in progress")
		}
		e := s.experiment()
		e.mu.Lock()
		if len(e.Arms) == 0 {
			e.mu.Unlock()
			return nil, fmt.Errorf("no arm is defined - experiment.vary or experiment.define first")
		}
		arm := e.Arms[0]
		seeds := append([]uint64(nil), e.Seeds...)
		runFor := e.RunForMs
		e.mu.Unlock()
		if len(seeds) == 0 {
			return nil, fmt.Errorf("no seeds are defined")
		}

		name, _ := stringField(p, "name")
		if name == "" {
			name = sanitiseCaseName(arm.Label)
		}
		id, _ := experimentID(e.inputsFor(s))

		// A placeholder that always passes: this run is a measurement, not a
		// check. Reusing the loaded fixture and seeds directly rather than a
		// draft Case that would need to resolve a path baseDir does not have.
		draft := regression.NewCase(name, s.fixturePath, seeds,
			arm.RepeaterVersion, arm.CompanionVersion, runFor, nil,
			[]fixture.Assertion{{Kind: "delivered", AtLeast: 0}}, 0, id)

		s.regRunning = true
		w.Jobs = append(w.Jobs, state.Job{ID: "regressions", What: "calibrating " + name, Total: 1})
		terr := s.terrain()
		go func() {
			res, err := regression.RunCase(context.Background(), terr, draft, "")
			_, _ = st.Do(context.Background(), "regression.export_finished",
				map[string]any{"case": draft, "result": res, "err": err})
		}()
		return map[string]any{"calibrating": true, "name": name}, nil
	})

	st.Handle("regression.export_finished", func(w *state.World, p any) (any, error) {
		s.regRunning = false
		w.Jobs = finishJob(w.Jobs, "regressions")
		m, _ := p.(map[string]any)
		draft, _ := m["case"].(regression.Case)
		res, _ := m["result"].(regression.CaseResult)
		if err, _ := m["err"].(error); err != nil {
			w.Say("regression export: " + err.Error())
			return nil, err
		}

		// The floor is the worst delivery count this measurement run
		// actually saw - a bound a passing run can always clear - and the
		// band is the relative spread across its seeds, the same shape
		// plan 2's confidence intervals use, derived rather than guessed.
		minD, maxD, sumD, n := -1, -1, 0, 0
		for _, sr := range res.Seeds {
			if sr.Err != "" {
				continue
			}
			if minD < 0 || sr.Delivered < minD {
				minD = sr.Delivered
			}
			if sr.Delivered > maxD {
				maxD = sr.Delivered
			}
			sumD += sr.Delivered
			n++
		}
		if n == 0 {
			w.Say(fmt.Sprintf("regression export %q: every seed failed to measure anything", draft.Name))
			return nil, fmt.Errorf("no seed produced a measurement")
		}
		mean := float64(sumD) / float64(n)
		bandPct := 0.0
		if mean > 0 {
			bandPct = float64(maxD-minD) / mean * 100
		}

		final := draft
		final.Assertions = []fixture.Assertion{{Kind: "delivered", AtLeast: minD}}
		final.ToleranceBandPct = bandPct

		dir, err := regressionsDir()
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, draft.Name+".json")
		if err := final.Save(path); err != nil {
			return nil, err
		}
		w.Say(fmt.Sprintf("saved regression case %q: at least %d deliveries, ±%.1f%% band (measured, not guessed)",
			draft.Name, minD, bandPct))
		return map[string]any{"path": path, "name": draft.Name, "at_least": minD, "band_pct": bandPct}, nil
	})

	// regression.run_dir: every case in a directory, on real firmware. Slow -
	// each case is a full simulated run - so it goes on a job, the same as an
	// experiment sweep.
	st.Handle("regression.run_dir", func(w *state.World, p any) (any, error) {
		dir, _ := stringField(p, "dir")
		if dir == "" {
			d, err := regressionsDir()
			if err != nil {
				return nil, err
			}
			dir = d
		}
		if s.regRunning {
			return nil, fmt.Errorf("regressions are already running")
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		total := 0
		for _, en := range entries {
			if !en.IsDir() && filepath.Ext(en.Name()) == ".json" {
				total++
			}
		}
		if total == 0 {
			return nil, fmt.Errorf("%s has no regression cases (*.json)", dir)
		}

		s.regRunning = true
		w.Jobs = append(w.Jobs, state.Job{ID: "regressions", What: "running regressions", Total: total})
		w.RegressionsDir = dir
		ctx := context.Background()
		terr := s.terrain()
		go func() {
			results, errs := regression.RunDir(ctx, terr, dir)
			_, _ = st.Do(context.Background(), "regression.finished",
				map[string]any{"results": results, "errs": errs})
		}()
		return map[string]any{"running": true, "dir": dir, "total": total}, nil
	})

	st.Handle("regression.finished", func(w *state.World, p any) (any, error) {
		s.regRunning = false
		w.Jobs = finishJob(w.Jobs, "regressions")
		m, _ := p.(map[string]any)
		results, _ := m["results"].([]regression.CaseResult)
		errs, _ := m["errs"].([]error)

		w.Regressions = w.Regressions[:0]
		passed, flagged, failed := 0, 0, 0
		for _, r := range results {
			detail := ""
			for _, sr := range r.Seeds {
				if sr.Err != "" {
					detail = fmt.Sprintf("seed %d: %s", sr.Seed, sr.Err)
					break
				}
				for _, c := range sr.Checks {
					if c.Verdict != regression.Pass {
						detail = fmt.Sprintf("seed %d, %s: %s", sr.Seed, c.Assertion.String(), c.Detail)
						break
					}
				}
				if detail != "" {
					break
				}
			}
			switch r.Verdict {
			case regression.Pass:
				passed++
			case regression.Flag:
				flagged++
			case regression.Fail:
				failed++
			}
			w.Regressions = append(w.Regressions, state.RegressionResult{
				Name: r.Case.Name, Verdict: string(r.Verdict), Detail: detail,
				Seeds: len(r.Seeds), TookMs: float64(r.Took.Milliseconds()),
			})
		}
		for _, e := range errs {
			w.Regressions = append(w.Regressions, state.RegressionResult{
				Name: "(could not load)", Verdict: "error", Detail: e.Error(),
			})
		}
		w.Say(fmt.Sprintf("regressions: %d passed, %d failed, %d flagged, %d error(s)",
			passed, flagged, failed, len(errs)))
		return map[string]any{"passed": passed, "failed": failed, "flagged": flagged,
			"errors": len(errs)}, nil
	})
}

func sanitiseCaseName(s string) string {
	if s == "" {
		s = "case"
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		case r == ' ' || r == '.':
			out = append(out, '-')
		}
	}
	name := string(out)
	if name == "" {
		name = "case"
	}
	return name + "-" + time.Now().UTC().Format("20060102-1504")
}
