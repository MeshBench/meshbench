// What a connected companion knows, in the shapes the client draws.
//
// The protocol layer already decodes frames into real values - a channel with
// a name and an index, a contact with a hop count, a message with a sender
// and a time - and all of it used to be flattened into console strings before
// anything else saw it. That is why the companion tab could only ever show a
// terminal: by the time the UI had the data, it was text.
//
// These are those values, carried through instead.
package state

import "time"

// Companion is one node's companion session, as the panel reads it.
type Companion struct {
	// Node names it, and Connected says whether the workbench holds the port.
	Node      string
	Connected bool
	// Name, Key and the radio are what the node said about itself when it was
	// asked, rather than what the scenario believes about it. The two can
	// differ, and when they do it is the node that is right.
	Name    string
	Key     string
	FreqKHz uint32
	BWKHz   uint32
	SF, CR  uint8
	TxDBm   uint8
	// Scope is the region the node sends under, read back from it. Empty
	// means unscoped, which is a different thing from unknown - see Scoped.
	Scope  string
	Scoped bool
	// Channels are the slots the node holds, by index. A companion addresses
	// channels by number on the wire; the name is what makes that legible.
	Channels []CompanionChannel
	// Contacts are the nodes it has heard advertise.
	Contacts []CompanionContact
	// Messages is everything sent and received this session, oldest first.
	Messages []CompanionMessage
	// Serving is the endpoint an outside client is offered, when the port has
	// been handed over instead of held. Empty when the workbench holds it.
	Serving Endpoint
}

// CompanionChannel is one channel slot.
type CompanionChannel struct {
	Index uint8
	Name  string
	// Unread is how many messages have arrived on it since it was last
	// looked at, which is the number the list exists to show.
	Unread int
}

// CompanionContact is a node this one has heard from.
type CompanionContact struct {
	Name string
	Key  string
	// Hops is the path length back to them, or -1 when the node has not
	// established one. A contact with no path is still a contact.
	Hops     int
	LastSeen time.Time
}

// CompanionMessage is one line of a conversation.
//
// Mine separates what this client sent from what the node received. Nothing
// comes back to echo a sent message, so without it the operator sends into a
// conversation that stays empty and cannot tell whether anything happened.
type CompanionMessage struct {
	Channel    bool
	ChannelIdx uint8
	From       string
	Text       string
	At         time.Time
	Mine       bool
	SNRdB      float64
	// Hops is the path length the message arrived over, for a received one.
	Hops int
	// Receipt is what became of a sent message, once the run knows: how far
	// it went and how many heard it. The simulator can answer this and a
	// real phone cannot, which is the one place this client can do better
	// than the thing it imitates. Empty until there is something to say.
	Receipt string
	// Failed marks a receipt that is bad news rather than good.
	Failed bool
}
