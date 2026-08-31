// Handing the companion sessions to the panels, decoded.
//
// compSession has always kept what it decoded - self info, channels by index,
// contacts, messages - and then published only c.Lines(), the console tail.
// Everything the client needs to draw a channel list or a conversation was
// already in memory and stopped here.
package session

import (
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/proto"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// companions is every session the workbench holds, plus the endpoints it has
// handed to somebody else.
//
// A node appears once whichever way it is reached: holding the port and
// serving it are two states of one thing, not two lists. The panel switches
// on Connected and Serving.Addr rather than looking in two places.
func (s *Sim) companions() []state.Companion {
	byNode := map[string]*state.Companion{}
	for node, c := range s.comps {
		v := c.view()
		byNode[node] = &v
	}
	s.eachServed(func(node string, l *engine.CompanionLink, _ []string) {
		v, ok := byNode[node]
		if !ok {
			byNode[node] = &state.Companion{
				Node:    node,
				Serving: state.Endpoint{Node: node, Kind: l.Kind, Addr: l.Addr, Attached: l.Attached()},
			}
			return
		}
		v.Serving = state.Endpoint{Node: node, Kind: l.Kind, Addr: l.Addr, Attached: l.Attached()}
	})
	out := make([]state.Companion, 0, len(byNode))
	for _, v := range byNode {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

// view is this session as the panel reads it.
func (c *compSession) view() state.Companion {
	c.mu.Lock()
	defer c.mu.Unlock()

	v := state.Companion{Node: c.node, Connected: true}
	if c.self != nil {
		v.Name = c.self.Name
		v.Key = shortKey(c.self.PublicKey)
		v.FreqKHz, v.BWKHz = c.self.FreqKHz, c.self.BWKHz
		v.SF, v.CR, v.TxDBm = c.self.SF, c.self.CR, c.self.TxPowerDBm
		v.MaxTxDBm = c.self.MaxTxPowerDBm
	}
	// Left at zero against firmware that does not report it, so the settings
	// pane can say "the node has not said" rather than show a default as a
	// fact.
	if c.device != nil && c.device.ModeKnown {
		v.PathHashBytes = proto.PathHashBytes(c.device.PathHashMode)
	}
	v.Scope, v.Scoped = c.scope, c.scopeKnown

	// Channels in index order, because that is the order the firmware holds
	// them in and the number is what a message is actually addressed to.
	idx := make([]int, 0, len(c.channels))
	for i := range c.channels {
		idx = append(idx, int(i))
	}
	sort.Ints(idx)
	for _, i := range idx {
		ch := c.channels[uint8(i)]
		name := ch.Name
		if name == "" {
			name = fmt.Sprintf("channel %d", ch.Index)
		}
		v.Channels = append(v.Channels, state.CompanionChannel{
			Index: ch.Index, Name: name, Unread: c.unread[ch.Index],
		})
	}
	for _, ct := range c.contacts {
		v.Contacts = append(v.Contacts, state.CompanionContact{
			Name: ct.Name, Key: shortKey(ct.PublicKey),
			Hops: int(ct.OutPathLen), LastSeen: ct.LastSeen,
		})
	}
	for i := range c.messages {
		v.Messages = append(v.Messages, messageView(c.messages[i]))
	}
	return v
}

// messageView is one message, as a line of conversation.
func messageView(m proto.Message) state.CompanionMessage {
	from := m.SenderName
	if from == "" {
		from = shortKey(m.SenderKey)
	}
	if m.Mine {
		from = "me"
	}
	return state.CompanionMessage{
		Channel: m.Channel, ChannelIdx: m.ChannelIdx,
		From: from, Text: m.Text, At: m.At, Mine: m.Mine,
		SNRdB: m.SNRdB, Hops: m.PathLen,
	}
}

// shortKey is a public key as it is shown: enough to recognise, not so much
// that it fills the column it sits in.
func shortKey(k []byte) string {
	if len(k) == 0 {
		return ""
	}
	s := hex.EncodeToString(k)
	if len(s) <= 12 {
		return s
	}
	return s[:4] + "…" + s[len(s)-4:]
}

// publishCompanions puts the current sessions in the world.
func (s *Sim) publishCompanions(w *state.World) {
	w.Companions = s.companions()
	s.compRev = s.companionRev()
}

// refreshCompanions rebuilds the view when a frame has arrived since the last
// one, and does nothing otherwise.
//
// Frames land on the bridge's goroutine; the world is only written on the
// store's. So something has to poll, and the cheap question is a counter
// rather than a rebuilt view compared field by field - a mesh of three
// hundred nodes with one companion open should not pay for the other 299.
func (s *Sim) refreshCompanions(w *state.World) {
	if len(s.comps) == 0 && s.servedCount() == 0 {
		return
	}
	s.collectWaiting()
	if rev := s.companionRev(); rev != s.compRev {
		s.publishCompanions(w)
		// The console tail travels with the rest of it. Replies arrive on the
		// bridge's goroutine long after the command that caused them
		// returned, so a console only written by console.cli shows the echo
		// and never the answer - which reads as a command line that ignores
		// you.
		if c, ok := s.comps[w.ConsoleNode]; ok {
			w.Console = c.Lines()
		}
	}
}

// collectWaiting asks for any message the node has said is ready.
//
// The firmware pushes the news, not the message: PUSH_CODE_MSG_WAITING says
// something has arrived and CMD_SYNC_NEXT_MESSAGE collects it. Sent from the
// tick rather than from the read path, because writing to the bridge while
// reading from it is a deadlock waiting for a quiet moment to happen in.
func (s *Sim) collectWaiting() {
	for node, c := range s.comps {
		c.mu.Lock()
		want := c.waiting
		c.waiting = false
		c.mu.Unlock()
		if !want {
			continue
		}
		if en, ok := s.eng.NodeByName(node); ok && en.Firmware != nil {
			_ = en.Firmware.Bridge.Type(compFrame(proto.SyncNextMessage()))
		}
	}
}

// companionRev is every session's frame count, summed.
func (s *Sim) companionRev() uint64 {
	var n uint64
	for _, c := range s.comps {
		c.mu.Lock()
		n += c.rev
		c.mu.Unlock()
	}
	return n
}

// companionChannelSlots is how many channel slots are asked for on connect.
//
// The firmware addresses channels by index and has no "list them" command, so
// discovery is asking for each slot in turn. Eight covers what a companion
// build holds; asking for one that is empty costs a frame and answers
// nothing, which is cheaper than a channel that never appears.
const companionChannelSlots = 8
