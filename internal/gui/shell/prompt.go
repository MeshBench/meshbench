// Asking for one thing.
//
// A menu entry carries no parameters, so every verb needing a name, a path or
// a number failed the moment somebody picked it: it fired, it errored, and all
// they saw was a status line. "Save this network" was the plainest case - a
// File menu staple that could not work, because there was nowhere to type the
// name.
//
// This is the smallest thing that fixes it. One question, one answer, Enter to
// accept and Escape to abandon.
package shell

import (
	"strings"
	"sync"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// Prompt is a question waiting for an answer.
type Prompt struct {
	// Title is the question. Ask is what to do with the answer.
	Title string
	Hint  string
	Ask   func(answer string)

	field  comp.Field
	ok     comp.Button
	cancel comp.Button
	built  bool
	open   bool

	// choices, when it has any, turns the question into a chooser: the same
	// modal, but answered by pointing at one of the things it offers rather
	// than by typing a name nobody can be expected to remember.
	choices []string
	shown   []int
	list    widget.List
	btns    []comp.Button

	// queued are questions raised from somewhere other than the frame loop -
	// a verb's reply, which arrives on a goroutine of its own. Opening one
	// there would write the widget state the frame loop is reading, so they
	// wait and are drained at the top of the next frame.
	mu     sync.Mutex
	queued []func(*Prompt)
}

// Post raises a question from another goroutine. It happens on the next frame.
func (p *Prompt) Post(f func(*Prompt)) {
	p.mu.Lock()
	p.queued = append(p.queued, f)
	p.mu.Unlock()
}

// drain runs what was posted, on the frame loop.
func (p *Prompt) drain() {
	p.mu.Lock()
	q := p.queued
	p.queued = nil
	p.mu.Unlock()
	for _, f := range q {
		f(p)
	}
}

// Open asks a question. Opening a second one replaces the first rather than
// stacking: two modal questions at once is a state nobody can reason about.
func (p *Prompt) Open(title, hint, initial string, ask func(string)) {
	p.Title, p.Hint, p.Ask = title, hint, ask
	p.choices, p.btns = nil, nil
	p.field.Editor.SetText(initial)
	p.field.Editor.SingleLine = true
	p.field.Editor.Submit = true
	p.open = true
}

// Choose asks the same question with the answers already on screen. An empty
// list still opens, saying so, because a chooser that silently does nothing is
// indistinguishable from a menu entry that is not wired up.
func (p *Prompt) Choose(title, hint string, choices []string, ask func(string)) {
	p.Open(title, hint, "", ask)
	p.choices = choices
	p.btns = make([]comp.Button, len(choices))
	for i := range p.btns {
		p.btns[i].Label, p.btns[i].Kind = choices[i], comp.Secondary
	}
	p.list.Axis = layout.Vertical
}

// Showing reports whether a question is on screen, so the shell can dim what
// is behind it.
func (p *Prompt) Showing() bool { return p.open }

func (p *Prompt) Close() { p.open = false }

// Layout draws the question over everything else.
func (p *Prompt) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	p.drain()
	if !p.open {
		return layout.Dimensions{}
	}
	if !p.built {
		p.ok.Label, p.ok.Kind = "OK", comp.Primary
		p.cancel.Label, p.cancel.Kind = "Cancel", comp.Quiet
		p.built = true
	}
	p.field.Hint = p.Hint

	submitted := false
	for {
		ev, ok := p.field.Editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			submitted = true
		}
	}
	// Escape abandons, which is the other half of a modal question and the
	// half people reach for when they opened it by accident.
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			p.open = false
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}
	}
	if p.cancel.Click.Clicked(gtx) {
		p.open = false
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	for i := range p.btns {
		if p.btns[i].Click.Clicked(gtx) {
			answer := p.choices[i]
			p.open = false
			if p.Ask != nil {
				p.Ask(answer)
			}
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}
	}
	if p.ok.Click.Clicked(gtx) || submitted {
		answer := p.field.Editor.Text()
		p.open = false
		if p.Ask != nil {
			p.Ask(answer)
		}
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}

	// Dim what is behind it, so it reads as one question rather than as a
	// panel that has appeared in the middle of the window.
	comp.FillRect(gtx, gtx.Constraints.Max, theme.Alpha(t.P.Ground, 0.82))

	// Modal means the frame underneath does not get the clicks. Drawing over
	// it is not enough: without an area of its own, a click lands on whatever
	// happened to be beneath the question, and the first build let people
	// press buttons they could not see.
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, p)
	for {
		if _, ok := gtx.Event(pointer.Filter{
			Target: p, Kinds: pointer.Press | pointer.Release,
		}); !ok {
			break
		}
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = gtx.Dp(520)
		gtx.Constraints.Min.X = gtx.Dp(520)
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(t.Sp.M).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Title, t.P.Ink, p.Title)),
				layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.field.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.chooser(t, gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Flexed(1, comp.Spacer),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: t.Sp.S}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									return p.cancel.Layout(t, gtx)
								})
						}),
						// No OK when the answers are on screen: picking one
						// is the answer, and a button that repeats it only
						// raises the question of what it does differently.
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if p.choices != nil {
								return layout.Dimensions{}
							}
							return p.ok.Layout(t, gtx)
						}),
					)
				}),
			)
		})
		call := macro.Stop()
		comp.RoundRect(gtx, dims.Size, 6, t.P.Panel)
		comp.Border(gtx, dims.Size, 6, 1, t.P.Rule)
		call.Add(gtx.Ops)
		return dims
	})
}

// chooser draws the answers, filtered by whatever is in the field.
//
// The field earns its keep either way: with choices it narrows them, without
// them it is where the answer is typed.
func (p *Prompt) chooser(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	if p.choices == nil {
		return layout.Dimensions{}
	}
	if len(p.choices) == 0 {
		return comp.Text(t, t.Sz.Caption, t.P.Faint, "there are none saved yet")(gtx)
	}
	want := strings.ToLower(strings.TrimSpace(p.field.Editor.Text()))
	shown := p.shown[:0]
	for i := range p.choices {
		if want == "" || strings.Contains(strings.ToLower(p.choices[i]), want) {
			shown = append(shown, i)
		}
	}
	p.shown = shown
	if len(shown) == 0 {
		return comp.Text(t, t.Sz.Caption, t.P.Faint, "nothing matches that")(gtx)
	}
	gtx.Constraints.Max.Y = gtx.Dp(320)
	return comp.List(t, &p.list, len(shown),
		func(gtx layout.Context, i int) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return p.btns[shown[i]].Layout(t, gtx)
				})
		})(gtx)
}
