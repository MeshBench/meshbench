package scenario

import "testing"

func mkNode(name, key string, lat float64) Node {
	return Node{Name: name, PublicKey: key, Position: LatLon{Lat: lat, Lon: -2}}
}

func TestMergeAddNewKeepsWhatIsThere(t *testing.T) {
	existing := []Node{mkNode("alpha", "aa11", 51), mkNode("hand-placed", "", 52)}
	incoming := []Node{mkNode("alpha-renamed", "aa11", 99), mkNode("charlie", "cc33", 53)}

	plan := PlanMerge(existing, incoming, MergeAddNew)
	if plan.Add != 1 || plan.Keep != 2 || plan.Replace != 0 || plan.Drop != 0 {
		t.Fatalf("plan = %+v", plan)
	}

	got := Merge(existing, incoming, MergeAddNew)
	if len(got) != 3 {
		t.Fatalf("got %d nodes", len(got))
	}
	// aa11 matched by key despite the rename, so the existing node survives
	// untouched — add-only-new never edits.
	if got[0].Name != "alpha" || got[0].Position.Lat != 51 {
		t.Fatalf("existing node modified: %+v", got[0])
	}
	if got[2].Name != "charlie" {
		t.Fatalf("addition missing: %+v", got[2])
	}
}

func TestMergeReplaceMatchingTakesTheImportWholesale(t *testing.T) {
	existing := []Node{mkNode("alpha", "aa11", 51), mkNode("hand-placed", "", 52)}
	incoming := []Node{mkNode("alpha", "aa11", 60)}

	plan := PlanMerge(existing, incoming, MergeReplaceMatching)
	if plan.Replace != 1 || plan.Keep != 1 || plan.Add != 0 {
		t.Fatalf("plan = %+v", plan)
	}

	got := Merge(existing, incoming, MergeReplaceMatching)
	if len(got) != 2 {
		t.Fatalf("got %d nodes", len(got))
	}
	if got[0].Position.Lat != 60 {
		t.Fatalf("matching node not replaced: %+v", got[0])
	}
	if got[1].Name != "hand-placed" {
		t.Fatalf("unmatched node lost: %+v", got[1])
	}
}

func TestMergeReplaceAllDropsTheRest(t *testing.T) {
	existing := []Node{mkNode("alpha", "aa11", 51), mkNode("beta", "bb22", 52)}
	incoming := []Node{mkNode("alpha", "aa11", 60)}

	plan := PlanMerge(existing, incoming, MergeReplaceAll)
	if plan.Replace != 1 || plan.Drop != 1 || plan.Keep != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	got := Merge(existing, incoming, MergeReplaceAll)
	if len(got) != 1 || got[0].Position.Lat != 60 {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeJoinsByNameWhenNoKey(t *testing.T) {
	existing := []Node{mkNode("Gateway", "", 51)}
	incoming := []Node{mkNode("gateway", "", 60)}
	plan := PlanMerge(existing, incoming, MergeAddNew)
	if plan.Add != 0 || plan.Keep != 1 {
		t.Fatalf("name join failed: %+v", plan)
	}
}

func TestMergeRenamesCollidingAdditions(t *testing.T) {
	// Same name, different key: a genuinely different node that must be added,
	// but not under a name that already exists.
	existing := []Node{mkNode("repeater", "aa11", 51)}
	incoming := []Node{mkNode("repeater", "bb22", 52)}
	got := Merge(existing, incoming, MergeAddNew)
	if len(got) != 2 {
		t.Fatalf("got %d nodes", len(got))
	}
	if got[1].Name == got[0].Name {
		t.Fatalf("duplicate name survived merge: %q", got[1].Name)
	}
}
