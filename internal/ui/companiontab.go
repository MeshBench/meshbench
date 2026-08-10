package ui

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/companion/proto"
	"github.com/A13xB0/meshcoresim/internal/provider"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// compSession is one open conversation with a companion node's firmware.
//
// It holds the node's UART exclusively while connected, exactly as a phone
// over TCP or a pty would - which is why connecting is a button and not
// something opening a tab does on its own. Something else may already be
// attached, and taking the port from it silently would be the workbench
// interfering with an experiment rather than running one.
type compSession struct {
	node    string
	release func()

	mu     sync.Mutex
	rx     []byte // partial frame from the firmware
	frames []proto.Frame
	// lastCmd is the command byte most recently sent, so an error frame can
	// name what was refused rather than only how.
	lastCmd byte
	// rawMsgs holds the first few received message frames, verbatim.
	rawMsgs  [][]byte
	messages []proto.Message
	channels []proto.ChannelInfo
	contacts []proto.Contact
	self     *proto.SelfInfo
	err      string
	// syncing marks that a message is waiting and has been asked for, so the
	// push does not queue a hundred requests for one message.
	syncing bool
}

// Write receives the firmware's serial output. It is the io.Writer the
// bridge claim hands bytes to, so it runs on the frame thread's step and
// must not block.
func (s *compSession) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rx = append(s.rx, b...)
	// MeshCore's own framing: '>' from the device, then a little-endian
	// length, then that many bytes. Anything else in the stream is the
	// firmware's console output and is skipped rather than guessed at.
	for {
		i := 0
		for i < len(s.rx) && s.rx[i] != '>' {
			i++
		}
		if i > 0 {
			s.rx = s.rx[i:]
		}
		if len(s.rx) < 3 {
			return len(b), nil
		}
		n := int(binary.LittleEndian.Uint16(s.rx[1:3]))
		if len(s.rx) < 3+n {
			return len(b), nil
		}
		payload := append([]byte(nil), s.rx[3:3+n]...)
		s.rx = s.rx[3+n:]
		f, err := proto.Decode(payload)
		if err != nil {
			s.err = err.Error()
			continue
		}
		s.take(f)
	}
}

// take files one decoded frame. Caller holds mu.
func (s *compSession) take(f proto.Frame) {
	s.frames = append(s.frames, f)
	if len(s.frames) > 200 {
		s.frames = s.frames[len(s.frames)-200:]
	}
	switch {
	case f.Code == proto.RespErr:
		s.err = f.Err + " (" + proto.CommandName(s.lastCmd) + ")"
	case f.Code == proto.RespOK, f.Code == proto.RespSent:
		// A channel send confirms with OK; only a direct message answers SENT.
		// Waiting for SENT alone left every channel message reading
		// "sending..." for ever, which is what a failed send looks like.
		// The node has taken the message. Mark our own most recent unconfirmed
		// one, so "sent" in the conversation means the firmware said so rather
		// than that we wrote it down.
		for i := len(s.messages) - 1; i >= 0; i-- {
			if s.messages[i].Mine && !s.messages[i].Confirmed {
				s.messages[i].Confirmed = true
				break
			}
		}
	case f.SelfInfo != nil:
		s.self = f.SelfInfo
	case f.Channel != nil:
		for i, c := range s.channels {
			if c.Index == f.Channel.Index {
				s.channels[i] = *f.Channel
				return
			}
		}
		if f.Channel.Name != "" {
			s.channels = append(s.channels, *f.Channel)
		}
	case f.Contact != nil:
		for i, c := range s.contacts {
			if string(c.PublicKey) == string(f.Contact.PublicKey) {
				s.contacts[i] = *f.Contact
				return
			}
		}
		s.contacts = append(s.contacts, *f.Contact)
	case f.Message != nil:
		// Keep the bytes of a few received messages. Frame layouts are the
		// thing this client keeps getting wrong, and a hex dump of what
		// actually arrived settles in one look what reading the firmware
		// source did not.
		if len(s.rawMsgs) < 8 {
			s.rawMsgs = append(s.rawMsgs, append([]byte(nil), f.Raw...))
		}
		s.messages = append(s.messages, *f.Message)
		s.syncing = false
	case f.Err != "":
		s.err = f.Err
	}
	if f.Code == proto.RespNoMoreMessages {
		s.syncing = false
	}
}

// send wraps a command in the transport's framing and hands it to the node.
func (a *App) compSend(s *compSession, payload []byte) error {
	if a.eng == nil {
		return fmt.Errorf("no simulation running")
	}
	n, ok := a.eng.NodeByName(s.node)
	if !ok || n.Firmware == nil {
		return fmt.Errorf("%s runs no firmware", s.node)
	}
	if len(payload) > 0 {
		s.mu.Lock()
		s.lastCmd = payload[0]
		s.mu.Unlock()
	}
	frame := make([]byte, 0, 3+len(payload))
	frame = append(frame, '<')
	frame = binary.LittleEndian.AppendUint16(frame, uint16(len(payload)))
	frame = append(frame, payload...)
	return n.Firmware.Bridge.Type(frame)
}

// compConnect claims the node's port and says hello.
func (a *App) compConnect(node string) error {
	if a.comps == nil {
		a.comps = map[string]*compSession{}
	}
	if a.comps[node] != nil {
		return nil
	}
	if a.eng == nil {
		return fmt.Errorf("no simulation running")
	}
	n, ok := a.eng.NodeByName(node)
	if !ok || n.Firmware == nil {
		return fmt.Errorf("%s runs no firmware - press play with real firmware on", node)
	}
	if a.compFocus == nil {
		a.compFocus = map[string]bool{}
	}
	a.compFocus[node] = true
	s := &compSession{node: node}
	s.release = n.Firmware.Bridge.Claim(s)
	a.comps[node] = s
	// The handshake. Everything else waits on the reply to this.
	if err := a.compSend(s, proto.AppStart("meshbench")); err != nil {
		a.compDisconnect(node)
		return err
	}
	a.stepEngine(20)
	// The companion's own radio preferences start empty - nothing has ever
	// told it, because provisioning speaks the repeater CLI and a companion
	// build does not take those commands. That is why it reported 0 MHz: not
	// a decoding fault, an unconfigured node. The scenario knows what this
	// node is meant to be on, so it is sent through the interface that can
	// actually set it.
	a.configureCompanion(node)
	_ = a.compSend(s, proto.GetContacts(time.Time{}))
	for i := uint8(0); i < 8; i++ {
		_ = a.compSend(s, proto.GetChannel(i))
	}
	a.stepEngine(20)
	return nil
}

// compDisconnect gives the port straight back.
func (a *App) compDisconnect(node string) {
	s := a.comps[node]
	if s == nil {
		return
	}
	if s.release != nil {
		s.release()
	}
	delete(a.comps, node)
}

// drawMiniCompanionTab is the mini companion: connect, pick a channel, send,
// read what comes back.
//
// Distinct from the Connect tab, which points the other way: that exposes
// this node's port so a real client can attach, this *is* the client.
func (a *App) drawMiniCompanionTab(i int) {
	n := &a.Nodes[i]
	s := a.comps[n.Name]

	// Connect first, and say what connecting would displace.
	if s == nil {
		claimed := false
		if a.eng != nil {
			if en, ok := a.eng.NodeByName(n.Name); ok && en.Firmware != nil {
				claimed = en.Firmware.Bridge.Claimed()
			}
		}
		if claimed {
			textColoured(colWarn, "another client holds this node's serial port")
			if imgui.Button("take it over") {
				if err := a.compConnect(n.Name); err != nil {
					a.status = err.Error()
				}
			}
		} else if primaryButton("Connect", imgui.NewVec2(0, 0)) {
			if err := a.compConnect(n.Name); err != nil {
				a.status = err.Error()
			}
		}
		textDimWrap("Connecting claims this node's serial port exclusively, the same way a " +
			"phone or meshcore-cli would. Nothing is sent until you do.")
		return
	}

	s.mu.Lock()
	self, chans, contacts, msgs, errText := s.self, append([]proto.ChannelInfo(nil), s.channels...),
		append([]proto.Contact(nil), s.contacts...), append([]proto.Message(nil), s.messages...), s.err
	s.mu.Unlock()

	textColoured(colOK, "connected")
	imgui.SameLine()
	if imgui.SmallButton("Disconnect") {
		a.compDisconnect(n.Name)
		return
	}
	imgui.SameLine()
	if self != nil {
		textDim(fmt.Sprintf("%s  %.3f MHz SF%d CR4/%d  %d dBm",
			self.Name, float64(self.FreqKHz)/1000, self.SF, self.CR, self.TxPowerDBm))
	} else {
		textDim("waiting for the firmware to answer the handshake...")
	}
	if errText != "" {
		textColoured(colErr, errText)
	}

	if imgui.BeginTabBarV("##comptabs", imgui.TabBarFlagsNone) {
		if imgui.BeginTabItem("Messages") {
			a.drawCompanionMessages(n.Name, s, chans, msgs)
			imgui.EndTabItem()
		}
		if imgui.BeginTabItem(fmt.Sprintf("Contacts (%d)", len(contacts))) {
			if imgui.SmallButton("send advert") {
				_ = a.compSend(s, proto.SendSelfAdvert(true))
				a.stepEngine(20)
			}
			imgui.SameLine()
			if imgui.SmallButton("sync contacts") {
				_ = a.compSend(s, proto.GetContacts(time.Time{}))
				a.stepEngine(20)
			}
			if len(contacts) == 0 {
				textDimWrap("none yet - contacts arrive from adverts, so run for a while or " +
					"send one of your own")
			}
			for _, c := range contacts {
				imgui.Text(c.Name)
				imgui.SameLine()
				if c.OutPathLen < 0 {
					textDim("no path known")
				} else {
					textDim(fmt.Sprintf("%d hops", c.OutPathLen))
				}
			}
			imgui.EndTabItem()
		}
		if imgui.BeginTabItem("Radio") {
			a.drawCompanionRadio(n, s, self)
			imgui.EndTabItem()
		}
		imgui.EndTabBar()
	}
}

// drawCompanionMessages is the channel list, the conversation and the send box.
func (a *App) drawCompanionMessages(node string, s *compSession,
	chans []proto.ChannelInfo, msgs []proto.Message) {
	cs := a.compUI[node]
	if cs == nil {
		cs = &compUIState{}
		if a.compUI == nil {
			a.compUI = map[string]*compUIState{}
		}
		a.compUI[node] = cs
	}

	imgui.SetNextItemWidth(180)
	label := "public"
	for _, c := range chans {
		if c.Index == cs.channel {
			label = c.Name
		}
	}
	if imgui.BeginCombo("channel", label) {
		for _, c := range chans {
			if imgui.SelectableBool(fmt.Sprintf("%s (%d)", c.Name, c.Index)) {
				cs.channel = c.Index
			}
		}
		imgui.EndCombo()
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Channels are addressed by slot, not by name: the name and the key\n" +
			"live in the device. These were read from it on connect.")
	}

	// Scope is not part of the send frame - it is the node's own transport
	// region - so it is set through the CLI before sending, and says so.
	imgui.SameLine()
	imgui.SetNextItemWidth(140)
	scope := cs.scope
	if scope == "" {
		scope = "(unchanged)"
	}
	if imgui.BeginCombo("scope", scope) {
		if imgui.SelectableBool("(unchanged)") {
			cs.scope = ""
		}
		if imgui.SelectableBool("unscoped") {
			cs.scope = scopeNull
		}
		for _, r := range a.knownRegions() {
			if imgui.SelectableBool(r) {
				cs.scope = r
			}
		}
		imgui.Separator()
		// A region nobody has been observed using is still a region you can
		// send to - the list is what has been seen, not what exists.
		imgui.SetNextItemWidth(140)
		if imgui.InputTextWithHint("##newscope", "add one, e.g. #fif", &cs.newScope,
			imgui.InputTextFlagsEnterReturnsTrue, nil) && cs.newScope != "" {
			cs.scope = cs.newScope
			a.addKnownRegion(cs.newScope)
			cs.newScope = ""
			imgui.CloseCurrentPopup()
		}
		imgui.EndCombo()
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Scope is the node's own transport region, not part of the message,\n" +
			"so choosing one sets the node's default scope before sending.")
	}

	if imgui.BeginChildStrV("##conv", imgui.NewVec2(0, -imgui.FrameHeight()*2.4),
		imgui.ChildFlagsFrameStyle, 0) {
		shown := 0
		for _, m := range msgs {
			if m.Channel && m.ChannelIdx != cs.channel {
				continue
			}
			shown++
			pushMono()
			imgui.Text(m.At.Local().Format("15:04:05"))
			popMono()
			imgui.SameLine()
			if m.SenderName != "" {
				textDim(m.SenderName)
				imgui.SameLine()
			}
			textWrap(m.Text)
			if m.Mine {
				// Ours, and whether the node has taken it. Without the
				// distinction a send that failed looks like one that worked.
				if m.Confirmed {
					textDim("   sent")
				} else {
					textDim("   sending...")
				}
			}
		}
		if shown == 0 {
			textDimWrap("nothing on this channel yet. Everything here came out of the " +
				"firmware; there are no invented replies.")
		}
	}
	imgui.EndChild()

	imgui.SetNextItemWidth(-90)
	entered := imgui.InputTextWithHint("##msg", "type a message", &cs.draft,
		imgui.InputTextFlagsEnterReturnsTrue, nil)
	imgui.SameLine()
	if (primaryButton("Send", imgui.NewVec2(0, 0)) || entered) && cs.draft != "" {
		a.compSendMessage(node, s, cs)
	}
}

// scopeNull is the sentinel for "send unscoped", distinct from "" which means
// leave the node's scope alone.
const scopeNull = "<null>"

// compSendMessage applies the scope, then sends.
func (a *App) compSendMessage(node string, s *compSession, cs *compUIState) {
	if cs.scope != "" {
		if err := a.compSetScope(s, node, cs.scope); err != nil {
			a.status = node + ": " + err.Error()
			return
		}
	}
	text := cs.draft
	if err := a.compSend(s, proto.SendChannelText(cs.channel, time.Now(), text)); err != nil {
		a.status = err.Error()
		return
	}
	// Show it. Nothing comes back to echo our own transmission, so a send used
	// to leave the conversation exactly as empty as before it - which reads as
	// the send having failed, and is indistinguishable from it.
	s.mu.Lock()
	s.messages = append(s.messages, proto.Message{
		Channel: true, ChannelIdx: cs.channel, SenderName: "me",
		Text: text, At: time.Now(), Mine: true,
	})
	s.mu.Unlock()
	cs.draft = ""
	// No stepping here. Advancing the engine from a UI action means 154 nodes
	// in lockstep on the frame thread, which is a visible stall every time a
	// message is sent. The send is on its way; the frame loop will carry it,
	// and the firmware's OK will arrive when it arrives.
}

// drawCompanionRadio configures the modem through the companion protocol.
func (a *App) drawCompanionRadio(n *scenario.Node, s *compSession, self *proto.SelfInfo) {
	if self == nil {
		textDimWrap("waiting for the firmware to say what it is running")
		return
	}
	cs := a.compUI[n.Name]
	if cs == nil {
		return
	}
	if cs.freqMHz == 0 {
		cs.freqMHz = float32(self.FreqKHz) / 1000
		cs.bwKHz = float32(self.BWKHz)
		cs.sf, cs.cr = int32(self.SF), int32(self.CR)
		cs.txDBm = int32(self.TxPowerDBm)
	}
	numF32("MHz", &cs.freqMHz, 100, 1000, "%.3f")
	numF32("kHz", &cs.bwKHz, 7, 500, "%.1f")
	imgui.SetNextItemWidth(110)
	imgui.InputIntV("spreading factor", &cs.sf, 0, 0, 0)
	imgui.SetNextItemWidth(110)
	imgui.InputIntV("coding rate", &cs.cr, 0, 0, 0)
	imgui.SetNextItemWidth(110)
	imgui.InputIntV("tx dBm", &cs.txDBm, 0, 0, 0)
	if imgui.Button("apply to the node") {
		_ = a.compSend(s, proto.SetRadioParams(uint32(cs.freqMHz*1000), uint32(cs.bwKHz*1000),
			uint8(cs.sf), uint8(cs.cr)))
		_ = a.compSend(s, proto.SetTxPower(uint8(cs.txDBm)))
		a.stepEngine(20)
		a.status = n.Name + ": radio set through its companion interface"
	}
	textDimWrap("Sent as the firmware's own commands, so it validates and stores them - " +
		"the scenario's own radio settings are separate and unchanged.")
}

// compUIState is what the tab holds per node.
type compUIState struct {
	channel  uint8
	scope    string
	draft    string
	newScope string
	freqMHz  float32
	bwKHz    float32
	sf       int32
	cr       int32
	txDBm    int32
}

// knownRegions is every region name the scenario has seen, for the scope
// picker - inference fills these in.
func (a *App) knownRegions() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range a.extraRegions {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for i := range a.Nodes {
		for _, r := range a.Nodes[i].Regions {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	return out
}

// pumpCompanions collects anything the firmware pushed, once per frame.
func (a *App) pumpCompanions() {
	for _, s := range a.comps {
		s.mu.Lock()
		wanted := !s.syncing
		waiting := false
		for _, f := range s.frames {
			if f.Push == proto.PushMsgWaiting {
				waiting = true
			}
		}
		s.frames = s.frames[:0]
		if waiting && wanted {
			s.syncing = true
		}
		s.mu.Unlock()
		if waiting && wanted {
			_ = a.compSend(s, proto.SyncNextMessage())
		}
	}
}

// compTabName is what the tab is called, and whether a node has one at all.
func compTabName(kind string) (string, bool) {
	if strings.Contains(kind, "companion") {
		return "Companion", true
	}
	return "", false
}

// addKnownRegion remembers a region the operator named, so it is offered
// again without having to be typed twice.
func (a *App) addKnownRegion(name string) {
	for _, r := range a.extraRegions {
		if r == name {
			return
		}
	}
	a.extraRegions = append(a.extraRegions, name)
}

// compSetScope sets the scope a companion sends under.
//
// Over the companion protocol, not the repeater CLI. The first version of this
// issued `region default <name>` and reported success: a companion build has no
// CLI, so the command went nowhere and every message went out unscoped while the
// UI said otherwise. On a transport-scoped mesh that is not a cosmetic fault -
// unscoped traffic is carried by different repeaters, so the run measures a
// different network from the one asked for.
func (a *App) compSetScope(s *compSession, node, scope string) error {
	if scope == scopeNull {
		if err := a.compSend(s, proto.ClearDefaultScope()); err != nil {
			return err
		}
		a.status = node + ": sending unscoped"
		return nil
	}
	if err := a.compSend(s, proto.SetDefaultScope(scope, provider.RegionKey(scope))); err != nil {
		return err
	}
	a.status = node + ": default scope set to " + scope
	return nil
}

// configureCompanion applies the scenario's configuration to a companion.
//
// The companion equivalent of provisioning, and it exists because provisioning
// is repeater CLI: a companion build has no CLI, so an imported companion kept
// the firmware's default name, no radio and no scope while the fleet window
// reported it configured. Everything here is the same settings a repeater gets,
// sent over the interface a companion actually has.
//
// Callable only while connected - the port is claimed, so there is no separate
// provisioning path fighting the tab for it.
func (a *App) configureCompanion(node string) {
	s := a.comps[node]
	i := a.nodeIndex(node)
	if s == nil || i < 0 {
		return
	}
	n := a.Nodes[i]
	// The clock first: a message sent before it is set carries a timestamp
	// from an epoch nobody else is in.
	_ = a.compSend(s, proto.SetDeviceTime(uint32(a.scenarioEpoch())))
	r := n.Radio
	_ = a.compSend(s, proto.SetRadioParams(
		uint32(r.CentreHz/1000), uint32(r.BandwidthHz),
		uint8(r.SpreadFactor), uint8(r.CodingRate+4)))
	// Clamped to what this node says it can do. The firmware refuses a power
	// above its ceiling outright rather than clamping it, and the refusal
	// arrives as a bare error code well after the command that caused it - so
	// asking a 20 dBm build for the scenario's 22 dBm produced an unexplained
	// "firmware error 6" on every companion.
	want := n.TxPowerDBm
	s.mu.Lock()
	self := s.self
	s.mu.Unlock()
	if self != nil && self.MaxTxPowerDBm > 0 && want > float64(self.MaxTxPowerDBm) {
		a.status = fmt.Sprintf("%s: scenario asks for %.0f dBm, this build tops out at %d",
			node, want, self.MaxTxPowerDBm)
		want = float64(self.MaxTxPowerDBm)
	}
	_ = a.compSend(s, proto.SetTxPower(uint8(want)))
	if n.Name != "" {
		_ = a.compSend(s, proto.SetAdvertName(truncateRunes(n.Name, maxNodeNameLen)))
	}
	if n.DefaultScope != "" {
		_ = a.compSetScope(s, node, n.DefaultScope)
	}
	a.stepEngine(20)
	// Read back rather than assume: what the node reports is what the run uses,
	// and a command the firmware rejected is otherwise invisible.
	_ = a.compSend(s, proto.AppStart("meshbench"))
	_ = a.compSend(s, proto.GetDefaultScope())
	a.stepEngine(10)
}

// compAddChannel puts a named channel in the next free slot.
//
// A companion out of the box has only "Public". A hashtag channel is a public
// name whose secret is derived from the name, exactly as a transport region's
// key is - so anyone who knows the name can join it, which is the whole point
// of a hashtag channel.
func (a *App) compAddChannel(s *compSession, name string) (int, error) {
	s.mu.Lock()
	used := map[int]bool{}
	for _, c := range s.channels {
		used[int(c.Index)] = true
		if strings.EqualFold(c.Name, name) {
			i := int(c.Index)
			s.mu.Unlock()
			return i, nil // already there; do not burn a second slot on it
		}
	}
	s.mu.Unlock()
	idx := -1
	for i := 1; i < 8; i++ { // slot 0 is Public, which is not ours to overwrite
		if !used[i] {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, fmt.Errorf("no free channel slot on this node")
	}
	if err := a.compSend(s, proto.SetChannel(uint8(idx), name, provider.RegionKey(name))); err != nil {
		return 0, err
	}
	a.stepEngine(10)
	// Read it back, so the list reflects the node rather than our intent.
	_ = a.compSend(s, proto.GetChannel(uint8(idx)))
	a.stepEngine(10)
	return idx, nil
}
