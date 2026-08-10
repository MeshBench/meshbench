package ui

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/companion/proto"
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

	mu       sync.Mutex
	rx       []byte // partial frame from the firmware
	frames   []proto.Frame
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
	s := &compSession{node: node}
	s.release = n.Firmware.Bridge.Claim(s)
	a.comps[node] = s
	// The handshake. Everything else waits on the reply to this.
	if err := a.compSend(s, proto.AppStart("meshbench")); err != nil {
		a.compDisconnect(node)
		return err
	}
	a.stepEngine(20)
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
			cs.scope = "<null>"
		}
		for _, r := range a.knownRegions() {
			if imgui.SelectableBool(r) {
				cs.scope = r
			}
		}
		imgui.EndCombo()
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Scope is the node's own transport region, not part of the message,\n" +
			"so choosing one issues 'region default' at its CLI before sending.")
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
			textWrap(m.Text)
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

// compSendMessage applies the scope, then sends.
func (a *App) compSendMessage(node string, s *compSession, cs *compUIState) {
	if cs.scope != "" {
		// The one place this reaches for the CLI rather than the companion
		// protocol, because the scope is the node's configuration and not a
		// property of the message.
		if err := a.typeAt(node, "region default "+cs.scope); err != nil {
			a.status = err.Error()
			return
		}
		a.stepEngine(20)
		a.status = node + ": default scope set to " + cs.scope + " before sending"
	}
	if err := a.compSend(s, proto.SendChannelText(cs.channel, time.Now(), cs.draft)); err != nil {
		a.status = err.Error()
		return
	}
	cs.draft = ""
	a.stepEngine(40)
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
		_ = a.compSend(s, proto.SetRadioParams(uint32(cs.freqMHz*1000), uint32(cs.bwKHz),
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
	channel uint8
	scope   string
	draft   string
	freqMHz float32
	bwKHz   float32
	sf      int32
	cr      int32
	txDBm   int32
}

// knownRegions is every region name the scenario has seen, for the scope
// picker - inference fills these in.
func (a *App) knownRegions() []string {
	seen := map[string]bool{}
	var out []string
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
