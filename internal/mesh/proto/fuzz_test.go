package proto_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/proto"
)

// FuzzDecode throws arbitrary bytes at the companion protocol codec, which
// reads whatever the firmware on the other end of the TCP or PTY transport
// chooses to send. Every RespXxx branch in Decode has its own hand-rolled
// offsets; a short or adversarial frame must come back as an error, never a
// panic in the goroutine reading the transport.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{0x7E, 1, 2, 3}) // unknown code, kept rather than rejected
	f.Add([]byte{byte(proto.RespErr), 6})

	selfInfo := make([]byte, 0, 64)
	selfInfo = append(selfInfo, byte(proto.RespSelfInfo))
	selfInfo = append(selfInfo, 1, 22, 30)
	selfInfo = append(selfInfo, make([]byte, 32)...)
	var lat, lon int32 = 56747200, -3741100
	selfInfo = binary.LittleEndian.AppendUint32(selfInfo, uint32(lat))
	selfInfo = binary.LittleEndian.AppendUint32(selfInfo, uint32(lon))
	selfInfo = append(selfInfo, 0, 0, 0, 0)
	selfInfo = binary.LittleEndian.AppendUint32(selfInfo, 869618)
	selfInfo = binary.LittleEndian.AppendUint32(selfInfo, 62*1000)
	selfInfo = append(selfInfo, 8, 8)
	selfInfo = append(selfInfo, "Dunkeld Companion"...)
	f.Add(selfInfo)
	f.Add(selfInfo[:34]) // one byte short of decodeSelfInfo's floor

	contact := []byte{byte(proto.RespContact)}
	contact = append(contact, bytes.Repeat([]byte{0xAB}, 32)...)
	contact = append(contact, 1, 0, 3)
	contact = append(contact, bytes.Repeat([]byte{0x07}, 64)...)
	name := make([]byte, 32)
	copy(name, "Ben Vrackie")
	contact = append(contact, name...)
	contact = binary.LittleEndian.AppendUint32(contact, 1767225600)
	contact = binary.LittleEndian.AppendUint32(contact, uint32(lat))
	contact = binary.LittleEndian.AppendUint32(contact, uint32(lon))
	contact = binary.LittleEndian.AppendUint32(contact, 0)
	f.Add(contact)
	f.Add(contact[:130]) // one byte short of the name

	channelMsg := []byte{byte(proto.RespChannelMsgRecv), 7, 2, 0}
	channelMsg = binary.LittleEndian.AppendUint32(channelMsg, uint32(time.Now().Unix()))
	channelMsg = append(channelMsg, "GM5JFC: testing"...)
	f.Add(channelMsg)
	f.Add(channelMsg[:2]) // one byte short of decodeMessage's floor

	v3Msg := []byte{byte(proto.RespChannelMsgRecvV3), 24, 0, 0, 7, 3, 0}
	v3Msg = binary.LittleEndian.AppendUint32(v3Msg, uint32(time.Now().Unix()))
	v3Msg = append(v3Msg, "GM5JFC: testing"...)
	f.Add(v3Msg)

	deviceInfo := append([]byte{byte(proto.RespDeviceInfo)}, make([]byte, 82)...)
	deviceInfo[1] = 10
	deviceInfo[81] = 2
	f.Add(deviceInfo)

	channelInfo := append([]byte{byte(proto.RespChannelInfo), 3}, make([]byte, 33+16)...)
	f.Add(channelInfo)

	scope := append([]byte{byte(proto.RespDefaultFloodScope)}, make([]byte, 31+16)...)
	copy(scope[1:], "corescope")
	f.Add(scope)
	f.Add([]byte{byte(proto.RespDefaultFloodScope)})

	f.Add([]byte{})
	f.Add([]byte{0x80}) // push code, no body

	f.Fuzz(func(t *testing.T, frame []byte) {
		_, _ = proto.Decode(frame)
	})
}

// FuzzDecodeDefaultScope covers the one decoder Decode calls on the whole
// frame rather than the body, which makes it easy to get its offsets one
// byte out of step with the others.
func FuzzDecodeDefaultScope(f *testing.F) {
	full := append([]byte{byte(proto.RespDefaultFloodScope)}, make([]byte, 31+16)...)
	copy(full[1:], "corescope")
	f.Add(full)
	f.Add([]byte{byte(proto.RespDefaultFloodScope)})
	f.Add(full[:30])
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, b []byte) {
		proto.DecodeDefaultScope(b)
	})
}
