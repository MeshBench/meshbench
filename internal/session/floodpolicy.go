// Flood policy: the scattered forwarding knobs, composed (plan §9/14).
//
// AllowAnyFlood, flood.max/flood.max.advert, region put/allowf/denyf and
// path.hash.mode already exist, each its own field on its own struct,
// unable to be compared as one configuration or carried by one arm. A
// FloodPolicy is one value: what a node forwards, how far, and at what
// path-hash size - translated to the exact commands the provisioning panel
// already sends, not a new firmware behaviour.
//
// "Blacklist node names" is in the plan's own mockup and is not in this
// file: MeshCore's CLI has no per-node-name forwarding rule to translate it
// to (ACL/setperm governs a companion's own permissions, not what a
// repeater relays), and inventing one in the engine would decide, at the
// channel, something CLAUDE.md is explicit belongs to the firmware -
// "never add a rule like 'if two transmissions overlap, both fail'" is the
// same principle applied to a different shortcut. Recorded here rather
// than shipped as a filter with no real switch behind it.
package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/console"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// FloodPolicy is one composed configuration: what a node forwards.
type FloodPolicy struct {
	Label string `json:"label"`
	// AllowRegions are put and allowf'd; "*" allows the wildcard
	// (AllowAnyFlood's own "region allowf *"). Empty means "leave the
	// node's own regions alone" - a policy that says nothing about
	// forwarding is not the same claim as one that denies everything.
	AllowRegions []string `json:"allow_regions,omitempty"`
	// DenyRegions are explicitly denyf'd - "*" blocks unscoped flood
	// traffic outright (the CLI's own documented alternative to leaving a
	// node permissive by omission).
	DenyRegions []string `json:"deny_regions,omitempty"`
	// MaxHops caps both flood.max and flood.max.advert together - the
	// plan's single "max hops" field, not the two separate ones
	// provisioning exposes for studies that want them independent. Zero
	// leaves the firmware's own default alone.
	MaxHops int `json:"max_hops,omitempty"`
	// DropFloodAdverts sets flood.max.advert to zero: an advert is heard
	// and never relayed, which is what "drop" means for a packet type
	// this simulator cannot originate scoped in the first place.
	DropFloodAdverts bool `json:"drop_flood_adverts,omitempty"`
	// OneBytePathIDs sets path.hash.mode to its smallest size. Named for
	// what an operator is choosing, not the integer the CLI takes.
	OneBytePathIDs bool `json:"one_byte_path_ids,omitempty"`
}

// relaysFlood is which kinds these settings mean anything for: a repeater
// or room server has region/flood.max/path.hash console commands to answer
// to; a companion has no CLI at all ("a companion has no command line" -
// its own console.type wall, the same one plan §6's sweeps hit), and an
// observer or emitter forwards nothing regardless of what it is told.
func relaysFlood(k scenario.Kind) bool {
	switch k {
	case scenario.SimpleRepeater, scenario.AdvancedRepeater, scenario.RoomServer:
		return true
	}
	return false
}

// commandsFor is the exact console lines this policy sends a transmitting
// node - the same shape as Provisioning.commandsFor, so a policy is read
// the same way the rest of provisioning already is.
func (p FloodPolicy) commandsFor(n scenario.Node) []string {
	if !relaysFlood(n.Kind) {
		return nil
	}
	var out []string
	for _, r := range p.AllowRegions {
		if r == "*" {
			out = append(out, "region allowf *")
			continue
		}
		out = append(out, "region put "+r, "region allowf "+r)
	}
	for _, r := range p.DenyRegions {
		out = append(out, "region denyf "+r)
	}
	if len(p.AllowRegions) > 0 || len(p.DenyRegions) > 0 {
		out = append(out, "region save")
	}
	if p.MaxHops > 0 {
		out = append(out,
			fmt.Sprintf("set flood.max %d", p.MaxHops),
			fmt.Sprintf("set flood.max.advert %d", min(p.MaxHops, 64)))
	}
	if p.DropFloodAdverts {
		out = append(out, "set flood.max.advert 0")
	}
	if p.OneBytePathIDs {
		out = append(out, "set path.hash.mode 1")
	}
	return out
}

// readBack is what commandsFor implies each node should now answer to "get
// <setting>" - the precondition discipline the plan's own risk section
// names: a policy that silently failed to apply must not be allowed to
// count as the policy it claims to be. Keyed by the exact command, so a
// caller can walk the same map it provisioned from.
func (p FloodPolicy) readBack() map[string]string {
	want := map[string]string{}
	if p.MaxHops > 0 {
		want["get flood.max"] = fmt.Sprintf("%d", p.MaxHops)
		want["get flood.max.advert"] = fmt.Sprintf("%d", min(p.MaxHops, 64))
	}
	if p.DropFloodAdverts {
		want["get flood.max.advert"] = "0"
	}
	if p.OneBytePathIDs {
		want["get path.hash.mode"] = "1"
	}
	return want
}

// checkIsolation is plan §14's own "report isolation as an outcome": did a
// message ever reach a node whose own regions do not include any of the
// policy's allowed ones - a named result read straight from the ledger
// (which node heard what), not inferred from how many deliveries there
// were. leakedRegions is which of a leaking node's own regions were the
// ones not on the allow list - "leaked to #ioi", the plan's own phrasing,
// not a list of node names nobody defining the policy chose by hand.
//
// A policy with no AllowRegions makes no containment claim, so there is
// nothing to check against and everything reports clean - the same reading
// String() gives an empty policy: it is not "deny everything", it is "this
// policy said nothing about forwarding".
func checkIsolation(events []engine.Event, regionsOf map[string][]string, allowRegions []string) (clean bool, leakedRegions []string) {
	if len(allowRegions) == 0 {
		return true, nil
	}
	// Regions are spelled two ways in this codebase and both are correct -
	// bare at the CLI ("region put sco"), "#"-prefixed on a node's own
	// saved Regions (fixture.RegionCommands' own documented asymmetry).
	// AllowRegions arrives in the CLI's own bare form (it is what
	// commandsFor sends), so it is normalised here rather than trusting a
	// caller to have matched a fixture's own spelling.
	allowed := map[string]bool{}
	wildcard := false
	for _, r := range allowRegions {
		if r == "*" {
			wildcard = true
		}
		allowed[strings.TrimPrefix(r, "#")] = true
	}
	if wildcard {
		return true, nil // an intentionally open policy has no leak to report
	}
	seenNode := map[string]bool{}
	seenRegion := map[string]bool{}
	for _, ev := range events {
		if ev.Kind != "rx" || seenNode[ev.To] {
			continue
		}
		inAllowed := false
		for _, r := range regionsOf[ev.To] {
			if allowed[strings.TrimPrefix(r, "#")] {
				inAllowed = true
				break
			}
		}
		if !inAllowed {
			seenNode[ev.To] = true
			for _, r := range regionsOf[ev.To] {
				if !seenRegion[r] {
					seenRegion[r] = true
					leakedRegions = append(leakedRegions, r)
				}
			}
		}
	}
	return len(leakedRegions) == 0, leakedRegions
}

// applyFloodPolicy sends the policy to every transmitting node and reads
// each field back before trusting it - the 1.17.1 study's own precondition
// discipline: a setting that was sent and never confirmed is a claim, not
// a result. Returns a non-empty string naming the first mismatch found.
func applyFloodPolicy(ctx context.Context, eng *engine.Engine, nodes []scenario.Node, p FloodPolicy) string {
	want := p.readBack()
	if len(want) == 0 && len(p.AllowRegions) == 0 && len(p.DenyRegions) == 0 {
		return "" // a policy that sets nothing has nothing to confirm
	}
	bufs := map[string]*console.Buf{}
	marks := map[string]int{}
	for _, n := range nodes {
		en, ok := eng.NodeByName(n.Name)
		if !ok || en.Firmware == nil || !relaysFlood(n.Kind) {
			continue
		}
		buf := &console.Buf{Bridge: en.Firmware.Bridge}
		en.Firmware.Bridge.Console(buf)
		bufs[n.Name] = buf
		marks[n.Name] = buf.Mark()
		for _, line := range p.commandsFor(n) {
			if err := en.Firmware.Bridge.Type([]byte(line + "\r\n")); err != nil {
				return n.Name + ": " + err.Error()
			}
		}
		for cmd := range want {
			if err := en.Firmware.Bridge.Type([]byte(cmd + "\r\n")); err != nil {
				return n.Name + ": " + err.Error()
			}
		}
	}
	if err := stepFor(ctx, eng, 2*time.Second); err != nil {
		return "reading the policy back: " + err.Error()
	}
	for name, buf := range bufs {
		// Tokenised rather than matched line-for-line: every line in the
		// buffer carries its own timestamp column and the firmware's reply
		// may carry its own label ("flood.max: 4") or prompt ("> 4") ahead
		// of the value - what is common to every shape is that the value
		// itself shows up as one whole token somewhere in the reply, not
		// stitched to neighbouring digits.
		tokens := map[string]bool{}
		for _, l := range buf.LinesSince(marks[name]) {
			for _, f := range strings.Fields(l) {
				tokens[strings.Trim(f, ">,")] = true
			}
		}
		for cmd, wantVal := range want {
			if !tokens[wantVal] {
				return fmt.Sprintf("%s: %s did not read back %q", name, cmd, wantVal)
			}
		}
	}
	return ""
}
