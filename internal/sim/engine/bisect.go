package engine

import (
	"context"
	"fmt"
)

// BisectNodes finds one node whose membership in the "changed" set flips the
// outcome of test.
//
// The A/B question in its useful form: a run with every node changed diverges
// from the baseline, and the operator wants to know *which node* carries the
// difference — which repeater's firmware swap, settings change or move is the
// one that altered the network's behaviour. Testing one node at a time is
// O(n) runs of a slow simulation; halving is O(log n).
//
// test reports whether running with exactly `changed` nodes changed still
// diverges from the baseline. It is called with progressively smaller sets.
// The search assumes at least one single node reproduces the divergence; when
// the effect needs several nodes together, the smallest set the halving
// reaches is returned instead of one name.
func BisectNodes(ctx context.Context, all []string,
	test func(ctx context.Context, changed []string) (bool, error)) ([]string, error) {
	if len(all) == 0 {
		return nil, fmt.Errorf("engine: bisect: no nodes")
	}
	suspects := append([]string(nil), all...)
	for len(suspects) > 1 {
		if err := ctx.Err(); err != nil {
			return suspects, err
		}
		half := suspects[:len(suspects)/2]
		diverged, err := test(ctx, half)
		if err != nil {
			return suspects, err
		}
		if diverged {
			suspects = half
			continue
		}
		other := suspects[len(suspects)/2:]
		diverged, err = test(ctx, other)
		if err != nil {
			return suspects, err
		}
		if !diverged {
			// Neither half alone reproduces it: the effect needs nodes from
			// both. Said honestly rather than narrowed to a wrong answer.
			return suspects, nil
		}
		suspects = other
	}
	return suspects, nil
}
