// Where a workbench answers, on whichever operating system it is running on.
//
// A unix socket was the whole story while this only ran on Linux, and two
// things about it do not travel. Windows has no AF_UNIX that Python can
// reach - CPython has never exposed socket.AF_UNIX there - so a Python client
// on Windows cannot speak to one at all. And the per-user default was built
// out of XDG_RUNTIME_DIR and os.Getuid(), neither of which means anything off
// Linux; Getuid does not even fail there, it returns -1, so the fallback
// quietly produced one shared path called meshbench--1.sock.
//
// So there are two transports:
//
//   - A unix socket, where the OS has one. The filesystem is the access
//     control: 0600, and the kernel enforces it.
//   - Loopback TCP with a token, where it does not. Bound to 127.0.0.1 on an
//     ephemeral port, with the address and a random token written to a 0600
//     file for a client to find. Nothing is on the network - ADR-0005 stands -
//     but the guarantee is a convention rather than the kernel's, because any
//     local process can reach a loopback port and only the token stops it.
//
// The choice is by operating system, not by language: two clients that had to
// speak different transports on the same machine would be a worse thing than
// two transports. And TCP can be asked for anywhere, which is the only way the
// Windows path gets exercised by anyone who is not on Windows.
package control

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Kind is which transport an address names.
type Kind string

const (
	// Unix is an AF_UNIX socket at a path.
	Unix Kind = "unix"
	// TCP is a loopback listener, addressed through a rendezvous file.
	TCP Kind = "tcp"
)

// Address is a resolved place to listen or dial.
type Address struct {
	Kind Kind
	// Addr is the socket path for Unix, or host:port for TCP. For a TCP
	// address that has not been bound yet the port may be 0, meaning "any".
	Addr string
	// Token authorises a TCP connection. Empty for Unix, where the socket's
	// permissions do the same job and do it better.
	Token string
}

// String is the address as somebody would type it back in.
func (a Address) String() string {
	if a.Kind == Unix {
		return a.Addr
	}
	return "tcp:" + a.Addr
}

// SocketEnv names the environment variable that chooses where to answer.
const SocketEnv = "MESHBENCH_CONTROL_SOCKET"

// RendezvousEnv names the file a TCP listener writes its address and token to.
//
// Per user by default, which is right for somebody's own desktop and wrong for
// two runs at once: the second would overwrite the first's file and a client
// reading it would reach the wrong session, or the right one with the wrong
// token. A client that starts a workbench gives it a file of its own.
const RendezvousEnv = "MESHBENCH_CONTROL_RENDEZVOUS"

// maxUnixPath is the shortest sun_path any platform we run on allows.
//
// 108 bytes on Linux, 104 on macOS and the BSDs. Checked against the smaller,
// because the failure otherwise is a bind refusing with something about an
// invalid argument, and macOS temporary directories are long enough to reach
// it - /var/folders/xy/…/T/ is most of the budget before a name is added.
const maxUnixPath = 104

// Resolve works out where to answer or dial: what the caller asked for, then
// the environment, then this operating system's default.
//
// Accepted forms:
//
//	/run/user/1000/meshbench.sock   a unix socket at a path
//	unix:/tmp/mb.sock                 the same, said explicitly
//	tcp                               loopback, an ephemeral port
//	tcp:127.0.0.1:5599                loopback, a port somebody chose
func Resolve(want string) (Address, error) {
	if want == "" {
		want = os.Getenv(SocketEnv)
	}
	if want == "" {
		return defaultAddress()
	}
	switch {
	case want == "tcp":
		return Address{Kind: TCP, Addr: "127.0.0.1:0"}, nil
	case strings.HasPrefix(want, "tcp:"):
		hostPort := strings.TrimPrefix(want, "tcp:")
		if !strings.Contains(hostPort, ":") {
			hostPort = "127.0.0.1:" + hostPort
		}
		// Loopback only, and refused rather than quietly narrowed: somebody
		// who typed an outward-facing address meant something this program
		// does not do, and silently binding somewhere else would leave them
		// believing it had.
		if h := hostPort[:strings.LastIndex(hostPort, ":")]; !loopback(h) {
			return Address{}, fmt.Errorf(
				"control: %s is not a loopback address - the control socket is "+
					"local to this machine and does not listen on the network", h)
		}
		return Address{Kind: TCP, Addr: hostPort}, nil
	default:
		path := strings.TrimPrefix(want, "unix:")
		if err := checkUnixPath(path); err != nil {
			return Address{}, err
		}
		return Address{Kind: Unix, Addr: path}, nil
	}
}

func loopback(host string) bool {
	switch strings.Trim(host, "[]") {
	case "127.0.0.1", "::1", "localhost", "":
		return true
	}
	return strings.HasPrefix(host, "127.")
}

func checkUnixPath(path string) error {
	if len(path) <= maxUnixPath {
		return nil
	}
	return fmt.Errorf(
		"control: %s is %d bytes and a unix socket path may be at most %d - "+
			"choose a shorter one, or use tcp", path, len(path), maxUnixPath)
}

// defaultAddress is where this operating system answers when nobody says.
// DefaultAddress is where a workbench answers unless it was told otherwise,
// as a string to hand back to Resolve or to -control-socket.
//
// Exported for the clients' attach-or-start pair, which has to *name* the
// default rather than leave it implied: an unnamed launch invents a private
// address, and a session started at one is a session the next run will not
// find. Empty when this machine has nowhere to put one, which Resolve then
// reports properly.
func DefaultAddress() string {
	// Through Resolve, so the environment override is honoured. Calling
	// defaultAddress directly skipped it, which would have had a client start a
	// session at the per-user path while MESHBENCH_SOCKET said somewhere else -
	// and then fail to attach to its own session next time.
	a, err := Resolve("")
	if err != nil {
		return ""
	}
	if a.Kind == TCP {
		// The caller wants something to hand to -control-socket, and an
		// ephemeral 127.0.0.1:0 is not it: the port is not known until it is
		// bound, and the rendezvous file is how it is found.
		return "tcp"
	}
	return a.Addr
}

func defaultAddress() (Address, error) {
	if runtime.GOOS == "windows" {
		// No AF_UNIX a Python client can reach, so loopback and a token.
		return Address{Kind: TCP, Addr: "127.0.0.1:0"}, nil
	}
	// Linux keeps exactly the path it has always had: scripts
	// and tools/soak all name it, and moving it would break them for no gain.
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		p := filepath.Join(dir, "meshbench.sock")
		return Address{Kind: Unix, Addr: p}, checkUnixPath(p)
	}
	// Everywhere else, a per-user directory the OS already defines - which is
	// what Getuid was standing in for, less portably. On macOS this is
	// ~/Library/Caches, short enough to stay inside sun_path where $TMPDIR
	// would not be.
	dir, err := os.UserCacheDir()
	if err != nil {
		return Address{}, fmt.Errorf("control: no per-user directory to answer in: %w", err)
	}
	dir = filepath.Join(dir, "meshbench")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Address{}, err
	}
	p := filepath.Join(dir, "control.sock")
	return Address{Kind: Unix, Addr: p}, checkUnixPath(p)
}

// rendezvous is the file a TCP listener writes so a client can find it.
//
// A TCP address needs one and a unix socket does not: the socket *is* its own
// address, whereas an ephemeral port is not knowable until it has been bound.
type rendezvous struct {
	Address string `json:"address"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
}

// RendezvousPath is where that file lives: where the environment says, or per
// user.
func RendezvousPath() (string, error) {
	if p := os.Getenv(RendezvousEnv); p != "" {
		p = filepath.Clean(p)
		// The path comes from this process's own environment, set by whoever
		// started it - the same person who can pass -control-socket, and who
		// can already create directories as this user. There is no
		// lesser-privileged source for it to be tainted by.
		//nolint:gosec // G703: the value is the operator's own, like the flag
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return "", err
		}
		return p, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "meshbench")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "control.json"), nil
}

// newToken is what stands in for filesystem permissions on a loopback port.
//
// Any local process can connect to 127.0.0.1, so the port alone protects
// nothing. The token is written to a 0600 file, which puts the guarantee back
// where a unix socket had it - by convention rather than by the kernel, which
// is the honest cost of a transport Windows can speak.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func writeRendezvous(addr, token string) (string, error) {
	path, err := RendezvousPath()
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(rendezvous{Address: addr, Token: token, PID: os.Getpid()})
	if err != nil {
		return "", err
	}
	// 0600, and written whole: a client reading a half-written token would
	// fail to connect for a reason nothing could explain.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

func readRendezvous() (rendezvous, error) {
	var r rendezvous
	path, err := RendezvousPath()
	if err != nil {
		return r, err
	}
	body, err := os.ReadFile(path) //nolint:gosec // a per-user path this package chose
	if err != nil {
		return r, fmt.Errorf(
			"control: no workbench has left an address at %s: %w", path, err)
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return r, fmt.Errorf("control: %s is not readable as an address: %w", path, err)
	}
	return r, nil
}
