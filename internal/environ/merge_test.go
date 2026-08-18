package environ

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

func poly(lon, lat, size float64, props string) string {
	return fmt.Sprintf(`{"type":"Feature","geometry":{"type":"Polygon","coordinates":`+
		`[[[%[1]f,%[2]f],[%[3]f,%[2]f],[%[3]f,%[4]f],[%[1]f,%[4]f],[%[1]f,%[2]f]]]},`+
		`"properties":{%[5]s}}`, lon, lat, lon+size, lat+size, props)
}

// The plan's rule in one scene: a detected building with an OSM twin comes
// out once, wearing OSM's tags; OSM's stated levels beat the detected
// height; a detection with no twin and an OSM building with no detection
// both survive.
func TestMergeExplicitOverridesInferred(t *testing.T) {
	primary := strings.Join([]string{
		poly(-3.0000, 56.0000, 0.0004, `"height":9.5`),  // has an OSM twin
		poly(-3.0100, 56.0100, 0.0004, `"height":4.0`),  // detection only
	}, "\n")
	enrich := strings.Join([]string{
		poly(-3.0001, 56.0001, 0.0003, `"building":"church","building:levels":"1"`),
		poly(-3.0200, 56.0200, 0.0004, `"building":"shed"`), // OSM only
	}, "\n")
	rd, stats, err := MergeGeoJSON(strings.NewReader(primary), strings.NewReader(enrich))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Matched != 1 || stats.EnrichOnly != 1 {
		t.Fatalf("stats %+v, want 1 matched and 1 OSM-only", stats)
	}
	out, _ := io.ReadAll(rd)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 3 {
		t.Fatalf("merged to %d buildings, want 3", len(lines))
	}
	merged := string(out)
	if !strings.Contains(merged, `"building":"church"`) {
		t.Fatal("the twin lost its OSM type")
	}
	// The church stated levels, so the detection's 9.5 m must not appear on it.
	for _, l := range lines {
		if strings.Contains(l, "church") && strings.Contains(l, "9.5") {
			t.Fatal("detected height overrode OSM's stated levels")
		}
	}
	if !strings.Contains(merged, `"height":4`) {
		t.Fatal("the detection-only building lost its height")
	}
	if !strings.Contains(merged, `"building":"shed"`) {
		t.Fatal("the OSM-only building vanished")
	}
}

// Where OSM knows the type but not the height, the detection's height is
// the only height there is, and it must carry over.
func TestMergeInheritsHeightWhereOSMIsSilent(t *testing.T) {
	primary := poly(-3.0000, 56.0000, 0.0004, `"height":12.0`)
	enrich := poly(-3.0001, 56.0001, 0.0003, `"building":"warehouse"`)
	rd, stats, err := MergeGeoJSON(strings.NewReader(primary), strings.NewReader(enrich))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Matched != 1 {
		t.Fatalf("stats %+v, want the pair matched", stats)
	}
	out, _ := io.ReadAll(rd)
	if !strings.Contains(string(out), `"height":12`) ||
		!strings.Contains(string(out), `"building":"warehouse"`) {
		t.Fatalf("merged feature lost a half: %s", out)
	}
}

// The merged stream must feed the ingester as-is - the merge and the tile
// writer drifting apart would be found by an operator, not a test.
func TestMergeOutputIngests(t *testing.T) {
	primary := poly(-3.0000, 56.0000, 0.0004, `"height":9.5`)
	enrich := poly(-3.0001, 56.0001, 0.0003, `"building":"church"`)
	rd, _, err := MergeGeoJSON(strings.NewReader(primary), strings.NewReader(enrich))
	if err != nil {
		t.Fatal(err)
	}
	stats, err := IngestGeoJSON(rd, t.TempDir(), "uk")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Buildings != 1 {
		t.Fatalf("ingested %d buildings, want the merged 1", stats.Buildings)
	}
}
