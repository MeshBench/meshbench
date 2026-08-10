package proto_test

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/A13xB0/meshcoresim/internal/companion/proto"
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
	b = binary.LittleEndian.AppendUint32(b, 869618)
	b = binary.LittleEndian.AppendUint32(b, 62)
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
	if si.TxPowerDBm != 22 || si.SF != 8 || si.CR != 8 || si.FreqKHz != 869618 {
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
	b := []byte{byte(8), 24, 7} // channel msg, SNR 6 dB, channel 7
	b = binary.LittleEndian.AppendUint32(b, uint32(time.Now().Unix()))
	b = append(b, "GM5JFC: testing"...)
	f, err := proto.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.Message == nil || !f.Message.Channel {
		t.Fatal("not decoded as a channel message")
	}
	if f.Message.ChannelIdx != 7 || f.Message.Text != "GM5JFC: testing" {
		t.Fatalf("message = %+v", f.Message)
	}
	if f.Message.SNRdB != 6 {
		t.Fatalf("snr = %v, want 6 (quarter-dB units)", f.Message.SNRdB)
	}
}
