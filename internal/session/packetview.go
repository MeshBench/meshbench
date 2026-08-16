// One packet, dissected, with everywhere it went.
//
// The old workbench's packet view, behind a verb: CoreScope's dissection with
// the thing CoreScope cannot have - the same packet's fate at every node,
// because the simulator watched all of them. The dissection is shared with
// the Wireshark plugin's field set, so the two cannot drift into disagreeing
// about the same bytes.
package session

import (
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/capture"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gui/state"
)

func registerPacket(st *state.Store, s *Sim) {
	// packet.open dissects one transmission by its id. On the store's
	// goroutine deliberately: the ledger and the event log belong to the
	// engine, which steps on this goroutine, and a click can afford the
	// milliseconds a scan costs.
	st.Handle("packet.open", func(w *state.World, p any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no simulation")
		}
		id := uint64(0)
		if v, ok := numField(p, "id"); ok {
			id = uint64(v)
		}
		if id == 0 {
			return nil, fmt.Errorf("packet.open needs the packet's id")
		}
		// seek walks to the next packet that still exists in that direction,
		// for the view's previous/next arrows - ids are dense but a frame can
		// be gone from the ledger.
		seek := 0
		if v, ok := numField(p, "seek"); ok {
			seek = int(v)
		}
		pk := s.buildPacket(id)
		for tries := 0; pk == nil && seek != 0 && tries < 50; tries++ {
			if seek < 0 && id <= 1 {
				break
			}
			id = uint64(int64(id) + int64(seek))
			pk = s.buildPacket(id)
		}
		if pk == nil {
			return nil, fmt.Errorf("packet %d is no longer in the ledger", id)
		}
		w.Packet = pk
		w.Say(fmt.Sprintf("packet #%d: from %s, heard by %d, missed by %d",
			id, pk.Origin, pk.Heard, pk.Missed))
		// Transmissions is how many times the message was put on the air, so a
		// caller can tell a relayed flood from a single advert without opening
		// the window and reading the header.
		return map[string]any{"id": id, "origin": pk.Origin,
			"heard": pk.Heard, "missed": pk.Missed,
			"transmissions": pk.Transmissions, "reached": pk.Reached}, nil
	})

	st.Handle("packet.close", func(w *state.World, _ any) (any, error) {
		w.Packet = nil
		return nil, nil
	})
}

// refreshOpenPacket keeps an open packet view live while its message is
// still propagating, instead of freezing it at whatever the run looked like
// the moment it was clicked - the whole point of opening one mid-flood is
// watching the Journey and the reception ledger keep growing under it.
//
// Two gates before the rebuild, because the rebuild is expensive: it copies
// the whole event log, walks it several times and dissects every transmission
// in the run. Doing that per tick is the quadratic cost EventsTail was
// introduced to remove, and "a packet window is open" is the state anybody
// watching a flood is permanently in.
//
//   - EventCount is length-only, so a tick where nothing happened anywhere
//     costs nothing.
//   - When something did happen, only the events that are actually new are
//     copied, and the rebuild is skipped unless one of them belongs to the
//     message being followed. On a busy mesh almost all traffic belongs to
//     some other message, so almost every tick stops here.
func (s *Sim) refreshOpenPacket(w *state.World) {
	if w.Packet == nil {
		return
	}
	n := s.eng.EventCount()
	if n == s.lastPacketEvents {
		return
	}
	arrived := n - s.lastPacketEvents
	s.lastPacketEvents = n
	if arrived > 0 && !s.touchesPacket(w.Packet, arrived) {
		return
	}
	// A rebuild that finds nothing (the frame has aged out of the event
	// log) leaves the last good view in place rather than blanking the
	// window out from under whoever is looking at it.
	if pk := s.buildPacket(w.Packet.ID); pk != nil {
		w.Packet = pk
	}
}

// touchesPacket reports whether any of the last n events belongs to the
// message this view is following - by message where the frame carried one,
// and by packet id otherwise, so a view opened on a frame the engine never
// gave a message id still refreshes.
func (s *Sim) touchesPacket(pk *state.Packet, n int) bool {
	tail, _ := s.eng.EventsTail(n)
	for _, ev := range tail {
		if ev.PacketID == pk.ID {
			return true
		}
		if pk.MessageID != 0 && ev.MessageID == pk.MessageID {
			return true
		}
	}
	return false
}

// buildPacket gathers everything the run knows about one transmission.
func (s *Sim) buildPacket(id uint64) *state.Packet {
	events := s.eng.Events()

	pk := &state.Packet{ID: id}
	var frame []byte
	var msgEvents []engine.Event
	// One pass: this packet's own events, and every event of the message it
	// carries, for the journey.
	var msgID uint64
	for _, ev := range events {
		if ev.PacketID == id {
			if msgID == 0 && ev.MessageID != 0 {
				msgID = ev.MessageID
			}
			if ev.Frame != nil && frame == nil {
				frame = ev.Frame
			}
			switch ev.Kind {
			case "tx":
				pk.Origin, pk.AtMs = ev.From, ev.AtMs
			case "rx":
				pk.Heard++
			case "miss":
				pk.Missed++
			}
			node := ev.To
			what := ev.Detail
			if ev.Kind == "tx" {
				node, what = ev.From, "transmitted"
			}
			pk.Fates = append(pk.Fates, state.PacketFate{
				AtMs: ev.AtMs, Node: node, Kind: ev.Kind,
				SNRdB: ev.SNRdB, What: what,
			})
		}
	}
	if frame == nil {
		return nil
	}
	pk.MessageID = msgID
	for _, ev := range events {
		if msgID != 0 && ev.MessageID == msgID {
			msgEvents = append(msgEvents, ev)
		}
	}

	// The dissection, in the plugin's own field set.
	d := capture.Dissect(frame)
	if d.Truncated {
		pk.Malformed = d.Problem
	}
	pk.RouteType = fmt.Sprintf("%s (%d)", d.RouteName, d.RouteType)
	pk.PayloadType = fmt.Sprintf("%s (0x%02X)", d.PayloadName, d.PayloadType)
	pk.Version = fmt.Sprint(d.Version)
	if d.HasTransport {
		pk.Transport = fmt.Sprintf("%04X %04X", d.TransportCodes[0], d.TransportCodes[1])
	}
	// Hop hashes resolved to names where the run has watched that node relay:
	// MeshCore's path hash is the first byte of a public key the simulator
	// does not hold, so this matches on observed relays. Approximate by
	// construction, and left as hex where it fails.
	hashNames := map[byte]string{}
	for _, ev := range events {
		if ev.Kind != "tx" || ev.Frame == nil {
			continue
		}
		td := capture.Dissect(ev.Frame)
		if n := len(td.PathHashes); n > 0 {
			hashNames[td.PathHashes[n-1]] = ev.From
		}
	}
	// One entry per hop, not per byte. MeshCore's hash size is variable and
	// encoded in the path-length byte, so iterating the bytes reported six
	// hops for a two-hop path of three-byte hashes - and the Structure panel
	// beside it, reading the same frame, correctly said two.
	//
	// A trace is skipped entirely: its path area carries the SNR each hop
	// measured, not hashes, so there are no relay names in there to resolve.
	if d.PayloadType != 0x09 {
		for i := 0; i < d.HopCount(); i++ {
			h := d.Hop(i)
			if len(h) == 0 {
				continue
			}
			// Named on the hash's last byte, which is what the transmit scan
			// above keys on.
			if name, ok := hashNames[h[len(h)-1]]; ok {
				pk.Path = append(pk.Path, fmt.Sprintf("%s (%X)", name, h))
			} else {
				pk.Path = append(pk.Path, fmt.Sprintf("%X", h))
			}
		}
	}
	pk.Hops = d.HopCount()
	pk.PayloadFields = asPacketFields(d.PayloadFields)
	pk.PathFields = asPacketFields(d.PathFields)
	for _, sp := range d.Spans {
		pk.Spans = append(pk.Spans, state.PacketSpan{
			Name: sp.Name, Offset: sp.Offset, Size: sp.Length, Detail: sp.Detail})
		if strings.HasPrefix(sp.Name, "payload") {
			pk.Readable = sp.Detail
		}
	}
	if len(d.PayloadFields) == 0 {
		pk.PayloadNote = fmt.Sprintf(
			"%d bytes, encrypted or of a type with nothing in clear", len(d.Payload))
	}
	pk.Scope = s.scopeOf(frame, d)
	pk.RawLines = hexDump(frame)
	pk.Raw = frame

	// The journey: one row per transmission of the message, in time order,
	// with who heard that particular relay. Possible only because identity
	// is taken from the payload - the bytes on the air differ at every hop,
	// because each relay appends itself to the path.
	byPacket := map[uint64]int{}
	for _, ev := range msgEvents {
		switch ev.Kind {
		case "tx":
			hops := 0
			if ev.Frame != nil {
				hops = capture.Dissect(ev.Frame).HopCount()
			}
			byPacket[ev.PacketID] = len(pk.Journey)
			pk.Journey = append(pk.Journey, state.PacketHop{
				AtMs: ev.AtMs, By: ev.From, Hops: hops, PacketID: ev.PacketID})
		case "rx":
			if i, ok := byPacket[ev.PacketID]; ok {
				pk.Journey[i].Heard = append(pk.Journey[i].Heard, ev.To)
			}
		case "miss":
			if i, ok := byPacket[ev.PacketID]; ok {
				pk.Journey[i].MissWhy = append(pk.Journey[i].MissWhy, ev.Detail)
				pk.Journey[i].Missed++
				pk.Journey[i].MissedBy = append(pk.Journey[i].MissedBy, ev.To)
			}
		}
	}
	reached := map[string]bool{}
	for _, h := range pk.Journey {
		for _, n := range h.Heard {
			reached[n] = true
		}
	}
	pk.Transmissions, pk.Reached = len(pk.Journey), len(reached)

	// The radio-level truth, per receiver, for every transmission of the
	// message - a journey spans every hop, and the clicked packet is only
	// one of them. ForPacket(id) alone told the ledger about a single frame
	// instance and left every other hop's receptions out of it.
	pk.LedgerFull = ledgerAcrossJourney(s.eng.Ledger, pk.Journey)
	pk.Ledger = collapseLedger(pk.LedgerFull)
	return pk
}

// asPacketFields carries the dissector's fields across to the view, offsets
// and all - they are what lets a selected field highlight its own bytes.
func asPacketFields(in []capture.Field) []state.PacketField {
	out := make([]state.PacketField, 0, len(in))
	for _, f := range in {
		out = append(out, state.PacketField{
			Name: f.Name, Value: f.Value, Decoded: f.Decoded,
			Description: f.Description, Offset: f.Offset, Size: f.Length,
		})
	}
	return out
}

// hexDump is the frame as the eye expects it: offset, sixteen hex bytes, the
// printable ASCII beside them.
func hexDump(frame []byte) []string {
	var out []string
	for off := 0; off < len(frame); off += 16 {
		end := off + 16
		if end > len(frame) {
			end = len(frame)
		}
		var hex, ascii strings.Builder
		for _, b := range frame[off:end] {
			fmt.Fprintf(&hex, "%02X ", b)
			if b >= 32 && b < 127 {
				ascii.WriteByte(b)
			} else {
				ascii.WriteByte('.')
			}
		}
		out = append(out, fmt.Sprintf("%04X  %-48s %s", off, hex.String(), ascii.String()))
	}
	return out
}
