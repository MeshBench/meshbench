// Whether real firmware can start at all, asked before the first process is.
//
// In core rather than in the firmware library package, and not by preference:
// playready and runkind consult this gate, and core cannot import a package
// that imports core. That constraint is the right way round here anyway - "may
// this run start" is the session's own question, and the library panel is one
// of the things it consults rather than the thing that decides.
package session

import (
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// buildsMissing is every node that would fail to start, by name.
//
// Checked before the first process is launched rather than discovered node by
// node afterwards: a half-started mesh measures a network that does not exist,
// and the operator sees a status line that never changes.
func (s *Sim) buildsMissing() []string {
	installed := firmware.ListInstalled(firmware.DefaultCacheDir())
	// And whatever an override supplies, because this gate has to ask the
	// question the engine asks. firmware.Resolve tries FindNative before it
	// looks in the cache, so MESHBENCH_NATIVE, or a build sitting beside the
	// simulator, satisfies a node whatever version it is pinned to: the
	// version is never consulted on that path.
	//
	// Reading the cache alone made this stricter than the thing it guards, and
	// it refused runs that would have started. A firmware developer pointed at
	// their own build was told to pin one in the Firmware panel, which is the
	// one thing that would not have helped, and the nightly - which downloads
	// its builds into a directory of its own and names it - failed on a
	// version nothing had asked it to have.
	//
	// One lookup per role rather than per node: it is a stat, and a national
	// network asks it three hundred times.
	overrides := map[string]bool{}
	overridden := func(role string) bool {
		if v, ok := overrides[role]; ok {
			return v
		}
		_, err := firmware.FindNative("", role)
		overrides[role] = err == nil
		return overrides[role]
	}
	var out []string
	for _, n := range s.nodes {
		if !n.Kind.RunsFirmware() {
			continue
		}
		role := string(n.Firmware.Role)
		if role == "" {
			role = string(n.Kind.Application())
		}
		// A node with nothing pinned is reported whatever else is on the
		// machine. An override would in fact start it - Resolve reaches
		// FindNative before it looks at the version - but "no build chosen"
		// is a gap in the scenario rather than a gap in the cache, and one
		// worth seeing before a run rather than inferring from the results.
		// Answering it from an override also made this gate depend on the
		// environment: the same fixture refused on one machine and played on
		// another, which is how it reached the nightly.
		if n.Firmware.Version == "" {
			out = append(out, n.Name+" (no version pinned)")
			continue
		}
		ok, others := BuildAnswers(installed, role, n.Firmware.Version)
		if ok {
			continue
		}
		// A version was chosen and the cache has not got it for this role.
		// An override supplies it anyway, because Resolve never consults the
		// version on that path. A board image is not a native build, so an
		// override of one says nothing about the other.
		if n.Firmware.Board == "" && overridden(role) {
			continue
		}
		// The cache has that version, for another role. This is the pin
		// that used to pass on its name alone: a repeater build on six
		// companions started eighteen of twenty-four nodes and nothing said
		// why, which is the half mesh this gate exists to refuse.
		if len(others) > 0 {
			out = append(out, fmt.Sprintf("%s (%s, pinned to %s, a %s build)",
				n.Name, role, n.Firmware.Version, strings.Join(others, " and ")))
			continue
		}
		out = append(out, fmt.Sprintf("%s (%s %s)", n.Name, role, n.Firmware.Version))
	}
	return out
}

// BuildAnswers says whether an installed build can run this role at this
// version, and when none can, which roles that version is installed for.
//
// The role, and not the version alone. Both this gate and firmware.needed
// used to accept a pin on its version - have[version] beside
// have[role@version] - so a repeater build pinned across a mesh satisfied
// its companions too, and the engine, which resolves by role, then started
// eighteen of twenty-four nodes without a word. A board image carries its
// transport in its role (companion_radio_usb), so a node's role matches an
// image whose role extends it.
func BuildAnswers(installed []firmware.Installed, role, version string) (ok bool, others []string) {
	seen := map[string]bool{}
	for _, b := range installed {
		if b.Version != version {
			continue
		}
		if b.Role == role || strings.HasPrefix(b.Role, role+"_") {
			return true, nil
		}
		if !seen[b.Role] {
			seen[b.Role] = true
			others = append(others, b.Role)
		}
	}
	return false, others
}

// namedFew is the list a refusal quotes: the first few by name, and a count of
// the rest. Naming forty nodes helps nobody; naming four and counting the rest
// does.
//
// Kept apart from buildsMissing because the caller needs both halves and used
// to have only this one. It counted the shortened list, so a mesh with
// fifty-six nodes missing a build was refused with "no firmware for 5 of 58
// nodes ... and 52 more" - a sentence that contradicts itself, and understates
// the gap in the direction that reads as ignorable.
func namedFew(all []string) []string {
	const few = 4
	if len(all) <= few {
		return all
	}
	return append(append([]string{}, all[:few]...),
		fmt.Sprintf("and %d more", len(all)-few))
}

// firmwareStartBlocker names why real firmware cannot start yet, or nil if it
// can - shared by every caller that would otherwise launch half a mesh and
// only find out node by node afterwards. sim.start's own play-button guard
// and a one-shot script both call this rather than each formatting the same
// refusal its own way.
func (s *Sim) firmwareStartBlocker() error {
	missing := s.buildsMissing()
	if len(missing) == 0 {
		return nil
	}
	// Both halves of the count come from the whole list: how many nodes are
	// missing a build, and how many could have run one. len(s.nodes) counted
	// the fleet, which includes the emitter and the observer that never boot
	// firmware at all, so a 58-node Fife mesh reported a denominator two
	// larger than the number of nodes the sentence is about.
	return fmt.Errorf(
		"no firmware for %d of %d nodes, so this run would be half a mesh: %s. "+
			"Pin one in the Firmware panel, or download it there",
		len(missing), s.firmwareNodes(), strings.Join(namedFew(missing), ", "))
}

// firmwareNodes is how many nodes could run firmware at all - the denominator
// the gate's refusal is about. An SDR observer and an emitter are nodes and
// never boot one, so counting them makes the sentence describe a fleet where
// it means to describe the candidates.
func (s *Sim) firmwareNodes() int {
	n := 0
	for _, node := range s.nodes {
		if node.Kind.RunsFirmware() {
			n++
		}
	}
	return n
}

// nodeRole is what a node runs: its pinned role, or the one its kind implies.
func NodeRole(n scenario.Node) string {
	if r := string(n.Firmware.Role); r != "" {
		return r
	}
	return string(n.Kind.Application())
}
