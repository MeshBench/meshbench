package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A checkpoint is the whole session frozen: the network, how it is being run,
// and where the clock had got to. Restoring one rebuilds the session and
// replays deterministically to that moment - same seed, same scenario, same
// result - which is why it need not store the firmware's own RAM or the
// waveforms in flight: they are reconstructed, not saved.
//
// The cost of that choice is that the replay runs in the mesh's own time, so
// restoring a long run takes a while, shown as a run in progress. An instant
// restore would have to freeze the emulators mid-write, which a native firmware
// process cannot do at all - so replay is the honest mechanism, and the one
// that keeps determinism a feature rather than a thing a snapshot could break.
type checkpoint struct {
	Name  string    `json:"name"`
	Saved time.Time `json:"saved"`

	// The scenario in its canonical form - the same nodes the engine is built
	// from, so a restore is a fresh build rather than a lossy reconstruction.
	Nodes      []scenario.Node   `json:"nodes"`
	Seed       uint64            `json:"seed"`
	FreqMHz    float64           `json:"freq_mhz"`
	MarginKm   float64           `json:"margin_km"`
	Areas      []state.Area      `json:"areas,omitempty"`
	Sends      []state.Send      `json:"sends,omitempty"`
	Assertions []state.Assertion `json:"assertions,omitempty"`

	// Where the run had got to, and what kind of run it was.
	NowMs      uint32 `json:"now_ms"`
	RunUntilMs uint32 `json:"run_until_ms"`
	Playing    bool   `json:"playing"`
	StepMs     uint32 `json:"step_ms"`

	// The physics, because a checkpoint restored under different settings is a
	// different study wearing the same name.
	RFMode       string          `json:"rf_mode,omitempty"`
	Realism      state.RFRealism `json:"realism"`
	ExcessLossDB float64         `json:"excess_loss_db"`
	Calibrated   bool            `json:"calibrated"`
	RealFirmware bool            `json:"real_firmware"`
}

func checkpointsDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "meshbench", "checkpoints"), nil
}

// safeCheckpointName maps a chosen name onto a filename that stays inside the
// checkpoints directory - a name is a label, not a path, and a "../" in one
// must not reach out of it.
var notNameChar = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func safeCheckpointName(name string) string {
	s := notNameChar.ReplaceAllString(name, "_")
	return strings.Trim(s, "_")
}

func registerCheckpoint(st *state.Store, s *Sim) {
	// session.checkpoint: freeze the whole session to a named file, so it can
	// be taken back to this exact moment later.
	st.Handle("session.checkpoint", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "name")
		if name == "" {
			return nil, fmt.Errorf("session.checkpoint needs a name")
		}
		if len(s.nodes) == 0 {
			return nil, fmt.Errorf("there is no network to checkpoint")
		}
		file := safeCheckpointName(name)
		if file == "" {
			return nil, fmt.Errorf("%q is not a usable checkpoint name", name)
		}
		cp := checkpoint{
			Name: name, Saved: time.Now(),
			Nodes:    append([]scenario.Node(nil), s.nodes...),
			Seed:     w.Seed,
			FreqMHz:  s.freqMHz,
			MarginKm: w.MarginKm,
			Areas:    w.Areas, Sends: w.Sends, Assertions: w.Assertions,
			NowMs: w.NowMs, RunUntilMs: w.RunUntilMs, Playing: w.Playing,
			StepMs:       st.StepMs(),
			RFMode:       s.rfMode,
			Realism:      s.realism,
			ExcessLossDB: w.ExcessLossDB, Calibrated: w.Calibrated,
			RealFirmware: w.RealFirmware,
		}
		dir, err := checkpointsDir()
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		path := filepath.Join(dir, file+".json")
		b, err := json.MarshalIndent(cp, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, b, 0o644); err != nil { //nolint:gosec // path composed from a sanitised name in this dir
			return nil, err
		}
		w.Say(fmt.Sprintf("checkpoint %q at %.1fs", name, float64(w.NowMs)/1000))
		return map[string]any{"checkpoint": name, "path": path,
			"now_ms": w.NowMs, "nodes": len(s.nodes)}, nil
	})

	// session.restore: rebuild the session a checkpoint holds and replay to the
	// moment it was taken. Deterministic, so the mesh comes back to exactly
	// where it was - at the cost of the replay taking the run's own time.
	st.Handle("session.restore", func(w *state.World, p any) (any, error) {
		path, err := checkpointPath(p)
		if err != nil {
			return nil, err
		}
		cp, err := loadCheckpoint(path)
		if err != nil {
			return nil, err
		}
		if len(cp.Nodes) == 0 {
			return nil, fmt.Errorf("%s holds no network", path)
		}

		// The same route project.open takes, so a restored session is not a
		// second kind of session with its own set of things left unset.
		loaded := Loaded{
			scene:      cp.Nodes,
			nodes:      statesFromScene(cp.Nodes),
			areas:      cp.Areas,
			margin:     cp.MarginKm,
			sends:      cp.Sends,
			assertions: cp.Assertions,
		}
		s.installFn(st, w, loaded, cp.Name)

		// Reapply what install hardcodes: the seed, the frequency and the
		// physics all come back from the checkpoint rather than the defaults a
		// blank network is built with.
		w.Seed = cp.Seed
		if cp.FreqMHz > 0 {
			s.freqMHz = cp.FreqMHz
		}
		s.realism = cp.Realism
		if cp.Calibrated {
			s.excessLossDB = cp.ExcessLossDB
		}
		w.ExcessLossDB, w.Calibrated = cp.ExcessLossDB, cp.Calibrated
		w.RealFirmware = cp.RealFirmware
		if cp.StepMs > 0 {
			st.SetStepMs(cp.StepMs)
		}
		if err := s.rebuild(w); err != nil {
			return nil, err
		}
		if cp.RFMode != "" {
			_, _ = setRFMode(w, s, cp.RFMode)
		}
		s.resetSendClock()
		w.Links = nil
		s.warm(st, len(s.nodes))

		// The rebuild put the clock at zero. Running to the checkpoint's time
		// reproduces the run up to that moment and stops there, because the
		// tick turns Playing off once NowMs reaches RunUntilMs.
		replaying := cp.NowMs > 0
		if replaying {
			w.RunUntilMs = cp.NowMs
			w.Playing = true
			w.Say(fmt.Sprintf("restoring %q - replaying to %.1fs",
				cp.Name, float64(cp.NowMs)/1000))
		} else {
			w.Say(fmt.Sprintf("restored %q", cp.Name))
		}
		return map[string]any{"restored": cp.Name, "nodes": len(s.nodes),
			"now_ms": w.NowMs, "target_ms": cp.NowMs, "replaying": replaying}, nil
	})

	// session.checkpoints: what can be restored.
	st.Handle("session.checkpoints", func(_ *state.World, _ any) (any, error) {
		dir, err := checkpointsDir()
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"checkpoints": []string{}}, nil
			}
			return nil, err
		}
		var names []string
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				names = append(names, strings.TrimSuffix(e.Name(), ".json"))
			}
		}
		sort.Strings(names)
		return map[string]any{"checkpoints": names}, nil
	})
}

// checkpointPath resolves a restore request to a file: an explicit path as
// given, or a name looked up in the checkpoints directory.
func checkpointPath(p any) (string, error) {
	if path, _ := stringField(p, "path"); path != "" {
		return path, nil
	}
	name, _ := stringField(p, "name")
	if name == "" {
		return "", fmt.Errorf("session.restore needs a name or a path")
	}
	dir, err := checkpointsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, safeCheckpointName(name)+".json"), nil
}

func loadCheckpoint(path string) (*checkpoint, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the caller's own path, as project.open reads one
	if err != nil {
		return nil, fmt.Errorf("no checkpoint at %s: %w", path, err)
	}
	var cp checkpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		return nil, fmt.Errorf("%s is not a checkpoint: %w", path, err)
	}
	return &cp, nil
}
