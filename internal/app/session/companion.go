// Talking to a node the way a phone does.
//
// A companion speaks a framed binary protocol over the same serial port the
// console uses, so one of them has to own it: two protocols interleaved on one
// UART is neither of them. Claim gives that ownership, and releasing it hands
// the console back.
//
// This exists so an application developer can point a client at a simulated
// mesh, and so the workbench can show what the client would see. The verbs
// live here; the session they act on - the claim, the frame reassembly, the
// decoded state - is compSession, in companionsession.go, and the port under
// them is companionport.go.
package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/proto"
	"github.com/MeshBench/meshbench/internal/world/provider"
)

func registerCompanion(st *state.Store, s *Sim) {
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
		// One holder at a time, and an outside client outranks us: connecting
		// over a served port would steal it from whatever is attached, with
		// nothing said at either end.
		//
		// Attached, not merely served. A listener with nobody on it was
		// refused with "is being served to an outside client" while the panel
		// beside it said "waiting" - which is the interface contradicting
		// itself, and a dead end: the only way on was to stop serving, and
		// nothing said so. An idle port has no client to steal from, so it is
		// taken back and said out loud.
		if l, serving := s.servedLink(node); serving {
			if l.Attached() {
				return nil, fmt.Errorf("%s is being served to an outside client; stop serving first", node)
			}
			s.stopServing(node)
			w.Endpoints = s.endpoints()
			w.Say("took " + node + "'s port back from an idle listener")
		}
		c := &compSession{node: node}
		c.release = en.Firmware.Bridge.Claim(c)
		s.comps[node] = c
		// AppStart then a device query: the same opening a phone makes, so a
		// node that answers one and not the other is visible as such.
		if err := en.Firmware.Bridge.Type(compFrame(proto.AppStart("meshbench"))); err != nil {
			return nil, err
		}
		for _, f := range companionBootFrames(en, s.eng.NowMs()) {
			_ = en.Firmware.Bridge.Type(compFrame(f))
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
		text, _ := namedField(p, "text")
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("companion.send needs text")
		}
		c, en, err := s.companionFor(node)
		if err != nil {
			return nil, err
		}
		idx := uint8(0)
		if v, ok := namedNum(p, "channel"); ok {
			idx = uint8(v)
		}
		// The path hash size ahead of the message, when one was chosen.
		//
		// It is a node preference, not a field on the packet - the firmware
		// reads _prefs.path_hash_mode at send time - so "send this message
		// with two-byte hashes" is necessarily two commands in order. Doing
		// it here rather than in the client keeps the order on the wire,
		// which a UI that fires two verbs cannot promise.
		if n, ok := namedNum(p, "path_hash"); ok {
			if err := setPathHash(en, uint8(n)); err != nil {
				return nil, err
			}
			// Re-read what the node now holds, as companion.configure does.
			// Without this the published PathHashBytes stays at its old value,
			// the composer's only-when-it-differs guard keeps firing, and
			// every subsequent message rewrites flash with the value already
			// there - the exact cost the guard exists to avoid.
			_ = en.Firmware.Bridge.Type(compFrame(proto.DeviceQuery()))
		}
		if err := silentCompanion(c, node); err != nil {
			return nil, err
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
		if err := silentCompanion(c, node); err != nil {
			return nil, err
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
		if v, ok := namedNum(p, "index"); ok {
			idx = uint8(v)
		}
		if err := en.Firmware.Bridge.Type(compFrame(proto.GetChannel(idx))); err != nil {
			return nil, err
		}
		return map[string]any{"asked_for_channel": idx}, nil
	})

	registerCompanionConfig(st, s)
	registerCompanionRaw(st, s)

	// Unread counts are the channel list's whole job, and a count that does not
	// clear when you read the conversation is a count nobody trusts.
	st.Handle("companion.read", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		c, ok := s.comps[node]
		if !ok {
			return nil, fmt.Errorf("%s is not connected", node)
		}
		idx := 0
		if v, ok := namedNum(p, "channel"); ok {
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

	// A companion build has no command line - only a serial rescue mode - so
	// the repeater's "region default" goes nowhere on one. An earlier version
	// set scope that way and every message went out unscoped while the
	// interface reported the scope applied, which on a mesh that is entirely
	// transport-scoped measures a different network from the one asked for.
	st.Handle("companion.scope", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		name, _ := namedField(p, "scope")
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
