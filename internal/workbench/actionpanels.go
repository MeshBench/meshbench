package workbench

import (
	"fmt"
	"image/color"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
	"github.com/MeshBench/meshbench/internal/session"
)

// Do is how these panels reach the store. One function rather than a callback
// per control, because every one of them is "run this verb with these
// parameters and say what came back".
type Do func(verb string, params any)

// fleetControls sends one command to every node, or to a filtered subset.
type fleetControls struct {
	bar     actionBar
	command comp.Field
	// kind is a dropdown, not a box: the tokens are the scenario's own -
	// "simple-repeater", not "repeater" - and a box that wants the exact
	// token is a box that silently filters to nobody when given the word a
	// person would type.
	kind comp.Dropdown
	// kindTok is the chosen scenario token; empty sends to every kind.
	kindTok string
	// choose opens the shell's chooser, the one way anything picks from a list.
	choose  func(title string, opts []string, pick func(string))
	send    comp.Button
	regions comp.Field
	setReg  comp.Button
	allow   comp.Button
	// quick are the lines people send often enough that typing them is the
	// only thing standing between a mesh and saying something. The old
	// workbench put them behind a button each; the same idea, and they fill
	// the box rather than going out the door, so the exact line is read
	// before forty nodes are asked to obey it.
	quick [4]comp.Button
	do    Do
	built bool
}

// fleetKinds is the kind filter, in a person's words on the left and the
// scenario's own token on the right.
var fleetKinds = []struct{ label, token string }{
	{"every kind", ""},
	{"repeaters", "simple-repeater"},
	{"advanced repeaters", "advanced-repeater"},
	{"room servers", "room-server"},
	{"companions", "companion"},
	{"observers", "sdr-observer"},
	{"emitters", "emitter"},
}

// fleetQuick is what those buttons put in the box.
var fleetQuick = [4]struct{ label, cmd string }{
	{"advert", "advert"},
	{"flood advert", "floodadv"},
	{"what version", "ver"},
	{"how it is", "status"},
}

func (c *fleetControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.command.Hint = "a MeshCore command, sent to every node"
		c.command.Editor.SingleLine = true
		c.kind.Value = "every kind"
		c.kind.OnOpen = func() {
			if c.choose == nil {
				return
			}
			opts := make([]string, 0, len(fleetKinds))
			for _, k := range fleetKinds {
				opts = append(opts, k.label)
			}
			c.choose("Send to which kind?", opts, func(picked string) {
				for _, k := range fleetKinds {
					if k.label == picked {
						c.kindTok, c.kind.Value = k.token, k.label
					}
				}
			})
		}
		c.regions.Hint = "regions, space separated"
		c.regions.Editor.SingleLine = true
		c.send.Label, c.send.Kind = "send to the fleet", comp.Primary
		c.setReg.Label, c.setReg.Kind = "set regions", comp.Secondary
		c.allow.Label, c.allow.Kind = "allow any flood", comp.Secondary
		c.bar.fields = []*comp.Field{&c.command}
		c.bar.extras = []func(*theme.Theme, layout.Context) layout.Dimensions{
			func(t *theme.Theme, gtx layout.Context) layout.Dimensions {
				return c.kind.Layout(t, gtx)
			},
		}
		c.bar.buttons = []*comp.Button{&c.send}
		for i := range c.quick {
			c.quick[i].Label, c.quick[i].Kind = fleetQuick[i].label, comp.Quiet
		}
		c.bar.note = "a command that changes what the nodes are makes anything " +
			"already measured a different mesh; the reply says so before it is sent"
		c.built = true
	}
	if c.send.Click.Clicked(gtx) && c.do != nil {
		c.do("fleet.send", map[string]any{
			"command": fieldText(&c.command), "kind": c.kindTok,
		})
	}
	if c.setReg.Click.Clicked(gtx) && c.do != nil {
		var rs []any
		for _, r := range splitFields(fieldText(&c.regions)) {
			rs = append(rs, r)
		}
		c.do("nodes.regions", map[string]any{"regions": rs})
	}
	if c.allow.Click.Clicked(gtx) && c.do != nil {
		c.do("nodes.allow_flood", map[string]any{"on": true})
	}
	for i := range c.quick {
		if c.quick[i].Click.Clicked(gtx) {
			c.command.Editor.SetText(fleetQuick[i].cmd)
		}
	}
	second := actionBar{
		fields:  []*comp.Field{&c.regions},
		buttons: []*comp.Button{&c.setReg, &c.allow},
	}
	third := actionBar{
		buttons: []*comp.Button{&c.quick[0], &c.quick[1], &c.quick[2], &c.quick[3]},
		note: "nothing in a mesh speaks until it is asked to: MeshCore will not " +
			"advertise more often than hourly, so a short run is made to talk " +
			"with advert",
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.bar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return second.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return third.layout(t, gtx) }),
	)
}

// scheduleControls adds sends and assertions to a scenario.
type scheduleControls struct {
	bar     actionBar
	node    comp.Field
	at      comp.Field
	every   comp.Field
	add     comp.Button
	clear   comp.Button
	kind    comp.Field
	atLeast comp.Field
	addAss  comp.Button
	check   comp.Button
	do      Do
	built   bool
}

func (c *scheduleControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.node.Hint = "node (blank: the selected one)"
		c.at.Hint = "at, seconds"
		c.every.Hint = "every, seconds (blank: once)"
		c.kind.Hint = "assert: delivered, sent"
		c.atLeast.Hint = "at least"
		for _, f := range []*comp.Field{&c.node, &c.at, &c.every, &c.kind, &c.atLeast} {
			f.Editor.SingleLine = true
		}
		c.add.Label, c.add.Kind = "add send", comp.Primary
		c.clear.Label, c.clear.Kind = "clear sends", comp.Quiet
		c.addAss.Label, c.addAss.Kind = "add assertion", comp.Secondary
		c.check.Label, c.check.Kind = "check now", comp.Primary
		c.bar.fields = []*comp.Field{&c.node, &c.at, &c.every}
		c.bar.buttons = []*comp.Button{&c.add, &c.clear}
		c.built = true
	}
	if c.add.Click.Clicked(gtx) && c.do != nil {
		node := fieldText(&c.node)
		if node == "" {
			node = selectedNodeName(s)
		}
		p := map[string]any{"node": node}
		if v, ok := num(&c.at); ok {
			p["at_ms"] = v * 1000
		}
		if v, ok := num(&c.every); ok {
			p["every_ms"] = v * 1000
		}
		c.do("schedule.add", p)
	}
	if c.clear.Click.Clicked(gtx) && c.do != nil {
		c.do("schedule.clear", nil)
	}
	if c.addAss.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{"kind": fieldText(&c.kind)}
		if v, ok := num(&c.atLeast); ok {
			p["at_least"] = v
		}
		c.do("assert.add", p)
	}
	if c.check.Click.Clicked(gtx) && c.do != nil {
		c.do("assert.check", nil)
	}
	second := actionBar{
		fields:  []*comp.Field{&c.kind, &c.atLeast},
		buttons: []*comp.Button{&c.addAss, &c.check},
		note:    "an assertion whose kind this build does not understand fails rather than passing quietly",
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.bar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return second.layout(t, gtx) }),
	)
}

// importControls is the order that matters, as buttons in that order.
type importControls struct {
	bar    actionBar
	url    comp.Field
	fetch  comp.Button
	commit comp.Button
	infer  comp.Button
	apply  comp.Button
	do     Do
	built  bool
}

func (c *importControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.url.Hint = "a CoreScope deployment URL"
		c.url.Editor.SingleLine = true
		c.fetch.Label, c.fetch.Kind = "1. fetch", comp.Primary
		c.commit.Label, c.commit.Kind = "2. commit", comp.Secondary
		c.infer.Label, c.infer.Kind = "3. read traffic", comp.Secondary
		c.apply.Label, c.apply.Kind = "4. apply regions", comp.Primary
		c.bar.fields = []*comp.Field{&c.url}
		c.bar.buttons = []*comp.Button{&c.fetch, &c.commit, &c.infer, &c.apply}
		c.bar.note = "numbered because the order matters and every step has been " +
			"skipped: a mesh with regions inferred but not applied transmits " +
			"everything, relays nothing, and reports no error"
		c.built = true
	}
	if c.fetch.Click.Clicked(gtx) && c.do != nil {
		c.do("import.fetch", map[string]any{"url": fieldText(&c.url)})
	}
	if c.commit.Click.Clicked(gtx) && c.do != nil {
		c.do("import.commit", map[string]any{"strategy": "replace-all"})
	}
	if c.infer.Click.Clicked(gtx) && c.do != nil {
		c.do("infer.run", map[string]any{"hours": 168})
	}
	if c.apply.Click.Clicked(gtx) && c.do != nil {
		c.do("infer.apply", nil)
	}
	return c.bar.layout(t, gtx)
}

// boundaryControls chooses the study area.
type boundaryControls struct {
	bar    actionBar
	place  comp.Field
	margin comp.Field
	search comp.Button
	accept comp.Button
	prune  comp.Button
	do     Do
	built  bool
}

func (c *boundaryControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.place.Hint = "a place: Fife, Scotland, Perth and Kinross"
		c.margin.Hint = "margin km"
		c.place.Editor.SingleLine = true
		c.margin.Editor.SingleLine = true
		c.search.Label, c.search.Kind = "search", comp.Primary
		c.accept.Label, c.accept.Kind = "accept it", comp.Secondary
		c.prune.Label, c.prune.Kind = "delete what is outside", comp.Destructive
		c.bar.fields = []*comp.Field{&c.place, &c.margin}
		c.bar.buttons = []*comp.Button{&c.search, &c.accept, &c.prune}
		c.bar.note = "the study area decides which nodes are measured, not which " +
			"packets are forwarded - the margin keeps what interferes from outside it"
		c.built = true
	}
	if c.search.Click.Clicked(gtx) && c.do != nil {
		c.do("boundary.set", map[string]any{"query": fieldText(&c.place)})
	}
	if c.accept.Click.Clicked(gtx) && c.do != nil {
		c.do("boundary.accept", map[string]any{"name": fieldText(&c.place)})
	}
	if c.prune.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{}
		if v, ok := num(&c.margin); ok {
			p["margin_km"] = v
		}
		c.do("boundary.prune", p)
	}
	return c.bar.layout(t, gtx)
}

// validateControls is the calibration chain.
type validateControls struct {
	bar   actionBar
	hours comp.Field
	db    comp.Field
	fetch comp.Button
	cal   comp.Button
	uncal comp.Button
	do    Do
	built bool
}

func (c *validateControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.hours.Hint = "hours to look back"
		c.db.Hint = "excess loss dB (blank: what was measured)"
		c.hours.Editor.SingleLine = true
		c.db.Editor.SingleLine = true
		c.fetch.Label, c.fetch.Kind = "fetch and compare", comp.Primary
		c.cal.Label, c.cal.Kind = "apply calibration", comp.Secondary
		c.uncal.Label, c.uncal.Kind = "back to the default", comp.Quiet
		c.bar.fields = []*comp.Field{&c.hours, &c.db}
		c.bar.buttons = []*comp.Button{&c.fetch, &c.cal, &c.uncal}
		c.bar.note = "positive residual means the model predicted more signal than " +
			"was heard, so it is optimistic and the excess loss should go up"
		c.built = true
	}
	if c.fetch.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{}
		if v, ok := num(&c.hours); ok {
			p["hours"] = v
		}
		c.do("validate.fetch", p)
	}
	if c.cal.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{}
		if v, ok := num(&c.db); ok {
			p["db"] = v
		}
		c.do("validate.calibrate", p)
	}
	if c.uncal.Click.Clicked(gtx) && c.do != nil {
		c.do("validate.uncalibrate", nil)
	}
	return c.bar.layout(t, gtx)
}

// planningControls asks the three network-wide questions.
type planningControls struct {
	bar   actionBar
	best  comp.Button
	gaps  comp.Button
	red   comp.Button
	here  comp.Button
	do    Do
	built bool
}

func (c *planningControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.best.Label, c.best.Kind = "best server", comp.Primary
		c.gaps.Label, c.gaps.Kind = "gaps", comp.Secondary
		c.red.Label, c.red.Kind = "redundancy", comp.Secondary
		c.here.Label, c.here.Kind = "coverage from the selected node", comp.Secondary
		c.bar.buttons = []*comp.Button{&c.best, &c.gaps, &c.red, &c.here}
		c.bar.note = "for a person with a handheld at 1.5 m, which is the assumption " +
			"every one of these makes"
		c.built = true
	}
	for b, mode := range map[*comp.Button]string{
		&c.best: "best", &c.gaps: "gaps", &c.red: "redundancy", &c.here: "node",
	} {
		if b.Click.Clicked(gtx) && c.do != nil {
			c.do("coverage.start", map[string]any{"mode": mode})
		}
	}
	return c.bar.layout(t, gtx)
}

func splitFields(s string) []string {
	var out []string
	out = append(out, fieldsOf(s)...)
	return out
}

func fieldsOf(s string) []string {
	var out, cur []rune
	_ = out
	var res []string
	for _, r := range s {
		if r == ' ' || r == ',' || r == '\t' {
			if len(cur) > 0 {
				res = append(res, string(cur))
				cur = cur[:0]
			}
			continue
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		res = append(res, string(cur))
	}
	return res
}

var _ = fmt.Sprintf

// benchControls is the companion bench: a mesh and an endpoint, then the
// faults a happy path never reaches.
type benchControls struct {
	bar    actionBar
	node   comp.Field
	tcp    comp.Button
	serial comp.Button
	drop   comp.Button
	stray  comp.Button
	conn   comp.Button
	msg    comp.Field
	send   comp.Button
	advert comp.Button
	do     Do
	built  bool
}

func (c *benchControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.node.Hint = "companion (blank: the selected node)"
		c.msg.Hint = "a message to send from it"
		c.node.Editor.SingleLine = true
		c.msg.Editor.SingleLine = true
		c.tcp.Label, c.tcp.Kind = "serve TCP", comp.Primary
		c.serial.Label, c.serial.Kind = "serve serial", comp.Secondary
		c.drop.Label, c.drop.Kind = "drop clients", comp.Destructive
		c.stray.Label, c.stray.Kind = "inject a stray frame", comp.Secondary
		c.conn.Label, c.conn.Kind = "connect as a client", comp.Primary
		c.send.Label, c.send.Kind = "send", comp.Secondary
		c.advert.Label, c.advert.Kind = "advert", comp.Secondary
		c.bar.fields = []*comp.Field{&c.node}
		c.bar.buttons = []*comp.Button{&c.tcp, &c.serial, &c.drop, &c.stray}
		c.bar.note = "both transports carry the firmware's own serial protocol byte " +
			"for byte; the faults are what an application that reconnects cleanly survives"
		c.built = true
	}
	who := func() string {
		if n := fieldText(&c.node); n != "" {
			return n
		}
		return selectedNodeName(s)
	}
	if c.tcp.Click.Clicked(gtx) && c.do != nil {
		c.do("bench.serve", map[string]any{"node": who(), "kind": "tcp"})
	}
	if c.serial.Click.Clicked(gtx) && c.do != nil {
		c.do("bench.serve", map[string]any{"node": who(), "kind": "serial"})
	}
	if c.drop.Click.Clicked(gtx) && c.do != nil {
		c.do("bench.drop", nil)
	}
	if c.stray.Click.Clicked(gtx) && c.do != nil {
		c.do("bench.stray", map[string]any{"node": who()})
	}
	if c.conn.Click.Clicked(gtx) && c.do != nil {
		c.do("companion.connect", map[string]any{"node": who()})
	}
	if c.send.Click.Clicked(gtx) && c.do != nil {
		c.do("companion.send", map[string]any{"node": who(), "text": fieldText(&c.msg)})
	}
	if c.advert.Click.Clicked(gtx) && c.do != nil {
		c.do("companion.advert", map[string]any{"node": who()})
	}
	second := actionBar{
		fields:  []*comp.Field{&c.msg},
		buttons: []*comp.Button{&c.conn, &c.send, &c.advert},
		note:    "connecting claims the node's port, so its console goes quiet until you disconnect",
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.bar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return second.layout(t, gtx) }),
	)
}

// feedControls replays the real network's traffic into the simulation.
type feedControls struct {
	bar   actionBar
	url   comp.Field
	start comp.Button
	stop  comp.Button
	do    Do
	built bool
}

func (c *feedControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.url.Hint = "a CoreScope deployment URL"
		c.url.Editor.SingleLine = true
		c.start.Label, c.start.Kind = "start live feed", comp.Primary
		c.stop.Label, c.stop.Kind = "stop", comp.Quiet
		c.bar.fields = []*comp.Field{&c.url}
		c.bar.buttons = []*comp.Button{&c.start, &c.stop}
		c.bar.note = "packets are taken at their first hop and re-transmitted by the " +
			"same-named node here, so what you watch is the simulated mesh relaying real traffic"
		c.built = true
	}
	if c.start.Click.Clicked(gtx) && c.do != nil {
		c.do("feed.pull", map[string]any{"url": fieldText(&c.url)})
	}
	if c.stop.Click.Clicked(gtx) && c.do != nil {
		c.do("feed.stop", nil)
	}
	return c.bar.layout(t, gtx)
}

// sweepControls defines an A/B experiment: arms, seeds, sender, timing.
type sweepControls struct {
	// addArm and sender are dropdowns rather than boxes because both are a
	// choice from a list this machine already knows: the firmware library, and
	// the companions in the scenario. Typing a version was the old shape and it
	// was wrong twice over - a misremembered tag defines an arm that cannot
	// start, and nothing in the panel said which tags existed.
	addArm comp.Dropdown
	sender comp.Dropdown

	// arms are the versions chosen, in the order chosen. An arm is one column
	// of the comparison, so this is what "define" turns into experiment.vary.
	// armRm is one remove button per arm, pooled by version: a widget rebuilt
	// each frame is a widget that never registers a press.
	armRm map[string]*comp.Button

	// senders are the companions that will originate. A list, not one name:
	// a single originator makes every seed of an arm return the same numbers,
	// so the seed cannot bound the noise and no difference between arms has
	// anything to be called larger than.
	senders   []string
	senderRm  map[string]*comp.Button
	allSend   comp.Button
	noneSend  comp.Button
	spreadFld comp.Field
	bytesFld  comp.Field

	// scopeFld is the region every sender originates under.
	//
	// A box rather than a dropdown, because the regions a scenario's repeaters
	// hold are theirs and not a list this panel knows: they are inferred from
	// captured traffic, and a menu of the ones we happen to have seen would be
	// wrong the first time somebody imports a mesh we have not.
	//
	// It has to be here at all because a sweep that sends unscoped is carried
	// by a different set of repeaters and measures a different network, with
	// nothing at either end reporting it.
	scopeFld comp.Field

	// vary crosses a parameter across the arms already defined. This is the
	// gesture the whole panel exists for: three path hash sizes by two
	// firmware versions is six arms, and without it the matrix can only ever
	// compare builds.
	varyDD   comp.Dropdown
	varyName string // the VaryParams name, or set:<setting>
	varyVals comp.Field
	addArms  comp.Button

	// Long lists scroll. Thirty-five companions pushed the panels below them
	// off the bottom of the window, and there was no way to reach the end of
	// the list or anything under it.
	armScroll, sendScroll, panelScroll widget.List

	seeds  comp.Field
	runFor comp.Field
	// snap is the latest snapshot, kept because a dropdown's chooser runs
	// after the frame that opened it.
	snap   *state.Snapshot
	define comp.Button
	start  comp.Button
	stop   comp.Button
	export comp.Button
	// choose opens the shell's chooser, the one way anything picks from a list.
	choose func(title string, opts []string, pick func(string))
	// askText is for a setting no list could hold: the firmware has more
	// switches than this build knows the names of, and the interesting one is
	// usually the one nobody enumerated.
	askText func(title, hint, initial string, got func(string))
	do      Do
	built   bool
	// copyID copies the currently defined sweep's ID to the clipboard, so an
	// operator comparing notes with somebody else does not have to retype it.
	copyID comp.Button
}

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

func (c *sweepControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.seeds.Hint = "seeds, e.g. 1 2 3 4"
		c.runFor.Hint = "run for, seconds"
		for _, f := range []*comp.Field{&c.seeds, &c.runFor} {
			f.Editor.SingleLine = true
		}
		c.addArm.Label, c.addArm.Value = "arms: repeater firmware to compare", "add an arm..."
		c.sender.Label, c.sender.Value = "sender", "choose a companion..."
		c.armRm = map[string]*comp.Button{}
		c.senderRm = map[string]*comp.Button{}
		c.allSend.Label, c.allSend.Kind = "all", comp.Quiet
		c.noneSend.Label, c.noneSend.Kind = "none", comp.Quiet
		c.spreadFld.Hint = "fired N s apart (0 = together)"
		c.bytesFld.Hint = "message size, bytes (0 = a short label)"
		c.scopeFld.Hint = "scope, e.g. sco (blank = unscoped)"
		c.spreadFld.Editor.SingleLine = true
		c.bytesFld.Editor.SingleLine = true
		c.scopeFld.Editor.SingleLine = true
		c.varyDD.Label, c.varyDD.Value = "vary a parameter across arms", "choose a parameter..."
		c.varyVals.Hint = "values, comma separated"
		c.varyVals.Editor.SingleLine = true
		c.addArms.Label, c.addArms.Kind = "add arms", comp.Secondary
		c.armScroll.Axis, c.sendScroll.Axis = layout.Vertical, layout.Vertical
		c.panelScroll.Axis = layout.Vertical
		c.varyDD.OnOpen = func() {
			if c.choose == nil {
				return
			}
			opts := make([]string, 0, len(session.VaryParams)+1)
			for _, p := range session.VaryParams {
				opts = append(opts, p.Label)
			}
			// Anything the enumerated list does not cover. The AGC reset
			// interval is the example that matters: it is what makes the
			// 1.17.1 gain fault reachable, and nothing here has a field for it.
			opts = append(opts, anySetting)
			c.choose("Vary which parameter?", opts, func(picked string) {
				if picked == anySetting {
					c.choose("Which firmware setting?",
						append([]string{customSetting}, commonSettings...),
						func(name string) {
							if name == customSetting {
								if c.askText == nil {
									return
								}
								c.askText("Firmware setting", "as the CLI spells it, e.g. rxdelay.base", "",
									func(typed string) {
										typed = strings.TrimSpace(strings.TrimPrefix(typed, "set "))
										if typed == "" {
											return
										}
										c.varyName, c.varyDD.Value = "set:"+typed, typed
										c.varyVals.Editor.SetText("")
									})
								return
							}
							c.varyName = "set:" + name
							c.varyDD.Value = name
							c.varyVals.Editor.SetText("")
						})
					return
				}
				for _, p := range session.VaryParams {
					if p.Label == picked {
						c.varyName, c.varyDD.Value = p.Name, p.Label
						c.varyVals.Editor.SetText(p.Defaults)
					}
				}
			})
		}
		c.define.Label, c.define.Kind = "define", comp.Secondary
		c.start.Label, c.start.Kind = "run it", comp.Primary
		c.stop.Label, c.stop.Kind = "stop", comp.Destructive
		c.export.Label, c.export.Kind = "export", comp.Quiet
		c.copyID.Label, c.copyID.Kind = "copy id", comp.Quiet
		c.addArm.OnOpen = func() {
			if c.choose == nil {
				return
			}
			opts := repeaterBuilds(c.snap)
			if len(opts) == 0 {
				if c.do != nil {
					c.do("ui.said", "no repeater firmware in the library to compare; "+
						"download one from the Firmware panel first")
				}
				return
			}
			c.choose("Which firmware is this arm?", opts, func(picked string) {
				if c.do != nil {
					c.do("experiment.vary", map[string]any{
						"parameter": "repeater_version", "values": []any{picked}})
				}
			})
		}
		c.sender.OnOpen = func() {
			if c.choose == nil {
				return
			}
			opts := companionsIn(c.snap)
			if len(opts) == 0 {
				if c.do != nil {
					c.do("ui.said", "no companion in this network to originate a message")
				}
				return
			}
			c.choose("Who sends?", opts, func(picked string) {
				now := c.sendersNow()
				for _, n := range now {
					if n == picked {
						return
					}
				}
				var next []any
				for _, n := range now {
					next = append(next, n)
				}
				next = append(next, picked)
				if c.do != nil {
					c.do("experiment.senders", map[string]any{"senders": next})
				}
			})
		}
		c.built = true
	}
	// The dropdowns are opened from a callback that runs outside this frame, so
	// the snapshot they read has to be kept rather than captured.
	c.snap = s
	if c.define.Click.Clicked(gtx) && c.do != nil {
		// Said aloud when nothing was defined.
		//
		// Every branch below is guarded on its box holding something, so
		// pressing this with the boxes empty did nothing at all and reported
		// nothing at all - which is indistinguishable from a button that is
		// not connected.
		asked := 0
		var ss []any
		for _, v := range splitFields(fieldText(&c.seeds)) {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				ss = append(ss, n)
			}
		}
		if len(ss) > 0 {
			asked++
			c.do("experiment.seeds", map[string]any{"seeds": ss})
		}

		if v, ok := num(&c.spreadFld); ok {
			asked++
			c.do("experiment.define", map[string]any{"spread_ms": v * 1000})
		}
		if v, ok := num(&c.bytesFld); ok && v > 0 {
			asked++
			c.do("experiment.define", map[string]any{"bytes": v})
		}
		// Sent even when blank, because clearing it is a real instruction:
		// "send unscoped" has to be reachable once a scope has been set.
		asked++
		c.do("experiment.define", map[string]any{
			"scope": strings.TrimSpace(fieldText(&c.scopeFld))})
		if v, ok := num(&c.runFor); ok {
			asked++
			c.do("experiment.base", map[string]any{"run_for_ms": v * 1000})
		}
		if asked == 0 {
			c.do("ui.said", "nothing to define: add an arm to compare, "+
				"the seeds to run each on, who sends, or how long a cell runs")
		}
	}
	if c.addArms.Click.Clicked(gtx) && c.do != nil {
		var vs []any
		for _, v := range strings.Split(fieldText(&c.varyVals), ",") {
			if v = strings.TrimSpace(v); v != "" {
				vs = append(vs, v)
			}
		}
		switch {
		case c.varyName == "":
			c.do("ui.said", "choose a parameter to vary first")
		case len(vs) == 0:
			c.do("ui.said", "no values to cross: type them comma separated")
		default:
			c.do("experiment.vary", map[string]any{
				"parameter": c.varyName, "values": vs})
		}
	}
	if c.start.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.start", nil)
	}
	if c.stop.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.stop", nil)
	}
	if c.export.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.export", nil)
	}
	if c.copyID.Click.Clicked(gtx) && c.do != nil {
		id := ""
		if s != nil {
			id = s.ExperimentID
		}
		if id == "" {
			c.do("ui.said", "no experiment is defined yet: fill in versions or seeds above, or press define")
		} else {
			copyText(gtx, id)
			c.do("ui.said", "experiment ID copied: "+id)
		}
	}
	// The definition lives in the session, not here: the control socket defines
	// arms too, and a panel holding its own copy showed "no arms yet" over a
	// session with four. Removing one asks for the list without it.
	arms := c.armsNow()
	for i := range arms {
		b := c.armRm[arms[i]]
		if b == nil || !b.Click.Clicked(gtx) {
			continue
		}
		var keep []any
		for j, a := range arms {
			if j != i {
				keep = append(keep, a)
			}
		}
		if c.do != nil {
			c.do("experiment.define", map[string]any{"arms": armObjs(keep)})
		}
		break
	}

	sendersNow := c.sendersNow()
	for i := range sendersNow {
		b := c.senderRm[sendersNow[i]]
		if b == nil || !b.Click.Clicked(gtx) {
			continue
		}
		var keep []any
		for j, n := range sendersNow {
			if j != i {
				keep = append(keep, n)
			}
		}
		if c.do != nil {
			c.do("experiment.senders", map[string]any{"senders": keep})
		}
		break
	}
	if c.allSend.Click.Clicked(gtx) && c.do != nil {
		var all []any
		for _, n := range companionsIn(c.snap) {
			all = append(all, n)
		}
		c.do("experiment.senders", map[string]any{"senders": all})
	}
	if c.noneSend.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.senders", map[string]any{"senders": []any{}})
	}

	bar := actionBar{
		fields:  []*comp.Field{&c.seeds, &c.runFor, &c.spreadFld, &c.bytesFld, &c.scopeFld},
		buttons: []*comp.Button{&c.define, &c.start, &c.stop, &c.export},
		note: "a message is originated by a companion, so the sender has to be " +
			"one; two seeds that agree exactly are one draw repeated, not a spread",
	}
	// The whole panel scrolls.
	//
	// It is a column of a dozen controls in a fixed-width rail, and the ones
	// that matter most - define, run it, and the estimate saying what run it is
	// about to cost - are at the bottom. Without this they are simply not on
	// the screen, and nothing indicates that there is more.
	sections := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return c.addArm.Layout(t, gtx)
			})
		},
		func(gtx layout.Context) layout.Dimensions { return c.armList(t, gtx) },
		// Crossing, under the list it crosses onto.
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return c.varyDD.Layout(t, gtx)
			})
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return c.varyVals.Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.addArms.Layout(t, gtx)
					}),
				)
			})
		},
		comp.Text(t, t.Sz.Caption, t.P.Faint,
			"crossed onto the arms above: three values by two arms is six"),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return c.sender.Layout(t, gtx) })
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return c.allSend.Layout(t, gtx)
						}),
						layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return c.noneSend.Layout(t, gtx)
						}),
					)
				})
		},
		func(gtx layout.Context) layout.Dimensions { return c.senderList(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return bar.layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return c.identity(t, gtx, s) },
		func(gtx layout.Context) layout.Dimensions { return c.estimate(t, gtx) },
	}
	sections = append(sections, func(gtx layout.Context) layout.Dimensions {
		return layout.Spacer{Height: t.Sp.M}.Layout(gtx)
	})
	return comp.List(t, &c.panelScroll, len(sections), func(gtx layout.Context, i int) layout.Dimensions {
		return sections[i](gtx)
	})(gtx)
}

// armList draws the chosen arms, each with a way to take it off again.
func (c *sweepControls) armList(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	if len(c.armsNow()) == 0 {
		return comp.Text(t, t.Sz.Caption, t.P.Faint,
			"no arms yet - add one, or cross a parameter across them")(gtx)
	}
	// Capped and scrolling. Six arms crossed from two parameters is already
	// taller than the panel, and a list that grows without bound pushes
	// everything under it off the window with no way back.
	arms := c.armsNow()
	return capped(t, gtx, &c.armScroll, len(arms), 200, func(gtx layout.Context, i int) layout.Dimensions {
		a := arms[i]
		if c.armRm[a] == nil {
			c.armRm[a] = &comp.Button{Label: "x", Kind: comp.Quiet}
		}
		b := c.armRm[a]
		return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return b.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
				layout.Flexed(1, comp.OneLine(t, t.Sz.Body, t.P.Ink, a, true)),
			)
		})
	})
}

// capped draws a list inside a bounded, scrolling area.
func capped(t *theme.Theme, gtx layout.Context, l *widget.List, n, maxDp int,
	row func(layout.Context, int) layout.Dimensions) layout.Dimensions {
	h := gtx.Dp(unit.Dp(maxDp))
	if gtx.Constraints.Max.Y < h {
		h = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Max.Y = h
	return comp.List(t, l, n, row)(gtx)
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

// estimate says what pressing "run it" is about to cost, before it is pressed.
func (c *sweepControls) estimate(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	seeds := len(splitFields(fieldText(&c.seeds)))
	arms := len(c.armsNow())
	if seeds == 0 || arms == 0 {
		return layout.Dimensions{}
	}
	runs := arms * seeds
	line := fmt.Sprintf("%d arms x %d seeds = %d runs", arms, seeds, runs)
	if secs, ok := num(&c.runFor); ok && secs > 0 {
		line += fmt.Sprintf(", about %s", roughDuration(runs*int(secs)))
	}
	return layout.Inset{Top: t.Sp.S}.Layout(gtx,
		comp.Text(t, t.Sz.Caption, t.P.Dim, line))
}

// roughDuration is minutes and seconds, because a sweep is measured in the
// first and nobody waits on the second.
func roughDuration(secs int) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
}

// armCol is one column of the matrix.
//
// Better says which way is good, because it decides the colour of a delta and
// getting it backwards makes a regression look like a win. Most of these are
// costs - transmissions, collisions, airtime - where less is better, which is
// the opposite of the convention most tables use.
type armCol struct {
	title  string
	width  int
	value  func(state.ArmSummary) float64
	unit   string
	better int // -1 less is better, +1 more is better, 0 no delta
}

var armCols = []armCol{
	{title: "arm", width: 230},
	{title: "runs", width: 60,
		value: func(a state.ArmSummary) float64 { return float64(a.Runs) }},
	{title: "tx", width: 90, better: -1,
		value: func(a state.ArmSummary) float64 { return a.TX }},
	{title: "rx", width: 90, better: +1,
		value: func(a state.ArmSummary) float64 { return a.RX }},
	{title: "delivered", width: 100, better: +1,
		value: func(a state.ArmSummary) float64 { return a.Delivered }},
	{title: "redundant", width: 100, better: -1,
		value: func(a state.ArmSummary) float64 { return a.Redundant }},
	{title: "collisions", width: 100, better: -1,
		value: func(a state.ArmSummary) float64 { return a.Collided }},
	{title: "airtime", width: 100, unit: " s", better: -1,
		value: func(a state.ArmSummary) float64 { return a.AirtimeMs / 1000 }},
	{title: "seed spread", width: 110,
		value: func(a state.ArmSummary) float64 { return a.RXSpread * 100 }, unit: "%"},
}

// identity is the ID strip: what the currently defined sweep hashes to, and
// what went into that hash - fixture, firmware, geometry, and the build this
// binary was made from, dirty tree and all. "Can somebody else reproduce
// this?" is answered by whether their strip reads the same as this one.
func (c *sweepControls) identity(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	// Drawn even with nothing defined yet, rather than hidden: a button that
	// only lays out once something exists to copy is a button no pointer can
	// ever find beforehand, and the control audit presses every one of them.
	shownID := "not defined yet"
	var id state.ExperimentIdentity
	if s != nil && s.ExperimentID != "" {
		shownID, id = s.ExperimentID, s.ExperimentIdentity
	}
	fixture := "no fixture loaded"
	if id.Fixture != "" {
		fixture = filepath.Base(id.Fixture)
	}
	firmware := "no firmware pinned"
	if len(id.Firmware) > 0 {
		firmware = strings.Join(id.Firmware, ", ")
	}
	build := "unknown build"
	buildColor := t.P.Dim
	if id.MeshBench != "" {
		if id.Dirty {
			build = "meshbench " + id.MeshBench + " (dirty tree)"
			buildColor = t.P.Warn
		} else {
			build = "meshbench " + id.MeshBench + " (clean tree)"
		}
	}
	line := fmt.Sprintf("fixture %s  ·  firmware %s  ·  geometry %s",
		fixture, firmware, id.GeometryFP)
	return layout.Inset{Top: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle,
					Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "experiment ")),
							layout.Rigid(comp.Mono(t, t.Sz.Body, t.P.Ink, shownID)),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.copyID.Layout(t, gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
			layout.Rigid(comp.OneLine(t, t.Sz.Caption, t.P.Dim, line, false)),
			layout.Rigid(comp.OneLine(t, t.Sz.Caption, buildColor, build, true)),
		)
	})
}

// sweepResults is what the arms came back with, and whether it is a result.
type sweepResults struct {
	init   bool
	narrow comp.Button
	do     Do
}

func (p *sweepResults) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.narrow.Label, p.narrow.Kind = "narrow it", comp.Secondary
		p.init = true
	}
	if s == nil {
		return layout.Dimensions{}
	}
	if p.narrow.Click.Clicked(gtx) && p.do != nil {
		if s.ExperimentNarrowSeeds > 0 {
			p.do("experiment.extend", map[string]any{"count": s.ExperimentNarrowSeeds})
		} else {
			p.do("ui.said", "nothing to narrow: run the sweep first, or every arm is already tight")
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.ExperimentWarning == "" {
				return layout.Dimensions{}
			}
			// Said above the numbers, not below them: a warning underneath a
			// table is read after somebody has already believed it.
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx,
				comp.OneLine(t, t.Sz.Body, t.P.Warn,
					"not a result yet: "+s.ExperimentWarning, false))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.ExperimentVerdict == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx,
				comp.OneLine(t, t.Sz.Body, t.P.Ink, s.ExperimentVerdict, false))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Drawn even with nothing to narrow yet, rather than hidden: a
			// button that only lays out once a sweep has run is a button no
			// pointer can ever find beforehand, and the control audit presses
			// every one of them.
			caption, mark := s.ExperimentNarrow, "▲ "
			if caption == "" {
				caption, mark = "no sweep has run yet", ""
			}
			// The pill and the seed count stay together: a claim resting on
			// two seeds says so right beside the button that would fix it.
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return comp.Text(t, t.Sz.Caption, t.P.Warn, mark)(gtx)
					}),
					layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Dim, caption, false)),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.narrow.Layout(t, gtx) }),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(s.Experiment) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
					"define arms and seeds above, then run it"))
			}
			return p.matrix(t, gtx, s)
		}),
	)
}

// verdictColor is the "vs baseline" cell's text colour: green for
// significant, amber for a claim the interval cannot yet support, dim for
// the baseline itself.
func verdictColor(t *theme.Theme, a state.ArmSummary) color.NRGBA {
	switch {
	case !a.HasDelta:
		return t.P.Dim
	case a.Verdict == "significant":
		return t.P.Good
	default:
		return t.P.Warn
	}
}

// matrix draws the arms against the first of them.
//
// Every cell but the baseline's shows only how far it moved, because that is
// the question: six columns of absolute figures make the reader do the
// subtraction, and they do it on the two arms that happen to be adjacent.
func (p *sweepResults) matrix(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	base := s.Experiment[0]

	cell := func(text string, colour color.NRGBA, w int, mono bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			px := gtx.Dp(unit.Dp(w))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
			d := comp.OneLine(t, t.Sz.Body, colour, text, mono)(gtx)
			// Forced to the declared width: a cell that measures itself slides
			// every column after it out from under its own header.
			d.Size.X = px
			return d
		})
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			head := make([]layout.FlexChild, 0, len(armCols)+2)
			for _, c := range armCols {
				head = append(head, cell(c.title, t.P.Dim, c.width, false))
			}
			head = append(head, cell("95% ci", t.P.Dim, 170, false), cell("vs baseline", t.P.Dim, 150, false))
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, head...)
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
	}

	for i, a := range s.Experiment {
		i, a := i, a
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			row := make([]layout.FlexChild, 0, len(armCols))
			for _, c := range armCols {
				switch {
				case c.value == nil:
					row = append(row, cell(a.Arm, t.P.Ink, c.width, false))
				case i == 0 || c.better == 0:
					row = append(row, cell(
						fmt.Sprintf("%.0f%s", c.value(a), c.unit), t.P.Ink, c.width, true))
				default:
					ref := c.value(base)
					if ref == 0 {
						row = append(row, cell(
							fmt.Sprintf("%.0f%s", c.value(a), c.unit), t.P.Ink, c.width, true))
						continue
					}
					d := (c.value(a) - ref) / ref * 100
					colour := t.P.Dim
					switch {
					case d > 0.5:
						colour = t.P.Bad
					case d < -0.5:
						colour = t.P.Good
					}
					// Flipped where more is the good direction. Without this a
					// firmware that delivered more would be painted red.
					if c.better > 0 && d > 0.5 {
						colour = t.P.Good
					} else if c.better > 0 && d < -0.5 {
						colour = t.P.Bad
					}
					row = append(row, cell(fmt.Sprintf("%+.1f%%", d), colour, c.width, true))
				}
			}
			// The confidence interval and the significance verdict ride beside
			// the diff columns rather than inside armCols: neither is a value
			// with a "better" direction to diff against the baseline, they are
			// the thing that says whether a diff is real.
			ci := "—"
			if a.HasCI {
				ci = fmt.Sprintf("%.0f … %.0f", a.RXLo, a.RXHi)
			} else if a.Runs > 0 {
				ci = "n=1, no interval"
			}
			vs := "baseline"
			switch {
			case !a.HasDelta:
			case a.Verdict == "significant":
				vs = fmt.Sprintf("%+.1f%%  significant", a.DeltaPct)
			default:
				vs = fmt.Sprintf("%+.1f%%  not yet", a.DeltaPct)
			}
			row = append(row, cell(ci, t.P.Ink, 170, true), cell(vs, verdictColor(t, a), 150, false))
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, row...)
		}))
	}
	children = append(children,
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Rigid(comp.OneLine(t, t.Sz.Caption, t.P.Faint,
			"against "+base.Arm+"; every column but rx and delivered is a cost, "+
				"so red is worse in both directions", false)))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// provisioningControls is what every node is told at boot.
//
// Each switch here is a failure somebody has had. A node without its name
// reports as its board type, so an event log names hardware rather than
// places. One without its position advertises none, and a client draws it at
// null island. One whose clock disagrees rejects messages as replays, which
// reads as a radio fault.
type provisioningControls struct {
	name, pos, clock comp.Check
	region, scope    comp.Check
	hops, stagger    comp.Field
	extra            comp.Field
	apply            comp.Button
	toRunning        comp.Button
	do               Do
	built            bool
}

func (c *provisioningControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.name.Label, c.name.Bool.Value = "set each node's name", true
		c.pos.Label, c.pos.Bool.Value = "tell it where it is", true
		c.clock.Label, c.clock.Bool.Value = "give them all one clock", true
		c.region.Label = "define a region from the study area"
		c.scope.Label = "and make it the default scope"
		c.hops.Hint = "cap advert hops (blank: leave alone)"
		c.stagger.Hint = "stagger starts, ms"
		c.extra.Hint = "anything else this study needs, one command per line"
		c.hops.Editor.SingleLine = true
		c.stagger.Editor.SingleLine = true
		c.apply.Label, c.apply.Kind = "use for the next start", comp.Primary
		c.toRunning.Label, c.toRunning.Kind = "send to nodes already running", comp.Secondary
		c.built = true
	}
	settings := func() map[string]any {
		p := map[string]any{
			"set_name": c.name.Bool.Value, "set_position": c.pos.Bool.Value,
			"set_clock": c.clock.Bool.Value, "region_from_area": c.region.Bool.Value,
			"default_scope": c.scope.Bool.Value, "extra": fieldText(&c.extra),
		}
		if v, ok := num(&c.hops); ok {
			p["advert_hops"] = v
		}
		if v, ok := num(&c.stagger); ok {
			p["stagger_ms"] = v
		}
		return p
	}
	if c.apply.Click.Clicked(gtx) && c.do != nil {
		c.do("provisioning.set", settings())
	}
	if c.toRunning.Click.Clicked(gtx) && c.do != nil {
		c.do("provisioning.set", settings())
		c.do("provisioning.apply", nil)
	}

	checks := []*comp.Check{&c.name, &c.pos, &c.clock, &c.region, &c.scope}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var kids []layout.FlexChild
			for _, ch := range checks {
				ch := ch
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: t.Sp.M, Bottom: t.Sp.XS}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return ch.Layout(t, gtx)
						})
				}))
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bar := actionBar{
				fields:  []*comp.Field{&c.hops, &c.stagger},
				buttons: []*comp.Button{&c.apply, &c.toRunning},
				note: "these are the lines the node panel shows under \"is told, at boot\", " +
					"so what you read there and what a node is sent cannot drift apart",
			}
			return bar.layout(t, gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return c.extra.Layout(t, gtx)
			})
		}),
	)
}
