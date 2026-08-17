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

	"github.com/MeshBench/meshbench/internal/companion/proto"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/provider"
)

// compSession is one connected companion.
type compSession struct {
	node    string
	release func()

	mu       sync.Mutex
	self     *proto.SelfInfo
	device   *proto.DeviceInfo
	contacts []proto.Contact
	messages []proto.Message
	channels map[uint8]proto.ChannelInfo
	last     []string
	partial  []byte
	// scope is the region the node says it sends under, and scopeKnown
	// whether it has been asked. Empty-and-known is unscoped; empty-and-not
	// is simply not read yet, and the two must not look the same.
	scope      string
	scopeKnown bool
	// unread counts what has arrived on each channel since it was last
	// looked at. The channel list exists to show it.
	unread map[uint8]int
	// seen is the channel the client is currently reading, so arrivals on it
	// are not counted as unread.
	seen uint8
	// waiting is set when the node says a message is ready to collect, and
	// cleared once the sync command has been sent for it.
	waiting bool
	// rev counts decoded frames. Frames arrive on the bridge's goroutine and
	// the world is only written on the store's, so the tick needs some way to
	// ask "has anything happened" that is cheaper than rebuilding the view
	// and comparing it.
	rev uint64
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
	c.rev++
	// A message is not pushed to us, only the news that one exists. Asking
	// for it is a command, so it cannot be sent from here - this runs on the
	// bridge's own goroutine, and typing into the bridge from inside its read
	// path deadlocks. The flag is picked up by the next tick instead.
	if f.Push == proto.PushMsgWaiting {
		c.waiting = true
	}
	switch {
	case f.SelfInfo != nil:
		c.self = f.SelfInfo
		c.note("self: " + f.SelfInfo.Name)
	case f.Device != nil:
		c.device = f.Device
		if f.Device.ModeKnown {
			c.note(fmt.Sprintf("path hashes: %d byte(s)",
				proto.PathHashBytes(f.Device.PathHashMode)))
		}
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
		// Not counted against the channel being read, and never against our
		// own sends - an unread badge on the conversation you are looking at
		// is noise, and one for a message you just typed is wrong.
		if f.Message.Channel && !f.Message.Mine && f.Message.ChannelIdx != c.seen {
			if c.unread == nil {
				c.unread = map[uint8]int{}
			}
			c.unread[f.Message.ChannelIdx]++
		}
		c.note("message: " + f.Message.Text)
	case f.Scope != nil:
		c.scope, c.scopeKnown = f.Scope.Name, true
		if f.Scope.Name == "" {
			c.note("scope: unscoped")
		} else {
			c.note("scope: " + f.Scope.Name)
		}
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
		// A companion build's own name starts blank - nothing on a phone's
		// onboarding path has run for it - so left alone every one of them
		// showed as nameless, which read as the client not knowing who it had
		// connected to rather than as a node with nothing set. The scenario's
		// name is the one honest answer to send.
		//
		// CMD_SET_ADVERT_NAME answers with a bare OK, not a fresh self info -
		// RESP_CODE_SELF_INFO only ever comes back from CMD_APP_START, so the
		// only way to see the rename take is to ask again the same way the
		// name is asked for in the first place, rather than assume the write
		// landed and show the operator our own guess as the node's answer.
		_ = en.Firmware.Bridge.Type(compFrame(proto.SetAdvertName(node)))
		if err := en.Firmware.Bridge.Type(compFrame(proto.AppStart("meshbench"))); err != nil {
			return nil, err
		}
		_ = en.Firmware.Bridge.Type(compFrame(proto.DeviceQuery()))
		// And what it holds, rather than what the scenario believes: the
		// scope it actually sends under, and the channel slots it has. Asked
		// on connect because the client draws both, and a list that fills in
		// only after somebody presses a button is a list that looks empty.
		_ = en.Firmware.Bridge.Type(compFrame(proto.GetDefaultScope()))
		for i := uint8(0); i < companionChannelSlots; i++ {
			_ = en.Firmware.Bridge.Type(compFrame(proto.GetChannel(i)))
		}
		s.publishCompanions(w)
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
		s.publishCompanions(w)
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
		// The path hash size ahead of the message, when one was chosen.
		//
		// It is a node preference, not a field on the packet - the firmware
		// reads _prefs.path_hash_mode at send time - so "send this message
		// with two-byte hashes" is necessarily two commands in order. Doing
		// it here rather than in the client keeps the order on the wire,
		// which a UI that fires two verbs cannot promise.
		if n, ok := numField(p, "path_hash"); ok {
			if err := setPathHash(en, uint8(n)); err != nil {
				return nil, err
			}
		}
		at := time.Now()
		if err := en.Firmware.Bridge.Type(compFrame(proto.SendChannelText(idx, at, text))); err != nil {
			return nil, err
		}
		// Kept, because nothing comes back to echo it. The node transmits and
		// says nothing about having done so, so a client that only shows what
		// arrives shows an empty conversation however much you send into it.
		c.mu.Lock()
		c.messages = append(c.messages, proto.Message{
			Channel: true, ChannelIdx: idx, Text: text, At: at, Mine: true,
		})
		c.rev++
		c.mu.Unlock()
		c.note("sent: " + text)
		s.publishCompanions(w)
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

	registerCompanionConfig(st, s)

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

	// companion.read marks a channel as the one being looked at.
	//
	// Unread counts are the channel list's whole job, and a count that does
	// not clear when you read the conversation is a count nobody trusts. The
	// client sends this when the selection changes; nothing goes on the wire.
	st.Handle("companion.read", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		c, ok := s.comps[node]
		if !ok {
			return nil, fmt.Errorf("%s is not connected", node)
		}
		idx := 0
		if v, ok := numField(p, "channel"); ok {
			idx = int(v)
		}
		c.mu.Lock()
		c.seen = uint8(idx)
		if c.unread != nil {
			delete(c.unread, uint8(idx))
		}
		c.mu.Unlock()
		s.publishCompanions(w)
		return map[string]any{"node": node, "channel": idx}, nil
	})

	// companion.scope sets the region this node sends under, and reads it
	// back rather than assuming it landed.
	//
	// A companion build has no command line - only a serial rescue mode - so
	// the repeater's "region default" goes nowhere on one. An earlier version
	// set scope that way and every message went out unscoped while the
	// interface reported the scope applied, which on a mesh that is entirely
	// transport-scoped measures a different network from the one asked for.
	st.Handle("companion.scope", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		name, _ := stringField(p, "scope")
		c, en, err := s.companionFor(node)
		if err != nil {
			return nil, err
		}
		name = canonicalScope(strings.TrimSpace(name))
		frame := proto.ClearDefaultScope()
		if name != "" {
			// The key with the name, always: the firmware stores both and
			// matches on the key, so a name sent alone scopes nothing. It is
			// derived the way every other scope in this codebase is,
			// sha256 of the "#"-prefixed name.
			frame = proto.SetDefaultScope(name, provider.RegionKey(name))
		}
		if err := en.Firmware.Bridge.Type(compFrame(frame)); err != nil {
			return nil, err
		}
		// Ask, rather than record what we meant. Both name and key are stored
		// by the firmware and it matches on the key, so a name that was
		// accepted is the only proof the scope is real.
		_ = en.Firmware.Bridge.Type(compFrame(proto.GetDefaultScope()))
		c.note("scope set to " + orUnscoped(name))
		s.publishCompanions(w)
		w.Say(node + " sends under " + orUnscoped(name))
		return map[string]any{"node": node, "scope": name}, nil
	})

	// companion.refresh asks the node for what the client draws: its own
	// details, the channel slots and the contact list.
	st.Handle("companion.refresh", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		c, en, err := s.companionFor(node)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.contacts = nil
		c.mu.Unlock()
		_ = en.Firmware.Bridge.Type(compFrame(proto.DeviceQuery()))
		_ = en.Firmware.Bridge.Type(compFrame(proto.GetDefaultScope()))
		_ = en.Firmware.Bridge.Type(compFrame(proto.GetContacts(time.Unix(0, 0))))
		for i := uint8(0); i < companionChannelSlots; i++ {
			_ = en.Firmware.Bridge.Type(compFrame(proto.GetChannel(i)))
		}
		s.publishCompanions(w)
		return map[string]any{"node": node}, nil
	})
}

// orUnscoped names the empty scope, because "" in a status line reads as a
// missing value rather than as an answer.
func orUnscoped(s string) string {
	if s == "" {
		return "no scope"
	}
	return s
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

// Lines is the session's recent traffic, for a console to draw.
func (c *compSession) Lines() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.last...)
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
	// Named here too: the CLI can be the first thing to connect, not only
	// the client, and a companion is nameless until something sets it.
	_ = en.Firmware.Bridge.Type(compFrame(proto.SetAdvertName(node)))
	return en.Firmware.Bridge.Type(compFrame(proto.AppStart("meshbench")))
}
