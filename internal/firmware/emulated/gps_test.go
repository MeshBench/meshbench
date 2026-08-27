package emulated

import (
	"strings"
	"testing"
	"time"
)

// Are the sentences ones a receiver would send?
//
// Checked against the two things that are easy to get wrong and impossible to
// see afterwards: coordinates in this format are degrees followed by minutes
// rather than a decimal of a degree, and a sentence with a wrong checksum is
// dropped in silence by every parser there is - so a mistake in either shows
// up as a handheld that simply never gets a fix.
func TestTheSentencesAreWellFormed(t *testing.T) {
	at := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	for _, s := range []string{gga(at, 56.7, -3.85, 120), rmc(at, 56.7, -3.85)} {
		if !strings.HasPrefix(s, "$") || !strings.HasSuffix(s, "\r\n") {
			t.Fatalf("not a sentence: %q", s)
		}
		body := s[1:strings.LastIndex(s, "*")]
		var sum byte
		for i := 0; i < len(body); i++ {
			sum ^= body[i]
		}
		got := strings.TrimSpace(s[strings.LastIndex(s, "*")+1:])
		if want := strings.ToUpper(hexByte(sum)); got != want {
			t.Fatalf("checksum %s, want %s, in %q", got, want, s)
		}
		// 56.7 N is 56 degrees and 42 minutes exactly, and -3.85 is 3 degrees
		// 51 minutes west. Anything else means the two are being confused.
		if !strings.Contains(s, "5642.00000,N") || !strings.Contains(s, "00351.00000,W") {
			t.Fatalf("coordinates are not degrees and minutes: %q", s)
		}
	}
}

func hexByte(b byte) string {
	const d = "0123456789ABCDEF"
	return string([]byte{d[b>>4], d[b&0xf]})
}
