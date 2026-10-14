package proto_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/proto"
)

// The two halves of a direct message's receipt, laid out as MeshCore's
// companion_radio writes them: RESP_CODE_SENT is the flood flag, the expected
// ack and the timeout; PUSH_CODE_SEND_CONFIRMED is the ack and the round trip.
func TestSentAndConfirmedDecode(t *testing.T) {
	sent := []byte{byte(proto.RespSent), 1}
	sent = binary.LittleEndian.AppendUint32(sent, 0xA1B2C3D4)
	sent = binary.LittleEndian.AppendUint32(sent, 12000)
	f, err := proto.Decode(sent)
	if err != nil || f.Sent == nil {
		t.Fatalf("RESP_CODE_SENT did not decode: %v %+v", err, f)
	}
	if !f.Sent.Flood || f.Sent.Ack != 0xA1B2C3D4 || f.Sent.TimeoutMs != 12000 {
		t.Fatalf("sent = %+v", *f.Sent)
	}

	conf := []byte{byte(proto.PushSendConfirmed)}
	conf = binary.LittleEndian.AppendUint32(conf, 0xA1B2C3D4)
	conf = binary.LittleEndian.AppendUint32(conf, 2345)
	f, err = proto.Decode(conf)
	if err != nil || f.Confirmed == nil {
		t.Fatalf("PUSH_CODE_SEND_CONFIRMED did not decode: %v %+v", err, f)
	}
	if f.Confirmed.Ack != 0xA1B2C3D4 || f.Confirmed.RoundTripMs != 2345 {
		t.Fatalf("confirmed = %+v", *f.Confirmed)
	}
}

// A login or status answer names who answered by the six bytes after the
// leading byte, and a status reply carries the rest as the status itself.
func TestAnswersNameWhoAnswered(t *testing.T) {
	from := []byte{1, 2, 3, 4, 5, 6}
	frame := append(append([]byte{byte(proto.PushStatusResp), 0}, from...), 9, 9, 9)
	f, err := proto.Decode(frame)
	if err != nil || f.Answer == nil {
		t.Fatalf("status response did not decode: %v %+v", err, f)
	}
	if !bytes.Equal(f.Answer.From, from) || len(f.Answer.Status) != 3 {
		t.Fatalf("answer = %+v", *f.Answer)
	}
	frame = append(append([]byte{byte(proto.PushLoginFail), 0}, from...))
	if f, _ = proto.Decode(frame); f.Answer == nil || f.Answer.Push != proto.PushLoginFail {
		t.Fatalf("login refusal did not decode: %+v", f)
	}
}

// The request commands carry the whole key, which is what the firmware looks
// the contact up by.
func TestRequestLayouts(t *testing.T) {
	key := bytes.Repeat([]byte{0xAB}, 32)
	l := proto.Login(key, "hunter2")
	if proto.Command(l[0]) != proto.CmdSendLogin || !bytes.Equal(l[1:33], key) || string(l[33:]) != "hunter2" {
		t.Fatalf("login = %x", l)
	}
	s := proto.StatusRequest(key)
	if proto.Command(s[0]) != proto.CmdSendStatusReq || len(s) != 33 {
		t.Fatalf("status request = %x", s)
	}
}
