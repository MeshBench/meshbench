// Choosing what a sweep varies and who sends: the arm and sender pickers,
// and the settings an arm can carry.
package workbench

import (
	"sort"

	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// anySetting is the last entry in the vary list: a firmware setting with no
// field of its own.
const anySetting = "a firmware setting..."

// customSetting is the way out of the list, for a switch this build has never
// heard of. The firmware has far more of them than anything here enumerates,
// and the one worth crossing arms on is usually the one nobody thought to add.
const customSetting = "type one in..."

// commonSettings are the CLI switches worth crossing arms on, as the firmware
// spells them.
//
// A list rather than a free-text box because a misspelt setting is accepted by
// the node with "Err - ??" and then reported nowhere: the arm runs, differs
// from its sibling in nothing, and the sweep concludes the setting does not
// matter. Anything not here can still be set for every arm at once through the
// Provisioning panel's extra lines.
var commonSettings = []string{
	"agc.reset.interval",
	"radio.rxgain",
	"radio.fem.rxgain",
	"flood.max.advert",
	"advert.interval",
	"flood.advert.interval",
	"rxdelay.base",
	"txdelay.factor",
	"direct.tx.delay",
	"interference.threshold",
	"multi.acks",
	"allow.read.only",
}

// repeaterBuilds is every repeater firmware this machine holds, newest name
// last, deduplicated: the library lists one entry per board as well as the
// native build, and an arm is a version rather than a file.
func repeaterBuilds(s *state.Snapshot) []string {
	if s == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, b := range s.Builds {
		if b.Role != "simple_repeater" || !b.Native || seen[b.Version] {
			continue
		}
		seen[b.Version] = true
		out = append(out, b.Version)
	}
	sort.Strings(out)
	return out
}

// companionsIn is who can originate a message. A repeater relays; only a
// companion starts anything, which is why the sender list is not every node.
func companionsIn(s *state.Snapshot) []string {
	if s == nil {
		return nil
	}
	var out []string
	for i := range s.Nodes {
		if s.Nodes[i].Kind == "companion" {
			out = append(out, s.Nodes[i].Name)
		}
	}
	sort.Strings(out)
	return out
}

// armsNow and sendersNow are what the session has defined, which is the only
// answer: this panel is one of several things that can define them.
func (c *sweepControls) armsNow() []string {
	if c.snap == nil {
		return nil
	}
	return c.snap.ExperimentArms
}

func (c *sweepControls) sendersNow() []string {
	if c.snap == nil {
		return nil
	}
	return c.snap.ExperimentSenders
}

// armObjs is the shape experiment.define wants its arms in. Labels only: an
// arm defined by crossing carries settings this panel never had, and sending
// a label back is how it says "keep this one" without claiming to know what is
// in it.
func armObjs(labels []any) []any {
	out := make([]any, 0, len(labels))
	for _, l := range labels {
		out = append(out, map[string]any{"label": l})
	}
	return out
}

// senderList draws who will originate, each with a way to take them off.
func (c *sweepControls) senderList(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	if len(c.sendersNow()) == 0 {
		// Red rather than grey: a sweep with no sender runs every cell and
		// measures nothing, and it is the easiest thing here to forget.
		return comp.OneLine(t, t.Sz.Caption, t.P.Bad,
			"nobody sends: every cell would run and measure nothing", false)(gtx)
	}
	// Thirty-five companions is a normal fixture, and "all" is one press away,
	// so this is bounded and scrolls.
	senders := c.sendersNow()
	return capped(t, gtx, &c.sendScroll, len(senders), 180, func(gtx layout.Context, i int) layout.Dimensions {
		n := senders[i]
		if c.senderRm[n] == nil {
			c.senderRm[n] = &comp.Button{Label: "x", Kind: comp.Quiet}
		}
		b := c.senderRm[n]
		return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return b.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
				layout.Flexed(1, comp.OneLine(t, t.Sz.Body, t.P.Ink, n, false)),
			)
		})
	})
}
