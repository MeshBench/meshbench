package meshbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/MeshBench/meshbench/internal/app/control"
)

// maxUnixPath is the shortest sun_path any platform we run on allows: 108 on
// Linux, 104 on macOS and the BSDs.
const maxUnixPath = 104

// Workbench is a connection to a running session.
//
// Safe from several goroutines: the socket serialises calls, which is what the
// control package already guarantees.
type Workbench struct {
	conn  *control.Client
	hello Hello
	// versionCheck is what became of the release check, for VersionCheck.
	versionCheck string

	// owned is the process this client started, if it started one. Attach
	// never sets it: a script must not be able to close the workbench
	// somebody is looking at by falling off the end of a function.
	owned *exec.Cmd
}

// BinaryEnv names the workbench to start when nothing else does.
const BinaryEnv = "MESHBENCH_BINARY"

// Option configures a connection.
type Option func(*dialOptions)

type dialOptions struct {
	socket  string
	binary  string
	fixture string
	seed    uint64
	args    []string
	waitFor time.Duration
	stderr  *os.File
	// rendezvous is the address file a launched process was told to write,
	// empty when this client did not start one or the transport has no need.
	rendezvous string
}

// Socket chooses which socket to use, rather than the per-user default.
func Socket(path string) Option { return func(o *dialOptions) { o.socket = path } }

// Binary names the meshbench executable to launch. The default is whatever
// "meshbench" resolves to on PATH.
func Binary(path string) Option { return func(o *dialOptions) { o.binary = path } }

// Fixture opens a network as the session starts.
func Fixture(name string) Option { return func(o *dialOptions) { o.fixture = name } }

// Seed fixes the run's seed. Same seed, same scenario, same result - which is
// what makes a changed result mean something.
func Seed(n uint64) Option { return func(o *dialOptions) { o.seed = n } }

// Args passes extra flags to a launched process.
func Args(a ...string) Option { return func(o *dialOptions) { o.args = append(o.args, a...) } }

// StartTimeout bounds how long Launch and Headless wait for the socket.
func StartTimeout(d time.Duration) Option { return func(o *dialOptions) { o.waitFor = d } }

// LogTo sends a launched process's stderr somewhere. The default is this
// process's own, because a scripted run that fails silently is the worst of
// both worlds.
func LogTo(f *os.File) Option { return func(o *dialOptions) { o.stderr = f } }

func opts(in []Option) *dialOptions {
	o := &dialOptions{waitFor: 60 * time.Second, stderr: os.Stderr}
	for _, f := range in {
		f(o)
	}
	return o
}

// Attach connects to a workbench that is already running.
//
// It never owns the process: Close hangs up, and whatever was on screen stays
// on screen.
func Attach(ctx context.Context, options ...Option) (*Workbench, error) {
	o := opts(options)
	conn, err := control.DialAt(o.socket)
	if err != nil {
		return nil, err
	}
	wb := &Workbench{conn: conn}
	return wb, wb.greet(ctx)
}

// Headless starts a session with no window and owns it.
//
// The one to use from a test or from CI: no display, no GPU, no toolkit.
func Headless(ctx context.Context, options ...Option) (*Workbench, error) {
	o := opts(options)
	args := []string{"headless"}
	args = append(args, launchArgs(o)...)
	return launch(ctx, o, args)
}

// Launch opens the desktop workbench and owns it. Needs a display.
func Launch(ctx context.Context, options ...Option) (*Workbench, error) {
	o := opts(options)
	args := []string{"workbench"}
	args = append(args, launchArgs(o)...)
	return launch(ctx, o, args)
}

func launchArgs(o *dialOptions) []string {
	var a []string
	if o.socket != "" {
		a = append(a, "-control-socket", o.socket)
	}
	if o.fixture != "" {
		a = append(a, "-fixture", o.fixture)
	}
	if o.seed != 0 {
		a = append(a, "-seed", fmt.Sprint(o.seed))
	}
	return append(a, o.args...)
}

func launch(ctx context.Context, o *dialOptions, args []string) (*Workbench, error) {
	// An address of its own unless one was named, so launching two of these
	// does not have them fight over the per-user default - which is the fault
	// #211 was about, arriving from the other side.
	var env []string
	if o.socket == "" {
		dir, err := os.MkdirTemp("", "meshbench")
		if err != nil {
			return nil, err
		}
		o.socket = filepath.Join(dir, "control.sock")
		// Two reasons that path may not do. Windows has no unix socket a Python
		// client can reach, and a temporary directory on macOS is long enough on
		// its own to exceed sun_path - /var/folders/xy/.../T/ is most of 104
		// bytes before a name is added. Either way, loopback.
		if runtime.GOOS == "windows" || len(o.socket) > maxUnixPath {
			o.socket = "tcp"
			// A rendezvous file of its own too, or two sessions would overwrite
			// each other's port and token in the per-user one.
			o.rendezvous = filepath.Join(dir, "control.json")
			env = append(env, control.RendezvousEnv+"="+o.rendezvous)
		}
		args = append(args, "-control-socket", o.socket)
	}
	bin := o.binary
	if bin == "" {
		// A checkout has one built but not installed, and every example and
		// every test then needs the same three lines to find it. The variable
		// the test harness already used is honoured here too, so that
		// MESHBENCH_BINARY means what the README says it means.
		bin = os.Getenv(BinaryEnv)
	}
	if bin == "" {
		bin = "meshbench"
	}
	// The binary and its arguments come from this process's own caller, not
	// from anything the workbench or a network said. Launching a named
	// executable is the whole point of Launch and Headless; a caller who can
	// choose it can already run anything.
	//nolint:gosec // G204: the command is the caller's own, by design
	cmd := exec.CommandContext(ctx, bin, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stderr = o.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("client: starting %s: %w", bin, err)
	}

	// Wait for the socket rather than for a fixed moment. A national fixture
	// takes a while to open and a small one does not, and a sleep long enough
	// for the first is wasted on every run of the second.
	// Dialling has to look in the same rendezvous the process was told to
	// write, or it would find whatever else on this machine had left one.
	if o.rendezvous != "" {
		restore, had := os.LookupEnv(control.RendezvousEnv)
		_ = os.Setenv(control.RendezvousEnv, o.rendezvous)
		defer func() {
			if had {
				_ = os.Setenv(control.RendezvousEnv, restore)
				return
			}
			_ = os.Unsetenv(control.RendezvousEnv)
		}()
	}
	deadline := time.Now().Add(o.waitFor)
	for {
		conn, err := control.DialAt(o.socket)
		if err == nil {
			wb := &Workbench{conn: conn, owned: cmd}
			if err := wb.greet(ctx); err != nil {
				_ = wb.Close()
				return nil, err
			}
			return wb, nil
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf(
				"client: %s did not answer at %s within %s", bin, o.socket, o.waitFor)
		}
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Hello is what this connection is talking to, read once at connect.
func (w *Workbench) Hello() Hello { return w.hello }

// Headless reports whether this session has no interface, so a caller can
// check once rather than learn it from a dozen refusals.
func (w *Workbench) Headless() bool { return w.hello.Mode == "headless" }

// Close hangs up, and stops the process if this client started it.
func (w *Workbench) Close() error {
	err := w.conn.Close()
	if w.owned == nil {
		return err
	}
	if w.owned.Process != nil {
		// Interrupt where the OS has one, so the run stops its firmware on the
		// way out. Windows has no SIGINT to send a process that is not sharing
		// a console - Process.Signal(os.Interrupt) returns "not supported by
		// windows" and the process would carry on - so it is killed there, and
		// whatever the run was holding is the operating system's problem.
		if runtime.GOOS == "windows" {
			_ = w.owned.Process.Kill()
		} else {
			_ = w.owned.Process.Signal(os.Interrupt)
		}
		done := make(chan struct{})
		go func() { _, _ = w.owned.Process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			// Interrupt asks the run to stop its firmware on the way out,
			// which on fifty-eight emulated nodes is not instant. Twenty
			// seconds, then take it.
			_ = w.owned.Process.Kill()
		}
	}
	return err
}

// Call runs one verb and returns its result as raw JSON.
//
// Public and documented, not an escape hatch to be ashamed of: the façade will
// never cover every verb the socket answers, and a verb added tomorrow is
// usable today. Ask session.verbs for the list this build actually offers -
// two counts written down here have already gone stale, and a number nothing
// checks is worse than no number.
func (w *Workbench) Call(ctx context.Context, verb string, params any) (json.RawMessage, error) {
	type result struct {
		raw json.RawMessage
		err error
	}
	// The socket call is synchronous and has no context of its own, so the
	// context is honoured here rather than pretended at. A verb that has been
	// sent will still be performed - the store does not know the caller gave
	// up - which is why Close is what actually stops things.
	ch := make(chan result, 1)
	go func() {
		raw, err := w.conn.Call(verb, params)
		ch <- result{raw, err}
	}()
	select {
	case r := <-ch:
		return r.raw, wrap(verb, r.err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CallInto runs a verb and decodes its result.
func (w *Workbench) CallInto(ctx context.Context, verb string, params, into any) error {
	raw, err := w.Call(ctx, verb, params)
	if err != nil {
		return err
	}
	if into == nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("client: %s returned something unexpected: %w", verb, err)
	}
	return nil
}

// Do runs a verb and discards its result, for the ones whose answer is only
// an acknowledgement.
func (w *Workbench) Do(ctx context.Context, verb string, params any) error {
	_, err := w.Call(ctx, verb, params)
	return err
}

// Describe is the cheap summary: how many nodes, what time it is, whether it
// is playing.
func (w *Workbench) Describe(ctx context.Context) (Describe, error) {
	var d Describe
	return d, w.CallInto(ctx, "session.describe", nil, &d)
}

// Journal is the command history, for picking up a session cold: how the world
// got here, and whether the process has been restarted since it was built.
func (w *Workbench) Journal(ctx context.Context) (Journal, error) {
	var j Journal
	return j, w.CallInto(ctx, "session.journal", nil, &j)
}

// Snapshot is the whole session as the socket summarises it - counts, jobs,
// endpoints, the status line. Decoded loosely on purpose: it grows, and a
// client that failed on a field it had not heard of would break on every
// release.
func (w *Workbench) Snapshot(ctx context.Context) (map[string]any, error) {
	var m map[string]any
	return m, w.CallInto(ctx, "session.snapshot", nil, &m)
}

// Verbs is every method this build answers.
func (w *Workbench) Verbs(ctx context.Context) ([]string, error) {
	var v struct {
		Verbs []string `json:"verbs"`
	}
	return v.Verbs, w.CallInto(ctx, "session.verbs", nil, &v)
}

// Say puts a line in the session's log, which is how a script leaves a note
// for whoever is watching the window or reading the run's stderr.
func (w *Workbench) Say(ctx context.Context, text string) error {
	return w.Do(ctx, "ui.said", text)
}

// Window opens a node's own window, on a named tab.
//
// Windowed sessions only, and it says so here rather than appearing to work: a
// headless run has nothing to open, and a script that "opened the Hardware
// tab" in CI and saw no error will be written to assume it did.
//
// The tab names are the ones on the strip - Console, Companion, SDR, Settings,
// Radio, Stats, Activity, Connect, Hardware - and an empty one takes the
// default. It returns the tab it opened on.
// KeepAbove reads whether a panel opened in its own window stays above the
// main one.
//
// The preference exists for Linux under Wayland, where no client may ask a
// normal window to stay above others. What can be asked for is a layer-shell
// surface, and that is a different kind of window: the compositor gives it no
// title bar, no taskbar entry and no minimise, so the window draws its own bar
// and its close button returns the panel to the main window. On macOS and
// Windows always-on-top costs nothing and the preference does not apply.
func (w *Workbench) KeepAbove(ctx context.Context) (bool, error) {
	var out struct {
		On bool `json:"on"`
	}
	return out.On, w.CallInto(ctx, "ui.keep_above", nil, &out)
}

// SetKeepAbove sets it, and reports what it now is.
func (w *Workbench) SetKeepAbove(ctx context.Context, on bool) (bool, error) {
	var out struct {
		On bool `json:"on"`
	}
	return out.On, w.CallInto(ctx, "ui.keep_above",
		map[string]any{"on": on}, &out)
}

func (w *Workbench) Window(ctx context.Context, node string, tab Tab) (Tab, error) {
	if w.Headless() {
		return "", &Refused{
			Verb: "node.window", Code: "unavailable",
			Message: "this session has no interface attached, so there is nothing to show",
			kind:    ErrUnavailable,
		}
	}
	var out struct {
		Tab Tab `json:"tab"`
	}
	err := w.CallInto(ctx, "node.window",
		map[string]any{"node": node, "tab": tab}, &out)
	return out.Tab, err
}

// BringUp opens a node's bring-up window and reports the board it checks.
//
// What the board's profile declares, beside what the firmware left in the chip,
// and where the two differ. Windowed sessions only, for the same reason Window
// is, and refused for a node running a host build: there is no board to check
// it against.
func (w *Workbench) BringUp(ctx context.Context, node string) (string, error) {
	if w.Headless() {
		return "", &Refused{
			Verb: "node.bringup", Code: "unavailable",
			Message: "this session has no interface attached, so there is nothing to show",
			kind:    ErrUnavailable,
		}
	}
	var out struct {
		Board string `json:"board"`
	}
	err := w.CallInto(ctx, "node.bringup", map[string]any{"node": node}, &out)
	return out.Board, err
}

var errNoProcess = errors.New("this client did not start the workbench")

// Stop ends a workbench this client started. Attach's connection has nothing
// to stop, and says so rather than quietly doing nothing.
func (w *Workbench) Stop() error {
	if w.owned == nil {
		return errNoProcess
	}
	return w.Close()
}

// AttachOrLaunch uses the session that is already running, or opens one.
//
// For a script somebody runs repeatedly by hand: the second run carries on
// from the first rather than clearing everything down and starting again.
//
// Note which half you got, because they differ in one important way. Attaching
// does not own the process and Close leaves it running; launching owns it and
// Close stops it. Owned reports which happened, so a script that must not take
// the session down with it can say so.
func AttachOrLaunch(ctx context.Context, options ...Option) (*Workbench, error) {
	return attachOr(ctx, Launch, options)
}

// AttachOrHeadless is AttachOrLaunch without a window, for a machine with no
// display.
func AttachOrHeadless(ctx context.Context, options ...Option) (*Workbench, error) {
	return attachOr(ctx, Headless, options)
}

// attachOr starts a session at the address it has just tried, which is the
// whole point of the pair.
//
// Launch and Headless called directly invent a private address, so that two of
// them - two tests, two scripts - do not fight over the per-user default.
// Inheriting that here made AttachOrLaunch useless: every run failed to
// attach, started a session somewhere nobody would look again, and the next
// run did the same. It read as "reuse does not work" rather than as an address
// nobody had named.
func attachOr(ctx context.Context, start func(context.Context, ...Option) (*Workbench, error),
	options []Option,
) (*Workbench, error) {
	if opts(options).socket == "" {
		options = append(options, Socket(control.DefaultAddress()))
	}
	if wb, err := Attach(ctx, options...); err == nil {
		return wb, nil
	}
	return start(ctx, options...)
}

// Owned reports whether Close will stop the session or only hang up on it.
//
// Worth asking after AttachOrLaunch, where either is possible and the
// difference is whether the workbench is still there afterwards.
func (w *Workbench) Owned() bool { return w.owned != nil }
