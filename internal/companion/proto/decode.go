// Reading what a companion says back.
package proto

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

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
