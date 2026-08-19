package session

import (
	"net"
	"strings"
	"testing"
)

// A served companion says where a client can reach it.
//
// It was bound to 127.0.0.1, so the port existed and nothing outside this
// computer could use it - and once it was bound to every interface the panel
// showed "[::]:43437", which is not a thing anybody can type into a client on
// another machine.
func TestReachableAddrsAreOnesAClientCanUse(t *testing.T) {
	got := reachableAddrs("[::]:43437")
	if len(got) == 0 {
		t.Fatal("a bound port produced no address a client could use")
	}
	for _, a := range got {
		host, port, err := net.SplitHostPort(a)
		if err != nil {
			t.Fatalf("%q is not an address:port", a)
		}
		if port != "43437" {
			t.Errorf("%q carries the wrong port", a)
		}
		ip := net.ParseIP(host)
		if ip == nil {
			t.Errorf("%q has no address in it", a)
			continue
		}
		if ip.IsUnspecified() {
			t.Errorf("%q is a bind address, not somewhere to point a client", a)
		}
		if ip.To4() == nil {
			t.Errorf("%q is not IPv4; a zone-carrying address cannot be typed in", a)
		}
	}
	// Loopback last rather than missing: on a machine with no network it is
	// the only answer there is.
	if last := got[len(got)-1]; !strings.HasPrefix(last, "127.0.0.1:") {
		t.Errorf("the last address is %q, want loopback as the fallback", last)
	}
}

func TestReachableAddrsRefusesNonsense(t *testing.T) {
	if got := reachableAddrs("not-an-address"); got != nil {
		t.Fatalf("an unparseable bind address produced %v", got)
	}
}
