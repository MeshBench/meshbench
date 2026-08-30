// Bounding one JSON value before it is decoded.
//
// json.Decoder buffers a whole value before it hands anything back, so a
// connection that never authenticates and never proves it is the operator's
// own tooling can still make the process allocate for whatever it sends.
// Nothing below stops that value being read at all - a peer is still allowed
// to say what it likes - it stops it being read past a size decided in
// advance, so the cost of a hostile or broken frame is a closed connection
// rather than a hole growing in memory.
package control

import (
	"errors"
	"io"
)

// helloFrameLimit bounds the one frame read before a TCP connection has
// proven it holds the token: a hello line is a token and nothing else, a few
// dozen bytes of JSON, so it is held to far less than an ordinary request
// rather than trusted to behave before anybody has trusted it at all.
const helloFrameLimit = 1 << 10 // 1 KiB

// requestFrameLimit bounds every frame read once a connection is trusted -
// authenticated on TCP, or a unix socket from the start, where the
// filesystem already did that job.
//
// Nothing a verb in this tree takes as a parameter is bulk data: an import
// carries a URL and fetches it itself, firmware.import carries a local path
// and reads the file itself, both on the workbench's own goroutine and never
// through a request's params. So this only has to clear the largest request
// a script legitimately sends - a bulk node list, a long notes field - with
// headroom, while staying far short of what would let a connected peer force
// a multi-gigabyte allocation before Pump ever looks at what it asked for.
const requestFrameLimit = 8 << 20 // 8 MiB

// errFrameTooLarge is what a peer that outran its limit is told, and what
// serve and authorised use to tell that refusal apart from an ordinary
// decoding failure. A json.Decoder reading past a plain io.LimitReader sees
// an unexplained end of input and reports a confusing syntax error; a peer
// that sent something oversized should be told exactly why it was closed.
var errFrameTooLarge = errors.New(
	"control: the frame is larger than this connection is allowed to send")

// limitedReader caps how many bytes a single logical frame may read off the
// wire, and answers errFrameTooLarge once it does rather than the bare EOF an
// io.LimitReader would give - the difference between a peer being told why
// and a peer being told nothing.
//
// One is kept per connection and reset between frames rather than made fresh:
// json.Decoder may already hold a few bytes of the next frame in its own
// buffer by the time one Decode call returns, and a reader that forgot that
// would spend part of the next frame's budget paying for bytes the last frame
// already used.
type limitedReader struct {
	r io.Reader
	n int64
}

func newLimitedReader(r io.Reader, limit int64) *limitedReader {
	return &limitedReader{r: r, n: limit}
}

// reset extends the budget for the next frame, in place: the same reader
// keeps wrapping the same connection, only the count changes.
func (l *limitedReader) reset(limit int64) { l.n = limit }

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errFrameTooLarge
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}
