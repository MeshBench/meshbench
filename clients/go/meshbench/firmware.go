// What this machine can run, and what it is running.
package meshbench

import (
	"context"
	"fmt"
	"strings"
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
func (f Firmware) Find(ctx context.Context, version string, board Board) (Build, error) {
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
// role is a plain string here and a Role everywhere else, deliberately: this
// one names a published release asset, and the catalogue's own spellings are
// not always the application names the verbs are keyed on. Typing it as Role
// would have the compiler vouch for something nobody has checked.
func (f Firmware) Download(ctx context.Context, role, version string, board Board) error {
	p := map[string]any{"role": role, "version": version}
	if board != "" {
		p["board"] = board
	}
	return f.w.Do(ctx, "firmware.download", p)
}

// Import takes a build from a path - the one way a locally built image gets
// into the library.
//
// label is what the library will know it by and what a node pins. Left empty
// it is a timestamp, so importing twice gives two builds rather than one that
// quietly replaced the other - which matters the moment you want to put the
// new one on a node and delete the old.
func (f Firmware) Import(ctx context.Context, path string, role Role, board Board,
	label string) (Build, error) {
	p := map[string]any{"path": path, "role": role}
	if board != "" {
		p["board"] = board
	}
	if label != "" {
		p["label"] = label
	}
	var out Build
	return out, f.w.CallInto(ctx, "firmware.import", p, &out)
}

// Delete removes a build from the cache, and says what it removed.
//
// By path, and the workbench refuses any path outside the firmware cache. A
// build nodes are still pinned to will go: they keep the pin, which then
// cannot be honoured and fails at start - so move them onto the replacement
// first.
func (f Firmware) Delete(ctx context.Context, b Build) (string, error) {
	if b.Path == "" {
		return "", fmt.Errorf("meshbench: %s has no path on this machine to delete",
			b.Describe())
	}
	var out struct {
		Deleted string `json:"deleted"`
	}
	return out.Deleted, f.w.CallInto(ctx, "firmware.delete",
		map[string]any{"path": b.Path}, &out)
}

// Build compiles a MeshCore checkout and puts the results in the library.
//
// Both roles from one call unless one is named, deliberately. A locally built
// repeater compiled against a stale shim once answered console output with
// 0x06 where the host expects 0x07: it connected, misbehaved and exited. Two
// arms of a comparison built at different moments from different trees measure
// the build process rather than the firmware, so the easy thing to do here is
// the thing that builds them together.
//
// It returns once the work has started. Wait on it: a MeshCore build is a
// minute or two per role.
func (f Firmware) Build(ctx context.Context, checkout, role, label string) (Job, error) {
	p := map[string]any{"source": checkout}
	if role != "" {
		p["role"] = role
	}
	if label != "" {
		p["label"] = label
	}
	var out struct {
		Job string `json:"job"`
	}
	if err := f.w.CallInto(ctx, "firmware.build", p, &out); err != nil {
		return Job{}, err
	}
	return Job{w: f.w, id: out.Job}, nil
}

// BuildAndWait is the same, blocking, for a caller with nothing else to do -
// which is most of them.
func (f Firmware) BuildAndWait(ctx context.Context, checkout string,
	timeout time.Duration) ([]Build, error) {
	job, err := f.Build(ctx, checkout, "", "")
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	if err := job.Wait(ctx, timeout); err != nil {
		return nil, err
	}
	// What the library holds afterwards that it did not before is the honest
	// answer, and it is also the one a caller can pin a node to.
	all, err := f.Library(ctx)
	if err != nil {
		return nil, err
	}
	var built []Build
	for _, b := range all {
		if strings.HasPrefix(b.Version, "local-") {
			built = append(built, b)
		}
	}
	return built, nil
}

// UseWhatIsHere pins every role that needs one to the newest build on this
// machine, and reports what it chose.
//
// What a script wants almost every time: this mesh, whatever this machine
// holds, rather than a version typed into the script that goes stale. A run
// refuses to start until every role is answered, so the alternative is the
// same loop written out in every caller.
//
// It refuses by name when a role has nothing, because "no companion build" is
// a thing to go and fix rather than a reason to start a mesh with a silent
// hole in it.
func (f Firmware) UseWhatIsHere(ctx context.Context) (map[Role]Build, error) {
	needed, err := f.Needed(ctx)
	if err != nil {
		return nil, err
	}
	have, err := f.OnDisk(ctx)
	if err != nil {
		return nil, err
	}
	chosen := map[Role]Build{}
	for _, want := range needed {
		var pick Build
		for _, b := range have {
			if b.Role == want.Role && b.Board == "" {
				pick = b
			}
		}
		if pick.Version == "" {
			return nil, &Refused{Verb: "firmware.needed", Code: "not_found",
				Message: fmt.Sprintf(
					"no %s build on this machine: meshcoresim firmware download %s",
					want.Role, want.Role),
				kind: ErrNotFound}
		}
		if err := f.UseForRole(ctx, want.Role, pick); err != nil {
			return nil, err
		}
		chosen[want.Role] = pick
	}
	return chosen, nil
}

// UseForRole pins every node of a role to one build.
func (f Firmware) UseForRole(ctx context.Context, role Role, b Build) error {
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
	Role    Role     `json:"role"`
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
