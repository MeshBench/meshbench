// One packet, as the packet view reads it: the dissection, the journey it
// made, and what every node did about it.
package state

// Packet is one transmission, dissected, with everywhere it went - the view a
// real capture cannot produce, because no observer is everywhere. Built by
// packet.open when an event is clicked.
type Packet struct {
	ID        uint64
	MessageID uint64
	Origin    string
	AtMs      uint32
	Heard     int
	Missed    int
	// Malformed is the dissection's complaint, empty when the frame parsed.
	Malformed string
	// The header, in the dissector's words.
	RouteType   string
	PayloadType string
	Version     string
	Transport   string
	// Path is the hop hashes, resolved to node names where the run knows
	// them - approximate by construction and labelled where it fails. One
	// entry per hop; a trace has none, because its path area carries SNR.
	Path []string
	// Hops is the frame's own hop count, off the path-length byte. Not
	// len(Path): the hash size is variable, and a trace has a hop count with
	// no hashes to name.
	Hops int
	// PayloadFields are what the payload carries in clear; PayloadNote is
	// what to say when it carries nothing readable.
	PayloadFields []PacketField
	PayloadNote   string
	// PathFields names the path area entry by entry - relay hashes, or, for a
	// trace, the SNR each hop measured as it passed.
	PathFields []PacketField
	// Spans is the frame's shape, in order, tiling every byte of it.
	Spans []PacketSpan
	// Readable says how much of this payload type can be read at all, so
	// nobody hunts through ciphertext for a message body.
	Readable string
	// Scope is the region this packet was sent to, as far as it can be
	// confirmed.
	Scope PacketScope
	// RawLines is the frame as a formatted hex dump, one line per 16 bytes.
	RawLines []string
	// Raw is the frame itself, so the hex view can colour a span of bytes
	// rather than only print them - a pre-formatted line cannot be
	// highlighted from the middle.
	Raw []byte
	// Fates is what happened at every node that logged an event for this
	// packet. Ledger is the radio-level truth for every receiver, collapsed
	// to the one answer that matters - did it ever get the message, and if
	// not, why not - across the whole journey; LedgerFull keeps every
	// attempt uncollapsed, for the "why?" modal's exhaustive per-hop history.
	Fates      []PacketFate
	Ledger     []PacketReception
	LedgerFull []PacketReception
	// Journey follows the message this packet carried across every relay.
	Journey       []PacketHop
	Transmissions int
	Reached       int
}

// PacketField is one dissected field: what it is called, what is on the wire,
// what that means, and which bytes it came from.
//
// Offset and Size are carried rather than dropped so the hex view can
// highlight the bytes a field was read from - the dissector has computed them
// all along and this type used to throw them away on the way to the UI.
type PacketField struct {
	Name, Value string
	// Decoded is the value as a person reads it; empty when the raw value is
	// already the readable one.
	Decoded string
	// Description is what the field is for, in a few words.
	Description  string
	Offset, Size int
}

// PacketSpan is one structural region of the frame - header, transport codes,
// path, payload - so the panel can show the shape before the detail.
type PacketSpan struct {
	Name         string
	Offset, Size int
	Detail       string
}

// PacketScope is what can be said about the region a packet was sent to.
//
// Three states, and the difference between the last two matters: a region key
// is never in the packet, so a scope code can only be *confirmed* against a
// candidate name. Not matching means we did not hold the name, which is not
// the same as the packet having no scope, and saying so is the difference
// between a result and a guess.
type PacketScope struct {
	// Scoped is whether the frame carries a scope code at all.
	Scoped bool
	// Name is the candidate whose key reproduces the code, when one did.
	Name string
	// Code is the scope code itself, always shown when Scoped.
	Code string
	// Candidates is how many names were checked, so a non-match reads as
	// "none of the N we hold" rather than as an absence of scope.
	Candidates int
	// Note carries the one case with its own meaning: codes {0,0}, which the
	// firmware treats as addressed to nowhere.
	Note string
}

// PacketFate is one node's outcome for one packet.
type PacketFate struct {
	AtMs  uint32
	Node  string
	Kind  string
	SNRdB float64
	What  string
}

// PacketReception is one row of the reception ledger: what one receiver's
// radio made of one packet, whether or not its firmware ever knew.
type PacketReception struct {
	Node     string
	From     string
	Offered  bool
	RSSIdBm  float64
	SNRdB    float64
	Demod    bool
	CRCOK    bool
	Firmware string // accepted, dropped, never saw it
	// Why is the engine's own words for this specific reception when it was
	// offered and still failed - empty when it succeeded outright, and empty
	// when Offered is false, because nothing measurable arrived to have a
	// reason at all.
	Why string
	// Hop is which transmission of the journey this reception answers for -
	// a node offered on more than one hop has more than one row, and this is
	// what tells them apart.
	Hop int
}

// PacketHop is one transmission of the followed message.
type PacketHop struct {
	AtMs  uint32
	By    string
	Hops  int
	Heard []string
	// MissedBy names who could not decode this relay; Missed keeps the count
	// for callers that only need the number.
	MissedBy []string
	Missed   int
	// MissWhy is the engine's own words for each entry in MissedBy, in the
	// same order.
	//
	// Carried here rather than looked up in Fates because Fates covers one
	// packet and a journey covers every transmission of the message - the
	// join by node and time silently missed almost all of them, and every
	// failed hop was drawn as "other".
	MissWhy []string
	// MissClass is the cause the engine established for each entry in
	// MissedBy, in the same order: the class, not the sentence.
	//
	// The picture used to read the words, which meant it recognised the three
	// wordings somebody had thought to match and drew every other cause as
	// "other". The class is what the engine actually decided, so a reworded
	// message cannot change what the drawing says happened.
	MissClass []string
	PacketID  uint64
}
