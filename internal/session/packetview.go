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

	"github.com/A13xB0/meshcoresim/internal/capture"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
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
		return map[string]any{"id": id, "origin": pk.Origin,
			"heard": pk.Heard, "missed": pk.Missed}, nil
	})

	st.Handle("packet.close", func(w *state.World, _ any) (any, error) {
		w.Packet = nil
		return nil, nil
	})
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
	for _, h := range d.PathHashes {
		if name, ok := hashNames[h]; ok {
			pk.Path = append(pk.Path, fmt.Sprintf("%s (%02X)", name, h))
		} else {
			pk.Path = append(pk.Path, fmt.Sprintf("%02X", h))
		}
	}
	for _, f := range d.PayloadFields {
		pk.PayloadFields = append(pk.PayloadFields, state.PacketField{
			Name: f.Name, Value: f.Value})
	}
	if len(d.PayloadFields) == 0 {
		pk.PayloadNote = fmt.Sprintf(
			"%d bytes, encrypted or of a type with nothing in clear", len(d.Payload))
	}
	pk.RawLines = hexDump(frame)

	// The radio-level truth, per receiver.
	for _, r := range s.eng.Ledger.ForPacket(id) {
		fw := "never saw it"
		switch {
		case r.Demod && r.CRCOK && r.FirmwareSaw:
			fw = "accepted"
		case r.Demod && r.CRCOK:
			fw = "dropped"
		}
		pk.Ledger = append(pk.Ledger, state.PacketReception{
			Node: r.ToNode, From: r.FromNode, Offered: r.Offered,
			RSSIdBm: r.RSSIdBm, SNRdB: r.SNRdB,
			Demod: r.Demod, CRCOK: r.CRCOK, Firmware: fw,
		})
	}

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
	return pk
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
