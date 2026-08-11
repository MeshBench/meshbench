package capture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// PseudoHeaderVersion is carried in every packet and MUST be incremented on any
// layout change. Captures outlive the code that wrote them, so a reader has to
// be able to tell which layout it is looking at.
const PseudoHeaderVersion = 1

// linkTypeUser0 is the DLT_USER0 range, reserved for private link types. The
// dissector registers against this value.
const linkTypeUser0 = 147

// PseudoHeader carries what only the simulator knows, prefixed to every frame.
//
// The receiving node is in here deliberately: the same transmission is a
// different capture at every receiver, and a packet node A heard while node B
// did not is the most informative event in a mesh.
type PseudoHeader struct {
	Version  uint8
	Outcome  uint8 // index into outcomeCodes
	FromNode uint16
	ToNode   uint16
	RSSIdBm  int16 // dBm × 10
	SNRdB    int16 // dB × 10
	FreqHz   uint32
	SF       uint8
	BWkHz    uint16
	CR       uint8
	CRCOK    uint8
}

var outcomeCodes = map[Outcome]uint8{
	OutOfRange: 0, NotDemodulated: 1, CRCFailed: 2,
	DroppedByFirmware: 3, Accepted: 4, Relayed: 5,
}

// OutcomeCode maps an outcome to its wire value.
func OutcomeCode(o Outcome) uint8 { return outcomeCodes[o] }

func (h PseudoHeader) encode() []byte {
	// Writes to a bytes.Buffer cannot fail, so the errors are dropped rather
	// than checked five times over — the alternative buries the layout, which
	// is the only thing worth reading here.
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, h.Version)
	_ = binary.Write(&b, binary.LittleEndian, h.Outcome)
	_ = binary.Write(&b, binary.LittleEndian, h.FromNode)
	_ = binary.Write(&b, binary.LittleEndian, h.ToNode)
	_ = binary.Write(&b, binary.LittleEndian, h.RSSIdBm)
	_ = binary.Write(&b, binary.LittleEndian, h.SNRdB)
	_ = binary.Write(&b, binary.LittleEndian, h.FreqHz)
	_ = binary.Write(&b, binary.LittleEndian, h.SF)
	_ = binary.Write(&b, binary.LittleEndian, h.BWkHz)
	_ = binary.Write(&b, binary.LittleEndian, h.CR)
	_ = binary.Write(&b, binary.LittleEndian, h.CRCOK)
	return b.Bytes()
}

// PcapngWriter writes a merged capture: every receiver's view of every frame in
// one file, distinguished by the pseudo-header's ToNode.
type PcapngWriter struct {
	w io.Writer
}

func NewPcapngWriter(w io.Writer) (*PcapngWriter, error) {
	p := &PcapngWriter{w: w}
	if err := p.writeSectionHeader(); err != nil {
		return nil, err
	}
	return p, p.writeInterfaceDescription()
}

func (p *PcapngWriter) writeSectionHeader() error {
	body := new(bytes.Buffer)
	_ = binary.Write(body, binary.LittleEndian, uint32(0x1A2B3C4D)) // byte-order magic
	_ = binary.Write(body, binary.LittleEndian, uint16(1))          // major
	_ = binary.Write(body, binary.LittleEndian, uint16(0))          // minor
	_ = binary.Write(body, binary.LittleEndian, int64(-1))          // section length unknown
	return p.block(0x0A0D0D0A, body.Bytes())
}

func (p *PcapngWriter) writeInterfaceDescription() error {
	body := new(bytes.Buffer)
	_ = binary.Write(body, binary.LittleEndian, uint16(linkTypeUser0))
	_ = binary.Write(body, binary.LittleEndian, uint16(0))     // reserved
	_ = binary.Write(body, binary.LittleEndian, uint32(65535)) // snaplen
	// if_tsresol = 9 → nanosecond timestamps, which the simulator has and a
	// microsecond default would silently truncate.
	_ = binary.Write(body, binary.LittleEndian, uint16(9))
	_ = binary.Write(body, binary.LittleEndian, uint16(1))
	body.WriteByte(9)
	body.Write([]byte{0, 0, 0})
	_ = binary.Write(body, binary.LittleEndian, uint32(0)) // opt_endofopt
	return p.block(0x00000001, body.Bytes())
}

// WritePacket appends one receiver's view of one frame.
func (p *PcapngWriter) WritePacket(ts uint64, h PseudoHeader, frame []byte) error {
	h.Version = PseudoHeaderVersion
	payload := append(h.encode(), frame...)

	body := new(bytes.Buffer)
	_ = binary.Write(body, binary.LittleEndian, uint32(0)) // interface id
	_ = binary.Write(body, binary.LittleEndian, uint32(ts>>32))
	_ = binary.Write(body, binary.LittleEndian, uint32(ts&0xFFFFFFFF))
	_ = binary.Write(body, binary.LittleEndian, uint32(len(payload)))
	_ = binary.Write(body, binary.LittleEndian, uint32(len(payload)))
	body.Write(payload)
	for body.Len()%4 != 0 {
		body.WriteByte(0)
	}
	return p.block(0x00000006, body.Bytes())
}

// block writes a pcapng block with its total-length fields.
func (p *PcapngWriter) block(blockType uint32, body []byte) error {
	total := uint32(len(body) + 12)
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, blockType)
	_ = binary.Write(&b, binary.LittleEndian, total)
	b.Write(body)
	_ = binary.Write(&b, binary.LittleEndian, total)
	if _, err := p.w.Write(b.Bytes()); err != nil {
		return fmt.Errorf("pcapng: %w", err)
	}
	return nil
}
