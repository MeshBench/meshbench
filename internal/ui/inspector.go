package ui

import (
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/capture"
	"github.com/A13xB0/meshcoresim/internal/engine"
)

// packetState is the inspector's selection: one packet, by id.
type packetState struct {
	id       uint64
	selected bool
	// wantJourney selects the Journey tab on the next draw — how "follow this
	// message" in a timeline's right-click lands on the right view.
	wantJourney bool
}

// drawPacketWindow dissects one packet and shows everywhere it went.
//
// CoreScope's packet view, with the thing CoreScope cannot have: the same
// packet's fate at every node, because the simulator watched all of them. The
// dissection itself is shared with the Wireshark plugin's field set, so the two
// cannot drift into disagreeing about the same bytes.
func (a *App) drawPacketWindow() {
	if !a.pkt.selected {
		return
	}
	imgui.SetNextWindowSizeV(imgui.NewVec2(640, 520), imgui.CondFirstUseEver)
	open := a.pkt.selected
	if imgui.BeginV(fmt.Sprintf("Packet #%d", a.pkt.id), &open, 0) {
		a.drawPacketBody()
	}
	imgui.End()
	a.pkt.selected = open
}

func (a *App) drawPacketBody() {
	events := a.events()
	var frame []byte
	var origin string
	var atMs uint32
	var rx, missed int
	var rows []engine.Event
	for _, ev := range events {
		if ev.PacketID != a.pkt.id {
			continue
		}
		rows = append(rows, ev)
		if ev.Frame != nil && frame == nil {
			frame = ev.Frame
		}
		switch ev.Kind {
		case "tx":
			origin, atMs = ev.From, ev.AtMs
		case "rx":
			rx++
		case "miss":
			missed++
		}
	}
	if frame == nil {
		imgui.TextDisabled("this packet is no longer in the ledger")
		return
	}

	imgui.Text(fmt.Sprintf("from %s at %.2f s", origin, float64(atMs)/1000))
	imgui.SameLine()
	imgui.TextDisabled(fmt.Sprintf("|  heard by %d, missed by %d", rx, missed))

	d := capture.Dissect(frame)
	if d.Truncated {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		imgui.TextWrapped("malformed: " + d.Problem)
		imgui.PopStyleColor()
	}

	if !imgui.BeginTabBar("##pkttabs") {
		return
	}
	if imgui.BeginTabItem("Dissection") {
		a.drawDissection(d, frame)
		imgui.EndTabItem()
	}
	jflags := imgui.TabItemFlags(0)
	if a.pkt.wantJourney {
		jflags = imgui.TabItemFlags(imgui.TabItemFlagsSetSelected)
		a.pkt.wantJourney = false
	}
	if imgui.BeginTabItemV("Journey", nil, jflags) {
		a.drawMessageJourney()
		imgui.EndTabItem()
	}
	if imgui.BeginTabItem("Reception ledger") {
		a.drawReceptionLedger()
		imgui.EndTabItem()
	}
	if imgui.BeginTabItem(fmt.Sprintf("Where it went (%d)", len(rows))) {
		a.drawPacketFates(rows)
		imgui.EndTabItem()
	}
	imgui.EndTabBar()
}

func (a *App) drawDissection(d capture.Dissection, frame []byte) {
	imgui.SeparatorText("Header")
	kv := func(k, v string) {
		imgui.TextDisabled(k)
		imgui.SameLineV(150, -1)
		imgui.Text(v)
	}
	kv("route type", fmt.Sprintf("%s (%d)", d.RouteName, d.RouteType))
	kv("payload type", fmt.Sprintf("%s (0x%02X)", d.PayloadName, d.PayloadType))
	kv("version", fmt.Sprint(d.Version))
	if d.HasTransport {
		kv("transport codes", fmt.Sprintf("%04X %04X", d.TransportCodes[0], d.TransportCodes[1]))
	}

	imgui.SeparatorText(fmt.Sprintf("Path — %d hop(s)", d.HopCount()))
	if len(d.PathHashes) == 0 {
		imgui.TextDisabled("none; this packet has not been relayed")
	} else {
		// Hashes resolved to names where the scenario knows them: a path of
		// "AB CD 4F" is data, "Ben Vrackie -> Dunkeld -> ..." is an answer.
		var parts []string
		for _, h := range d.PathHashes {
			if name, ok := a.nodeByHash(h); ok {
				parts = append(parts, fmt.Sprintf("%s (%02X)", name, h))
			} else {
				parts = append(parts, fmt.Sprintf("%02X", h))
			}
		}
		imgui.TextWrapped(strings.Join(parts, "  ->  "))
	}

	imgui.SeparatorText("Payload")
	if len(d.PayloadFields) == 0 {
		imgui.TextDisabled(fmt.Sprintf("%d bytes, encrypted or of a type with nothing in clear",
			len(d.Payload)))
	}
	for _, f := range d.PayloadFields {
		kv(f.Name, f.Value)
	}

	imgui.SeparatorText(fmt.Sprintf("Raw — %d bytes", len(frame)))
	if imgui.BeginChildStrV("##hex", imgui.NewVec2(0, 150), 0, 0) {
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
			imgui.Text(fmt.Sprintf("%04X  %-48s %s", off, hex.String(), ascii.String()))
		}
	}
	imgui.EndChild()
}

// drawPacketFates is what happened to this packet at every node — the view a
// real capture cannot produce, because no observer is everywhere.
func (a *App) drawPacketFates(rows []engine.Event) {
	if !imgui.BeginTableV("##fates", 4,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY,
		imgui.NewVec2(0, 0), 0) {
		return
	}
	imgui.TableSetupColumnV("t", imgui.TableColumnFlagsWidthFixed, 56, 0)
	imgui.TableSetupColumnV("node", imgui.TableColumnFlagsWidthFixed, 120, 0)
	imgui.TableSetupColumnV("SNR", imgui.TableColumnFlagsWidthFixed, 56, 0)
	imgui.TableSetupColumnV("outcome", imgui.TableColumnFlagsWidthStretch, 0, 0)
	imgui.TableHeadersRow()
	for _, ev := range rows {
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(fmt.Sprintf("%.2f", float64(ev.AtMs)/1000))
		imgui.TableSetColumnIndex(1)
		if ev.Kind == "tx" {
			imgui.Text(ev.From)
		} else {
			imgui.Text(ev.To)
		}
		imgui.TableSetColumnIndex(2)
		if ev.Kind != "tx" {
			imgui.Text(fmt.Sprintf("%+.1f", ev.SNRdB))
		}
		imgui.TableSetColumnIndex(3)
		imgui.PushStyleColorVec4(imgui.ColText, eventColour(ev))
		if ev.Kind == "tx" {
			imgui.Text("transmitted")
		} else {
			imgui.Text(ev.Detail)
		}
		imgui.PopStyleColor()
	}
	imgui.EndTable()
}

// nodeByHash resolves a path hash to a node name.
//
// MeshCore's path hash is the first byte of a node's public key, which the
// simulator does not hold — so this matches on what the ledger saw relaying
// this packet rather than computing the hash. Approximate by construction, and
// labelled as such where it fails.
func (a *App) nodeByHash(h byte) (string, bool) {
	if a.hashNames == nil {
		return "", false
	}
	n, ok := a.hashNames[h]
	return n, ok
}

// drawReceptionLedger is the five outcomes as columns, per node, per packet.
//
// The distinctions are the point, and the engine computes every one of them:
//
//   - never in range — nothing measurable arrived
//   - arrived, not demodulated — below the floor, or lost to capture
//   - demodulated, CRC failed — the radio heard it; the firmware never did
//   - firmware saw it and dropped it — dedup, hop limit, region deny
//   - accepted
//
// The third and fourth are the ones no firmware instrumentation could show, and
// they are the reason this table exists rather than a log.
func (a *App) drawReceptionLedger() {
	if a.eng == nil {
		imgui.TextDisabled("no simulation - press run in the strip above")
		return
	}
	rows := a.eng.Ledger.ForPacket(a.pkt.id)
	if len(rows) == 0 {
		imgui.TextDisabled("no receptions recorded for this packet")
		return
	}

	var reached, decoded int
	for _, r := range rows {
		if r.Offered {
			reached++
		}
		if r.Demod && r.CRCOK {
			decoded++
		}
	}
	imgui.TextDisabled(fmt.Sprintf("offered to %d nodes · decoded at %d", reached, decoded))

	if !imgui.BeginTableV("##ledger", 7,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY,
		imgui.NewVec2(0, 0), 0) {
		return
	}
	for _, h := range []string{"node", "offered", "RSSI", "SNR", "demod", "CRC", "firmware"} {
		imgui.TableSetupColumnV(h, imgui.TableColumnFlagsWidthStretch, 0, 0)
	}
	imgui.TableHeadersRow()

	yes := imgui.NewVec4(0.45, 0.85, 0.5, 1)
	no := imgui.NewVec4(0.85, 0.45, 0.45, 1)
	dim := imgui.NewVec4(0.5, 0.53, 0.6, 1)
	mark := func(ok, applicable bool) {
		if !applicable {
			// A dash, not a cross: "did not apply" and "failed" are different
			// facts, and a column that conflates them is why people read logs
			// instead of tables.
			imgui.PushStyleColorVec4(imgui.ColText, dim)
			imgui.Text("-")
			imgui.PopStyleColor()
			return
		}
		col, txt := no, "no"
		if ok {
			col, txt = yes, "yes"
		}
		imgui.PushStyleColorVec4(imgui.ColText, col)
		imgui.Text(txt)
		imgui.PopStyleColor()
	}

	for _, r := range rows {
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(r.ToNode)
		imgui.TableSetColumnIndex(1)
		mark(r.Offered, true)
		imgui.TableSetColumnIndex(2)
		if r.Offered {
			imgui.Text(fmt.Sprintf("%.1f", r.RSSIdBm))
		} else {
			imgui.TextDisabled("-")
		}
		imgui.TableSetColumnIndex(3)
		if r.Offered {
			imgui.Text(fmt.Sprintf("%+.1f", r.SNRdB))
		} else {
			imgui.TextDisabled("-")
		}
		imgui.TableSetColumnIndex(4)
		mark(r.Demod, r.Offered)
		imgui.TableSetColumnIndex(5)
		mark(r.CRCOK, r.Demod)
		imgui.TableSetColumnIndex(6)
		switch {
		case !r.Demod || !r.CRCOK:
			imgui.PushStyleColorVec4(imgui.ColText, dim)
			imgui.Text("never saw it")
			imgui.PopStyleColor()
		case r.FirmwareSaw:
			imgui.PushStyleColorVec4(imgui.ColText, yes)
			imgui.Text("accepted")
			imgui.PopStyleColor()
		default:
			imgui.Text("dropped")
		}

		// Why?, per row — the spec's own control, and the thing that makes the
		// table drillable rather than final.
		imgui.SameLine()
		if imgui.SmallButton("why?##" + r.ToNode) {
			a.selectPairByName(r.FromNode, r.ToNode)
		}
	}
	imgui.EndTable()
}

// selectPairByName selects two nodes, so the profile and budget panels answer
// about exactly this pair.
func (a *App) selectPairByName(from, to string) {
	i, j := a.nodeIndex(from), a.nodeIndex(to)
	if i < 0 || j < 0 {
		return
	}
	a.SelectNode(i, false)
	a.SelectNode(j, true)
}

// drawMessageJourney follows one message across every hop it took.
//
// A packet is one transmission; a message is the thing an operator cares about,
// and it is relayed by node after node with different bytes on the air each
// time — MeshCore appends a hop hash at every relay. Following it is only
// possible because identity is taken from the payload rather than the frame.
func (a *App) drawMessageJourney() {
	events := a.events()
	var msgID uint64
	for _, ev := range events {
		if ev.PacketID == a.pkt.id {
			msgID = ev.MessageID
			break
		}
	}
	if msgID == 0 {
		imgui.TextDisabled("this message cannot be followed; its frame did not parse")
		return
	}

	// One row per transmission of this message, in time order: who relayed it,
	// how many hops in, and who heard that particular relay.
	type hop struct {
		at     uint32
		by     string
		hops   int
		heard  []string
		missed int
		packet uint64
	}
	var hops []hop
	byPacket := map[uint64]int{}
	for _, ev := range events {
		if ev.MessageID != msgID {
			continue
		}
		switch ev.Kind {
		case "tx":
			d := capture.Dissect(ev.Frame)
			byPacket[ev.PacketID] = len(hops)
			hops = append(hops, hop{at: ev.AtMs, by: ev.From, hops: d.HopCount(), packet: ev.PacketID})
		case "rx":
			if i, ok := byPacket[ev.PacketID]; ok {
				hops[i].heard = append(hops[i].heard, ev.To)
			}
		case "miss":
			if i, ok := byPacket[ev.PacketID]; ok {
				hops[i].missed++
			}
		}
	}
	if len(hops) == 0 {
		imgui.TextDisabled("no transmissions of this message")
		return
	}

	reached := map[string]bool{}
	for _, h := range hops {
		for _, n := range h.heard {
			reached[n] = true
		}
	}
	imgui.Text(fmt.Sprintf("%d transmissions, reaching %d distinct nodes over %.1f s",
		len(hops), len(reached), float64(hops[len(hops)-1].at-hops[0].at)/1000))
	imgui.TextDisabled("one message, followed by its payload — the bytes on the air differ " +
		"at every hop, because each relay appends itself to the path")

	if !imgui.BeginTableV("##journey", 5,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY,
		imgui.NewVec2(0, 0), 0) {
		return
	}
	imgui.TableSetupColumnV("t", imgui.TableColumnFlagsWidthFixed, 56, 0)
	imgui.TableSetupColumnV("relayed by", imgui.TableColumnFlagsWidthFixed, 120, 0)
	imgui.TableSetupColumnV("hop", imgui.TableColumnFlagsWidthFixed, 44, 0)
	imgui.TableSetupColumnV("heard by", imgui.TableColumnFlagsWidthStretch, 0, 0)
	imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthFixed, 54, 0)
	imgui.TableHeadersRow()

	for _, h := range hops {
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(fmt.Sprintf("%.2f", float64(h.at)/1000))
		imgui.TableSetColumnIndex(1)
		imgui.Text(h.by)
		imgui.TableSetColumnIndex(2)
		imgui.Text(fmt.Sprint(h.hops))
		imgui.TableSetColumnIndex(3)
		if len(h.heard) == 0 {
			// A relay nobody heard is the interesting kind: airtime spent for
			// nothing, which is what the scoreboard's redundancy figure counts.
			imgui.TextDisabled(fmt.Sprintf("nobody (%d could not decode)", h.missed))
		} else {
			imgui.Text(strings.Join(h.heard, ", "))
		}
		imgui.TableSetColumnIndex(4)
		if imgui.SmallButton(fmt.Sprintf("this##h%d", h.packet)) {
			a.pkt.id = h.packet
		}
	}
	imgui.EndTable()
}
