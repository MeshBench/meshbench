package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/capture"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
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

// A packet view opened while its message is still spreading has to keep
// growing, not freeze at whatever the flood looked like on the click - that
// is the entire point of opening one mid-run instead of waiting for it to
// finish.
func TestRefreshOpenPacketGrowsTheViewAsTheFloodContinues(t *testing.T) {
	s := &Sim{}
	nodes := []scenario.Node{
		repeaterNode("Alpha"), repeaterNode("Beta"), repeaterNode("Gamma"),
	}
	nodes[1].Position.Lon += 0.001
	nodes[2].Position.Lon -= 0.001
	s.build(nodes, 869.618)

	s.eng.Inject(0, []byte("msim-live-packet-test"))
	for i := 0; i < 10; i++ {
		_ = s.eng.Step(context.Background())
	}
	var id uint64
	for _, ev := range s.eng.Events() {
		if ev.Kind == "tx" {
			id = ev.PacketID
			break
		}
	}
	if id == 0 {
		t.Fatal("the injection produced no transmission")
	}

	// packet.open's own path: build once, synced to the event count as of
	// that build - refreshOpenPacket only knows to skip once it has a
	// baseline to compare against.
	w := &state.World{Packet: s.buildPacket(id)}
	if w.Packet == nil {
		t.Fatal("buildPacket returned nil for a real transmission")
	}
	s.lastPacketEvents = s.eng.EventCount()
	firstTransmissions := w.Packet.Transmissions

	// Nothing new has happened: a refresh must not even rebuild, let alone
	// change anything - the pointer staying put is how the skip is proven.
	before := w.Packet
	s.refreshOpenPacket(w)
	if w.Packet != before {
		t.Error("refreshOpenPacket rebuilt the view with no new events since the last one")
	}

	// More of the run happens - possibly more relays of the same message,
	// possibly not, but definitely more events - and the view has to notice.
	for i := 0; i < 20; i++ {
		_ = s.eng.Step(context.Background())
	}
	s.refreshOpenPacket(w)
	if w.Packet == before {
		t.Error("refreshOpenPacket did not rebuild after new events landed")
	}
	if w.Packet.Transmissions < firstTransmissions {
		t.Errorf("transmissions went from %d to %d - the view lost ground, not gained it",
			firstTransmissions, w.Packet.Transmissions)
	}
}

// ForPacket answers with every node in the scenario for one transmission, by
// its own design - a node clean out of range still gets a row saying so.
// Merging that across a multi-hop journey by simple concatenation repeats
// that full census once per hop: this is the bug reported as "offered to
// 732" on a 360-node scenario that only took two hops. The merge has to keep
// every row where something measurable happened, including a node offered
// on more than one hop, but say "never saw it" about any one node exactly
// once for the whole journey, not once per transmission it missed.
func TestLedgerAcrossJourneyDoesNotRepeatTheFullCensusPerHop(t *testing.T) {
	var ledger capture.Ledger
	// Hop 0, sent by orig: a and b are in range, c is not.
	ledger.Record(capture.Reception{PacketID: 1, FromNode: "orig", ToNode: "a", Offered: true, Demod: true, CRCOK: true, FirmwareSaw: true})
	ledger.Record(capture.Reception{PacketID: 1, FromNode: "orig", ToNode: "b", Offered: true, Demod: true, CRCOK: true, FirmwareSaw: true})
	ledger.Record(capture.Reception{PacketID: 1, FromNode: "orig", ToNode: "c", Offered: false})
	// Hop 1, sent by a: b hears it again (genuinely offered twice), c still
	// out of range, and orig - the transmitter three hops back - is now out
	// of range too.
	ledger.Record(capture.Reception{PacketID: 2, FromNode: "a", ToNode: "b", Offered: true, Demod: true, CRCOK: true, FirmwareSaw: true})
	ledger.Record(capture.Reception{PacketID: 2, FromNode: "a", ToNode: "c", Offered: false})
	ledger.Record(capture.Reception{PacketID: 2, FromNode: "a", ToNode: "orig", Offered: false})
	// Hop 2, sent by b: c finally hears one, orig still does not.
	ledger.Record(capture.Reception{PacketID: 3, FromNode: "b", ToNode: "c", Offered: true, Demod: true, CRCOK: true, FirmwareSaw: true})
	ledger.Record(capture.Reception{PacketID: 3, FromNode: "b", ToNode: "orig", Offered: false})

	hops := []state.PacketHop{
		{PacketID: 1, By: "orig", Hops: 0},
		{PacketID: 2, By: "a", Hops: 1},
		{PacketID: 3, By: "b", Hops: 2},
	}
	rows := ledgerAcrossJourney(ledger, hops)

	byNode := map[string][]state.PacketReception{}
	for _, r := range rows {
		byNode[r.Node] = append(byNode[r.Node], r)
	}
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5 (a:1, b:2, c:1, orig:1): %+v", len(rows), rows)
	}
	if got := len(byNode["a"]); got != 1 || !byNode["a"][0].Offered {
		t.Errorf("a: got %d rows (want 1, offered): %+v", got, byNode["a"])
	}
	if got := len(byNode["b"]); got != 2 {
		t.Errorf("b was genuinely offered on two hops but got %d rows: %+v", got, byNode["b"])
	}
	for _, r := range byNode["b"] {
		if !r.Offered {
			t.Errorf("b's row %+v is not marked offered", r)
		}
	}
	// c was out of range for two hops and offered on the third: only the
	// offered row should survive, not the two "never saw it" rows that came
	// before it actually heard something.
	if got := len(byNode["c"]); got != 1 || !byNode["c"][0].Offered {
		t.Errorf("c: got %d rows (want 1, offered): %+v", got, byNode["c"])
	}
	// orig was out of range on both later hops: one row, not two.
	if got := len(byNode["orig"]); got != 1 || byNode["orig"][0].Offered {
		t.Errorf("orig: got %d rows (want 1, not offered): %+v", got, byNode["orig"])
	}
}

// The "why?" button reads Why off the row it was clicked on, so a reception
// that was offered and failed has to carry the engine's own reason - not a
// stranger's, and not silence for the one case (out of range) that never
// had a reason to begin with.
func TestLedgerAcrossJourneyCarriesTheFailureReason(t *testing.T) {
	var ledger capture.Ledger
	ledger.Record(capture.Reception{PacketID: 1, FromNode: "orig", ToNode: "weak", Offered: true, Demod: false})
	ledger.Record(capture.Reception{PacketID: 1, FromNode: "orig", ToNode: "far", Offered: false})

	hops := []state.PacketHop{
		{
			PacketID: 1, By: "orig", Hops: 0,
			MissedBy: []string{"weak"},
			MissWhy:  []string{"SNR -9 dB against -10 dB needed at SF8"},
		},
	}
	rows := ledgerAcrossJourney(ledger, hops)

	byNode := map[string]state.PacketReception{}
	for _, r := range rows {
		byNode[r.Node] = r
	}
	if got := byNode["weak"].Why; got != "SNR -9 dB against -10 dB needed at SF8" {
		t.Errorf("weak's Why = %q, want the engine's own reason", got)
	}
	if got := byNode["far"].Why; got != "" {
		t.Errorf("far was never offered and has no reason to carry, got Why = %q", got)
	}
}

// This is the bug reported against the reception ledger: a node missed on an
// early hop and accepted on a later one showed as "never saw it" in the
// table while the "why?" modal - reading the very same history - showed the
// later hop's accept. Both read pk.Ledger; the table just never collapsed
// the journey's several rows for one node down to the one that mattered.
func TestCollapseLedgerPrefersAnAcceptOverAnEarlierMiss(t *testing.T) {
	var ledger capture.Ledger
	ledger.Record(capture.Reception{PacketID: 1, FromNode: "relay1", ToNode: "target", Offered: true, Demod: false})
	ledger.Record(capture.Reception{PacketID: 2, FromNode: "relay2", ToNode: "target", Offered: true, Demod: true, CRCOK: true, FirmwareSaw: true})

	hops := []state.PacketHop{
		{
			PacketID: 1, By: "relay1", Hops: 1,
			MissedBy: []string{"target"},
			MissWhy:  []string{"its own transmitter was keyed; LoRa is half duplex"},
		},
		{PacketID: 2, By: "relay2", Hops: 2, Heard: []string{"target"}},
	}
	collapsed := collapseLedger(ledgerAcrossJourney(ledger, hops))

	var got *state.PacketReception
	for i := range collapsed {
		if collapsed[i].Node == "target" {
			got = &collapsed[i]
		}
	}
	if got == nil {
		t.Fatal("target has no row at all")
	}
	if got.Firmware != "accepted" {
		t.Errorf("target's row says %q, want \"accepted\" - the earlier miss must not win over the later accept", got.Firmware)
	}
	if got.From != "relay2" {
		t.Errorf("target's row credits %q, want relay2 (who it actually accepted the message from)", got.From)
	}
}

// A node offered on several hops and never accepted still collapses to one
// row, and that row carries a real reason rather than "never saw it" - the
// out-of-range answer is reserved for a node that was never offered at all.
func TestCollapseLedgerFallsBackToAReasonedMiss(t *testing.T) {
	var ledger capture.Ledger
	ledger.Record(capture.Reception{PacketID: 1, FromNode: "relay1", ToNode: "target", Offered: true, Demod: false})
	ledger.Record(capture.Reception{PacketID: 2, FromNode: "relay2", ToNode: "target", Offered: true, Demod: false})

	hops := []state.PacketHop{
		{
			PacketID: 1, By: "relay1", Hops: 1,
			MissedBy: []string{"target"},
			MissWhy:  []string{"its own transmitter was keyed; LoRa is half duplex"},
		},
		{
			PacketID: 2, By: "relay2", Hops: 2,
			MissedBy: []string{"target"},
			MissWhy:  []string{"would have decoded at 3.1 dB, lost to a stronger interferer"},
		},
	}
	collapsed := collapseLedger(ledgerAcrossJourney(ledger, hops))

	if len(collapsed) != 1 {
		t.Fatalf("got %d rows, want 1 (one per node): %+v", len(collapsed), collapsed)
	}
	if collapsed[0].Firmware != "never saw it" {
		t.Errorf("Firmware = %q, want \"never saw it\" - it was offered, but never decoded", collapsed[0].Firmware)
	}
	if collapsed[0].Why != "its own transmitter was keyed; LoRa is half duplex" {
		t.Errorf("Why = %q, want the first hop's own reason", collapsed[0].Why)
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
