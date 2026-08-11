package scenario

import (
	"fmt"
	"strings"
)

// MergeStrategy is how an import lands on a scenario that already has nodes.
//
// Merging is the default posture: the point of importing onto an existing
// network is to extend a study, not to start it again. The old import had one
// silent append-or-replace checkbox, which is how operators lost hand-placed
// nodes; each strategy here states what it keeps before it runs.
type MergeStrategy string

const (
	// MergeAddNew keeps everything present and adds only nodes the scenario
	// does not already have.
	MergeAddNew MergeStrategy = "add-only-new"
	// MergeReplaceMatching overwrites nodes the import also has, keeps the rest.
	MergeReplaceMatching MergeStrategy = "replace-matching"
	// MergeReplaceAll discards the scenario's nodes for the import's.
	MergeReplaceAll MergeStrategy = "replace-all"
)

// MergePlan says what a merge would do, before it does it — the numbers the
// Import preview shows next to the commit button.
type MergePlan struct {
	Add     int
	Replace int
	Keep    int
	Drop    int
}

func (p MergePlan) String() string {
	return fmt.Sprintf("%d to add, %d to replace, %d kept, %d dropped",
		p.Add, p.Replace, p.Keep, p.Drop)
}

// mergeKey joins on public key where the node has one, name otherwise —
// keys are identities, names are labels humans reuse.
func mergeKey(n Node) string {
	if n.PublicKey != "" {
		return "k:" + strings.ToLower(n.PublicKey)
	}
	return "n:" + strings.ToLower(n.Name)
}

// PlanMerge computes what Merge would do without touching anything.
func PlanMerge(existing, incoming []Node, s MergeStrategy) MergePlan {
	have := map[string]bool{}
	for _, n := range existing {
		have[mergeKey(n)] = true
	}
	var p MergePlan
	for _, n := range incoming {
		if have[mergeKey(n)] {
			if s == MergeAddNew {
				p.Keep++
			} else {
				p.Replace++
			}
		} else {
			p.Add++
		}
	}
	if s == MergeReplaceAll {
		// Everything present that the import does not re-supply is dropped.
		p.Drop = len(existing) - p.Replace
		if p.Drop < 0 {
			p.Drop = 0
		}
	} else {
		p.Keep = len(existing) - p.Replace
	}
	return p
}

// Merge applies the strategy and returns the resulting node list.
//
// Existing nodes keep their order; additions land at the end in import order.
// A replacement keeps the existing node's slot but takes the import's fields
// wholesale — a half-merge that kept "just the position" would leave a node
// whose radio config and firmware silently belong to a different point in time.
func Merge(existing, incoming []Node, s MergeStrategy) []Node {
	if s == MergeReplaceAll {
		out := make([]Node, len(incoming))
		copy(out, incoming)
		return out
	}
	byKey := map[string]int{}
	out := make([]Node, len(existing))
	copy(out, existing)
	for i, n := range out {
		byKey[mergeKey(n)] = i
	}
	// Names must stay unique across the merged set, whichever side they came
	// from — two nodes sharing a name are indistinguishable in every ledger.
	names := map[string]bool{}
	for _, n := range out {
		names[strings.ToLower(n.Name)] = true
	}
	for _, n := range incoming {
		if at, ok := byKey[mergeKey(n)]; ok {
			if s == MergeReplaceMatching {
				out[at] = n
			}
			continue
		}
		if names[strings.ToLower(n.Name)] {
			n.Name = uniqueName(n.Name, n.PublicKey, names)
		}
		names[strings.ToLower(n.Name)] = true
		byKey[mergeKey(n)] = len(out)
		out = append(out, n)
	}
	return out
}
