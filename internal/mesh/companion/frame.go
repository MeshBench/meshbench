package companion

import "encoding/binary"

// Frame wraps a payload the way a companion device expects it.
//
// '<' towards the node, a little-endian length, then the payload. Sending the
// payload bare is not a malformed frame, it is console text: the firmware reads
// it as somebody typing and answers nothing, which is what an experiment
// measuring zero transmissions looked like, and what a fixture whose schedule
// aimed a message at a companion looked like after that.
//
// Here rather than beside one caller because two programs now put this envelope
// on a payload - the workbench when somebody types at a companion, and the
// headless fixture runner when a schedule says something. Serial deliberately
// does not frame anything, and still does not: this is what a caller wraps a
// payload in before handing it to the transport, not something the transport
// does on the way past.
func Frame(payload []byte) []byte {
	out := make([]byte, 0, 3+len(payload))
	out = append(out, '<')
	out = binary.LittleEndian.AppendUint16(out, uint16(len(payload)))
	return append(out, payload...)
}
