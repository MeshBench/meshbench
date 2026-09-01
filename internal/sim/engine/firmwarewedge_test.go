// A node that does not answer a tick must not stop the run.
//
// Inside the package rather than beside it, because the fault being pinned is
// one goroutine publishing a node into the engine while another is mid-tick
// over it, and standing in for the attach means making the same two writes
// under the same lock it makes them under.
package engine

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The wire's own message kinds, mirrored rather than reached for: a test
// double should look like the thing it stands in for.
const (
	peerKindTick = 0x02
	peerKindAck  = 0x03
)

// quietBackend runs nothing. The bridge and the connection are what these
// tests drive.
type quietBackend struct{}

func (quietBackend) Start(context.Context, string) error { return nil }
func (quietBackend) Stop() error                         { return nil }
func (quietBackend) Kind() string                        { return "test" }
func (quietBackend) HasConsole() bool                    { return false }
func (quietBackend) ConsoleIn() io.Writer                { return nil }

// peer is a firmware process that is alive and connected, and acknowledges a
// tick only when the test says so.
//
// The gate is the whole point. A node that has died is already handled - its
// connection drops and the bridge says so - and the failure this file is
// about is the opposite one: a node in perfect health, waited on for an
// acknowledgement nobody asked it for.
type peer struct {
	conn  net.Conn
	ticks chan uint32
}

func dialPeer(t *testing.T, addr string) *peer {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial bridge: %v", err)
	}
	p := &peer{conn: c, ticks: make(chan uint32, 64)}
	go p.serve()
	t.Cleanup(func() { _ = c.Close() })
	return p
}

func (p *peer) serve() {
	var hdr [3]byte
	for {
		if _, err := io.ReadFull(p.conn, hdr[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint16(hdr[1:])
		buf := make([]byte, n)
		if n > 0 {
			if _, err := io.ReadFull(p.conn, buf); err != nil {
				return
			}
		}
		if hdr[0] == peerKindTick && n >= 4 {
			p.ticks <- binary.BigEndian.Uint32(buf[:4])
		}
	}
}

// ack answers one tick, the way a node that has caught up does.
func (p *peer) ack(t *testing.T, atMs uint32) {
	t.Helper()
	msg := make([]byte, 3+4)
	msg[0] = peerKindAck
	binary.BigEndian.PutUint16(msg[1:], 4)
	binary.BigEndian.PutUint32(msg[3:], atMs)
	if _, err := p.conn.Write(msg); err != nil {
		t.Fatalf("ack: %v", err)
	}
}

// nextTick is the instant this peer was last asked to reach.
func (p *peer) nextTick(t *testing.T) uint32 {
	t.Helper()
	select {
	case at := <-p.ticks:
		return at
	case <-time.After(5 * time.Second):
		t.Fatal("the engine never sent this node a tick")
		return 0
	}
}

func attachedNode(t *testing.T, e *Engine, name string) *peer {
	t.Helper()
	br, err := firmware.Listen("127.0.0.1:0", name)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	e.Add(testNode(name), &firmware.Node{Bridge: br, Backend: quietBackend{}})
	p := dialPeer(t, br.Addr())
	waitUntilAttached(t, br)
	return p
}

func waitUntilAttached(t *testing.T, br *firmware.Bridge) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if br.Attached() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the bridge never reported a connection attached")
}

func testNode(name string) scenario.Node {
	return scenario.Node{
		Name: name, Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.2, Lon: -3.2}, HeightAGLm: 10,
		Radio: scenario.RadioConfig{
			CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1},
		TxPowerDBm: 22, NoiseFigureDB: 6,
		Antenna: antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: "vertical"},
	}
}

// A node that stops acknowledging must not take the caller with it.
//
// The caller is the store's own goroutine: every verb and every frame in the
// application is behind it, so a wait with no deadline there is not one node's
// problem. It ends now, and it says which node and why - a run that quietly
// dropped somebody would be worse than one that hung, because nobody would
// know the mesh being measured is not the mesh that was described.
func TestANodeThatNeverAcknowledgesEndsItsOwnWait(t *testing.T) {
	e := New(flatGround{}, Config{StepMs: 10, FirmwareTickTimeout: 300 * time.Millisecond})
	defer func() { _ = e.Close() }()

	silent := attachedNode(t, e, "silent")
	talker := attachedNode(t, e, "talker")

	done := make(chan error, 1)
	go func() { done <- e.Step(context.Background()) }()

	// The healthy one answers; the silent one reads its tick and says nothing,
	// which is a node that is up and stuck rather than a node that has gone.
	talker.ack(t, talker.nextTick(t))
	silent.nextTick(t)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Step: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Step never returned: one node that will not acknowledge has stopped the run")
	}

	down := e.FirmwareFailures()
	if len(down) != 1 || down[0].Name != "silent" {
		t.Fatalf("FirmwareFailures = %+v, want the one node that did not answer", down)
	}
	if !strings.Contains(down[0].Why, "silent") {
		t.Errorf("the reason given was %q, which does not name the node", down[0].Why)
	}
}

// A node that arrives mid-tick is not waited on until the tick after.
//
// This is the wedge itself. An attach runs on its own goroutine and publishes
// a node's bridge and its boot offset into the engine when that node is up,
// and the tick walks the same nodes twice - once to send, once to wait. A
// node that appears between the two passes was never sent this tick, so the
// acknowledgement being waited for cannot arrive, and the wait had no
// deadline: every verb and every frame stopped, permanently, on a node that
// was in perfect health.
func TestANodeAttachedMidTickIsNotWaitedOn(t *testing.T) {
	e := New(flatGround{}, Config{StepMs: 10, FirmwareTickTimeout: 2 * time.Second})
	defer func() { _ = e.Close() }()

	held := attachedNode(t, e, "held")

	// The node the attach is about to finish with: in the engine, up, and
	// connected, but not yet published as running firmware.
	lateBr, err := firmware.Listen("127.0.0.1:0", "late")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	e.Add(testNode("late"), nil)
	late := dialPeer(t, lateBr.Addr())
	waitUntilAttached(t, lateBr)

	done := make(chan error, 1)
	go func() { done <- e.Step(context.Background()) }()

	// The tick has reached the first node, so the pass that sends is past the
	// second one. The pause is what makes that certain rather than likely.
	at := held.nextTick(t)
	time.Sleep(50 * time.Millisecond)

	// What an attach does the instant its node is ready, and in the order it
	// does it.
	e.mu.Lock()
	e.nodes[1].Firmware = &firmware.Node{Bridge: lateBr, Backend: quietBackend{}}
	e.nodes[1].BootOffsetMs = 60_000
	e.mu.Unlock()

	held.ack(t, at)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Step: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Step never returned: the tick is waiting on a node it never ticked")
	}

	if down := e.FirmwareFailures(); len(down) != 0 {
		t.Fatalf("a node that was never ticked was dropped from the run: %+v", down)
	}
	select {
	case at := <-late.ticks:
		t.Fatalf("the node that arrived mid-tick was sent tick %d anyway", at)
	default:
	}

	// And it is ticked on the next one, offset and all.
	go func() { done <- e.Step(context.Background()) }()
	if got, want := late.nextTick(t), uint32(20)+60_000; got != want {
		t.Errorf("the second tick asked for %d ms, want %d", got, want)
	}
	late.ack(t, uint32(20)+60_000)
	held.ack(t, held.nextTick(t))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second Step: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the tick after the attach did not finish either")
	}
}

// flatGround is terrain that answers everywhere, so these tests are about the
// firmware handshake and nothing else.
type flatGround struct{}

func (flatGround) ElevationM(_, _ float64) (float64, bool) { return 100, true }
