// Package provision is what a node is told, and what it is asked back - the
// firmware's own command table, and the rules that decide who gets what.
//
// It exists because "provisioning" used to mean one fixed script, sent
// fire-and-forget with nobody reading the reply. That could not express two
// things a real study needs: different settings for different groups of
// nodes, and knowing whether a node actually took what it was sent. This
// package is deliberately state-free and I/O-free - it is rules and firmware
// vocabulary, tested on their own, with no bridge and no engine in sight. The
// wiring that sends bytes down a serial port lives in internal/session.
package provision

// Kind is the shape of a command's value, which decides what control edits it
// and what a condition may compare it against.
type Kind int

const (
	// KindString is free text: a name, a password, owner info. No range check
	// is possible, so none is offered.
	KindString Kind = iota
	// KindInt is a whole number, optionally bounded by Min/Max.
	KindInt
	// KindBool is on/off, spelled the way the firmware spells it.
	KindBool
	// KindEnum is a fixed, named set of values - not necessarily numbers on
	// the wire (loop.detect sends the word, not the index).
	KindEnum
)

// Key is one thing MeshCore can be told or asked, exactly as CommonCLI.cpp
// spells it.
//
// Generated from the firmware's own command table rather than curated by
// hand, on purpose: a hand-picked list is exactly how "set advert.hops" - a
// command that does not exist - ended up wired into a working panel. A key
// with no row here cannot be offered as a chooser, and cannot be set without
// saying so.
type Key struct {
	// Name is the field's own identifier, used in rules and effects:
	// "path.hash.mode", "loop.detect", "name".
	Name string
	// Set is the command prefix; the value is appended after a space.
	// Empty means the firmware has no way to change it from here.
	Set string
	// Get is the full command that reads it back - "get path.hash.mode",
	// "region default", "clock", "region". Empty means it cannot be read,
	// which the caller must treat as "unknown", never as "off" or zero.
	Get  string
	Kind Kind
	// Enum is the legal values, in the order offered, for KindEnum. The
	// second element of a pair is what the firmware actually consumes on the
	// wire, which is not always Enum[i] itself (path.hash.mode is offered in
	// bytes but sent as bytes-minus-one).
	Enum []string
	// Min, Max bound a KindInt. Nil means the firmware's own check is the
	// only one - which still refuses out-of-range values, just later and
	// less legibly.
	Min, Max *int
	// CompanionField is the proto.SelfInfo/DeviceInfo field this maps to for
	// a node that speaks the app protocol instead of this CLI, or "" if there
	// is no equivalent - loop.detect has none, and a rule that touches it
	// must skip a companion rather than guess.
	CompanionField string
	// Note explains a firmware quirk worth surfacing next to the control -
	// units that do not match between set and get, a value that means
	// something unexpected, a default worth knowing.
	Note string
}

func intPtr(n int) *int { return &n }

// Table is every key this package knows about, drawn from
// src/helpers/CommonCLI.cpp's handleSetCmd/handleGetCmd at 727fc051. Where the
// firmware has both a setter and a getter, both are here; a few - clock, the
// region family - live outside the set/get dispatch entirely and are named to
// match.
var Table = []Key{
	{Name: "name", Set: "set name", Get: "get name", Kind: KindString,
		CompanionField: "name"},
	{Name: "lat", Set: "set lat", Get: "get lat", Kind: KindString,
		CompanionField: "lat"},
	{Name: "lon", Set: "set lon", Get: "get lon", Kind: KindString,
		CompanionField: "lon"},
	{Name: "clock", Set: "time", Get: "clock", Kind: KindString,
		Note: "the clock cannot go backwards - a value not greater than what " +
			"the node already holds is refused, not clamped"},

	{Name: "path.hash.mode", Set: "set path.hash.mode", Get: "get path.hash.mode",
		Kind: KindEnum, Enum: []string{"1", "2", "3"}, CompanionField: "pathHash",
		Note: "offered in bytes; the wire value is bytes minus one, and the " +
			"firmware rejects anything above 2"},
	{Name: "loop.detect", Set: "set loop.detect", Get: "get loop.detect",
		Kind: KindEnum, Enum: []string{"off", "minimal", "moderate", "strict"},
		Note: "the CLI takes the word, not the number - and at 3-byte path " +
			"hashes the three non-off levels are indistinguishable by " +
			"construction"},
	{Name: "cad", Set: "set cad", Get: "get cad", Kind: KindBool},
	{Name: "repeat", Set: "set repeat", Get: "get repeat", Kind: KindBool},
	{Name: "allow.read.only", Set: "set allow.read.only", Get: "get allow.read.only",
		Kind: KindBool},

	{Name: "advert.interval", Set: "set advert.interval", Get: "get advert.interval",
		Kind: KindInt, Min: intPtr(60), Max: intPtr(240),
		Note: "minutes; the firmware stores half of it and doubles it back on read"},
	{Name: "flood.advert.interval", Set: "set flood.advert.interval",
		Get: "get flood.advert.interval", Kind: KindInt},
	{Name: "flood.max", Set: "set flood.max", Get: "get flood.max", Kind: KindInt},
	{Name: "flood.max.advert", Set: "set flood.max.advert", Get: "get flood.max.advert",
		Kind: KindInt, Note: "firmware default is 8; a national mesh needs more"},
	{Name: "flood.max.unscoped", Set: "set flood.max.unscoped",
		Get: "get flood.max.unscoped", Kind: KindInt},

	{Name: "tx", Set: "set tx", Get: "get tx", Kind: KindInt, CompanionField: "txPower"},
	{Name: "freq", Set: "set freq", Get: "get freq", Kind: KindString},
	{Name: "radio", Set: "set radio", Get: "get radio", Kind: KindString,
		Note: "freq bw sf cr, all four together or the firmware refuses the frame"},
	{Name: "radio.rxgain", Set: "set radio.rxgain", Get: "get radio.rxgain", Kind: KindInt},
	{Name: "radio.fem.rxgain", Set: "set radio.fem.rxgain",
		Get: "get radio.fem.rxgain", Kind: KindInt},
	{Name: "rxdelay", Set: "set rxdelay", Get: "get rxdelay", Kind: KindInt},
	{Name: "txdelay", Set: "set txdelay", Get: "get txdelay", Kind: KindInt},
	{Name: "direct.txdelay", Set: "set direct.txdelay", Get: "get direct.txdelay",
		Kind: KindInt},
	{Name: "dutycycle", Set: "set dutycycle", Get: "get dutycycle", Kind: KindString},
	{Name: "af", Set: "set af", Get: "get af", Kind: KindString},
	{Name: "int.thresh", Set: "set int.thresh", Get: "get int.thresh", Kind: KindInt},
	{Name: "agc.reset.interval", Set: "set agc.reset.interval",
		Get: "get agc.reset.interval", Kind: KindInt},
	{Name: "multi.acks", Set: "set multi.acks", Get: "get multi.acks", Kind: KindInt},
	{Name: "extra.sf", Set: "set extra.sf", Get: "", Kind: KindInt,
		Note: "write-only in this firmware"},
	{Name: "adc.multiplier", Set: "set adc.multiplier", Get: "", Kind: KindString,
		Note: "write-only in this firmware"},

	{Name: "owner.info", Set: "set owner.info", Get: "", Kind: KindString,
		Note: "write-only in this firmware"},
	{Name: "guest.password", Set: "set guest.password", Get: "get guest.password",
		Kind: KindString},
	{Name: "prv.key", Set: "set prv.key", Get: "", Kind: KindString,
		Note: "serial console only - refused when the sender is a remote admin"},

	{Name: "bridge.enabled", Set: "set bridge.enabled", Get: "", Kind: KindBool},
	{Name: "bridge.channel", Set: "set bridge.channel", Get: "", Kind: KindInt},
	{Name: "bridge.baud", Set: "set bridge.baud", Get: "", Kind: KindInt},
	{Name: "bridge.delay", Set: "set bridge.delay", Get: "", Kind: KindInt},
	{Name: "bridge.secret", Set: "set bridge.secret", Get: "", Kind: KindString},
	{Name: "bridge.source", Set: "set bridge.source", Get: "", Kind: KindString},
}

// ByName is Table indexed for lookup, built once.
var ByName = func() map[string]Key {
	m := make(map[string]Key, len(Table))
	for _, k := range Table {
		m[k.Name] = k
	}
	return m
}()

// Readable is every key with a Get command, which is every field a condition
// or a diff can be evaluated against.
func Readable() []Key {
	var out []Key
	for _, k := range Table {
		if k.Get != "" {
			out = append(out, k)
		}
	}
	return out
}
