// Turning what a companion actually sent into something a person can read.
//
// A companion's console is not text. It carries MeshCore's own framing - '>'
// from the node or '<' towards it, a little-endian length, then a payload - and
// a pane that draws those bytes shows a wall of escapes with the answer buried
// in it. Typing "ver" at one and getting `\xaf(\x00...Heltec V3...v1.17.1` back
// is the whole problem: the information is there and unreadable.
//
// So this decodes what is on screen rather than replacing it with something
// else. The tick used to swap the pane for the transcript console.cli had
// already decoded, which is a different conversation: it answers "what did I
// type and what came back", not "what is this board saying", and it drops
// everything the board said on its own account - every push, every advert, and
// the boot log around them.
//
// What is not a frame stays as it was. A board prints plain text on the same
// port it frames on, so a decoder that consumed everything would hide the
// bootloader, and the bootloader is what says whether the board started.
package boardview

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/MeshBench/meshbench/internal/mesh/proto"
)

// maxFrame is the largest payload length this will believe.
//
// A length is two bytes of whatever happened to follow a '>' in ordinary text,
// so most candidates are nonsense. MeshCore's own frames are well under this;
// anything larger is a coincidence rather than a frame, and treating it as one
// would swallow the rest of the log.
const maxFrame = 1024

// decodeFrames renders a console pane with its frames read rather than escaped.
//
// The pane's lines are what printableLines made: printable bytes as themselves
// and everything else as `\xNN`. That escaping is reversible, which is what
// makes decoding possible this far from the serial port - the bytes were never
// lost, only spelled.
func decodeFrames(lines []string) []string {
	raw := unescape(strings.Join(lines, "\n"))
	var out []string
	var text strings.Builder
	flushText := func() {
		for _, l := range strings.Split(text.String(), "\n") {
			if strings.TrimSpace(l) != "" {
				out = append(out, l)
			}
		}
		text.Reset()
	}

	for i := 0; i < len(raw); {
		payload, dir, size, ok := frameAt(raw, i)
		if !ok {
			text.WriteByte(raw[i])
			i++
			continue
		}
		flushText()
		out = append(out, describe(dir, payload))
		i += size
	}
	flushText()
	return out
}

// frameAt reads a frame starting at i, or reports that there is not one.
func frameAt(b []byte, i int) (payload []byte, dir byte, size int, ok bool) {
	if b[i] != '>' && b[i] != '<' {
		return nil, 0, 0, false
	}
	if i+3 > len(b) {
		return nil, 0, 0, false
	}
	n := int(binary.LittleEndian.Uint16(b[i+1:]))
	// A zero-length frame is not one, and a length running past the end of what
	// has been read is a frame still arriving rather than a frame to draw.
	if n == 0 || n > maxFrame || i+3+n > len(b) {
		return nil, 0, 0, false
	}
	return b[i+3 : i+3+n], b[i], 3 + n, true
}

// describe is one frame in a line of prose.
func describe(dir byte, payload []byte) string {
	arrow := "<-"
	if dir == '<' {
		arrow = "->"
	}
	f, err := proto.Decode(payload)
	if err != nil {
		// Said rather than swallowed: a frame this package cannot read is still
		// a frame, and that it was there is usually the thing being looked for.
		return fmt.Sprintf("%s %d bytes, undecoded: %v", arrow, len(payload), err)
	}
	return arrow + " " + body(f, payload)
}

// body is what one decoded frame says.
func body(f proto.Frame, payload []byte) string {
	if f.Push != 0 {
		return pushName(f.Push)
	}
	switch {
	case f.Err != "":
		return "error: " + f.Err
	case f.SelfInfo != nil:
		si := f.SelfInfo
		return fmt.Sprintf("self: %q at %.5f,%.5f, %.3f MHz SF%d CR%d, %d dBm",
			si.Name, si.AdvLat, si.AdvLon, float64(si.FreqKHz)/1000,
			si.SF, si.CR, si.TxPowerDBm)
	case f.Device != nil:
		return "device: " + deviceLine(f)
	case f.Scope != nil:
		if f.Scope.Name == "" {
			return "default scope: none"
		}
		return "default scope: " + f.Scope.Name
	case f.Channel != nil:
		return fmt.Sprintf("channel %d: %q", f.Channel.Index, f.Channel.Name)
	case f.Contact != nil:
		return "contact: " + contactLine(f)
	case f.Message != nil:
		return "message: " + messageLine(f)
	}
	name := respName(f.Code)
	if len(payload) > 1 {
		return fmt.Sprintf("%s, %d bytes", name, len(payload)-1)
	}
	return name
}

// The three that need a field this package cannot assume the shape of are read
// through the printer rather than by naming members that may not exist.
func deviceLine(f proto.Frame) string  { return trimBrace(fmt.Sprintf("%+v", *f.Device)) }
func contactLine(f proto.Frame) string { return trimBrace(fmt.Sprintf("%+v", *f.Contact)) }
func messageLine(f proto.Frame) string { return trimBrace(fmt.Sprintf("%+v", *f.Message)) }

func trimBrace(s string) string { return strings.TrimSuffix(strings.TrimPrefix(s, "{"), "}") }

// respName is the response code's own name, or its number where this build does
// not know it.
//
// Unknown codes are numbered rather than hidden, for the reason proto.Decode
// gives for returning them: the firmware gains responses faster than anything
// tracks them, and a decoder that drew nothing for an unfamiliar one would be
// least useful exactly when something new is happening.
func respName(c proto.Response) string {
	if n, ok := respNames[c]; ok {
		return n
	}
	return "response " + strconv.Itoa(int(c))
}

var respNames = map[proto.Response]string{
	proto.RespOK: "ok", proto.RespErr: "error",
	proto.RespContactsStart: "contacts start", proto.RespContact: "contact",
	proto.RespEndOfContacts: "end of contacts", proto.RespSelfInfo: "self info",
	proto.RespSent: "sent", proto.RespContactMsgRecv: "contact message",
	proto.RespChannelMsgRecv: "channel message", proto.RespCurrTime: "current time",
	proto.RespNoMoreMessages: "no more messages",
	proto.RespExportContact:  "export contact",
	proto.RespBattAndStorage: "battery and storage",
	proto.RespDeviceInfo:     "device info", proto.RespPrivateKey: "private key",
	proto.RespDisabled:          "disabled",
	proto.RespContactMsgRecvV3:  "contact message (v3)",
	proto.RespChannelMsgRecvV3:  "channel message (v3)",
	proto.RespChannelInfo:       "channel info",
	proto.RespDefaultFloodScope: "default flood scope",
}

func pushName(p proto.Push) string {
	if n, ok := pushNames[p]; ok {
		return n
	}
	return "push " + strconv.Itoa(int(p))
}

var pushNames = map[proto.Push]string{
	proto.PushAdvert: "advert heard", proto.PushPathUpdated: "path updated",
	proto.PushSendConfirmed: "send confirmed", proto.PushMsgWaiting: "message waiting",
	proto.PushRawData: "raw data", proto.PushLoginSuccess: "login succeeded",
	proto.PushLoginFail: "login failed", proto.PushStatusResp: "status",
}

// unescape turns printableLines' `\xNN` back into the byte it stood for.
//
// Only that form: a board printing a literal backslash-x is far rarer than a
// board printing a byte, and treating every backslash as an escape would eat
// Windows paths out of a bootloader log.
func unescape(s string) []byte {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		if i+3 < len(s) && s[i] == '\\' && s[i+1] == 'x' {
			if v, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
				out = append(out, byte(v))
				i += 4
				continue
			}
		}
		out = append(out, s[i])
		i++
	}
	return out
}
