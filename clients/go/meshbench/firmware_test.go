// The library, and getting a locally built image into it.
package meshbench

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Labelled imports, which is what makes replacing a build possible. Every
// import used to be called "imported" and land in a directory nothing lists,
// so a second one replaced the first in place: the library showed a single
// entry, and there was no way to say which of two local builds a node was on,
// or to delete the older.
func TestTwoImportsOfOneFileAreTwoBuilds(t *testing.T) {
	wb, ctx := headless(t)
	image := filepath.Join(t.TempDir(), "firmware.bin")
	if err := os.WriteFile(image, []byte("not a firmware, but it is a .bin"), 0o600); err != nil {
		t.Fatal(err)
	}

	// These land in the machine's real firmware cache - the verb uses it and
	// nothing overrides it - so they come back out however this ends.
	var made []Build
	t.Cleanup(func() {
		for _, b := range made {
			_, _ = wb.Firmware().Delete(context.Background(), b)
		}
	})
	imp := func(label string) Build {
		t.Helper()
		b, err := wb.Firmware().Import(ctx, image, "companion_radio", "LilyGo_TDeck", label)
		if err != nil {
			t.Fatal(err)
		}
		made = append(made, b)
		return b
	}

	first, second := imp("wadamesh-a"), imp("wadamesh-b")
	if first.Version != "wadamesh-a" || second.Version != "wadamesh-b" {
		t.Fatalf("labels were not kept: %q and %q", first.Version, second.Version)
	}

	// Both in the library and both on disk - the part that was broken. The
	// library answered with a count and left the rows in the snapshot where
	// only a panel could reach them, so this list came back as an integer.
	if !holds(t, wb, ctx, "wadamesh-a") || !holds(t, wb, ctx, "wadamesh-b") {
		t.Fatal("an imported build is not in the library")
	}

	// And the older one goes by itself, which is the point.
	if _, err := wb.Firmware().Delete(ctx, first); err != nil {
		t.Fatal(err)
	}
	if holds(t, wb, ctx, "wadamesh-a") {
		t.Error("the deleted build is still in the library")
	}
	if !holds(t, wb, ctx, "wadamesh-b") {
		t.Error("deleting one build took the other with it")
	}
}

func holds(t *testing.T, wb *Workbench, ctx context.Context, version string) bool {
	t.Helper()
	builds, err := wb.Firmware().OnDisk(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range builds {
		if b.Version == version {
			return true
		}
	}
	return false
}
