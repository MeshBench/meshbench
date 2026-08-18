// The individual coding layers: Gray, whitening, Hamming, the diagonal
// interleaver, the explicit header and the payload CRC.
package lora

// Gray returns the Gray code of v - what a receiver applies to an FFT bin,
// so that the one-bin errors timing and frequency offsets cause become
// single-bit errors the Hamming layer can repair. That adjacency is the
// entire reason the layer exists.
func Gray(v int) int { return v ^ (v >> 1) }

// GrayInverse undoes Gray - the transmit side.
func GrayInverse(g int) int {
	v := 0
	for ; g != 0; g >>= 1 {
		v ^= g
	}
	return v
}

// Whiten XORs the LoRa whitening sequence over b in place. Its own inverse.
//
// The sequence is the LFSR x^8 + x^6 + x^5 + x^4 + 1 seeded 0xFF - the
// polynomial the reverse-engineering literature agrees on. Whitening exists
// so a run of identical payload bytes still produces symbol variety; without
// it a page of zeros is one chirp repeated, and every real receiver's timing
// tracking falls over.
func Whiten(b []byte) {
	state := byte(0xFF)
	for i := range b {
		b[i] ^= state
		next := ((state >> 7) ^ (state >> 5) ^ (state >> 4) ^ (state >> 3)) & 1
		state = state<<1 | next
	}
}

// hammingEncode expands a nibble to a 4+cr bit codeword.
//
// CR 4/7 and 4/8 are Hamming codes that correct one bit; 4/6 detects up to
// two and 4/5 one, correcting nothing - exactly the split the real chip has,
// and the reason 4/5 traffic dies where 4/8 traffic survives.
func hammingEncode(nibble byte, cr int) int {
	d0 := int(nibble) & 1
	d1 := int(nibble) >> 1 & 1
	d2 := int(nibble) >> 2 & 1
	d3 := int(nibble) >> 3 & 1
	p0 := d0 ^ d1 ^ d3 // the Hamming(7,4) parity triple
	p1 := d0 ^ d2 ^ d3
	p2 := d1 ^ d2 ^ d3
	p3 := d0 ^ d1 ^ d2 ^ d3 ^ p0 ^ p1 ^ p2 // overall parity, for 4/8's SECDED
	cw := int(nibble)
	switch cr {
	case 1:
		cw |= (d0 ^ d1 ^ d2 ^ d3) << 4
	case 2:
		cw |= p0<<4 | p1<<5
	case 3:
		cw |= p0<<4 | p1<<5 | p2<<6
	case 4:
		cw |= p0<<4 | p1<<5 | p2<<6 | p3<<7
	}
	return cw
}

// hammingDecode recovers the nibble. corrected reports a repaired single-bit
// error; bad reports damage this rate can only detect, which fails the frame.
func hammingDecode(cw, cr int) (nibble byte, corrected, bad bool) {
	d := byte(cw & 0xF)
	if cr <= 2 {
		// Detection-only rates: re-encode and compare.
		if hammingEncode(d, cr) != cw {
			return d, false, true
		}
		return d, false, false
	}
	syndrome := func(cw int) int {
		d0, d1, d2, d3 := cw&1, cw>>1&1, cw>>2&1, cw>>3&1
		s0 := d0 ^ d1 ^ d3 ^ (cw >> 4 & 1)
		s1 := d0 ^ d2 ^ d3 ^ (cw >> 5 & 1)
		s2 := d1 ^ d2 ^ d3 ^ (cw >> 6 & 1)
		return s0 | s1<<1 | s2<<2
	}
	s := syndrome(cw)
	if s == 0 {
		if cr == 4 {
			// Overall parity: a mismatch with a clean syndrome is a double
			// error - detectable, not repairable.
			if popcount(cw&0xFF)%2 != 0 {
				return d, false, true
			}
		}
		return d, false, false
	}
	// One bit is wrong; the syndrome says which of the seven it is.
	flip := map[int]int{
		0b011: 0, 0b101: 1, 0b110: 2, 0b111: 3, // data bits
		0b001: 4, 0b010: 5, 0b100: 6, // parity bits
	}[s]
	cw ^= 1 << flip
	if cr == 4 && popcount(cw&0xFF)%2 != 0 {
		return byte(cw & 0xF), false, true // correction left parity wrong: double error
	}
	return byte(cw & 0xF), true, false
}

func popcount(v int) int {
	n := 0
	for ; v != 0; v &= v - 1 {
		n++
	}
	return n
}

// encodeBlock turns ppm nibbles into cr+4 symbols of ppm bits: Hamming, then
// the diagonal interleaver, then inverse Gray so the wire carries values
// whose FFT-bin neighbours differ by one bit.
//
// The interleaver is the diagonal one the literature describes: bit r of
// symbol c is bit c of codeword (r+c) mod ppm. Its property - the reason it
// is worth having - is that one destroyed symbol costs every codeword in the
// block exactly one bit, which is precisely what the Hamming layer can
// repair. A test asserts that property rather than trusting this comment.
func encodeBlock(nibbles []byte, cr, ppm int) []int {
	cws := make([]int, ppm)
	for i, n := range nibbles {
		cws[i] = hammingEncode(n, cr)
	}
	syms := make([]int, cr+4)
	for c := 0; c < cr+4; c++ {
		v := 0
		for r := 0; r < ppm; r++ {
			bit := cws[(r+c)%ppm] >> c & 1
			v |= bit << r
		}
		syms[c] = GrayInverse(v)
	}
	return syms
}

// decodeBlock is encodeBlock's inverse, with damage accounting.
func decodeBlock(syms []int, cr, ppm int) (nibbles []byte, corrected, bad int) {
	cws := make([]int, ppm)
	for c := 0; c < len(syms); c++ {
		v := Gray(syms[c])
		for r := 0; r < ppm; r++ {
			bit := v >> r & 1
			cws[(r+c)%ppm] |= bit << c
		}
	}
	nibbles = make([]byte, ppm)
	for i, cw := range cws {
		n, corr, isBad := hammingDecode(cw, cr)
		nibbles[i] = n
		if corr {
			corrected++
		}
		if isBad {
			bad++
		}
	}
	return nibbles, corrected, bad
}

// headerNibbleCount is the explicit header's five nibbles: length, coding
// rate with the CRC flag, and the checksum.
const headerNibbleCount = 5

// headerNibbles builds the explicit header: [len_hi, len_lo, cr<<1|crc,
// chk_hi, chk_lo]. The checksum equations are the XOR set the
// reverse-engineering literature reports; the golden-vector suite is what
// holds them to real silicon.
func headerNibbles(length, cr int, crc bool) []byte {
	c := 0
	if crc {
		c = 1
	}
	n0 := byte(length >> 4)
	n1 := byte(length & 0xF)
	n2 := byte(cr<<1 | c)
	chk := headerChecksum(n0, n1, n2)
	return []byte{n0, n1, n2, byte(chk >> 4 & 1), byte(chk & 0xF)}
}

func parseHeader(nibbles []byte) (length, cr int, crc, ok bool) {
	if len(nibbles) < headerNibbleCount {
		return 0, 0, false, false
	}
	n0, n1, n2 := nibbles[0], nibbles[1], nibbles[2]
	chk := headerChecksum(n0, n1, n2)
	if nibbles[3]&1 != chk>>4&1 || nibbles[4] != chk&0xF {
		return 0, 0, false, false
	}
	return int(n0)<<4 | int(n1), int(n2 >> 1), n2&1 == 1, true
}

// headerChecksum is five bits over the twelve header bits.
func headerChecksum(n0, n1, n2 byte) byte {
	b := func(n byte, i int) byte { return n >> i & 1 }
	c4 := b(n0, 3) ^ b(n0, 2) ^ b(n0, 1) ^ b(n0, 0)
	c3 := b(n0, 3) ^ b(n1, 3) ^ b(n1, 2) ^ b(n1, 1) ^ b(n2, 3)
	c2 := b(n0, 2) ^ b(n1, 3) ^ b(n1, 0) ^ b(n2, 3) ^ b(n2, 1)
	c1 := b(n0, 1) ^ b(n1, 2) ^ b(n1, 0) ^ b(n2, 2) ^ b(n2, 1) ^ b(n2, 0)
	c0 := b(n0, 0) ^ b(n1, 1) ^ b(n2, 3) ^ b(n2, 2) ^ b(n2, 1) ^ b(n2, 0)
	return c4<<4 | c3<<3 | c2<<2 | c1<<1 | c0
}

// CRC16 is the payload CRC: CCITT polynomial 0x1021, zero initial value,
// computed over the unwhitened payload.
func CRC16(b []byte) uint16 {
	var crc uint16
	for _, v := range b {
		crc ^= uint16(v) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
