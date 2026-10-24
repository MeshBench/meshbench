package session

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/proto"
)

func frameOf(t *testing.T, b []byte) proto.Frame {
	t.Helper()
	f, err := proto.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// The firmware answers requests in the order it took them, and the answers do
// not say which request they belong to. A delivered line has to name the
// message that was delivered, not the one sent most recently.
func TestReceiptsMatchTheRequestTheyAnswer(t *testing.T) {
	var now uint32
	c := &compSession{node: "A", clock: func() uint32 { return now }}
	c.expect("msg", "Bravo", "first")
	c.expect("msg", "Charlie", "second")
	sent := func(ack uint32) []byte {
		b := binary.LittleEndian.AppendUint32([]byte{byte(proto.RespSent), 0}, ack)
		return binary.LittleEndian.AppendUint32(b, 5000)
	}
	now = 1500
	c.apply(frameOf(t, sent(11)))
	c.apply(frameOf(t, sent(22)))
	conf := binary.LittleEndian.AppendUint32([]byte{byte(proto.PushSendConfirmed)}, 22)
	conf = binary.LittleEndian.AppendUint32(conf, 900)
	now = 2400
	c.apply(frameOf(t, conf))

	joined := strings.Join(c.Lines(), "\n")
	for _, want := range []string{
		"sent msg to Bravo by direct",
		"sent msg to Charlie by direct",
		"[2.400 s] delivered to Charlie after 900 ms: second",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("transcript lacks %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "delivered to Bravo") {
		t.Errorf("Bravo's message was never acked:\n%s", joined)
	}
}

// A refusal belongs to the oldest unanswered request, and says why.
func TestARefusedRequestSaysSo(t *testing.T) {
	c := &compSession{node: "A"}
	c.expect("login", "Delta", "")
	c.apply(frameOf(t, []byte{byte(proto.RespErr), 2}))
	if j := strings.Join(c.Lines(), "\n"); !strings.Contains(j, "login to Delta refused") {
		t.Errorf("transcript lacks the refusal:\n%s", j)
	}
}

// The console of a companion the session holds is the session's transcript,
// not the binary frames its serial port carries.
func TestAHeldCompanionsConsoleIsItsTranscript(t *testing.T) {
	s := &Sim{comps: map[string]*compSession{"A": {node: "A"}}}
	s.comps["A"].note("> login Bravo hunter2")
	got := s.consoleLines("A", []string{">\x01\x02 raw"})
	if len(got) != 1 || got[0] != "> login Bravo hunter2" {
		t.Fatalf("console shows %q", got)
	}
	if got := s.consoleLines("B", []string{"kept"}); len(got) != 1 || got[0] != "kept" {
		t.Fatalf("a node with nothing to show lost its lines: %q", got)
	}
}

// A message whose ack never comes is given up on once the firmware's own wait
// is over, and the transcript says so, rather than waiting all session.
func TestAnUnackedMessageIsGivenUpOn(t *testing.T) {
	var now uint32
	c := &compSession{node: "A", clock: func() uint32 { return now }}
	c.expect("msg", "Bravo", "hello")
	sent := binary.LittleEndian.AppendUint32([]byte{byte(proto.RespSent), 0}, 11)
	sent = binary.LittleEndian.AppendUint32(sent, 5000)
	now = 1000
	c.apply(frameOf(t, sent))
	// Something else arrives after the wait is over.
	c.expect("msg", "Charlie", "later")
	later := binary.LittleEndian.AppendUint32([]byte{byte(proto.RespSent), 0}, 22)
	later = binary.LittleEndian.AppendUint32(later, 5000)
	now = 7000
	c.apply(frameOf(t, later))

	if _, ok := c.awaiting[11]; ok {
		t.Fatal("the message to Bravo is still waiting after its 5000 ms")
	}
	if !strings.Contains(strings.Join(c.Lines(), "\n"), "no ack from Bravo within 5000 ms: hello") {
		t.Fatalf("the transcript does not say the message went unacked:\n%s",
			strings.Join(c.Lines(), "\n"))
	}
	if _, ok := c.awaiting[22]; !ok {
		t.Fatal("the message to Charlie, still inside its wait, was given up on")
	}
}
