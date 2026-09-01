// One connected companion, at the wire.
//
// compSession is the session side of internal/session/companion.go's verbs:
// it holds the claim on a node's port, reassembles MeshCore's length-framed
// stream, and keeps what the frames decoded to - self info, channels by
// index, contacts, messages - so the panels can draw state rather than
// scrollback. Both directions of the framing live here: '>' from the device
// is picked apart in Write, and '<' towards it is put together by compFrame.
package session

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/MeshBench/meshbench/internal/mesh/companion"
	"github.com/MeshBench/meshbench/internal/mesh/proto"
)

// compSession is one connected companion.
type compSession struct {
	node    string
	release func()

	mu       sync.Mutex
	self     *proto.SelfInfo
	device   *proto.DeviceInfo
	contacts []proto.Contact
	messages []proto.Message
	channels map[uint8]proto.ChannelInfo
	last     []string
	partial  []byte
	// scope is the region the node says it sends under, and scopeKnown
	// whether it has been asked. Empty-and-known is unscoped; empty-and-not
	// is simply not read yet, and the two must not look the same.
	scope      string
	scopeKnown bool
	// unread counts what has arrived on each channel since it was last
	// looked at. The channel list exists to show it.
	unread map[uint8]int
	// seen is the channel the client is currently reading, so arrivals on it
	// are not counted as unread.
	seen uint8
	// waiting is set when the node says a message is ready to collect, and
	// cleared once the sync command has been sent for it.
	waiting bool
	// rev counts decoded frames. Frames arrive on the bridge's goroutine and
	// the world is only written on the store's, so the tick needs some way to
	// ask "has anything happened" that is cheaper than rebuilding the view
	// and comparing it.
	rev uint64
}

// Write receives whatever the node sends while the claim is held.
//
// The protocol is length-framed, so a partial frame is kept rather than
// decoded: a serial read boundary is not a message boundary, and treating it
// as one produces a stream of decode errors that look like a broken node.
func (c *compSession) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.partial = append(c.partial, b...)
	// MeshCore's own framing: '>' from the device, then a little-endian
	// length, then that many bytes. Anything else in the stream is the
	// firmware's console output and is skipped rather than guessed at - the
	// first version of this read the length from wherever the buffer happened
	// to start and decoded rubbish.
	for {
		i := 0
		for i < len(c.partial) && c.partial[i] != '>' {
			i++
		}
		if i > 0 {
			c.partial = c.partial[i:]
		}
		if len(c.partial) < 3 {
			break
		}
		n := int(binary.LittleEndian.Uint16(c.partial[1:3]))
		if len(c.partial) < 3+n {
			break
		}
		frame := append([]byte(nil), c.partial[3:3+n]...)
		c.partial = c.partial[3+n:]
		f, err := proto.Decode(frame)
		if err != nil {
			c.note("undecoded frame: " + err.Error())
			continue
		}
		c.apply(f)
	}
	return len(b), nil
}

// Answered reports whether this node has ever said anything back.
//
// A companion is spoken to by writing frames at its serial port, and writing
// succeeds whether or not anything is listening: a board running firmware that
// never starts, or that is not a companion at all, takes every frame and
// answers none. Commands then report themselves sent - which is exactly what
// "it says advert sent and nothing transmits" is.
func (c *compSession) Answered() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rev > 0
}

func (c *compSession) apply(f proto.Frame) {
	c.rev++
	// A message is not pushed to us, only the news that one exists. Asking
	// for it is a command, so it cannot be sent from here - this runs on the
	// bridge's own goroutine, and typing into the bridge from inside its read
	// path deadlocks. The flag is picked up by the next tick instead.
	if f.Push == proto.PushMsgWaiting {
		c.waiting = true
	}
	switch {
	case f.SelfInfo != nil:
		c.self = f.SelfInfo
		c.note("self: " + f.SelfInfo.Name)
	case f.Device != nil:
		c.device = f.Device
		if f.Device.ModeKnown {
			c.note(fmt.Sprintf("path hashes: %d byte(s)",
				proto.PathHashBytes(f.Device.PathHashMode)))
		}
	case f.Channel != nil:
		if c.channels == nil {
			c.channels = map[uint8]proto.ChannelInfo{}
		}
		c.channels[f.Channel.Index] = *f.Channel
		c.note(fmt.Sprintf("channel %d: %s", f.Channel.Index, f.Channel.Name))
	case f.Contact != nil:
		c.contacts = append(c.contacts, *f.Contact)
		c.note("contact: " + f.Contact.Name)
	case f.Message != nil:
		c.messages = append(c.messages, *f.Message)
		// Not counted against the channel being read, and never against our
		// own sends - an unread badge on the conversation you are looking at
		// is noise, and one for a message you just typed is wrong.
		if f.Message.Channel && !f.Message.Mine && f.Message.ChannelIdx != c.seen {
			if c.unread == nil {
				c.unread = map[uint8]int{}
			}
			c.unread[f.Message.ChannelIdx]++
		}
		c.note("message: " + f.Message.Text)
	case f.Scope != nil:
		c.scope, c.scopeKnown = f.Scope.Name, true
		if f.Scope.Name == "" {
			c.note("scope: unscoped")
		} else {
			c.note("scope: " + f.Scope.Name)
		}
	case f.Err != "":
		c.note("error: " + f.Err)
	default:
		c.note(fmt.Sprintf("frame %v", f.Code))
	}
}

// note keeps a short tail, so a caller can see what happened without holding
// every frame of a long session.
func (c *compSession) note(s string) {
	c.last = append(c.last, s)
	if len(c.last) > 200 {
		c.last = c.last[len(c.last)-200:]
	}
}

// Lines is the session's recent traffic, for a console to draw.
func (c *compSession) Lines() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.last...)
}

// compFrame is companion.Frame under the short name this package says at every
// write. The envelope moved down to the transport's own package when the
// headless fixture runner came to need it too, so that there is one of it.
func compFrame(payload []byte) []byte { return companion.Frame(payload) }
