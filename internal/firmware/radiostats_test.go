package firmware

import (
	"encoding/binary"
	"testing"
	"time"
)

// statsRecord is a kRadioStats payload of n bytes with IRQReads set to 7 and,
// where the record is long enough to carry it, a suppressed count of 41.
func statsRecord(n int) []byte {
	p := make([]byte, n)
	binary.BigEndian.PutUint32(p[0:], 7)
	if n >= 47 {
		binary.BigEndian.PutUint32(p[43:], 41)
	}
	return p
}

// statsAfter sends one stats record and waits for the bridge to have read it.
func statsAfter(t *testing.T, record []byte) RadioStats {
	t.Helper()
	b, err := Listen("127.0.0.1:0", "GB7XYZ")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	c := fakeRadio(t, b.Addr())
	defer func() { _ = c.Close() }()

	hdr := []byte{kindRadioStats, byte(len(record) >> 8), byte(len(record))}
	if _, err := c.Write(append(hdr, record...)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st := b.Stats(); st.IRQReads == 7 {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the stats record never reached the bridge")
	return RadioStats{}
}

// A chip that applies IrqMask says how much it kept out. A masked event is
// silence, so without the count a firmware that never enabled an interrupt
// and one that never had cause to see it report the same busy figures.
func TestTheSuppressedCountIsReadFromAChipThatKeepsOne(t *testing.T) {
	st := statsAfter(t, statsRecord(47))
	if !st.SuppressedReported || st.IRQSuppressed != 41 {
		t.Errorf("suppressed = %d, reported %v; want 41, true",
			st.IRQSuppressed, st.SuppressedReported)
	}
}

// An older chip ignored the mask and sends the shorter record. Its count is
// unknown rather than zero, because it had nothing to suppress.
func TestAnOlderChipLeavesTheSuppressedCountUnreported(t *testing.T) {
	st := statsAfter(t, statsRecord(43))
	if st.SuppressedReported {
		t.Errorf("a 43-byte record reported a suppressed count of %d", st.IRQSuppressed)
	}
}
