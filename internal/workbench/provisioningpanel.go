// The rule list: what a study adds on top of the base above it.
//
// Every rule is a when/then pair, matched against a readback rather than the
// import - a node is the authority on what it is. A rule with no conditions
// matches everything, which is deliberately how "just set this on every
// node" is expressed: there is no separate concept of a base rule other than
// the panel above this one, and the mechanism is identical.
package workbench

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// condSlots and effSlots bound how many conditions and effects one rule can
// hold in this panel. Every example in the design needs at most three of
// either; a rule that genuinely needs a fourth is two rules, which is also
// more legible once it is on screen.
const condSlots = 3
const effSlots = 3

// pseudoFields are the conditions and effects that are not one of the
// firmware's own keys - facts about the node's position, its role, or the
// interface's own selection, rather than something read or set over a wire.
var pseudoConditionFields = []string{"regions", "default-scope", "unscoped-flood", "kind", "selected", "area"}
var pseudoEffectFields = []string{"regions", "default-scope", "unscoped-flood"}

// ruleEditor is the widget set for one rule, pooled by its position in the
// list - simpler than pooling by name, and the cost is that a mid-list
// removal can show one frame of the wrong values in a field that is still
// being typed into, which self-corrects on the very next frame.
type ruleEditor struct {
	name      comp.Field
	condField [condSlots]comp.Dropdown
	condOp    [condSlots]comp.Dropdown
	condVal   [condSlots]comp.Field
	effField  [effSlots]comp.Dropdown
	effMode   [effSlots]comp.Dropdown
	effVal    [effSlots]comp.Field
	remove    comp.Button
	built     bool
}

// build wires every dropdown's OnOpen once, to the panel's own chooser - the
// same one-chooser-at-a-time pattern every other dropdown in the application
// uses. condFields/effFields are read live rather than captured, so the
// choices offered always match the firmware's current command table.
func (e *ruleEditor) build(p *provisioningRulesPanel) {
	e.name.Hint = "what this rule is for"
	e.name.Editor.SingleLine = true
	for i := range e.condVal {
		i := i
		e.condVal[i].Hint = "value"
		e.condVal[i].Editor.SingleLine = true
		e.condField[i].OnOpen = func() {
			if p.choose == nil {
				return
			}
			p.choose("Condition on", conditionFieldOptions(p.lastSnap), func(picked string) {
				e.condField[i].Value = picked
			})
		}
		e.condOp[i].OnOpen = func() {
			if p.choose == nil {
				return
			}
			p.choose("Operator", conditionOpOptions(e.condField[i].Value), func(picked string) {
				e.condOp[i].Value = picked
			})
		}
	}
	for i := range e.effVal {
		i := i
		e.effVal[i].Hint = "value"
		e.effVal[i].Editor.SingleLine = true
		e.effField[i].OnOpen = func() {
			if p.choose == nil {
				return
			}
			p.choose("Effect on", effectFieldOptions(p.lastSnap), func(picked string) {
				e.effField[i].Value = picked
			})
		}
		e.effMode[i].OnOpen = func() {
			if p.choose == nil {
				return
			}
			p.choose("Mode", effectModeOptions(e.effField[i].Value), func(picked string) {
				e.effMode[i].Value = picked
			})
		}
	}
	e.remove.Label, e.remove.Kind = "remove this rule", comp.Destructive
	e.built = true
}

// fromState fills an editor from a saved rule, called once when the panel's
// working copy is (re)seeded from the session.
func (e *ruleEditor) fromState(r state.ProvisionRule) {
	e.name.Editor.SetText(r.Name)
	for i := 0; i < condSlots; i++ {
		e.condField[i].Value, e.condOp[i].Value = "", ""
		e.condVal[i].Editor.SetText("")
		if i < len(r.Conditions) {
			c := r.Conditions[i]
			e.condField[i].Value = c.Field
			if c.Custom {
				e.condField[i].Value = "(custom) " + c.CustomGet
			}
			e.condOp[i].Value = c.Op
			e.condVal[i].Editor.SetText(c.Value)
		}
	}
	for i := 0; i < effSlots; i++ {
		e.effField[i].Value, e.effMode[i].Value = "", ""
		e.effVal[i].Editor.SetText("")
		if i < len(r.Effects) {
			ef := r.Effects[i]
			e.effField[i].Value = ef.Field
			if ef.Field == "" && ef.CustomSet != "" {
				e.effField[i].Value = "(custom command)"
				e.effVal[i].Editor.SetText(ef.CustomSet)
			} else {
				e.effVal[i].Editor.SetText(ef.Value)
			}
			e.effMode[i].Value = ef.Mode
		}
	}
}

// toState reads an editor back into a rule, dropping any slot whose field was
// never chosen - an empty slot contributes nothing, the same as a condition
// or effect list that was simply shorter.
func (e *ruleEditor) toState() state.ProvisionRule {
	r := state.ProvisionRule{Name: fieldText(&e.name)}
	for i := 0; i < condSlots; i++ {
		f := e.condField[i].Value
		if f == "" {
			continue
		}
		c := state.ProvisionCondition{Op: e.condOp[i].Value, Value: fieldText(&e.condVal[i])}
		if custom, ok := strings.CutPrefix(f, "(custom) "); ok {
			c.Custom, c.CustomGet = true, custom
		} else {
			c.Field = f
		}
		r.Conditions = append(r.Conditions, c)
	}
	for i := 0; i < effSlots; i++ {
		f := e.effField[i].Value
		if f == "" {
			continue
		}
		ef := state.ProvisionEffect{Mode: e.effMode[i].Value}
		if f == "(custom command)" {
			ef.CustomSet = fieldText(&e.effVal[i])
		} else {
			ef.Field, ef.Value = f, fieldText(&e.effVal[i])
		}
		r.Effects = append(r.Effects, ef)
	}
	return r
}

// provisioningRulesPanel is the study's own overrides: an ordered list of
// when/then rules, on top of the base the panel above it defines.
type provisioningRulesPanel struct {
	editors []*ruleEditor
	seeded  bool
	// lastSnap is the most recent snapshot Draw saw, kept so a dropdown's
	// OnOpen - wired once at build time, long before any particular frame's
	// snapshot exists - can still offer the current command table and node
	// list when it is eventually pressed.
	lastSnap *state.Snapshot

	addRule    comp.Button
	save       comp.Button
	reload     comp.Button
	readNow    comp.Button
	previewSel comp.Dropdown
	previewFor string
	list       widget.List

	choose func(title string, opts []string, pick func(string))
	do     Do
	built  bool
}

func (p *provisioningRulesPanel) build() {
	p.addRule.Label, p.addRule.Kind = "add a rule", comp.Secondary
	p.save.Label, p.save.Kind = "save the rules", comp.Primary
	p.reload.Label, p.reload.Kind = "discard changes, reload", comp.Quiet
	p.readNow.Label, p.readNow.Kind = "read every node now", comp.Secondary
	p.list.Axis = layout.Vertical
	p.previewSel.OnOpen = func() {
		if p.choose == nil || p.lastSnap == nil {
			return
		}
		var names []string
		for _, n := range p.lastSnap.Nodes {
			names = append(names, n.Name)
		}
		p.choose("Preview which node?", names, func(picked string) {
			p.previewFor, p.previewSel.Value = picked, picked
			if p.do != nil {
				p.do("provisioning.preview", map[string]any{"node": picked})
			}
		})
	}
	p.built = true
}

// seed copies the session's rule list into the panel's own editors - once,
// or on an explicit reload. Editing is local until Save; posting on every
// keystroke would mean a rule half-typed by one keystroke is a rule the
// engine has already tried to evaluate.
func (p *provisioningRulesPanel) seed(s *state.Snapshot) {
	p.editors = p.editors[:0]
	if s == nil {
		return
	}
	for _, r := range s.ProvisioningRules {
		// Rule zero is the legacy base, drawn by the panel above this one -
		// it is not edited here, only shown by its match count.
		if r.Name == "the session's own settings" {
			continue
		}
		e := &ruleEditor{}
		e.build(p)
		e.fromState(r)
		p.editors = append(p.editors, e)
	}
	p.seeded = true
}

func (p *provisioningRulesPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.built {
		p.build()
	}
	p.lastSnap = s
	if !p.seeded && s != nil && s.ProvisioningRules != nil {
		p.seed(s)
	}
	p.clicks(gtx, s)

	rows := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			bar := actionBar{
				buttons: []*comp.Button{&p.addRule, &p.save, &p.reload, &p.readNow},
				note: "conditions match what the last readback found, never what an " +
					"earlier rule in this list set - reordering rules cannot change " +
					"which nodes they match",
			}
			return bar.layout(t, gtx)
		},
	}
	if s != nil && !s.ProvisioningRead {
		rows = append(rows, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"no readback yet - match counts and the preview below are blank until "+
				"one has run"))
	}
	for i, e := range p.editors {
		i, e := i, e
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return p.ruleRow(t, gtx, s, i, e)
		})
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return p.previewAndResults(t, gtx, s)
	})

	return comp.List(t, &p.list, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, rows[i])
	})(gtx)
}

func (p *provisioningRulesPanel) ruleRow(t *theme.Theme, gtx layout.Context, s *state.Snapshot, i int, e *ruleEditor) layout.Dimensions {
	match := "not read yet"
	if s != nil && s.ProvisioningRead {
		name := fieldText(&e.name)
		if n, ok := s.ProvisioningMatch[name]; ok {
			match = fmt.Sprintf("%d node(s) match", n)
		} else {
			match = "0 nodes match"
		}
	}
	return comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return e.name.Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, match)),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return e.remove.Layout(t, gtx)
					}),
				)
			}),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "WHEN - every filled row must hold")),
			p.condRows(t, s, e),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "THEN")),
			p.effRows(t, s, e),
		)
	})(gtx)
}

func (p *provisioningRulesPanel) condRows(t *theme.Theme, s *state.Snapshot, e *ruleEditor) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		var kids []layout.FlexChild
		for i := 0; i < condSlots; i++ {
			i := i
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					fixed(gtx, 200, func(gtx layout.Context) layout.Dimensions {
						return e.condField[i].Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
					fixed(gtx, 130, func(gtx layout.Context) layout.Dimensions {
						return e.condOp[i].Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return e.condVal[i].Layout(t, gtx)
					}),
				)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}

func (p *provisioningRulesPanel) effRows(t *theme.Theme, s *state.Snapshot, e *ruleEditor) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		var kids []layout.FlexChild
		for i := 0; i < effSlots; i++ {
			i := i
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					fixed(gtx, 200, func(gtx layout.Context) layout.Dimensions {
						return e.effField[i].Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
					fixed(gtx, 130, func(gtx layout.Context) layout.Dimensions {
						return e.effMode[i].Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return e.effVal[i].Layout(t, gtx)
					}),
				)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}

func (p *provisioningRulesPanel) previewAndResults(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	var kids []layout.FlexChild
	kids = append(kids, layout.Rigid(comp.SectionTitle(t, "resolved preview and results")))
	kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, "preview for  ")),
			fixed(gtx, 200, func(gtx layout.Context) layout.Dimensions {
				return p.previewSel.Layout(t, gtx)
			}),
		)
	}))
	if s != nil && len(s.ProvisioningPreview) > 0 {
		for _, l := range s.ProvisioningPreview {
			l := l
			who := l.RuleName
			if who == "" {
				who = "structural"
			}
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					fixed(gtx, 260, comp.Mono(t, t.Sz.Data, t.P.Ink, l.Command)),
					layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Faint, who, false)),
				)
			}))
		}
	}
	if s != nil && len(s.ProvisioningResults) > 0 {
		kids = append(kids, layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "last run:")))
		for _, r := range s.ProvisioningResults {
			r := r
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				status := fmt.Sprintf("%d sent, %d refused", r.Sent, len(r.Refused))
				ink := t.P.Good
				detail := ""
				if len(r.Refused) > 0 {
					ink = t.P.Bad
					detail = strings.Join(r.Refused, "; ")
				}
				return layout.Flex{}.Layout(gtx,
					fixed(gtx, 200, comp.Text(t, t.Sz.Caption, t.P.Ink, r.Node)),
					fixed(gtx, 150, comp.Text(t, t.Sz.Caption, ink, status)),
					layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Faint, detail, false)),
				)
			}))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

func (p *provisioningRulesPanel) clicks(gtx layout.Context, s *state.Snapshot) {
	if p.addRule.Click.Clicked(gtx) {
		e := &ruleEditor{}
		e.build(p)
		p.editors = append(p.editors, e)
		// A rule that just appears at the bottom of a possibly long list is
		// easy to miss, and "add a rule" otherwise has no visible effect
		// until Save - which is also why it earns a status line rather than
		// silently mutating local state.
		if p.do != nil {
			p.do("ui.said", "a new rule was added; write it and save to use it")
		}
	}
	if p.reload.Click.Clicked(gtx) {
		p.seeded = false
		if p.do != nil {
			p.do("ui.said", "unsaved changes to the rules were discarded")
		}
	}
	if p.readNow.Click.Clicked(gtx) && p.do != nil {
		p.do("provisioning.readback", nil)
	}
	if p.save.Click.Clicked(gtx) && p.do != nil {
		var rules []any
		for _, e := range p.editors {
			r := e.toState()
			conds := make([]any, len(r.Conditions))
			for i, c := range r.Conditions {
				conds[i] = map[string]any{
					"field": c.Field, "op": c.Op, "value": c.Value,
					"custom": c.Custom, "custom_get": c.CustomGet,
				}
			}
			effs := make([]any, len(r.Effects))
			for i, ef := range r.Effects {
				effs[i] = map[string]any{
					"field": ef.Field, "mode": ef.Mode, "value": ef.Value,
					"custom_set": ef.CustomSet,
				}
			}
			rules = append(rules, map[string]any{
				"name": r.Name, "conditions": conds, "effects": effs,
			})
		}
		p.do("provisioning.rules.set", map[string]any{"rules": rules})
	}
	for i, e := range p.editors {
		if e.remove.Click.Clicked(gtx) {
			p.editors = append(p.editors[:i], p.editors[i+1:]...)
			break
		}
	}
}
