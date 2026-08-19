package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/MeshBench/meshbench/internal/packet"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// A scope code is an HMAC over the payload keyed on the region, so the only
// honest thing a dissector can do is recompute each candidate's code and see
// which reproduces the one on the wire. This builds a frame genuinely scoped
// to "#sco" the way the firmware would, and checks the panel confirms it.
func TestScopeIsConfirmedAgainstCandidates(t *testing.T) {
	payload := []byte{0x11, 0x90, 0x46, 0xAA, 0xBB}
	const payloadType = 0x05

	code := func(region string) uint16 {
		digest := sha256.Sum256([]byte(region))
		mac := hmac.New(sha256.New, digest[:16])
		mac.Write(append([]byte{payloadType}, payload...))
		return binary.LittleEndian.Uint16(mac.Sum(nil)[:2])
	}

	frame := []byte{0x00 | (payloadType << 2)} // transport flood
	frame = binary.LittleEndian.AppendUint16(frame, code("#sco"))
	frame = binary.LittleEndian.AppendUint16(frame, 0)
	frame = append(frame, 0x00) // no path
	frame = append(frame, payload...)

	s := &Sim{nodes: []scenario.Node{
		{Name: "a", Regions: []string{"#sco", "#fif"}},
		{Name: "b", Regions: []string{"#ioi"}},
	}}
	got := s.scopeOf(frame, packet.Dissect(frame))
	if !got.Scoped {
		t.Fatal("a transport-flood frame was reported as unscoped")
	}
	if got.Name != "#sco" {
		t.Errorf("scope = %q, want #sco (candidates: %v)", got.Name, s.regionCandidates())
	}
	if got.Candidates != 3 {
		t.Errorf("checked %d candidates, want 3", got.Candidates)
	}
}

// Not matching means we did not hold the name. That is a different fact from
// the packet having no scope, and conflating them is how a confident wrong
// answer gets into a screenshot.
func TestAnUnknownScopeIsNotReportedAsUnscoped(t *testing.T) {
	frame := []byte{0x00 | (0x05 << 2), 0x34, 0x12, 0x00, 0x00, 0x00, 0xAA}
	s := &Sim{nodes: []scenario.Node{{Name: "a", Regions: []string{"#sco"}}}}
	got := s.scopeOf(frame, packet.Dissect(frame))
	if !got.Scoped {
		t.Fatal("a frame carrying a scope code was reported as unscoped")
	}
	if got.Name != "" {
		t.Errorf("claimed scope %q for a code no candidate produces", got.Name)
	}
	if got.Code != "1234" {
		t.Errorf("code = %q, want 1234 - the code is shown even when unmatched", got.Code)
	}
}

// The firmware gives {0,0} its own meaning: addressed to nowhere.
func TestZeroScopeCodesAreNotTreatedAsARegion(t *testing.T) {
	frame := []byte{0x00 | (0x05 << 2), 0, 0, 0, 0, 0x00, 0xAA}
	s := &Sim{nodes: []scenario.Node{{Name: "a", Regions: []string{"#sco"}}}}
	got := s.scopeOf(frame, packet.Dissect(frame))
	if got.Note == "" {
		t.Error("codes {0,0} passed through without their own meaning")
	}
	if got.Name != "" {
		t.Errorf("matched %q against codes that address no region", got.Name)
	}
}
