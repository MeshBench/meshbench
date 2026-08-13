// Talking to a node the way a phone does.
//
// A companion speaks a framed binary protocol over the same serial port the
// console uses, so one of them has to own it: two protocols interleaved on one
// UART is neither of them. Claim gives that ownership, and releasing it hands
// the console back.
//
// This exists so an application developer can point a client at a simulated
// mesh, and so the workbench can show what the client would see.
package session

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/A13xB0/meshcoresim/internal/companion/proto"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// compSession is one connected companion.
type compSession struct {
	node    string
	release func()

	mu       sync.Mutex
	self     *proto.SelfInfo
	contacts []proto.Contact
	messages []proto.Message
	channels map[uint8]proto.ChannelInfo
	last     []string
	partial  []byte
}

// Write receives whatever the node sends while the claim is held.
//
// The protocol is length-framed, so a partial frame is kept rather than
// decoded: a serial read boundary is not a message boundary, and treating it
// as one produces a stream of decode errors that look like a broken node.
func (c *compSession) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.partial = append(c.partial, b...)
	// MeshCore's own framing: '>' from the device, then a little-endian
	// length, then that many bytes. Anything else in the stream is the
	// firmware's console output and is skipped rather than guessed at - the
	// first version of this read the length from wherever the buffer happened
	// to start and decoded rubbish.
	for {
		i := 0
		for i < len(c.partial) && c.partial[i] != '>' {
			i++
		}
		if i > 0 {
			c.partial = c.partial[i:]
		}
		if len(c.partial) < 3 {
			break
		}
		n := int(binary.LittleEndian.Uint16(c.partial[1:3]))
		if len(c.partial) < 3+n {
			break
		}
		frame := append([]byte(nil), c.partial[3:3+n]...)
		c.partial = c.partial[3+n:]
		f, err := proto.Decode(frame)
		if err != nil {
			c.note("undecoded frame: " + err.Error())
			continue
		}
		c.apply(f)
	}
	return len(b), nil
}

func (c *compSession) apply(f proto.Frame) {
	switch {
	case f.SelfInfo != nil:
		c.self = f.SelfInfo
		c.note("self: " + f.SelfInfo.Name)
	case f.Channel != nil:
		if c.channels == nil {
			c.channels = map[uint8]proto.ChannelInfo{}
		}
		c.channels[f.Channel.Index] = *f.Channel
		c.note(fmt.Sprintf("channel %d: %s", f.Channel.Index, f.Channel.Name))
	case f.Contact != nil:
		c.contacts = append(c.contacts, *f.Contact)
		c.note("contact: " + f.Contact.Name)
	case f.Message != nil:
		c.messages = append(c.messages, *f.Message)
		c.note("message: " + f.Message.Text)
	case f.Err != "":
		c.note("error: " + f.Err)
	default:
		c.note(fmt.Sprintf("frame %v", f.Code))
	}
}

// note keeps a short tail, so a caller can see what happened without holding
// every frame of a long session.
func (c *compSession) note(s string) {
	c.last = append(c.last, s)
	if len(c.last) > 200 {
		c.last = c.last[len(c.last)-200:]
	}
}

func registerCompanion(st *state.Store, s *Sim) {
	// companion.connect: claim the port and introduce ourselves.
	st.Handle("companion.connect", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		if node == "" {
			return nil, fmt.Errorf("companion.connect needs a node")
		}
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		en, ok := s.eng.NodeByName(node)
		if !ok || en.Firmware == nil {
			return nil, fmt.Errorf("%s runs no firmware, so it has no companion interface", node)
		}
		if s.comps == nil {
			s.comps = map[string]*compSession{}
		}
		if _, already := s.comps[node]; already {
			return nil, fmt.Errorf("%s is already connected", node)
		}
		c := &compSession{node: node}
		c.release = en.Firmware.Bridge.Claim(c)
		s.comps[node] = c
		// AppStart then a device query: the same opening a phone makes, so a
		// node that answers one and not the other is visible as such.
		if err := en.Firmware.Bridge.Type(compFrame(proto.AppStart("meshbench"))); err != nil {
			return nil, err
		}
		_ = en.Firmware.Bridge.Type(compFrame(proto.DeviceQuery()))
		w.Say("connected to " + node + " as a companion")
		return map[string]any{"connected": node}, nil
	})

	st.Handle("companion.disconnect", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		c, ok := s.comps[node]
		if !ok {
			return nil, fmt.Errorf("%s is not connected", node)
		}
		if c.release != nil {
			c.release()
		}
		delete(s.comps, node)
		w.Say("released " + node + "; the console has it back")
		return map[string]any{"disconnected": node}, nil
	})

	// companion.state: what the client would be showing.
	st.Handle("companion.state", func(_ *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		c, ok := s.comps[node]
		if !ok {
			return nil, fmt.Errorf("%s is not connected", node)
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		out := map[string]any{
			"node": node, "contacts": len(c.contacts),
			"messages": len(c.messages), "channels": len(c.channels),
			"recent": c.last,
		}
		if c.self != nil {
			out["name"] = c.self.Name
			out["freq_khz"] = c.self.FreqKHz
		}
		return out, nil
	})

	st.Handle("companion.send", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		text, _ := stringField(p, "text")
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("companion.send needs text")
		}
		c, en, err := s.companionFor(node)
		if err != nil {
			return nil, err
		}
		idx := uint8(0)
		if v, ok := numField(p, "channel"); ok {
			idx = uint8(v)
		}
		if err := en.Firmware.Bridge.Type(compFrame(proto.SendChannelText(idx, time.Now(), text))); err != nil {
			return nil, err
		}
		c.note("sent: " + text)
		w.Say(node + " sent a message")
		return map[string]any{"sent": text, "channel": idx}, nil
	})

	st.Handle("companion.advert", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		c, en, err := s.companionFor(node)
		if err != nil {
			return nil, err
		}
		flood := true
		if v, ok := boolField(p, "flood"); ok {
			flood = v
		}
		if err := en.Firmware.Bridge.Type(compFrame(proto.SendSelfAdvert(flood))); err != nil {
			return nil, err
		}
		c.note("advert sent")
		return map[string]any{"advert": node, "flood": flood}, nil
	})

	st.Handle("companion.add_channel", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		_, en, err := s.companionFor(node)
		if err != nil {
			return nil, err
		}
		idx := uint8(0)
		if v, ok := numField(p, "index"); ok {
			idx = uint8(v)
		}
		if err := en.Firmware.Bridge.Type(compFrame(proto.GetChannel(idx))); err != nil {
			return nil, err
		}
		return map[string]any{"asked_for_channel": idx}, nil
	})

	st.Handle("companion.configure", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		c, en, err := s.companionFor(node)
		if err != nil {
			return nil, err
		}
		done := []string{}
		if name, ok := stringField(p, "name"); ok && name != "" {
			if err := en.Firmware.Bridge.Type(compFrame(proto.SetAdvertName(name))); err != nil {
				return nil, err
			}
			done = append(done, "name")
		}
		if lat, ok := numField(p, "lat"); ok {
			lon, _ := numField(p, "lon")
			if err := en.Firmware.Bridge.Type(compFrame(proto.SetAdvertLatLon(lat, lon))); err != nil {
				return nil, err
			}
			done = append(done, "position")
		}
		if dbm, ok := numField(p, "tx_dbm"); ok {
			if err := en.Firmware.Bridge.Type(compFrame(proto.SetTxPower(uint8(dbm)))); err != nil {
				return nil, err
			}
			done = append(done, "tx power")
		}
		if len(done) == 0 {
			return nil, fmt.Errorf("companion.configure needs a name, a position or a tx_dbm")
		}
		c.note("configured: " + strings.Join(done, ", "))
		return map[string]any{"set": done}, nil
	})

	// companion.raw: whatever bytes the caller wants, for when the decode is
	// the thing in question.
	st.Handle("companion.raw", func(w *state.World, p any) (any, error) {
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

// compFrame wraps a payload the way the device expects it.
//
// '<' towards the node, a little-endian length, then the payload. Sending the
// payload bare is not a malformed frame, it is console text: the firmware
// reads it as somebody typing and answers nothing, which is what an
// experiment measuring zero transmissions looked like.
func compFrame(payload []byte) []byte {
	out := make([]byte, 0, 3+len(payload))
	out = append(out, '<')
	out = binary.LittleEndian.AppendUint16(out, uint16(len(payload)))
	return append(out, payload...)
}
