// The first line a TCP connection sends, and whether it is allowed to send a
// second.
//
// Split from control.go, which was over its length limit. The handshake is a
// self-contained conversation - one line in, one refusal or a cleared deadline
// out - and it is the part with the rules in it, so it is worth finding on its
// own rather than in the middle of the accept loop.
package control

import (
	"encoding/json"
	"errors"
	"net"
	"time"
)

// authorised reads the first line of a TCP connection and checks its token,
// returning what that line said so the caller can look at the rest of it.
//
// Only for TCP. A unix socket is protected by its permissions and always has
// been, and asking a token of it as well would break every script written
// against the socket so far for no gain.
//
// The refusal says what is wrong and closes. Not a timing-safe comparison:
// the token is 128 bits of randomness in a 0600 file on the same machine, and
// an attacker able to time this loop is an attacker who could read the file.
func (s *Server) authorised(c net.Conn, dec *json.Decoder, enc *json.Encoder) (hello, bool) {
	// A deadline, because a connection that opens and says nothing would
	// otherwise hold a goroutine for as long as it liked.
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	var h hello
	if err := dec.Decode(&h); err != nil {
		if errors.Is(err, errFrameTooLarge) {
			// Answered rather than just closed: a peer over the hello limit
			// gets told why, the same as a peer with the wrong token.
			_ = enc.Encode(Response{Error: errFrameTooLarge.Error(), Code: string(BadParams)})
		}
		return h, false
	}
	if h.Token != s.addr.Token {
		_ = enc.Encode(Response{
			Error: "this connection did not present the token from " +
				"the workbench's address file",
			Code: string(Unauthorised),
		})
		return h, false
	}
	// The token was right, and the line was also a request. Answered rather
	// than swallowed: this used to authorise, consume the call as the greeting
	// and reply to nothing, so the connection hung with no error at either end.
	//
	// After the token check, not before it: a first line with no token at all
	// is unauthorised, which is what it has always been and what a client
	// reading the wrong address file needs to hear.
	if h.Method != "" {
		_ = enc.Encode(Response{
			Error: "the first line on a TCP connection is the handshake and " +
				"carries only the token, as {\"token\":\"...\"} - this one " +
				"also carried a method, which would have been read as the " +
				"greeting and never answered. Send the token on its own line, " +
				"then the request on the next",
			Code: string(BadParams),
		})
		return h, false
	}
	// Cleared: a driven session is idle for minutes at a time between verbs.
	_ = c.SetReadDeadline(time.Time{})
	return h, true
}
