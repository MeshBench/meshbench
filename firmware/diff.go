package firmware

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Divergence is one difference between the two backends.
type Divergence struct {
	AtMs   uint32
	Kind   string
	Native string
	Emul   string
}

// DiffResult is a cross-check run.
//
// ADR-0010 keeps both backends so they can be compared. A comparison that only
// reports "they matched" is worth very little, because the interesting outcome
// is a difference and the second-most interesting is discovering the comparison
// could not have found one.
type DiffResult struct {
	TicksCompared int
	Divergences   []Divergence

	// NativeFrames and EmulFrames are what each backend transmitted, in order.
	NativeFrames [][]byte
	EmulFrames   [][]byte

	// Inconclusive marks a run that proves nothing — most often because one
	// backend transmitted nothing at all, so identical output means only that
	// both were silent.
	Inconclusive bool
	Why          string
}

// DiffOptions control a cross-check.
type DiffOptions struct {
	// UntilMs is how far to run, and StepMs the tick granularity. Both
	// backends see exactly the same ticks, which is the point: a divergence
	// must come from the target, not from the two being driven differently.
	UntilMs uint32
	StepMs  uint32

	// Deliver is called at each tick to supply frames to both nodes. The same
	// frames go to both, in the same order — anything else compares two
	// different experiments.
	Deliver func(atMs uint32) [][]byte

	Timeout time.Duration
}

// Diff runs two nodes in lockstep and compares what they transmit.
//
// The comparison is on transmitted frames rather than on internal state,
// because internal state is exactly what differs legitimately between a
// Cortex-M4 and an x86-64 host — pointer widths, padding, stack layout. What
// must not differ is what goes on the air.
func Diff(ctx context.Context, native, emulated *Node, o DiffOptions) (DiffResult, error) {
	if native == nil || emulated == nil {
		return DiffResult{}, fmt.Errorf("firmware: diff needs both a native and an emulated node")
	}
	if o.StepMs == 0 {
		o.StepMs = 10
	}
	if o.UntilMs == 0 {
		o.UntilMs = 2000
	}
	if o.Timeout <= 0 {
		o.Timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	var res DiffResult
	for at := o.StepMs; at <= o.UntilMs; at += o.StepMs {
		if o.Deliver != nil {
			for _, f := range o.Deliver(at) {
				// Both, always, and in the same order. Delivering to one and not
				// the other produces a divergence that says nothing about the
				// target.
				if err := native.Bridge.Deliver(f); err != nil {
					return res, fmt.Errorf("firmware: deliver to native at %d ms: %w", at, err)
				}
				if err := emulated.Bridge.Deliver(f); err != nil {
					return res, fmt.Errorf("firmware: deliver to emulated at %d ms: %w", at, err)
				}
			}
		}

		if err := native.Bridge.Advance(ctx, at); err != nil {
			return res, fmt.Errorf("firmware: native: %w", err)
		}
		if err := emulated.Bridge.Advance(ctx, at); err != nil {
			return res, fmt.Errorf("firmware: emulated: %w", err)
		}
		res.TicksCompared++

		n := drain(native.Bridge.Transmitted)
		e := drain(emulated.Bridge.Transmitted)
		res.NativeFrames = append(res.NativeFrames, n...)
		res.EmulFrames = append(res.EmulFrames, e...)

		if len(n) != len(e) {
			res.Divergences = append(res.Divergences, Divergence{
				AtMs: at, Kind: "frame count",
				Native: fmt.Sprintf("%d frames", len(n)),
				Emul:   fmt.Sprintf("%d frames", len(e)),
			})
			continue
		}
		for i := range n {
			if !equal(n[i], e[i]) {
				res.Divergences = append(res.Divergences, Divergence{
					AtMs: at, Kind: "frame contents",
					Native: preview(n[i]), Emul: preview(e[i]),
				})
			}
		}
	}

	// A run where neither side transmitted proves nothing. Reporting it as a
	// match is the failure mode this whole check exists to avoid: a green
	// cross-check that could never have gone red.
	if len(res.NativeFrames) == 0 && len(res.EmulFrames) == 0 {
		res.Inconclusive = true
		res.Why = "neither backend transmitted anything, so identical output means only " +
			"that both were silent — this run could not have found a difference"
	}
	sort.SliceStable(res.Divergences, func(a, b int) bool {
		return res.Divergences[a].AtMs < res.Divergences[b].AtMs
	})
	return res, nil
}

// Agreed reports whether the two backends matched *and* the run could have
// shown otherwise.
func (r DiffResult) Agreed() bool { return len(r.Divergences) == 0 && !r.Inconclusive }

// Describe is what to put in a report.
func (r DiffResult) Describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d ticks compared; native transmitted %d frames, emulated %d.\n",
		r.TicksCompared, len(r.NativeFrames), len(r.EmulFrames))

	switch {
	case r.Inconclusive:
		fmt.Fprintf(&b, "\nINCONCLUSIVE: %s.\n", r.Why)
	case len(r.Divergences) == 0:
		b.WriteString("\nThe two backends put identical frames on the air. That is the " +
			"result ADR-0010 wants, and it says nothing about internal state — pointer " +
			"widths, padding and stack layout differ legitimately between a Cortex-M4 " +
			"and an x86-64 host, and only what goes on the air has to match.\n")
	default:
		fmt.Fprintf(&b, "\n%d DIVERGENCES. A difference here is a real finding either way "+
			"round: the native build is what runs, and the emulated one is what is "+
			"authoritative.\n", len(r.Divergences))
		for i, d := range r.Divergences {
			if i >= 10 {
				fmt.Fprintf(&b, "  ... and %d more\n", len(r.Divergences)-10)
				break
			}
			fmt.Fprintf(&b, "  %6d ms  %-14s native=%s emulated=%s\n", d.AtMs, d.Kind, d.Native, d.Emul)
		}
	}
	return b.String()
}

func drain(ch chan []byte) [][]byte {
	var out [][]byte
	for {
		select {
		case f := <-ch:
			out = append(out, f)
		default:
			return out
		}
	}
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// preview renders the first few bytes of a frame for a divergence report.
func preview(b []byte) string {
	const digits = "0123456789abcdef"
	var s strings.Builder
	for i, v := range b {
		if i == 12 {
			fmt.Fprintf(&s, "...(%d bytes)", len(b))
			break
		}
		s.WriteByte(digits[v>>4])
		s.WriteByte(digits[v&0xF])
	}
	if s.Len() == 0 {
		return "(empty)"
	}
	return s.String()
}
