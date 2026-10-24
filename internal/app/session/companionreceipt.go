// What became of a direct message, a login or a status request.
//
// The firmware answers each with RESP_CODE_SENT, carrying the code it expects
// back, and later with the ack or the reply itself. None of those name the
// command they answer, so the session keeps what it asked for in order and
// matches: the next RESP_CODE_SENT or error belongs to the oldest request the
// firmware has not yet answered, because a companion handles its serial
// commands one at a time.
package session

import (
	"encoding/hex"
	"fmt"

	"github.com/MeshBench/meshbench/internal/mesh/proto"
)

// asked is one request waiting on the firmware.
type asked struct {
	kind, to, text string
	// sentMs is the simulated time the firmware took it, once it has, and
	// timeoutMs how long the firmware said it would wait for the ack.
	sentMs, timeoutMs uint32
}

// expect records a request so its answers can be matched to it.
func (c *compSession) expect(kind, to, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queued = append(c.queued, asked{kind: kind, to: to, text: text})
}

// receipt turns the answer frames into lines that say which request they
// answer, stamped with the simulated time they arrived. Called with c.mu held.
func (c *compSession) receipt(f proto.Frame) bool {
	at := fmt.Sprintf("[%.3f s] ", float64(c.simMs())/1000)
	c.expire(at)
	switch {
	case f.Sent != nil:
		if len(c.queued) == 0 {
			return false
		}
		a := c.queued[0]
		c.queued = c.queued[1:]
		a.sentMs, a.timeoutMs = c.simMs(), f.Sent.TimeoutMs
		how := "direct"
		if f.Sent.Flood {
			how = "flood"
		}
		if a.kind == "msg" {
			if c.awaiting == nil {
				c.awaiting = map[uint32]asked{}
			}
			c.awaiting[f.Sent.Ack] = a
		}
		c.note(fmt.Sprintf("%ssent %s to %s by %s, waiting up to %d ms",
			at, a.kind, a.to, how, f.Sent.TimeoutMs))
	case f.Confirmed != nil:
		a, ok := c.awaiting[f.Confirmed.Ack]
		if !ok {
			// The same ack can arrive more than once; the firmware says so.
			return true
		}
		delete(c.awaiting, f.Confirmed.Ack)
		c.note(fmt.Sprintf("%sdelivered to %s after %d ms: %s",
			at, a.to, f.Confirmed.RoundTripMs, a.text))
	case f.Answer != nil:
		from := hex.EncodeToString(f.Answer.From)
		switch f.Answer.Push {
		case proto.PushLoginSuccess:
			c.note(at + "login ok from " + from)
		case proto.PushLoginFail:
			c.note(at + "login refused by " + from)
		default:
			c.note(fmt.Sprintf("%sstatus from %s, %d bytes", at, from, len(f.Answer.Status)))
		}
	case f.Err != "" && len(c.queued) > 0:
		a := c.queued[0]
		c.queued = c.queued[1:]
		c.note(fmt.Sprintf("%s%s to %s refused: %s", at, a.kind, a.to, f.Err))
	default:
		return false
	}
	return true
}

// expire gives up on a message whose ack is past the time the firmware said it
// would wait, as the firmware does, and says so. Without it a message that was
// never acked stayed waiting for the whole session. Called with c.mu held.
func (c *compSession) expire(at string) {
	now := c.simMs()
	for ack, a := range c.awaiting {
		if now > a.sentMs+a.timeoutMs {
			delete(c.awaiting, ack)
			c.note(fmt.Sprintf("%sno ack from %s within %d ms: %s", at, a.to, a.timeoutMs, a.text))
		}
	}
}
