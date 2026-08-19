// Package capture records what happened on the air. It observes; it never
// alters what it records.
package capture

// Outcome is what became of one transmission at one receiver.
//
// Five values, because a packet-level simulator collapses all of them into "did
// not arrive" and they have completely different fixes. The distinction between
// DroppedByFirmware and NotDemodulated in particular is the difference between
// "working as designed" and "your link is broken".
type Outcome string

const (
	// OutOfRange — nothing measurable arrived. Terrain or distance.
	OutOfRange Outcome = "out-of-range"
	// NotDemodulated — arrived, too weak to recover. Raise power, lower SF,
	// better antenna.
	NotDemodulated Outcome = "not-demodulated"
	// CRCFailed — demodulated but corrupt. Marginal: interference or collision.
	CRCFailed Outcome = "crc-failed"
	// DroppedByFirmware — received correctly and deliberately discarded, e.g.
	// a dedup hit. Working as designed, not a fault.
	DroppedByFirmware Outcome = "dropped-by-firmware"
	// Relayed — received and retransmitted.
	Relayed Outcome = "relayed"
	// Accepted — received and kept, but not relayed onward.
	Accepted Outcome = "accepted"
)

// Reception is one row of the ledger: what one receiver made of one packet.
//
// Rows are produced at the RF layer, so they exist for every node regardless of
// firmware build — including nodes whose firmware never knew a frame arrived.
// That is a stronger answer than the node itself could give.
type Reception struct {
	PacketID uint64
	FromNode string
	ToNode   string
	Offered  bool // did any measurable energy arrive
	RSSIdBm  float64
	SNRdB    float64
	Demod    bool // did the demodulator recover symbols
	CRCOK    bool
	Outcome  Outcome
	// FirmwareSaw is false when the frame never reached the firmware — the gap
	// between what the radio heard and what the stack acted on, which is where
	// most confusing mesh behaviour lives.
	FirmwareSaw bool
}

// Ledger accumulates receptions and answers questions about them.
type Ledger struct {
	rows []Reception
}

func (l *Ledger) Record(r Reception) { l.rows = append(l.rows, r) }

// ForPacket returns every receiver's view of one transmission — the query that
// answers "who heard this, and why not".
func (l *Ledger) ForPacket(id uint64) []Reception {
	var out []Reception
	for _, r := range l.rows {
		if r.PacketID == id {
			out = append(out, r)
		}
	}
	return out
}

// Summary counts outcomes for one packet.
type Summary struct {
	Offered, Demodulated, Accepted, Relayed, Total int
}

// Summarise counts what became of one packet across all receivers.
func (l *Ledger) Summarise(id uint64) Summary {
	var s Summary
	for _, r := range l.ForPacket(id) {
		s.Total++
		if r.Offered {
			s.Offered++
		}
		if r.Demod {
			s.Demodulated++
		}
		switch r.Outcome {
		case Relayed:
			s.Relayed++
			s.Accepted++
		case Accepted, DroppedByFirmware:
			s.Accepted++
		}
	}
	return s
}

// Rows returns every recorded reception.
func (l *Ledger) Rows() []Reception { return l.rows }

// Classify derives the outcome from the physical facts plus what the firmware
// did. Kept in one place so the five states cannot drift apart across callers.
//
// The order matters: a frame that never demodulated cannot have failed CRC, and
// a frame the firmware dropped must have passed CRC first.
func Classify(offered, demod, crcOK, firmwareSaw, relayed bool) Outcome {
	switch {
	case !offered:
		return OutOfRange
	case !demod:
		return NotDemodulated
	case !crcOK:
		return CRCFailed
	case !firmwareSaw:
		// Demodulated and CRC-clean, but the stack never got it — a bug in our
		// plumbing rather than a radio outcome. Surfaced, never hidden.
		return CRCFailed
	case relayed:
		return Relayed
	default:
		return DroppedByFirmware
	}
}
