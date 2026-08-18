// Changing what a companion is, rather than reading it.
//
// Separate from companion.go because the reading side is already at the file
// limit, and because these are the commands that can leave a node worse than
// they found it: a modem setting that does not match its neighbours takes the
// node off the mesh silently, and the only sign is that nothing arrives any
// more.
//
// So each field is sent only when it was asked for, and the reply says which
// ones went. A configure that half worked and reported success is the reason
// the previous version's radio settings were untrustworthy.
package session

import (
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/companion/proto"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gui/state"
)

// pathHashModes is how many bytes of hash each mode makes a hop add.
//
// Three, not four. The path-length byte carries the size in two bits, so four
// looks reachable, but the firmware rejects any mode above 2 outright
// (ERR_CODE_ILLEGAL_ARG) and sends with mode+1 bytes. Offering a fourth
// choice would be offering one that always fails.
var pathHashModes = []int{1, 2, 3}

// setPathHash sets how many bytes of hash each hop adds when this node sends.
//
// Takes bytes, not the mode, because bytes is what the operator chose and the
// off-by-one belongs in one place rather than at every call site.
func setPathHash(en *engine.Node, bytes uint8) error {
	if bytes < 1 || int(bytes) > len(pathHashModes) {
		return fmt.Errorf("path hashes are 1, 2 or 3 bytes, not %d", bytes)
	}
	return en.Firmware.Bridge.Type(compFrame(proto.SetPathHashMode(bytes - 1)))
}

func registerCompanionConfig(st *state.Store, s *Sim) {
	// companion.configure: the node's own settings, as a phone would set them.
	//
	// Everything is optional and nothing is defaulted. A configure that sent
	// every field every time would rewrite settings the operator never
	// touched, and on a modem setting that is how a node leaves the mesh.
	st.Handle("companion.configure", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		c, en, err := s.companionFor(node)
		if err != nil {
			return nil, err
		}
		var done []string
		send := func(what string, frame []byte) error {
			if err := en.Firmware.Bridge.Type(compFrame(frame)); err != nil {
				return err
			}
			done = append(done, what)
			return nil
		}

		if name, ok := stringField(p, "name"); ok && name != "" {
			if err := send("name", proto.SetAdvertName(name)); err != nil {
				return nil, err
			}
		}
		if lat, ok := numField(p, "lat"); ok {
			lon, _ := numField(p, "lon")
			if err := send("position", proto.SetAdvertLatLon(lat, lon)); err != nil {
				return nil, err
			}
		}
		if dbm, ok := numField(p, "tx_dbm"); ok {
			if err := send("tx power", proto.SetTxPower(uint8(dbm))); err != nil {
				return nil, err
			}
		}
		if frame, ok, err := radioFrame(c, p); err != nil {
			return nil, err
		} else if ok {
			if err := send("modem", frame); err != nil {
				return nil, err
			}
		}
		if n, ok := numField(p, "path_hash"); ok {
			if err := setPathHash(en, uint8(n)); err != nil {
				return nil, err
			}
			done = append(done, "path hashes")
			// Read back rather than assumed: the firmware answers OK or
			// ILLEGAL_ARG, and only the device query says what it now holds.
			_ = en.Firmware.Bridge.Type(compFrame(proto.DeviceQuery()))
		}

		if len(done) == 0 {
			return nil, fmt.Errorf(
				"companion.configure needs a name, a position, a tx_dbm, " +
					"a modem setting (freq_khz, bw_khz, sf, cr) or a path_hash")
		}
		c.note("configured: " + strings.Join(done, ", "))
		// The node is the authority on what it now is, so ask it again rather
		// than reflecting back what was sent.
		_ = en.Firmware.Bridge.Type(compFrame(proto.AppStart("meshbench")))
		s.publishCompanions(w)
		return map[string]any{"set": done}, nil
	})
}

// radioFrame builds the modem command, filling anything not given from what
// the node last said it holds.
//
// Filled in rather than left zero because the firmware sets all four at once:
// a frame with SF given and frequency zero fails the range check and leaves
// every field unchanged, so a caller that wanted to change only the spreading
// factor would change nothing and be told it worked.
func radioFrame(c *compSession, p any) ([]byte, bool, error) {
	freq, hasFreq := numField(p, "freq_khz")
	bw, hasBW := numField(p, "bw_khz")
	sf, hasSF := numField(p, "sf")
	cr, hasCR := numField(p, "cr")
	if !hasFreq && !hasBW && !hasSF && !hasCR {
		return nil, false, nil
	}

	c.mu.Lock()
	self := c.self
	c.mu.Unlock()
	if self == nil {
		return nil, false, fmt.Errorf(
			"%s has not said what its radio is yet, so a partial change to it "+
				"cannot be safe - connect and let it answer first", c.node)
	}
	if !hasFreq {
		freq = float64(self.FreqKHz)
	}
	if !hasBW {
		bw = float64(self.BWKHz)
	}
	if !hasSF {
		sf = float64(self.SF)
	}
	if !hasCR {
		cr = float64(self.CR)
	}

	// The firmware's own ranges, checked here so a bad number is a sentence
	// rather than an ERR_CODE_ILLEGAL_ARG with nothing to say which field.
	switch {
	case freq < 150000 || freq > 2500000:
		return nil, false, fmt.Errorf("%g kHz is outside the radio's 150-2500 MHz", freq)
	case bw < 7 || bw > 500:
		return nil, false, fmt.Errorf("%g kHz bandwidth is outside 7-500 kHz", bw)
	case sf < 5 || sf > 12:
		return nil, false, fmt.Errorf("spreading factor %g is not between 5 and 12", sf)
	case cr < 5 || cr > 8:
		return nil, false, fmt.Errorf("coding rate 4/%g is not between 4/5 and 4/8", cr)
	}
	// Bandwidth goes out in Hz while it comes back in kHz - see SetRadioParams.
	return proto.SetRadioParams(uint32(freq), uint32(bw*1000), uint8(sf), uint8(cr)), true, nil
}
