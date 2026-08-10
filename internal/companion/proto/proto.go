// Package proto encodes and decodes MeshCore's companion protocol.
//
// This is what a phone app speaks to a companion_radio build over BLE, USB
// serial or TCP: little-endian binary frames, one command per frame, inside
// the transport's own '<' / '>' + LE16 length envelope that
// internal/companion already carries. Nothing here re-frames anything.
//
// The command and response numbers are MeshCore's, from
// examples/companion_radio/MyMesh.cpp. They are values on the wire, so they
// are written here as the constants they are rather than derived from
// anything: a renumbering upstream is a protocol change, and it should break
// this package loudly rather than be absorbed.
package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// Command is a frame's first byte.
type Command uint8

const (
	CmdAppStart          Command = 1
	CmdSendTxtMsg        Command = 2
	CmdSendChannelTxtMsg Command = 3
	CmdGetContacts       Command = 4
	CmdGetDeviceTime     Command = 5
	CmdSetDeviceTime     Command = 6
	CmdSendSelfAdvert    Command = 7
	CmdSetAdvertName     Command = 8
	CmdSyncNextMessage   Command = 10
	CmdSetRadioParams    Command = 11
	CmdSetRadioTxPower   Command = 12
	CmdSetAdvertLatLon   Command = 14
	CmdDeviceQuery       Command = 22
	CmdGetChannel        Command = 31
	CmdSetChannel        Command = 32
)

// Response is a reply frame's first byte.
type Response uint8

const (
	RespOK             Response = 0
	RespErr            Response = 1
	RespContactsStart  Response = 2
	RespContact        Response = 3
	RespEndOfContacts  Response = 4
	RespSelfInfo       Response = 5
	RespSent           Response = 6
	RespContactMsgRecv Response = 7
	RespChannelMsgRecv Response = 8
	RespCurrTime       Response = 9
	RespNoMoreMessages Response = 10
	RespExportContact  Response = 11
	RespBattAndStorage Response = 12
	RespDeviceInfo     Response = 13
	RespPrivateKey     Response = 14
	RespDisabled       Response = 15
	// v3 message variants: the firmware sends these instead of 7 and 8 once
	// the client declares a high enough protocol version, and a client that
	// only knows the old pair sees messages arrive and decodes none of them.
	RespContactMsgRecvV3 Response = 16
	RespChannelMsgRecvV3 Response = 17
	RespChannelInfo      Response = 18
)

// Push is an unsolicited frame: the firmware telling the client something
// happened rather than answering a question.
type Push uint8

const (
	PushAdvert        Push = 0x80
	PushPathUpdated   Push = 0x81
	PushSendConfirmed Push = 0x82
	PushMsgWaiting    Push = 0x83
	PushRawData       Push = 0x84
	PushLoginSuccess  Push = 0x85
	PushLoginFail     Push = 0x86
	PushStatusResp    Push = 0x87
	PushLogRXData     Push = 0x88
	PushTraceData     Push = 0x89
	PushNewAdvert     Push = 0x8A
)

// txtTypePlain is the only message type the firmware accepts on the channel
// send path; anything else answers ERR_CODE_UNSUPPORTED_CMD.
const txtTypePlain = 0

// AppStart is the handshake. The firmware answers with SelfInfo, and until
// it has, nothing else is worth sending.
func AppStart(appName string) []byte {
	// app ver, then six reserved bytes, then the name. The firmware reads a
	// fixed prefix and treats the rest as the name.
	b := []byte{byte(CmdAppStart), 1, 0, 0, 0, 0, 0, 0}
	return append(b, appName...)
}

// SendChannelText sends a plain text message to a channel *by index*.
//
// Index, not name: the firmware addresses channels by slot, and the slot's
// name and key live in the device. A client that wants to send to "#sco"
// must find which slot holds it first.
func SendChannelText(channelIdx uint8, at time.Time, text string) []byte {
	b := make([]byte, 0, 7+len(text))
	b = append(b, byte(CmdSendChannelTxtMsg), txtTypePlain, channelIdx)
	b = binary.LittleEndian.AppendUint32(b, uint32(at.Unix()))
	return append(b, text...)
}

// SendContactText sends to one contact, addressed by the first six bytes of
// its public key - which is what the firmware matches on.
func SendContactText(pubKeyPrefix []byte, at time.Time, attempt uint8, text string) ([]byte, error) {
	if len(pubKeyPrefix) < 6 {
		return nil, fmt.Errorf("proto: contact key prefix is %d bytes, need 6", len(pubKeyPrefix))
	}
	b := make([]byte, 0, 13+len(text))
	b = append(b, byte(CmdSendTxtMsg), txtTypePlain, attempt)
	b = binary.LittleEndian.AppendUint32(b, uint32(at.Unix()))
	b = append(b, pubKeyPrefix[:6]...)
	return append(b, text...), nil
}

// GetContacts asks for the contact list, optionally only those changed since
// a time - the firmware's own sync shortcut.
func GetContacts(since time.Time) []byte {
	if since.IsZero() {
		return []byte{byte(CmdGetContacts)}
	}
	return binary.LittleEndian.AppendUint32([]byte{byte(CmdGetContacts)}, uint32(since.Unix()))
}

// SendSelfAdvert advertises this node. flood true sends it to the whole
// mesh rather than only to neighbours.
func SendSelfAdvert(flood bool) []byte {
	kind := byte(1) // zero-hop
	if flood {
		kind = 2
	}
	return []byte{byte(CmdSendSelfAdvert), kind}
}

// SetRadioParams sets the modem.
//
// The units are not the same on both fields, and getting that wrong costs the
// whole command: the firmware validates frequency against 150000..2500000 and
// bandwidth against 7000..500000, then divides each by 1000 to store MHz and
// kHz. So frequency is kHz and bandwidth is Hz. Sending bandwidth in kHz - 250
// where it wants 250000 - fails the range check and the firmware answers
// ERR_CODE_ILLEGAL_ARG for the whole frame, leaving frequency unset too. That
// is why a configured companion still reported 0 MHz.
func SetRadioParams(freqKHz, bwHz uint32, sf, cr uint8) []byte {
	b := []byte{byte(CmdSetRadioParams)}
	b = binary.LittleEndian.AppendUint32(b, freqKHz)
	b = binary.LittleEndian.AppendUint32(b, bwHz)
	return append(b, sf, cr)
}

// SetTxPower sets transmit power in dBm.
func SetTxPower(dBm uint8) []byte { return []byte{byte(CmdSetRadioTxPower), dBm} }

// SetAdvertName sets the name this node advertises.
func SetAdvertName(name string) []byte {
	return append([]byte{byte(CmdSetAdvertName)}, name...)
}

// SetAdvertLatLon sets the position this node advertises, in the firmware's
// millionths of a degree.
func SetAdvertLatLon(lat, lon float64) []byte {
	b := []byte{byte(CmdSetAdvertLatLon)}
	b = binary.LittleEndian.AppendUint32(b, uint32(int32(lat*1e6)))
	return binary.LittleEndian.AppendUint32(b, uint32(int32(lon*1e6)))
}

// GetChannel reads one channel slot.
func GetChannel(idx uint8) []byte { return []byte{byte(CmdGetChannel), idx} }

// SyncNextMessage asks for the next queued inbound message. The firmware
// pushes MsgWaiting; this is how the message itself is collected.
func SyncNextMessage() []byte { return []byte{byte(CmdSyncNextMessage)} }

// DeviceQuery asks what the firmware is.
func DeviceQuery() []byte { return []byte{byte(CmdDeviceQuery), 3} }

// SelfInfo is the reply to AppStart: who this node is and how its radio is
// configured.
type SelfInfo struct {
	PublicKey     []byte
	Name          string
	AdvLat        float64
	AdvLon        float64
	FreqKHz       uint32
	BWKHz         uint32
	SF            uint8
	CR            uint8
	MaxTxPowerDBm uint8
	TxPowerDBm    uint8
}

// ChannelInfo is one channel slot.
type ChannelInfo struct {
	Index  uint8
	Name   string
	Secret []byte
}

// Message is an inbound text message, from a contact or a channel.
type Message struct {
	Channel    bool
	ChannelIdx uint8
	SenderKey  []byte
	SenderName string
	Text       string
	At         time.Time
	SNRdB      float64
	PathLen    int

	// Mine marks a message this client sent, rather than one the node
	// received. It is still the firmware's doing - the frame went to the node
	// and the node transmitted it - but nothing comes back to echo it, so
	// without this the operator sends into a conversation that stays empty.
	Mine bool
	// Confirmed is set when the firmware acknowledges the send.
	Confirmed bool
}

// Frame is a decoded reply or push.
type Frame struct {
	Code Response
	Push Push
	// Exactly one of these is set, according to Code.
	SelfInfo *SelfInfo
	Channel  *ChannelInfo
	Message  *Message
	Contact  *Contact
	Err      string
	// Raw is the whole frame, kept so anything this package does not decode
	// is still inspectable rather than lost.
	Raw []byte
}

// Contact is one entry of the contact list.
type Contact struct {
	PublicKey  []byte
	Type       uint8
	Flags      uint8
	Name       string
	LastSeen   time.Time
	OutPathLen int8
	Lat, Lon   float64
}

// Decode turns one reply frame into something usable.
//
// Unknown codes are returned rather than rejected: the firmware gains
// responses faster than any client tracks them, and a client that errors on
// an unfamiliar frame stops working the day it is upgraded.
func Decode(frame []byte) (Frame, error) {
	if len(frame) == 0 {
		return Frame{}, fmt.Errorf("proto: empty frame")
	}
	f := Frame{Code: Response(frame[0]), Raw: frame}
	if frame[0] >= 0x80 {
		f.Push = Push(frame[0])
		if f.Push == PushMsgWaiting {
			return f, nil
		}
	}
	body := frame[1:]

	switch f.Code {
	case RespErr:
		f.Err = ErrText(firstOr(body, 0))
	case RespSelfInfo:
		si, err := decodeSelfInfo(body)
		if err != nil {
			return f, err
		}
		f.SelfInfo = si
	case RespChannelInfo:
		if len(body) < 1 {
			return f, fmt.Errorf("proto: channel info with no index")
		}
		ci := &ChannelInfo{Index: body[0]}
		// name, then the shared secret; the name is NUL-padded.
		if len(body) > 1 {
			ci.Name = trimNUL(body[1:min(33, len(body))])
		}
		if len(body) > 33 {
			ci.Secret = body[33:]
		}
		f.Channel = ci
	case RespContact:
		c, err := decodeContact(body)
		if err != nil {
			return f, err
		}
		f.Contact = c
	case RespChannelMsgRecv, RespContactMsgRecv, RespChannelMsgRecvV3, RespContactMsgRecvV3:
		channel := f.Code == RespChannelMsgRecv || f.Code == RespChannelMsgRecvV3
		v3 := f.Code == RespChannelMsgRecvV3 || f.Code == RespContactMsgRecvV3
		m, err := decodeMessage(channel, v3, body)
		if err != nil {
			return f, err
		}
		f.Message = m
	}
	return f, nil
}

func decodeSelfInfo(b []byte) (*SelfInfo, error) {
	// The firmware's own write order (MyMesh.cpp, RESP_CODE_SELF_INFO):
	// advert type, tx power, max tx power, 32-byte public key, lat, lon,
	// then four preference bytes - multi_acks, advert_loc_policy, the packed
	// telemetry modes, manual_add_contacts - and only then the radio and the
	// name.
	//
	// Those four bytes were the whole bug: skipping them read the radio from
	// the wrong offset, so a live node came back as SF 208 at 0 MHz with a
	// name that started mid-key.
	if len(b) < 35 {
		return nil, fmt.Errorf("proto: self info is %d bytes, need 35", len(b))
	}
	// b[0] is the advert type, b[1] the current power, b[2] the node's ceiling.
	// The ceiling matters: the firmware refuses a power above it outright
	// rather than clamping, so a client that does not read it asks for
	// something impossible and gets ERR_CODE_ILLEGAL_ARG.
	si := &SelfInfo{
		TxPowerDBm:    b[1],
		MaxTxPowerDBm: b[2],
		PublicKey:     append([]byte(nil), b[3:35]...),
	}
	i := 35
	if len(b) >= i+8 {
		si.AdvLat = float64(int32(binary.LittleEndian.Uint32(b[i:]))) / 1e6
		si.AdvLon = float64(int32(binary.LittleEndian.Uint32(b[i+4:]))) / 1e6
		i += 8
	}
	// multi_acks, advert_loc_policy, telemetry modes, manual_add_contacts.
	if len(b) >= i+4 {
		i += 4
	}
	if len(b) >= i+10 {
		// The two fields are not in the same unit, because the preferences
		// they come from are not: the firmware sends freq = _prefs.freq * 1000
		// where that preference is MHz, and bw = _prefs.bw * 1000 where that
		// one is kHz. So frequency arrives in kHz and bandwidth in Hz, exactly
		// mirroring what CMD_SET_RADIO_PARAMS expects. Dividing both by 1000
		// reported an 869 MHz node as running on 0.869 MHz.
		si.FreqKHz = binary.LittleEndian.Uint32(b[i:])
		si.BWKHz = binary.LittleEndian.Uint32(b[i+4:]) / 1000
		si.SF, si.CR = b[i+8], b[i+9]
		i += 10
	}
	if len(b) > i {
		si.Name = trimNUL(b[i:])
	}
	return si, nil
}

// decodeContact reads one contact record.
//
// The layout, from writeContactRespFrame:
//
//	[pub_key:32][type][flags][out_path_len][out_path:64][name:32]
//	[last_advert:4][gps_lat:4][gps_lon:4][lastmod:4]
//
// The 64-byte out_path is the part worth stating: reading the name straight
// after out_path_len puts it 64 bytes early, in the middle of the path, so
// every contact came back with an empty name.
func decodeContact(b []byte) (*Contact, error) {
	const (
		keyLen  = 32
		pathLen = 64
		nameLen = 32
		nameAt  = keyLen + 3 + pathLen
	)
	if len(b) < nameAt+nameLen {
		return nil, fmt.Errorf("proto: contact is %d bytes, need %d", len(b), nameAt+nameLen)
	}
	c := &Contact{
		PublicKey:  append([]byte(nil), b[0:keyLen]...),
		Type:       b[keyLen],
		Flags:      b[keyLen+1],
		OutPathLen: int8(b[keyLen+2]),
	}
	c.Name = trimNUL(b[nameAt : nameAt+nameLen])
	i := nameAt + nameLen
	if len(b) >= i+4 {
		c.LastSeen = time.Unix(int64(binary.LittleEndian.Uint32(b[i:])), 0).UTC()
		i += 4
	}
	if len(b) >= i+8 {
		c.Lat = float64(int32(binary.LittleEndian.Uint32(b[i:]))) / 1e6
		c.Lon = float64(int32(binary.LittleEndian.Uint32(b[i+4:]))) / 1e6
	}
	return c, nil
}

// decodeMessage reads a received message.
//
// Two layouts, and the difference is not just extra fields at the end. From a
// live frame, 08 01 01 00 dc5e7a6a "fif-sender: HELLO-MARKER":
//
//	pre-v3: [channel_idx][path_len][txt_type][timestamp:4][text]
//	v3:     [snr][reserved][reserved][channel_idx][path_len][txt_type][timestamp:4][text]
//
// The pre-v3 frame carries no SNR at all. Reading its first byte as one made
// the channel index the signal level, put the text one byte late, and glued the
// top byte of the timestamp - 0x6A, 'j' - to the front of every sender's name.
//
// Which arrives depends on the protocol version this client advertised in
// CMD_APP_START, so a client cannot pick one and assume it.
func decodeMessage(channel, v3 bool, b []byte) (*Message, error) {
	if len(b) < 2 {
		return nil, fmt.Errorf("proto: message is %d bytes", len(b))
	}
	m := &Message{Channel: channel, PathLen: -1}
	i := 0
	if v3 {
		m.SNRdB = float64(int8(b[0])) / 4
		i = 3 // snr, then two reserved bytes
	}
	if channel {
		if len(b) <= i {
			return nil, fmt.Errorf("proto: channel message with no channel")
		}
		m.ChannelIdx = b[i]
		i++
	} else {
		if len(b) < i+6 {
			return nil, fmt.Errorf("proto: contact message with no sender")
		}
		m.SenderKey = append([]byte(nil), b[i:i+6]...)
		i += 6
	}
	if len(b) > i {
		// 0xFF is "did not arrive by flood", not a 255-hop path.
		if b[i] != 0xFF {
			m.PathLen = int(b[i])
		}
		i++
	}
	if len(b) > i {
		i++ // txt_type
	}
	if len(b) >= i+4 {
		m.At = time.Unix(int64(binary.LittleEndian.Uint32(b[i:])), 0).UTC()
		i += 4
	}
	if len(b) > i {
		text := trimNUL(b[i:])
		if channel {
			// Channel messages are "sender: text"; direct ones are bare.
			if k := strings.Index(text, ": "); k > 0 {
				m.SenderName, text = text[:k], text[k+2:]
			}
		}
		m.Text = text
	}
	return m, nil
}

func trimNUL(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func firstOr(b []byte, d byte) byte {
	if len(b) == 0 {
		return d
	}
	return b[0]
}

// Scope commands.
//
// A companion carries its own transport scope in prefs (default_scope_name,
// default_scope_key) and there is no CLI on a companion build to set it - the
// repeater's `region default` is not a command this firmware has. These are how
// a companion is scoped, and without them a companion sends unscoped no matter
// what the scenario says it belongs to.
const (
	CmdSetFloodScopeKey     Command = 54
	CmdSetDefaultFloodScope Command = 63
	CmdGetDefaultFloodScope Command = 64

	RespDefaultFloodScope Response = 28
)

// scopeNameLen is the firmware's fixed name field. The key sits immediately
// after it at a fixed offset, so the name is padded rather than terminated.
const scopeNameLen = 31

// SetDefaultScope sets the scope a companion sends under.
//
// Name and key both, because the firmware stores both and uses the key: it does
// not derive one from the other, so a name without its key scopes nothing.
func SetDefaultScope(name string, key [16]byte) []byte {
	b := make([]byte, 1+scopeNameLen+16)
	b[0] = byte(CmdSetDefaultFloodScope)
	copy(b[1:1+scopeNameLen-1], name) // -1 keeps the terminator the firmware reads
	copy(b[1+scopeNameLen:], key[:])
	return b
}

// ClearDefaultScope makes a companion send unscoped. A bare command, which the
// firmware reads as "null scope" rather than as a malformed one.
func ClearDefaultScope() []byte { return []byte{byte(CmdSetDefaultFloodScope)} }

// GetDefaultScope asks what scope the node actually holds.
func GetDefaultScope() []byte { return []byte{byte(CmdGetDefaultFloodScope)} }

// DecodeDefaultScope reads the reply. An empty name means unscoped, which the
// firmware signals by sending the response code alone.
func DecodeDefaultScope(b []byte) (name string, key [16]byte, ok bool) {
	if len(b) < 1+scopeNameLen+16 {
		return "", key, len(b) >= 1
	}
	n := b[1 : 1+scopeNameLen]
	if i := bytes.IndexByte(n, 0); i >= 0 {
		n = n[:i]
	}
	copy(key[:], b[1+scopeNameLen:])
	return string(n), key, true
}

// channelNameLen is the firmware's fixed name field, ahead of the secret.
const channelNameLen = 32

// SetChannel puts a channel in a slot: name, then a 128-bit secret.
//
// The firmware supports only 128-bit secrets here - it rejects the 256-bit
// form with ERR_CODE_UNSUPPORTED_CMD - so the secret is always 16 bytes.
func SetChannel(idx uint8, name string, secret [16]byte) []byte {
	b := make([]byte, 2+channelNameLen+16)
	b[0] = byte(CmdSetChannel)
	b[1] = idx
	copy(b[2:2+channelNameLen-1], name)
	copy(b[2+channelNameLen:], secret[:])
	return b
}

// SetDeviceTime sets the node's clock, in epoch seconds.
//
// A companion needs this as much as a repeater does, and cannot be given it the
// same way: provisioning speaks the repeater CLI, so `time <epoch>` never
// reaches a companion build. It timestamps the messages it sends, and those
// timestamps are what the rest of the mesh judges freshness by.
func SetDeviceTime(epoch uint32) []byte {
	b := []byte{byte(CmdSetDeviceTime)}
	return binary.LittleEndian.AppendUint32(b, epoch)
}

// ErrText names an error code.
//
// "firmware error 6" is true and useless: it says something was rejected
// without saying what kind of wrong it was, and the codes are a short fixed
// list the client already has to know.
func ErrText(code byte) string {
	switch code {
	case 1:
		return "firmware rejected the command as unsupported"
	case 2:
		return "firmware could not find what the command referred to"
	case 3:
		return "firmware table is full"
	case 4:
		return "firmware is in the wrong state for that"
	case 5:
		return "firmware storage error"
	case 6:
		return "firmware rejected an argument as out of range"
	default:
		return fmt.Sprintf("firmware error %d", code)
	}
}

// CommandName names a command byte, for error messages.
func CommandName(c byte) string {
	switch Command(c) {
	case CmdAppStart:
		return "app start"
	case CmdSendTxtMsg:
		return "send message"
	case CmdSendChannelTxtMsg:
		return "send channel message"
	case CmdGetContacts:
		return "get contacts"
	case CmdGetDeviceTime:
		return "get time"
	case CmdSetDeviceTime:
		return "set time"
	case CmdSendSelfAdvert:
		return "send advert"
	case CmdSetAdvertName:
		return "set name"
	case CmdSetRadioParams:
		return "set radio"
	case CmdSetRadioTxPower:
		return "set tx power"
	case CmdGetChannel:
		return "get channel"
	case CmdSetChannel:
		return "set channel"
	case CmdSetDefaultFloodScope:
		return "set default scope"
	case CmdGetDefaultFloodScope:
		return "get default scope"
	}
	return fmt.Sprintf("command %d", c)
}
