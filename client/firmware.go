// What this machine can run, and what it is running.
package client

import (
	"context"
	"fmt"
	"time"
)

// Firmware is the library. Live.
type Firmware struct{ w *Workbench }

// Firmware reaches the builds.
func (w *Workbench) Firmware() Firmware { return Firmware{w} }

// Library is every build, published or on disk, with what runs it.
func (f Firmware) Library(ctx context.Context) ([]Build, error) {
	var out struct {
		Builds []Build `json:"builds"`
	}
	return out.Builds, f.w.CallInto(ctx, "firmware.library", nil, &out)
}

// OnDisk is only the ones this machine actually holds, which is the only thing
// that decides what a node can run. A build that failed to download and one in
// daily use look identical from anywhere else.
func (f Firmware) OnDisk(ctx context.Context) ([]Build, error) {
	all, err := f.Library(ctx)
	if err != nil {
		return nil, err
	}
	var out []Build
	for _, b := range all {
		if b.OnDisk {
			out = append(out, b)
		}
	}
	return out, nil
}

// Find is one build by version, and by board where the version alone is
// ambiguous - which it is for every board image, because "wadamesh" is not a
// build until it is wadamesh for a particular piece of hardware.
func (f Firmware) Find(ctx context.Context, version, board string) (Build, error) {
	all, err := f.Library(ctx)
	if err != nil {
		return Build{}, err
	}
	for _, b := range all {
		if b.Version == version && (board == "" || b.Board == board) {
			return b, nil
		}
	}
	return Build{}, &Refused{Verb: "firmware.library", Code: "not_found",
		Message: fmt.Sprintf("no build %q for board %q", version, board),
		kind:    ErrNotFound}
}

// Scan asks the catalogue what is published, which is how a build nobody has
// downloaded becomes offerable.
func (f Firmware) Scan(ctx context.Context) error {
	return f.w.Do(ctx, "firmware.published", nil)
}

// Download fetches one. It returns once the download has been asked for, not
// once it has landed: wait on the job.
func (f Firmware) Download(ctx context.Context, role, version, board string) error {
	p := map[string]any{"role": role, "version": version}
	if board != "" {
		p["board"] = board
	}
	return f.w.Do(ctx, "firmware.download", p)
}

// Import takes a build from a path - the one way a locally built image gets
// into the library.
func (f Firmware) Import(ctx context.Context, path, role, board string) (Build, error) {
	p := map[string]any{"path": path, "role": role}
	if board != "" {
		p["board"] = board
	}
	var out Build
	return out, f.w.CallInto(ctx, "firmware.import", p, &out)
}

// UseForRole pins every node of a role to one build.
func (f Firmware) UseForRole(ctx context.Context, role string, b Build) error {
	return f.w.Do(ctx, "firmware.set", map[string]any{
		"role": role, "version": b.Version})
}

// Start brings up firmware on every node.
//
// Asynchronous, and always has been: it answers with what it has begun, not
// with what is up. It was synchronous once, and on 155 nodes that froze the
// window and the socket together for as long as it was left - which read as a
// crash and was reported as one. Wait with WaitStarted.
func (f Firmware) Start(ctx context.Context) error {
	return f.w.Do(ctx, "firmware.start", nil)
}

// State is how far a start has got.
func (f Firmware) State(ctx context.Context) (FirmwareState, error) {
	var st FirmwareState
	return st, f.w.CallInto(ctx, "firmware.state", nil, &st)
}

// WaitStarted waits for every node's firmware to be up.
//
// Generous by default where a caller passes nothing: real firmware on a large
// network is minutes, and on emulated boards it is longer.
func (f Firmware) WaitStarted(ctx context.Context, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return waitFor(ctx, timeout, "firmware to come up", func() (bool, string, error) {
		st, err := f.State(ctx)
		if err != nil {
			return false, "", err
		}
		if !st.Starting && st.Running >= st.Nodes && st.Nodes > 0 {
			return true, "", nil
		}
		return false, fmt.Sprintf("%d of %d running", st.Running, st.Nodes), nil
	})
}

// Needed is the roles this scenario has nodes for and no build pinned to, with
// what could be pinned. A run refuses to start until every one is answered.
func (f Firmware) Needed(ctx context.Context) ([]RoleNeed, error) {
	var out struct {
		Roles []RoleNeed `json:"roles"`
	}
	return out.Roles, f.w.CallInto(ctx, "firmware.needed", nil, &out)
}

// RoleNeed is one role with nothing to run. Snapshot.
type RoleNeed struct {
	Role    string   `json:"role"`
	Nodes   int      `json:"nodes"`
	Choices []string `json:"choices"`
}

// NodeStats is what every node is costing and doing. Snapshot.
//
// Separate from NodeInfo because one is what the network *is* and the other is
// what it is *doing*: they change on different timescales and the store
// publishes them apart.
type NodeStat struct {
	Name     string `json:"name"`
	Backend  string `json:"backend"`
	Firmware string `json:"firmware"`
	Running  bool   `json:"running"`
	State    string `json:"state"`
	Board    string `json:"board"`
	PID      int    `json:"pid"`
	RSSBytes int64  `json:"rss_bytes"`
	CPUms    int64  `json:"cpu_ms"`
	Sent     int    `json:"sent"`
	Heard    int    `json:"heard"`
}

// NodeStats samples every node and returns what it found.
//
// A sample, not a read: it costs a /proc read per node, which is why the
// window only does it while somebody is looking at the panel.
func (w *Workbench) NodeStats(ctx context.Context) ([]NodeStat, error) {
	var out struct {
		Stats []NodeStat `json:"stats"`
	}
	return out.Stats, w.CallInto(ctx, "nodes.stats", nil, &out)
}
