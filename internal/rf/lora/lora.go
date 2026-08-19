// Package lora is the LoRa coding chain: everything between payload bytes
// and chirp symbols that is not modulation.
//
// Whitening, Hamming coding, diagonal interleaving, Gray mapping, the
// explicit header and the payload CRC - the layers a real SX126x applies and
// the simulator previously skipped. It deliberately imports nothing from
// MeshBench: the package is rules about bits, reusable by the simulator, by
// SDR tooling and by anything else that speaks LoRa, and extractable to its
// own repository without surgery (plan decision D1).
//
// Bit-level conventions follow the published reverse-engineering literature
// (EPFL's gr-lora_sdr, and Xu et al., ACM ToSN 2023). Where implementations
// in the wild disagree on a convention - whitening sequence phase, header
// checksum equations, the sync-offset on symbol values - this package
// documents its choice at the site and the golden-vector suite exists to
// catch any that real hardware contradicts. Round-trip self-consistency and
// the structural properties (diagonality, coding distance, symbol counts
// matching RadioLib's airtime formula exactly) are tested locally.
package lora

import "fmt"

// Params is one transmission's coding configuration.
type Params struct {
	// SF is the spreading factor, 7..12. SF5/6 use a different frame
	// arrangement on the SX126x and are out of scope, as the plan states.
	SF int
	// CR is the coding-rate offset, 1..4 for 4/5..4/8 - RadioLib's own unit.
	CR int
	// LDRO is low-data-rate optimisation: two fewer bits per symbol in every
	// payload block. The chip enables it itself once a symbol reaches 16 ms;
	// the caller decides, so this package stays free of bandwidth.
	LDRO bool
	// CRC appends and checks the 16-bit payload CRC.
	CRC bool
}

func (p Params) check() error {
	if p.SF < 7 || p.SF > 12 {
		return fmt.Errorf("lora: SF%d is outside 7..12", p.SF)
	}
	if p.CR < 1 || p.CR > 4 {
		return fmt.Errorf("lora: CR offset %d is outside 1..4", p.CR)
	}
	return nil
}

// headerPPM is the bits-per-symbol of the first block, always SF-2: the
// header travels at reduced rate whatever LDRO says, which is why RadioLib's
// airtime formula carries its lone +8 for the block and -4*SF+8 in the bits.
func (p Params) headerPPM() int { return p.SF - 2 }

// dataPPM is the bits-per-symbol of every later block.
func (p Params) dataPPM() int {
	if p.LDRO {
		return p.SF - 2
	}
	return p.SF
}

// Encode turns payload bytes into chirp-shift symbols: header, whitening,
// CRC, Hamming, interleaving, Gray - the full transmit side.
//
// The count of returned symbols equals RadioLib's payload-symbol arithmetic
// exactly (8 + blocks*(CR+4)); a test holds the two to each other, because
// the firmware's CSMA is built on that number and a channel that occupies
// the air for a different time desynchronises silently.
func Encode(p Params, payload []byte) ([]int, error) {
	if err := p.check(); err != nil {
		return nil, err
	}
	if len(payload) > 255 {
		return nil, fmt.Errorf("lora: %d bytes exceeds the 255-byte payload field", len(payload))
	}

	// Whitening covers the payload only - the header and CRC travel clear,
	// which is what lets a receiver read the length before dewhitening.
	white := make([]byte, len(payload))
	copy(white, payload)
	Whiten(white)

	nibbles := headerNibbles(len(payload), p.CR, p.CRC)
	nibbles = append(nibbles, bytesToNibbles(white)...)
	if p.CRC {
		crc := CRC16(payload)
		nibbles = append(nibbles, byte(crc&0xF), byte(crc>>4&0xF),
			byte(crc>>8&0xF), byte(crc>>12&0xF))
	}

	var syms []int
	// The header block: SF-2 nibbles at CR 4/8, always. Short payloads fit
	// entirely inside it, which is why a 1-byte SF12 frame is 8 symbols.
	hn := p.headerPPM()
	block := padNibbles(nibbles, hn)
	syms = append(syms, encodeBlock(block[:hn], 4, hn)...)
	rest := nibbles[min(hn, len(nibbles)):]

	dn := p.dataPPM()
	for len(rest) > 0 {
		block = padNibbles(rest, dn)
		syms = append(syms, encodeBlock(block[:dn], p.CR, dn)...)
		rest = rest[min(dn, len(rest)):]
	}

	// Reduced-rate symbols occupy the top bits of the chirp shift: the two
	// (or more) dropped bits are the fine-grained ones timing error would
	// smear, which is the entire point of the reduced rate.
	shift := make([]int, len(syms))
	i := 0
	for ; i < 8 && i < len(shift); i++ {
		shift[i] = syms[i] << (p.SF - hn)
	}
	for ; i < len(shift); i++ {
		shift[i] = syms[i] << (p.SF - dn)
	}
	return shift, nil
}

// DecodeStats is what the receive side can say about how the decode went -
// telemetry for the ledger, never the verdict.
type DecodeStats struct {
	HeaderOK  bool
	Corrected int // codewords repaired by FEC
	Failed    int // codewords FEC could not repair or only detect as bad
	CRCOK     bool
	Length    int
}

// Decode is the receive side: Gray, deinterleave, Hamming, dewhiten, CRC.
// ok is true only for a fully valid frame - a valid header, every codeword
// decodable, and (when enabled) a matching CRC. Only that may reach MeshCore.
func Decode(p Params, shifts []int) (payload []byte, ok bool, stats DecodeStats) {
	if p.check() != nil || len(shifts) < 8 {
		return nil, false, stats
	}
	hn, dn := p.headerPPM(), p.dataPPM()

	hdrRaw := make([]int, 8)
	for i := 0; i < 8; i++ {
		hdrRaw[i] = shifts[i] >> (p.SF - hn)
	}
	hdrNibbles, corrected, failed := decodeBlock(hdrRaw, 4, hn)
	stats.Corrected += corrected
	stats.Failed += failed
	length, cr, hasCRC, hdrOK := parseHeader(hdrNibbles)
	stats.HeaderOK = hdrOK && failed == 0
	if !stats.HeaderOK {
		return nil, false, stats
	}
	// The header names the frame's own coding; trusting the caller's Params
	// over the wire would decode a frame the transmitter did not send.
	if cr != p.CR || hasCRC != p.CRC {
		// Not an error in this simulator - the engine always agrees with
		// itself - but a real capture may not, and the header is the truth.
		p.CR, p.CRC = cr, hasCRC
		dn = p.dataPPM()
	}
	stats.Length = length

	// Consume only the frame the header declares: a streaming receiver
	// hands this decoder a window, and whatever trails the frame is the
	// channel's business, not damage to this packet.
	target := length * 2
	if hasCRC {
		target += 4
	}
	nibbles := append([]byte(nil), hdrNibbles[headerNibbleCount:]...)
	at := 8
	for at+cr+4 <= len(shifts) && len(nibbles) < target {
		raw := make([]int, cr+4)
		for i := range raw {
			raw[i] = shifts[at+i] >> (p.SF - dn)
		}
		blk, corr, fail := decodeBlock(raw, cr, dn)
		stats.Corrected += corr
		stats.Failed += fail
		nibbles = append(nibbles, blk...)
		at += cr + 4
	}

	need := length
	crcNib := 0
	if hasCRC {
		crcNib = 4
	}
	if len(nibbles) < need*2+crcNib {
		return nil, false, stats
	}
	payload = nibblesToBytes(nibbles[:need*2])
	Whiten(payload) // whitening is an XOR stream: applying it again removes it

	if hasCRC {
		got := uint16(nibbles[need*2]) | uint16(nibbles[need*2+1])<<4 |
			uint16(nibbles[need*2+2])<<8 | uint16(nibbles[need*2+3])<<12
		stats.CRCOK = got == CRC16(payload)
	} else {
		stats.CRCOK = true
	}
	ok = stats.Failed == 0 && stats.CRCOK
	return payload, ok, stats
}

// SymbolCount is how many data symbols Encode will produce, without encoding.
func SymbolCount(p Params, payloadLen int) (int, error) {
	if err := p.check(); err != nil {
		return 0, err
	}
	nibbles := headerNibbleCount + payloadLen*2
	if p.CRC {
		nibbles += 4
	}
	rest := nibbles - p.headerPPM()
	if rest < 0 {
		rest = 0
	}
	blocks := (rest + p.dataPPM() - 1) / p.dataPPM()
	return 8 + blocks*(p.CR+4), nil
}

func bytesToNibbles(b []byte) []byte {
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, v&0xF, v>>4)
	}
	return out
}

func nibblesToBytes(n []byte) []byte {
	out := make([]byte, len(n)/2)
	for i := range out {
		out[i] = n[i*2]&0xF | n[i*2+1]<<4
	}
	return out
}

func padNibbles(n []byte, to int) []byte {
	if len(n) >= to {
		return n
	}
	out := make([]byte, to)
	copy(out, n)
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
