package meshbench

import "context"

// A checkpoint is the whole session frozen to a named file - the network, how
// it is being run, and where the clock had got to. Restoring one takes the mesh
// back to that exact moment by rebuilding and replaying deterministically, so
// there is nothing to capture that the seed does not already reproduce. Because
// it replays, restoring a long run takes the run's own time.

// Checkpoint is what a save reports back.
type Checkpoint struct {
	Name  string `json:"checkpoint"`
	Path  string `json:"path"`
	NowMs uint32 `json:"now_ms"`
	Nodes int    `json:"nodes"`
}

// Restored is what a restore reports: where it landed, and whether it is still
// replaying to get there.
type Restored struct {
	Name     string `json:"restored"`
	Nodes    int    `json:"nodes"`
	NowMs    uint32 `json:"now_ms"`
	TargetMs uint32 `json:"target_ms"`
	// Replaying is true while the run is stepping forward to the checkpoint's
	// time; wait on the sim reaching TargetMs to know it has arrived.
	Replaying bool `json:"replaying"`
}

// Checkpoint freezes the session under a name, so it can be taken back here.
func (w *Workbench) Checkpoint(ctx context.Context, name string) (Checkpoint, error) {
	var c Checkpoint
	return c, w.CallInto(ctx, "session.checkpoint", map[string]any{"name": name}, &c)
}

// Restore rebuilds a checkpoint and replays to the moment it was taken. It
// returns as soon as the replay is under way; the sim reaching TargetMs is when
// it has actually arrived.
func (w *Workbench) Restore(ctx context.Context, name string) (Restored, error) {
	var r Restored
	return r, w.CallInto(ctx, "session.restore", map[string]any{"name": name}, &r)
}

// Checkpoints is what can be restored, by name.
func (w *Workbench) Checkpoints(ctx context.Context) ([]string, error) {
	var out struct {
		Checkpoints []string `json:"checkpoints"`
	}
	return out.Checkpoints, w.CallInto(ctx, "session.checkpoints", nil, &out)
}
