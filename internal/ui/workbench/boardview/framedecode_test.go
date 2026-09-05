package boardview

import (
	"strings"
	"testing"
)

// A real frame, copied out of a running node's console log.
//
// A Heltec V3 companion answering with its own self-info: this is what the pane
// draws unaided, and what somebody looking for the board's radio settings has to
// read past. Kept verbatim rather than hand-built, because a hand-built frame
// tests the decoder against this file's idea of the protocol rather than
// against the firmware's.
const realSelfInfo = ">B\\x00\\x05\\x01\\x16\\x16\\xc4\\x89O\\xdf<[\\xae\\xf6R\tTZ\\x1a\\xb0'" +
	"\\xe7\\xe4.\\x18\\xce<H\\x8bg\\x9b\\xb3P$X\\x8f\\x18\\x1e\\x00\\x00\\x00\\x00\\x00" +
	"\\x00\\x00\\x00\\x00\\x00\\x00\\x00\\xf2D\\x0d\\x00$\\xf4\\x00\\x00\\x08\\x05C4894FDF"

// The decode reads what is on screen, rather than replacing it.
//
// The whole point of the tick: the frame carries the board's name, frequency,
// spreading factor and transmit power, and every one of them is unreadable in
// the escaped form. A reader who wants to know what radio the board is set to
// should be able to see it here.
func TestADecodedFrameSaysWhatTheBoardActuallyReported(t *testing.T) {
	got := decodeFrames([]string{realSelfInfo})
	if len(got) != 1 {
		t.Fatalf("one frame in, %d lines out: %v", len(got), got)
	}
	for _, want := range []string{"self:", "869.618 MHz", "SF8", "22 dBm"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("the decoded line does not carry %q: %s", want, got[0])
		}
	}
}

// Text that is not a frame stays as it was.
//
// A board prints plain text on the same port it frames on, so a decoder that
// consumed everything would hide the bootloader - and the bootloader is what
// says whether the board started at all.
func TestPlainTextSurvivesDecoding(t *testing.T) {
	got := decodeFrames([]string{
		"ESP-ROM:esp32s3-20210327",
		"E (145) SPIFFS: mount failed, -10025",
		realSelfInfo,
	})
	joined := strings.Join(got, "\n")
	for _, want := range []string{"ESP-ROM:esp32s3", "SPIFFS: mount failed", "self:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("decoding lost %q:\n%s", want, joined)
		}
	}
}

// A '>' in ordinary text is not a frame.
//
// The length is two bytes of whatever follows the marker, so most candidates
// are nonsense; believing one would swallow the rest of the log as a payload.
func TestAStrayMarkerIsNotReadAsAFrame(t *testing.T) {
	lines := []string{"> help", "  -> Unknown command", "regions", "  -> *^ F"}
	got := decodeFrames(lines)
	if strings.Join(got, "\n") != strings.Join(lines, "\n") {
		t.Errorf("console text was eaten by the frame reader:\ngot  %v\nwant %v",
			got, lines)
	}
}

// An escaped byte comes back as the byte it stood for, and nothing else does.
func TestUnescapingIsReversibleAndNarrow(t *testing.T) {
	if got := unescape(`\x00\xaf!`); string(got) != "\x00\xaf!" {
		t.Errorf("escapes did not come back: %q", got)
	}
	// A path out of a bootloader log is not an escape sequence.
	if got := unescape(`C:\xtra\path`); string(got) != `C:\xtra\path` {
		t.Errorf("a literal backslash was eaten: %q", got)
	}
}
