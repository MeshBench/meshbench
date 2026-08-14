package session

import (
	"context"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// Clicking a packet answers with its dissection and everywhere it went.
func TestPacketOpenBuildsTheView(t *testing.T) {
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	nodes := []scenario.Node{
		repeaterNode("Alpha"), repeaterNode("Beta"), repeaterNode("Gamma"),
	}
	// A hundred metres apart: everything hears everything over bare earth.
	nodes[1].Position.Lon += 0.001
	nodes[2].Position.Lon -= 0.001
	s.build(nodes, 869.618)

	s.eng.Inject(0, []byte("msim-packet-test"))
	for i := 0; i < 40; i++ {
		_ = s.eng.Step(context.Background())
	}
	events := s.eng.Events()
	var id uint64
	for _, ev := range events {
		if ev.Kind == "tx" {
			id = ev.PacketID
		}
	}
	if id == 0 {
		t.Fatal("the injection produced no transmission")
	}

	if _, err := st.Do(ctx, "packet.open", map[string]any{"id": float64(id)}); err != nil {
		t.Fatalf("packet.open: %v", err)
	}
	snap := st.Snapshot()
	pk := snap.Packet
	if pk == nil {
		t.Fatal("no packet in the snapshot")
	}
	if pk.Origin != "Alpha" {
		t.Errorf("origin %q, want Alpha", pk.Origin)
	}
	if len(pk.Fates) == 0 {
		t.Error("a packet with events has no fates")
	}
	if len(pk.RawLines) == 0 {
		t.Error("no hex dump for a frame that flew")
	}
	if len(pk.Ledger) == 0 {
		t.Error("the reception ledger is empty for a heard packet")
	}
	if pk.Transmissions == 0 {
		t.Error("the journey has no transmissions")
	}

	// The run's counts know about it too.
	if _, err := st.Do(ctx, "sim.state", nil); err != nil {
		t.Fatal(err)
	}
	if c := s.eventCounts(); c.Sent == 0 || c.Received == 0 {
		t.Errorf("event counts %+v never saw the injection", c)
	}

	if _, err := st.Do(ctx, "packet.close", nil); err != nil {
		t.Fatal(err)
	}
	if st.Snapshot().Packet != nil {
		t.Error("packet.close left the packet open")
	}
	// An unknown id refuses rather than answering with an empty view.
	if _, err := st.Do(ctx, "packet.open", map[string]any{"id": float64(1 << 60)}); err == nil {
		t.Error("an unknown packet id opened a view")
	}
}

// The classifier puts every kind somewhere the chips can find it.
func TestEventClassBuckets(t *testing.T) {
	cases := map[string]struct{ kind, detail string }{
		"sent":         {"tx", "32 bytes, 120 ms on air"},
		"received":     {"rx", "first time this node heard the message"},
		"half-duplex":  {"miss", "its own transmitter was keyed; LoRa is half duplex"},
		"interference": {"miss", "would have decoded at -3.0 dB, lost to a stronger interferer"},
		"floor":        {"miss", "SNR -14.0 dB against -12.5 dB needed at SF10"},
	}
	for want, c := range cases {
		if got := engine.EventClass(c.kind, c.detail); got != want {
			t.Errorf("EventClass(%q, %q) = %q, want %q", c.kind, c.detail, got, want)
		}
	}
}
