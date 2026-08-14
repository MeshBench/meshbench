package provider_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/MeshBench/meshbench/internal/provider"
)

// scopedFrame builds a transport-flood packet scoped to a region, the way the
// firmware does: the code is an HMAC over payload type and payload under the
// region's key.
func scopedFrame(region string, payloadType byte, payload []byte, path ...byte) []byte {
	key := provider.RegionKey(region)
	msg := append([]byte{payloadType}, payload...)
	mac := hmac.New(sha256.New, key[:])
	mac.Write(msg)
	code := binary.LittleEndian.Uint16(mac.Sum(nil)[:2])

	f := []byte{0x00 | (payloadType << 2)} // ROUTE_TYPE_TRANSPORT_FLOOD
	f = binary.LittleEndian.AppendUint16(f, code)
	f = binary.LittleEndian.AppendUint16(f, 0) // second code: the home region
	f = append(f, byte(len(path)))
	f = append(f, path...)
	return append(f, payload...)
}

// A candidate name turns "scoped to something" into "scoped to mesh-east".
func TestNamedRegionsIdentifiesTheRegion(t *testing.T) {
	payload := []byte{0x11, 0x22, 0x33, 0x44}
	frame := scopedFrame("#fife", 0x05, payload)

	m := provider.NewNamedRegions([]string{"#edi", "#fife", "#tay"})
	got := m.Match(frame, []uint16{binary.LittleEndian.Uint16(frame[1:3])})
	if len(got) != 1 || got[0] != "#fife" {
		t.Fatalf("matched %v, want [#fife]", got)
	}

	// A region nobody named cannot be identified — and must not be guessed at.
	// The traffic still reads as scoped, which is the honest answer.
	m2 := provider.NewNamedRegions([]string{"#edi", "#tay"})
	if got := m2.Match(frame, []uint16{binary.LittleEndian.Uint16(frame[1:3])}); len(got) != 0 {
		t.Errorf("a packet scoped to an unknown region matched %v", got)
	}
}

// The path is excluded from the code, which is what lets one message be
// recognised at every hop even though its bytes on the air change at each.
func TestRegionMatchSurvivesRelaying(t *testing.T) {
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	origin := scopedFrame("#tay", 0x04, payload)
	relayed := scopedFrame("#tay", 0x04, payload, 0x9F, 0x3C)

	m := provider.NewNamedRegions([]string{"#tay"})
	c1 := []uint16{binary.LittleEndian.Uint16(origin[1:3])}
	c2 := []uint16{binary.LittleEndian.Uint16(relayed[1:3])}
	if binary.LittleEndian.Uint16(origin[1:3]) != binary.LittleEndian.Uint16(relayed[1:3]) {
		t.Fatal("the transport code changed when the packet was relayed")
	}
	if len(m.Match(origin, c1)) != 1 || len(m.Match(relayed, c2)) != 1 {
		t.Error("a relayed packet no longer matches its own region")
	}
}

// End to end: observed traffic, candidate names, and what each node's own
// behaviour proves about its configuration.
func TestInferenceNamesRegionsFromObservedTraffic(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03}
	m := provider.NewNamedRegions([]string{"#fife", "#edi"})

	packets := []provider.PacketRecord{
		// alpha originated an advert scoped to #fife: that is its default scope.
		{Raw: scopedFrame("#fife", 0x04, payload), Sender: "alpha", Origin: "alpha"},
		// bravo relayed it, so it holds #fife and allows flooding for it.
		{Raw: scopedFrame("#fife", 0x04, payload, 0x9F), Sender: "bravo"},
		// bravo also relays #edi traffic — a node may hold several regions.
		{Raw: scopedFrame("#edi", 0x05, payload, 0x9F), Sender: "bravo"},
	}
	got := provider.InferFromPackets(packets, m)

	if a := got["alpha"]; a == nil || a.DefaultScope != "#fife" {
		t.Errorf("alpha's default scope came out as %q, want #fife", got["alpha"].DefaultScope)
	}
	b := got["bravo"]
	if b == nil || len(b.Regions) != 2 {
		t.Fatalf("bravo's regions: %v, want both #fife and #edi", b.Regions)
	}
}
