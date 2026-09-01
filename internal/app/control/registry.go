// Which workbenches are running on this machine, so a script facing several
// can pick one.
//
// There was no way to ask. One default address per user is resolved in
// address.go, and a second workbench needs an explicit -control-socket, so the
// only record of where the second one answers was in the head of whoever typed
// it. Nothing could enumerate them.
//
// So each server leaves a small file behind while it is listening, in a
// per-user directory: 0600, written whole through a temporary file and a
// rename, exactly as the TCP rendezvous file already is. It is the same idea
// as that file and deliberately not a second one - the fields it opens with
// are the rendezvous file's own - because two mechanisms for "where does a
// workbench answer" would drift apart.
//
// # Telling a live session from what a dead one left behind
//
// This is the part that has to be right, because a workbench killed with
// SIGKILL cannot clean up after itself and neither of the obvious checks can
// be trusted. A unix socket file outlives the process that bound it. A pid is
// reused, so a recorded pid that exists today may name somebody else's
// program. Both would report a dead session as running.
//
// The check is therefore a dial, which is what ListenAt already trusts: it
// refuses an address something answers on, and calls that "another workbench
// holds it". The lister has to believe the same thing, or the two disagree
// about what is running - a session ListenAt will not take the address of,
// while the listing says nothing is there. So: nothing answers, nothing is
// running, and the leftover file is removed on the spot. Correctness does not
// depend on a session having tidied up after itself; it only makes the
// directory shorter.
//
// What that leaves out on a loopback port is a stranger holding it: a dial
// proves a port is held, not by whom. It is the same residual ListenAt lives
// with, and the token in the session file is what then makes a client fail
// loudly instead of driving somebody else's process.
//
// # Why the description is asked for rather than written down
//
// The address, the pid and the start time never change, so the file records
// them. What a session is *running* does: a fixture is opened, nodes are
// added, and a note of that made when the session started would be a
// confident answer that had since stopped being true. The probe already has a
// connection open to prove the session is alive, so it asks on the same
// connection and the description cannot be stale. It names session.hello to do
// so, which is not a coupling this package did not already have: control
// defines the protocol number that verb reports, and refuses connections over
// it.
//
// # Windows
//
// Nothing here is unix-only: there the address is a loopback host and port and
// the probe is a TCP dial. It also fixes something broken there, since two TCP
// workbenches share one per-user rendezvous file and the second overwrites the
// first: the token beside the address in the session's own file is now the one
// a client needs.
package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// SessionsEnv names the directory the session files live in, for a test or a
// CI job that wants a registry of its own rather than the user's.
const SessionsEnv = "MESHBENCH_CONTROL_SESSIONS"

// detailWait bounds the one question asked of a live session.
//
// Generous, because the answer is not worth a wrong row: a session in the
// middle of something slow is still running, and is listed either way. Only
// its description goes missing.
const detailWait = 2 * time.Second

// Detail is what only a session can say about itself, asked of it rather than
// read off disk so that it cannot be out of date.
//
// Every field is omitted when the session did not answer in time. Mode is a
// string rather than a windowed flag for that reason: an absent bool reads as
// "headless", which would be a claim nobody made.
type Detail struct {
	Version string `json:"version,omitempty"`
	// Mode is "workbench" or "headless", and empty if it did not say.
	Mode string `json:"mode,omitempty"`
	// Project is the fixture or project last opened.
	Project string `json:"project,omitempty"`
	Nodes   int    `json:"nodes"`
}

// Session is one running workbench, as somebody choosing between several sees
// it.
type Session struct {
	// Address is where it answers, in the form -control-socket takes.
	Address string `json:"address"`
	// PID and StartedAt are what separates two otherwise identical runs.
	// StartedAt is when the socket opened, which is the moment the session
	// became something another process could reach.
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Detail
	// Self marks the row of the session that was asked, when one was. It is
	// how a script that is already attached to a workbench finds itself in a
	// list of them without comparing addresses that may be spelled two ways.
	Self bool `json:"self"`

	// Token authorises a TCP connection to this session, and is deliberately
	// never marshalled. A client that may have it can read the 0600 file it
	// came from; putting it in a verb's answer would copy a secret onto a
	// wire and into whatever logs that reply, for nothing.
	Token string `json:"-"`
}

// Dial connects to this session, with the token from its own file rather than
// from the per-user rendezvous file two TCP sessions share.
func (s Session) Dial() (*Client, error) {
	addr, err := s.address()
	if err != nil {
		return nil, err
	}
	return dialAddr(addr)
}

func (s Session) address() (Address, error) {
	addr, err := Resolve(s.Address)
	if err != nil {
		return Address{}, err
	}
	addr.Token = s.Token
	return addr, nil
}

// sessionFile is what a workbench leaves behind while it is listening: the
// rendezvous file's own three fields, plus when it started.
type sessionFile struct {
	Address   string    `json:"address"`
	Token     string    `json:"token,omitempty"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// SessionsDir is the per-user directory the session files live in, made if it
// is not there. 0700, like everything else that says where a control socket
// is.
func SessionsDir() (string, error) {
	dir := os.Getenv(SessionsEnv)
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf(
				"control: no per-user directory to list sessions in: %w", err)
		}
		dir = filepath.Join(base, "meshbench", "sessions")
	}
	// The value is this process's own environment or a directory this package
	// chose, set by the same person who can pass -control-socket.
	//nolint:gosec // G703: the operator's own path, like the flag
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// Set rather than left to whatever MkdirAll found already there, for the
	// reason ListenAt chmods the socket: what is in here says where a control
	// socket is and carries the token that drives it, so the filesystem is
	// the access control and it is not left to a umask or to a directory
	// somebody made earlier for something else.
	//
	// Best effort, because a filesystem that has no such notion - or Windows -
	// is not a reason to refuse to list anything.
	//nolint:gosec // G302 wants 0600, which for a directory nobody can enter
	_ = os.Chmod(dir, 0o700)
	return dir, nil
}

// sessionPath names one session's file after its address, so a workbench
// restarted at the same address writes over its own leftovers instead of
// adding to them.
func sessionPath(dir, address string) string {
	sum := sha256.Sum256([]byte(address))
	return filepath.Join(dir, hex.EncodeToString(sum[:8])+".json")
}

// registerSession writes this server's file and returns its path.
func registerSession(addr Address, started time.Time) (string, error) {
	dir, err := SessionsDir()
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(sessionFile{Address: addr.String(),
		Token: addr.Token, PID: os.Getpid(), StartedAt: started})
	if err != nil {
		return "", err
	}
	path := sessionPath(dir, addr.String())
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// Sessions lists the workbenches running on this machine, oldest first, and
// removes what dead ones left behind.
//
// self is the caller's own address, which is listed without being probed and
// without a description. A session asking this from inside one of its own
// verbs cannot answer its own session.hello: it is the one that would have to
// send the reply, and it is busy asking. A client with no session of its own
// passes "".
func Sessions(self string) ([]Session, error) {
	dir, err := SessionsDir()
	if err != nil {
		return nil, err
	}
	found, err := readSessionDir(dir, self)
	if err != nil {
		return nil, err
	}
	out := describeTheLiveOnes(found)
	// Oldest first, so two runs listed twice come back in the same order.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].Address < out[j].Address
	})
	return out, nil
}

// pending is one file and the row read out of it, before anything has asked
// whether the session it names is still there.
type pending struct {
	path string
	row  Session
}

func readSessionDir(dir, self string) ([]pending, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("control: reading %s: %w", dir, err)
	}
	out := make([]pending, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := readSessionFile(path)
		if err != nil {
			// Not an error to report: a file this package cannot read is a
			// file it did not write, and refusing to list anything because of
			// one would make the whole answer hostage to it.
			continue
		}
		row := Session{Address: f.Address, PID: f.PID,
			StartedAt: f.StartedAt, Token: f.Token,
			Self: self != "" && f.Address == self}
		out = append(out, pending{path: path, row: row})
	}
	return out, nil
}

// describeTheLiveOnes asks each session what it is running, drops the ones
// that are not there any more, and takes their files with them.
//
// Concurrently, because each probe is a round trip to another process and
// doing them in turn would make the answer take as long as the slowest session
// times the number of them.
func describeTheLiveOnes(found []pending) []Session {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out []Session
	)
	keep := func(s Session) {
		mu.Lock()
		defer mu.Unlock()
		out = append(out, s)
	}
	for _, p := range found {
		if p.row.Self {
			// Not probed and not described: a session asking this from inside
			// one of its own verbs is the one that would have to answer.
			keep(p.row)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			detail, alive := probe(p.row)
			if !alive {
				_ = os.Remove(p.path)
				return
			}
			p.row.Detail = detail
			keep(p.row)
		}()
	}
	wg.Wait()
	return out
}

func readSessionFile(path string) (sessionFile, error) {
	var f sessionFile
	body, err := os.ReadFile(path) //nolint:gosec // a per-user path this package chose
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(body, &f); err != nil {
		return f, err
	}
	if f.Address == "" {
		return f, fmt.Errorf("control: %s names no address", path)
	}
	return f, nil
}

// probe opens one connection and gets both answers out of it: whether anything
// is there, and what it is running. A connection that opens is a live session
// whether or not it finds a moment to describe itself.
func probe(s Session) (Detail, bool) {
	addr, err := s.address()
	if err != nil {
		return Detail{}, false
	}
	c, err := dialAddr(addr)
	if err != nil {
		return Detail{}, false
	}
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), detailWait)
	defer cancel()
	raw, err := c.CallContext(ctx, "session.hello", nil)
	if err != nil {
		return Detail{}, true
	}
	var d Detail
	if err := json.Unmarshal(raw, &d); err != nil {
		return Detail{}, true
	}
	return d, true
}
