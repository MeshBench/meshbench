package session

import (
	"context"
	"testing"
	"time"
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
	got := publishedBoards(ctx)
	if len(got) == 0 {
		t.Fatal("no published board images: the emulated half of the library is missing again")
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
