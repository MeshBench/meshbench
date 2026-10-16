// What a collision does to the symbols it lands on.
//
// Winning the demodulator is not the same as surviving it. A receiver locked
// onto a preamble goes on being hit by everything else that keys up during the
// rest of the packet, and the symbols underneath those hits are destroyed - so
// a packet can lock cleanly, be the strongest thing in the window on average,
// and still fail its CRC because something stamped on the middle of it.
//
// In waveform mode this needs no model at all: interferers are summed into the
// receiver's window as IQ and the demodulator finds out the hard way. The
// calculated path has no window, and without this it had no corruption either
// - an interferer was either loud enough to fail one ratio across the whole
// packet, or it may as well never have transmitted. Both are wrong in the same
// direction, which is the direction this simulator is already prone to.
//
// Still not a rule that overlapping packets fail. Whether the wanted signal
// rides over an interferer is the capture threshold's business; whether the
// damage that gets through is repairable is the interleaver's. Two packets can
// overlap completely and both be fine, which is what capture effect means.
package engine

import (
	"math"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// corruptedSymbols counts how many of a transmission's symbols were destroyed
// by co-channel traffic the receiver could not capture over, in the whole
// packet and in its header block alone.
//
// Overlaps are unioned, not summed: two interferers stamping on the same
// stretch of a packet destroy it once. Anything the wanted signal leads by the
// capture threshold contributes nothing, however long it overlaps for. A hit
// shorter than one symbol still costs a whole symbol, because a chirp
// interrupted anywhere lands in the wrong FFT bin.
//
// The header is counted apart because the chip answers differently for it: a
// header it cannot read raises HeaderErr and nothing more, where a payload it
// cannot read raises RxDone beside CrcErr. Which of the two a firmware is told
// decides which of its error paths runs.
func (e *Engine) corruptedSymbols(rx int, t transmission, wantedDBm float64,
	concurrent []transmission, nodes []*Node, txPHY phy) (total, header float64) {

	type span struct{ from, to uint32 }
	var hits []span
	for _, other := range concurrent {
		if other.packetID == t.packetID || other.from == rx || other.from == t.from {
			continue
		}
		from, to := maxU32(other.startMs, t.startMs), minU32(other.endMs, t.endMs)
		if to <= from {
			continue
		}
		if !e.phyOf(nodes[other.from].specRef()).sameChannel(txPHY) {
			continue
		}
		loss, ok := e.pathLoss(other.from, rx)
		if !ok {
			continue
		}
		otherDBm := e.rxPowerDBm(nodes, other.from, rx, loss)
		if wantedDBm-otherDBm >= captureThresholdDB {
			// Captured: the interferer is present, and irrelevant.
			continue
		}
		hits = append(hits, span{from, to})
	}
	if len(hits) == 0 {
		return 0, 0
	}
	// Union by sweep, so two interferers over the same symbols cost one packet
	// rather than two. Insertion sort: the list is one entry per concurrent
	// transmitter that beat capture, which on any real mesh is a handful.
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].from < hits[j-1].from; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	var damagedMs uint32
	cur := hits[0]
	for _, h := range hits[1:] {
		if h.from > cur.to {
			damagedMs += cur.to - cur.from
			cur = h
			continue
		}
		if h.to > cur.to {
			cur.to = h.to
		}
	}
	damagedMs += cur.to - cur.from

	symbolMs := 1000 * math.Pow(2, float64(txPHY.sf)) / txPHY.bandwidthHz
	if symbolMs <= 0 {
		return 0, 0
	}
	hdrFrom, hdrTo := headerBlockMs(t.startMs, txPHY.sf, symbolMs)
	var headerMs float64
	for _, h := range hits {
		from, to := math.Max(float64(h.from), hdrFrom), math.Min(float64(h.to), hdrTo)
		if to > from {
			headerMs += to - from
		}
	}
	return math.Ceil(float64(damagedMs) / symbolMs), math.Ceil(math.Min(headerMs, hdrTo-hdrFrom) / symbolMs)
}

// headerBlockMs is where a transmission's explicit header sits on the air: the
// first interleaver block after the preamble, the two sync symbols and the
// 2.25-symbol start of frame, eight symbols long and always coded at CR 4/8.
func headerBlockMs(startMs uint32, sf int, symbolMs float64) (from, to float64) {
	from = float64(startMs) + (float64(dsp.PreambleSymbols(sf))+4.25)*symbolMs
	return from, from + 8*symbolMs
}

// headerSurvives reports whether the header block can be put back together.
// It is coded at CR 4/8 whatever the payload uses, so one destroyed symbol is
// repaired and a second is fatal, by the same interleaver argument as below.
func headerSurvives(damagedSymbols float64) bool {
	return damagedSymbols <= 1
}

// failureOf is what the chip raises for a frame it locked onto and lost: a
// header it could not read is HeaderErr and nothing more; a header that read
// and a payload that did not is RxDone beside CrcErr, with the bytes the
// receive chain produced left in the buffer.
func failureOf(headerOK bool) firmware.FrameFailure {
	if headerOK {
		return firmware.FailedCRC
	}
	return firmware.FailedHeader
}

// survivesCorruption reports whether forward error correction can put a packet
// back together after that much damage.
//
// The answer is not an estimate; it falls out of the coding chain we already
// implement. internal/rf/lora's diagonal interleaver has exactly one useful
// property - one destroyed symbol costs every codeword in its block exactly
// one bit - and its Hamming layer, whose parity equations were solved from a
// real captured frame rather than from a paper, corrects one bit per codeword
// at CR 4/7 and 4/8, detects at 4/6 and does nothing at 4/5.
//
// So: one destroyed symbol is repairable at 4/7 and 4/8 and fatal below them,
// and two destroyed symbols put two bit errors into one codeword, which
// nothing in the chain can undo. That is why a real flood thins out and this
// one did not.
//
// Consecutive destroyed symbols are assumed to share an interleaver block,
// which is the common case rather than the certain one: a burst straddling a
// block boundary would put one bit into each of two blocks and survive. Block
// alignment is not tracked here, and the assumption errs towards the packet
// being lost - the opposite of this simulator's usual direction, deliberately.
func survivesCorruption(damagedSymbols float64, cr int) (repaired int, ok bool) {
	switch {
	case damagedSymbols <= 0:
		return 0, true
	case damagedSymbols > 1:
		return 0, false
	case cr >= 3: // 4/7 and 4/8 correct one bit per codeword
		return 1, true
	default: // 4/5 is a bare checksum, 4/6 only detects
		return 0, false
	}
}

func minU32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func maxU32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}
