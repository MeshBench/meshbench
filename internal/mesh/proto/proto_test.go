package proto_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/proto"
)

// The channel send is the frame the whole tab exists to produce, and its
// layout is exactly what MyMesh.cpp reads back: type, index, timestamp, text.
func TestSendChannelTextLayout(t *testing.T) {
	at := time.Unix(1786345204, 0)
	f := proto.SendChannelText(7, at, "hello #sco")
	if proto.Command(f[0]) != proto.CmdSendChannelTxtMsg {
		t.Fatalf("command = %d", f[0])
	}
	if f[1] != 0 {
		t.Fatalf("txt type = %d, want plain (0) - anything else is refused", f[1])
	}
	if f[2] != 7 {
		t.Fatalf("channel index = %d, want 7", f[2])
	}
	if ts := binary.LittleEndian.Uint32(f[3:7]); ts != uint32(at.Unix()) {
		t.Fatalf("timestamp = %d", ts)
	}
	if string(f[7:]) != "hello #sco" {
		t.Fatalf("text = %q", f[7:])
	}
}

func TestSetRadioParamsLayout(t *testing.T) {
	f := proto.SetRadioParams(869618, 62, 8, 8)
	if proto.Command(f[0]) != proto.CmdSetRadioParams {
		t.Fatalf("command = %d", f[0])
	}
	if binary.LittleEndian.Uint32(f[1:5]) != 869618 {
		t.Fatal("frequency wrong")
	}
	if binary.LittleEndian.Uint32(f[5:9]) != 62 {
		t.Fatal("bandwidth wrong")
	}
	if f[9] != 8 || f[10] != 8 {
		t.Fatalf("sf/cr = %d/%d", f[9], f[10])
	}
}

func TestDecodeSelfInfo(t *testing.T) {
	b := make([]byte, 0, 64)
	b = append(b, byte(5))   // RESP_CODE_SELF_INFO
	b = append(b, 1, 22, 30) // adv type, tx power, max tx power
	key := make([]byte, 32)
	key[0], key[31] = 0xAB, 0xCD
	b = append(b, key...)
	b = binary.LittleEndian.AppendUint32(b, uint32(int32(56747200)))
	lon := int32(-3741100)
	b = binary.LittleEndian.AppendUint32(b, uint32(lon))
	b = append(b, 0, 0, 0, 0) // multi_acks, loc policy, telemetry, manual add
	// As the firmware sends them: freq = _prefs.freq(MHz) * 1000, so kHz;
	// bw = _prefs.bw(kHz) * 1000, so Hz. Two fields, two units.
	b = binary.LittleEndian.AppendUint32(b, 869618)
	b = binary.LittleEndian.AppendUint32(b, 62*1000)
	b = append(b, 8, 8)
	b = append(b, "Dunkeld Companion"...)

	f, err := proto.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	si := f.SelfInfo
	if si == nil {
		t.Fatal("no self info decoded")
	}
	if si.Name != "Dunkeld Companion" {
		t.Fatalf("name = %q", si.Name)
	}
	if si.TxPowerDBm != 22 || si.SF != 8 || si.CR != 8 || si.FreqKHz != 869618 || si.BWKHz != 62 {
		t.Fatalf("radio = %+v", si)
	}
	if int(si.AdvLat*1000) != 56747 {
		t.Fatalf("lat = %f", si.AdvLat)
	}
	if len(si.PublicKey) != 32 || si.PublicKey[31] != 0xCD {
		t.Fatalf("key = %x", si.PublicKey)
	}
}

// An unfamiliar response must survive: the firmware gains codes faster than
// any client tracks them, and rejecting one is how a client stops working
// the day the mesh is upgraded.
func TestUnknownResponseIsKept(t *testing.T) {
	f, err := proto.Decode([]byte{0x7E, 1, 2, 3})
	if err != nil {
		t.Fatalf("unknown code rejected: %v", err)
	}
	if len(f.Raw) != 4 {
		t.Fatal("raw frame not kept")
	}
}

func TestDecodeChannelMessage(t *testing.T) {
	// Verbatim shape of a real frame: code, channel, path len, text type,
	// timestamp, text. No SNR byte - the pre-v3 frame has none.
	b := []byte{8, 7, 2, 0}
	b = binary.LittleEndian.AppendUint32(b, uint32(time.Now().Unix()))
	b = append(b, "GM5JFC: testing"...)
	f, err := proto.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.Message == nil || !f.Message.Channel {
		t.Fatal("not decoded as a channel message")
	}
	if f.Message.ChannelIdx != 7 || f.Message.SenderName != "GM5JFC" || f.Message.Text != "testing" {
		t.Fatalf("message = %+v", f.Message)
	}
	if f.Message.PathLen != 2 {
		t.Fatalf("path = %d, want 2", f.Message.PathLen)
	}
}

// The v3 channel frame carries two reserved bytes, a path length and a text
// type that the older one does not. Decoding it with the older layout starts
// the text six bytes early, which is how received messages arrived with
// fragments of their own header stuck to the sender's name.
func TestDecodeChannelMessageV3(t *testing.T) {
	b := []byte{17, 24, 0, 0, 7, 3, 0} // v3: SNR 6 dB, reserved x2, channel 7, 3 hops, plain
	b = binary.LittleEndian.AppendUint32(b, uint32(time.Now().Unix()))
	b = append(b, "GM5JFC: testing"...)
	f, err := proto.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	m := f.Message
	if m == nil || !m.Channel {
		t.Fatal("not decoded as a channel message")
	}
	if m.ChannelIdx != 7 || m.SenderName != "GM5JFC" || m.Text != "testing" {
		t.Fatalf("message = %+v", m)
	}
	if m.PathLen != 3 || m.SNRdB != 6 {
		t.Fatalf("path/snr = %+v", m)
	}
}

// 0xFF is the firmware's "this did not arrive by flood", not a 255-hop path.
func TestV3DirectMessageHasNoPath(t *testing.T) {
	b := []byte{17, 0, 0, 0, 1, 0xFF, 0}
	b = binary.LittleEndian.AppendUint32(b, uint32(time.Now().Unix()))
	b = append(b, "x: y"...)
	f, err := proto.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.Message.PathLen != -1 {
		t.Fatalf("path = %d, want -1 for a non-flood message", f.Message.PathLen)
	}
}

// Packet::path_len is not a bare hop count: the top two bits are the path
// hash size minus one and only the bottom six are hops (Packet.h,
// getPathHashCount()). 0x83 is three hops with a three-byte hash, and reading
// the whole byte showed a received message as 131 hops.
func TestDecodeChannelMessagePathLenIsPacked(t *testing.T) {
	b := []byte{8, 0, 0x83, 0}
	b = binary.LittleEndian.AppendUint32(b, uint32(time.Now().Unix()))
	b = append(b, "x: y"...)
	f, err := proto.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.Message.PathLen != 3 {
		t.Fatalf("path = %d, want 3 hops out of a 0x83 path_len byte", f.Message.PathLen)
	}
}

// The contact record carries a 64-byte out_path between the path length and
// the name. Reading the name straight after the length lands in the middle of
// the path, and every contact comes back nameless.
func TestDecodeContactName(t *testing.T) {
	b := []byte{3}
	b = append(b, bytes.Repeat([]byte{0xAB}, 32)...) // pub key
	b = append(b, 1, 0, 3)                           // type, flags, out_path_len
	b = append(b, bytes.Repeat([]byte{0x07}, 64)...) // out_path
	name := make([]byte, 32)
	copy(name, "Ben Vrackie")
	b = append(b, name...)
	b = binary.LittleEndian.AppendUint32(b, 1767225600)
	b = binary.LittleEndian.AppendUint32(b, uint32(int32(56747200)))
	lon := int32(-3741100)
	b = binary.LittleEndian.AppendUint32(b, uint32(lon))
	b = binary.LittleEndian.AppendUint32(b, 0)

	f, err := proto.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	c := f.Contact
	if c == nil {
		t.Fatal("no contact decoded")
	}
	if c.Name != "Ben Vrackie" {
		t.Fatalf("name = %q", c.Name)
	}
	if c.OutPathLen != 3 || int(c.Lat*1000) != 56747 {
		t.Fatalf("contact = %+v", c)
	}
}

// The device query is the only frame that says what path hash size a node
// will actually use, so a client that does not decode it can only show what
// it last asked for.
func TestDeviceInfoCarriesThePathHashMode(t *testing.T) {
	body := make([]byte, 82)
	body[0] = 10 // firmware version code
	copy(body[60:80], "v1.17.0")
	body[80] = 0 // repeat disabled
	body[81] = 2 // three-byte hashes
	f, err := proto.Decode(append([]byte{byte(proto.RespDeviceInfo)}, body...))
	if err != nil {
		t.Fatal(err)
	}
	if f.Device == nil {
		t.Fatal("the device query's reply was not decoded")
	}
	if !f.Device.ModeKnown || f.Device.PathHashMode != 2 {
		t.Errorf("mode %d known=%v, wanted 2 and true", f.Device.PathHashMode, f.Device.ModeKnown)
	}
	if proto.PathHashBytes(f.Device.PathHashMode) != 3 {
		t.Errorf("mode 2 is %d bytes, wanted 3", proto.PathHashBytes(f.Device.PathHashMode))
	}
	if f.Device.Version != "v1.17.0" {
		t.Errorf("version %q", f.Device.Version)
	}
}

// Firmware older than v10 stops before the mode, and a short frame is an old
// node rather than a broken one.
func TestOlderFirmwareSaysNothingAboutTheMode(t *testing.T) {
	f, err := proto.Decode(append([]byte{byte(proto.RespDeviceInfo)}, make([]byte, 81)...))
	if err != nil {
		t.Fatal(err)
	}
	if f.Device == nil || f.Device.ModeKnown {
		t.Errorf("a frame that stops before the mode must not claim to know it")
	}
}

// Self info needs 35 bytes before the tx power ceiling and the public key are
// both in. A short reply must be reported, not silently parsed with the key
// running off the end of the frame.
func TestDecodeSelfInfoTooShort(t *testing.T) {
	f, err := proto.Decode([]byte{byte(proto.RespSelfInfo), 1, 22, 30})
	if err == nil {
		t.Fatalf("a 3-byte self info body decoded as %+v, want an error", f.SelfInfo)
	}
}

// The contact record's name sits past a 64-byte out_path; anything shorter
// than that must be reported rather than read as a contact with a name taken
// from the middle of the path.
func TestDecodeContactTooShort(t *testing.T) {
	b := []byte{byte(proto.RespContact)}
	b = append(b, bytes.Repeat([]byte{0xAB}, 32)...) // pub key
	b = append(b, 1, 0, 3)                           // type, flags, out_path_len
	b = append(b, bytes.Repeat([]byte{0x07}, 60)...) // out_path, four bytes short
	f, err := proto.Decode(b)
	if err == nil {
		t.Fatalf("a body 4 bytes short of the name decoded as %+v, want an error", f.Contact)
	}
}

// A message body under two bytes has no channel index or sender key at all;
// decodeMessage must say so rather than reading past the end.
func TestDecodeMessageTooShort(t *testing.T) {
	f, err := proto.Decode([]byte{byte(proto.RespChannelMsgRecv), 7})
	if err == nil {
		t.Fatalf("a 1-byte message body decoded as %+v, want an error", f.Message)
	}
}
