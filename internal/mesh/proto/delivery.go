// Whether a direct message got there.
//
// A channel message has no receipt: it is flooded, and nobody owes the sender
// an answer. A direct message does. The firmware answers the send with
// RESP_CODE_SENT, carrying the acknowledgement code it expects back and how
// long it will wait, then pushes PUSH_CODE_SEND_CONFIRMED with that same code
// and the round trip once the recipient's ack arrives. Matching the two is the
// only way a client can say "delivered" rather than "sent".
package proto

import "encoding/binary"

// Sent is the firmware's answer to a direct send (RESP_CODE_SENT).
type Sent struct {
	// Flood is true when the firmware had no path to the contact and flooded
	// the message, false when it went direct along a known path.
	Flood bool
	// Ack is the acknowledgement code the recipient will send back.
	Ack uint32
	// TimeoutMs is how long the firmware will wait for that ack.
	TimeoutMs uint32
}

// Confirmed is the push that says an ack arrived (PUSH_CODE_SEND_CONFIRMED).
type Confirmed struct {
	Ack         uint32
	RoundTripMs uint32
}

// decodeSent reads RESP_CODE_SENT's body. MeshCore's MyMesh.cpp writes the
// flood flag, then the expected ack, then the estimated timeout, each ack and
// timeout four bytes little-endian.
func decodeSent(b []byte) *Sent {
	if len(b) < 5 {
		return nil
	}
	s := &Sent{Flood: b[0] == 1, Ack: binary.LittleEndian.Uint32(b[1:5])}
	if len(b) >= 9 {
		s.TimeoutMs = binary.LittleEndian.Uint32(b[5:9])
	}
	return s
}

// decodeConfirmed reads PUSH_CODE_SEND_CONFIRMED's body: the ack code, then
// the round trip in milliseconds.
func decodeConfirmed(b []byte) *Confirmed {
	if len(b) < 4 {
		return nil
	}
	c := &Confirmed{Ack: binary.LittleEndian.Uint32(b[0:4])}
	if len(b) >= 8 {
		c.RoundTripMs = binary.LittleEndian.Uint32(b[4:8])
	}
	return c
}

// Answer is a repeater or room server replying to a login or a status
// request: PUSH_CODE_LOGIN_SUCCESS, PUSH_CODE_LOGIN_FAIL or
// PUSH_CODE_STATUS_RESPONSE, each led by a reserved or permissions byte and
// the six-byte key prefix of whoever answered.
type Answer struct {
	Push   Push
	From   []byte
	Status []byte
}

func decodeAnswer(p Push, b []byte) *Answer {
	if len(b) < 7 {
		return nil
	}
	a := &Answer{Push: p, From: b[1:7]}
	if p == PushStatusResp {
		a.Status = b[7:]
	}
	return a
}

// Login sends CMD_SEND_LOGIN: the whole key, then the password. The firmware
// looks the key up among its contacts, so the recipient has to be one.
func Login(pubKey []byte, password string) []byte {
	b := append([]byte{byte(CmdSendLogin)}, pubKey...)
	return append(b, password...)
}

// StatusRequest sends CMD_SEND_STATUS_REQ to a contact, by its whole key.
func StatusRequest(pubKey []byte) []byte {
	return append([]byte{byte(CmdSendStatusReq)}, pubKey...)
}

// The two request commands, numbered as MeshCore's companion_radio numbers
// them.
const (
	CmdSendLogin     Command = 26
	CmdSendStatusReq Command = 27
)
