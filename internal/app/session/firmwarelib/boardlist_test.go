package firmwarelib

import (
	"context"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
)

// The published board images reach the library. This is the bug 0.0.1
// shipped: the panel only ever listed native builds, so the emulated half was
// invisible - and on a Mac, where no native build exists, the panel was empty.
func TestPublishedBoardsReachTheLibrary(t *testing.T) {
	if testing.Short() {
		t.Skip("asks GitHub for the catalogue")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// The catalogue is asked for directly first, so that an empty answer can
	// be attributed. publishedBoards swallows the fetch error and returns
	// nothing either way, which left this test unable to tell "GitHub said no"
	// from "we broke the mapping" - and it has now failed three times on the
	// former, each time passing locally straight afterwards.
	cat := &emulated.BoardCatalogue{CacheDir: firmware.DefaultCacheDir()}
	imgs, err := cat.ListAll(ctx)
	if err != nil {
		t.Skipf("the release catalogue was not reachable: %v", err)
	}
	if len(imgs) == 0 {
		t.Skip("the release catalogue came back empty, which is not this code's doing")
	}

	got := publishedBoards(ctx)
	if len(got) == 0 {
		t.Fatalf("the catalogue listed %d images and none of them reached the "+
			"library: the emulated half is missing again", len(imgs))
	}
	t.Logf("%d board builds", len(got))
	boards := map[string]bool{}
	for _, b := range got {
		boards[b.board] = true
		if b.board == "" || b.role == "" || b.version == "" {
			t.Errorf("incomplete row: %+v", b)
		}
	}
	t.Logf("boards: %d", len(boards))
	for i, b := range got {
		if i >= 5 {
			break
		}
		t.Logf("  %s %s %s", b.board, b.role, b.version)
	}
}
