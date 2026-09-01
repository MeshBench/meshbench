// Speaking to a companion, which is not typing at it.
//
// RegionCommands next door exists because two programs issue the same
// provisioning and a second copy would eventually disagree. This is the other
// half of that argument, and it is the half that had already gone wrong: a
// repeater has a text console and reads typed bytes, a companion speaks the
// framed protocol a phone speaks, and text sent at one is read as somebody
// typing at a device that answers nothing. The workbench learned that when an
// experiment measured zero transmissions; the headless fixture runner had never
// learned it at all, so a fixture whose schedule aimed a message at a companion
// ran in silence and failed its own assertion.
package fixture

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/proto"
	"github.com/MeshBench/meshbench/internal/world/provider"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// SpeaksCompanion reports whether a node's console is the framed companion
// protocol rather than a text CLI.
func SpeaksCompanion(k scenario.Kind) bool {
	return k == scenario.Companion || k == scenario.RoomServer
}

// CompanionProvisioning is what a companion must be told before a run, as
// protocol payloads in the order they have to be sent.
//
// AppStart is not here: it is the handshake, and everything below waits on its
// reply, so a caller sends it first and lets the firmware answer before saying
// any of this. Sent in the same breath, these land in front of a firmware that
// has not finished starting and it leaves the bridge.
//
// The scope is the one that is easy to leave out and impossible to see missing.
// A companion with no default scope originates unscoped traffic, which is
// carried by a different set of repeaters - in a strictly scoped mesh, by none
// of them - so the message is sent, no error is reported at either end, and
// nothing arrives.
func CompanionProvisioning(n scenario.Node, epoch uint32) [][]byte {
	out := [][]byte{proto.SetDeviceTime(epoch)}
	// Only what the scenario actually states. A zeroed radio sent anyway is
	// refused as an illegal argument, and a zeroed TX power is worse: 0 dBm is
	// a legal setting, so the firmware would take it and go quiet.
	if r := n.Radio; r.CentreHz > 0 && r.BandwidthHz > 0 {
		out = append(out, proto.SetRadioParams(uint32(r.CentreHz/1000),
			uint32(r.BandwidthHz), uint8(r.SpreadFactor), uint8(r.CodingRate+4)))
	}
	if n.TxPowerDBm > 0 {
		out = append(out, proto.SetTxPower(uint8(n.TxPowerDBm)))
	}
	if n.Name != "" {
		out = append(out, proto.SetAdvertName(n.Name))
	}
	if n.Kind.Transmits() {
		out = append(out, proto.SetAdvertLatLon(n.Position.Lat, n.Position.Lon))
	}
	// Canonicalised first, because a region is spelled two ways and both are
	// right: the repeater CLI takes `region put sco` while the key on the wire
	// is derived from "#sco". Send under the bare name and every repeater
	// receives the packet, computes a different key, and declines to forward
	// it, with no error at either end.
	if s := canonicalScope(n.DefaultScope); s != "" {
		out = append(out, proto.SetDefaultScope(s, provider.RegionKey(s)))
	}
	return out
}

// CompanionCommand is the protocol payload one line of a schedule means at a
// companion.
//
// meshcore-cli's spellings, because that is the tool people already use against
// real hardware and a schedule should not need a vocabulary of its own. The
// timestamp is a parameter rather than the wall clock: a fixture's traffic is
// placed in simulated time, and a message stamped with the hour it happened to
// be run at is a run that cannot be compared with another.
//
// An unknown word is an error rather than a silent no-op, which is the whole
// lesson of this file.
func CompanionCommand(line string, at time.Time) ([]byte, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil, fmt.Errorf("no command")
	}
	head, args := fields[0], fields[1:]
	switch head {
	case "advert", "a":
		return proto.SendSelfAdvert(false), nil
	case "floodadv":
		return proto.SendSelfAdvert(true), nil
	case "public", "dch":
		if len(args) == 0 {
			return nil, fmt.Errorf("public <message>")
		}
		return proto.SendChannelText(0, at, strings.Join(args, " ")), nil
	case "chan", "ch":
		if len(args) < 2 {
			return nil, fmt.Errorf("chan <number> <message>")
		}
		idx, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, fmt.Errorf("channel number: %w", err)
		}
		return proto.SendChannelText(uint8(idx), at, strings.Join(args[1:], " ")), nil
	}
	return nil, fmt.Errorf("no command %q for a companion: a companion takes "+
		"advert, floodadv, public <message> or chan <number> <message>", head)
}

// canonicalScope is the "#sco" spelling the wire key is derived from.
func canonicalScope(s string) string {
	if s = strings.TrimSpace(s); s == "" {
		return ""
	}
	return "#" + strings.TrimPrefix(s, "#")
}
