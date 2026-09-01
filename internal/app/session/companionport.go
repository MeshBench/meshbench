// The port under the companion verbs: who holds it, what is said on taking
// it, and writing bytes straight at it.
//
// Split out because companion.go, which holds the verbs a client's view is
// built from, had reached the file limit. What is here is the byte pipe below
// those verbs rather than any view of a node: the claim, the frames a fresh
// companion is told before anything else, the check that something is reading
// at the far end, and the one verb that writes bytes without decoding them.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/proto"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

func registerCompanionRaw(st *state.Store, s *Sim) {
	st.Handle("companion.raw", func(_ *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		c, en, err := s.companionFor(node)
		if err != nil {
			return nil, err
		}
		var b []byte
		if m, ok := p.(map[string]any); ok {
			if xs, ok := m["bytes"].([]any); ok {
				for _, x := range xs {
					if v, ok := x.(float64); ok {
						b = append(b, byte(v))
					}
				}
			}
		}
		if len(b) == 0 {
			return nil, fmt.Errorf("companion.raw needs bytes")
		}
		if err := en.Firmware.Bridge.Type(compFrame(b)); err != nil {
			return nil, err
		}
		c.note(fmt.Sprintf("raw: %d bytes", len(b)))
		return map[string]any{"sent_bytes": len(b)}, nil
	})
}

// companionBootFrames is the clock and the modem, queued before anything
// else a fresh companion is told.
//
// The same two a sweep's own companion sender needs before it can originate
// (companionSetup in experimentrun.go), and for the same reason: neither
// reaches a companion build through the text console the rest of
// provisioning uses - a companion has no command line, only this protocol -
// so left unset a companion boots on its firmware's own factory defaults,
// the deprecated wide preset and a clock nothing else in the run agrees
// with, rather than the scenario it was told to be. Untreated that read as a
// radio that had stopped receiving rather than one still on its factory
// settings.
//
// The clock is the run's own epoch plus however much simulated time has
// already passed, not the bare epoch: a companion connected after other
// nodes have been running would otherwise set its clock behind theirs and
// see every reply since as a replay.
func companionBootFrames(en *engine.Node, nowMs uint32) [][]byte {
	out := [][]byte{proto.SetDeviceTime(uint32(scenarioEpoch) + nowMs/1000)}
	// Only what the scenario actually states. A zeroed radio sent anyway is
	// ERR_CODE_ILLEGAL_ARG at best, and a zeroed TX power is worse: 0 dBm is
	// a legal setting, so the firmware would take it and go quiet.
	if r := en.Spec().Radio; r.CentreHz > 0 {
		out = append(out, proto.SetRadioParams(uint32(r.CentreHz/1000),
			uint32(r.BandwidthHz), uint8(r.SpreadFactor), uint8(r.CodingRate+4)))
	}
	if en.Spec().TxPowerDBm > 0 {
		out = append(out, proto.SetTxPower(uint8(en.Spec().TxPowerDBm)))
	}
	return out
}

// silentCompanion refuses a command to a node that has never answered.
//
// Writing at a serial port succeeds whether or not anything is reading it, so
// every companion command reported itself sent - against a board whose
// firmware never started, against a build that is not a companion, against a
// node that had crashed twenty minutes ago. "It says advert sent and nothing
// transmits" is that sentence, and the interface was the last thing anybody
// would suspect.
//
// A session that has decoded even one frame is talking, and stays trusted: a
// node can be busy without being dead, and refusing on a slow reply would be
// worse than the fault this catches.
func silentCompanion(c *compSession, node string) error {
	if c.Answered() {
		return nil
	}
	return fmt.Errorf(
		"%s has not answered anything since it was connected, so this would be "+
			"written at a port with nothing reading it - check the Output tab for "+
			"what its firmware is doing", node)
}

func (s *Sim) companionFor(node string) (*compSession, *engine.Node, error) {
	c, ok := s.comps[node]
	if !ok {
		return nil, nil, fmt.Errorf("%s is not connected; companion.connect first", node)
	}
	en, ok := s.eng.NodeByName(node)
	if !ok || en.Firmware == nil {
		return nil, nil, fmt.Errorf("%s runs no firmware", node)
	}
	return c, en, nil
}

// connectCompanion claims a node's port for the companion protocol.
//
// Called from inside a handler, so it does not go through the store: asking
// the store to do something while running on it is a wait for yourself.
func (s *Sim) connectCompanion(node string) error {
	if s.comps == nil {
		s.comps = map[string]*compSession{}
	}
	if _, already := s.comps[node]; already {
		return nil
	}
	// The same refusal companion.connect makes: typing a CLI line must not
	// quietly take the port from an attached outside client.
	if _, serving := s.servedLink(node); serving {
		return fmt.Errorf("%s is being served to an outside client; stop serving first", node)
	}
	if s.eng == nil {
		return fmt.Errorf("no network loaded")
	}
	en, ok := s.eng.NodeByName(node)
	if !ok || en.Firmware == nil {
		return fmt.Errorf("%s runs no firmware, so it has no companion interface", node)
	}
	c := &compSession{node: node}
	c.release = en.Firmware.Bridge.Claim(c)
	s.comps[node] = c
	if err := en.Firmware.Bridge.Type(compFrame(proto.AppStart("meshbench"))); err != nil {
		return err
	}
	for _, f := range companionBootFrames(en, s.eng.NowMs()) {
		_ = en.Firmware.Bridge.Type(compFrame(f))
	}
	// Named here too: the CLI can be the first thing to connect, not only
	// the client, and a companion is nameless until something sets it.
	_ = en.Firmware.Bridge.Type(compFrame(proto.SetAdvertName(node)))
	return en.Firmware.Bridge.Type(compFrame(proto.AppStart("meshbench")))
}
