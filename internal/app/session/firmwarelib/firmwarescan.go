// What the catalogue offers, and asking it again.
//
// The fetch and the verb that repeats it live together because they are one
// question asked twice: the library asks it once on the way past, and the scan
// button asks it again when somebody has published something since.
package firmwarelib

import (
	"context"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

func registerFirmwareScan(st *state.Store, s *session.Sim) {
	// The catalogue is read once and kept, so without this a session that
	// started before a release could never be told about it. Forgetting the
	// last answer is what makes it a rescan; a fetch already in flight is left
	// alone, because two in flight would race to land.
	st.Handle("firmware.rescan", func(w *state.World, _ any) (any, error) {
		if !catalogueOf(s).fetching {
			catalogueOf(s).published = nil
		}
		startPublishedFetch(s, st)
		fillLibrary(s, w)
		return map[string]any{
			"scanning": catalogueOf(s).fetching, "count": len(w.Library),
		}, nil
	})

	st.HandleInternal("firmware.published", func(w *state.World, p any) (any, error) {
		var ans publishedAnswer
		switch v := p.(type) {
		case publishedAnswer:
			ans = v
		case []publishedBuild:
			ans.builds = v
		default:
			return nil, session.WrongCallback("firmware.published")
		}
		list := ans.builds
		if list == nil {
			list = []publishedBuild{}
		}
		catalogueOf(s).published = list
		catalogueOf(s).nativeDown, catalogueOf(s).boardsDown = ans.nativeDown, ans.boardsDown
		catalogueOf(s).fetching = false
		// Rebuild the rows here, or the fetch lands in a field nobody reads
		// again: the panel asks for the library once, the network answers a
		// few seconds later, and without this the answer sits unused until
		// somebody presses refresh. Which is what "the published builds never
		// appear" looked like.
		fillLibrary(s, w)
		return map[string]any{"published": len(list), "builds": len(w.Library)}, nil
	})
}

// startPublishedFetch asks the catalogue what exists, once, off the store's
// goroutine, and hands the answer back through firmware.published.
//
// Called from the store's goroutine, which is what makes the two flags a
// single-flight guard rather than a race.
func startPublishedFetch(s *session.Sim, st *state.Store) {
	if catalogueOf(s).published != nil || catalogueOf(s).fetching {
		return
	}
	catalogueOf(s).fetching = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		list := []publishedBuild{}
		// The native builds: MeshCore compiled for this machine, which is how
		// most nodes run.
		cat := &firmware.NativeCatalogue{CacheDir: firmware.DefaultCacheDir()}
		images, nativeErr := cat.List(ctx)
		if nativeErr == nil {
			for _, img := range images {
				if img.ForThisMachine() {
					list = append(list, publishedBuild{
						role: img.Role, version: img.Version,
					})
				}
			}
		}
		// And the published board images - the ones the flasher serves, which
		// emulated boards run. workbench1 listed these and the Gio library
		// never did, so the whole emulated half of the library was missing: on
		// Linux the native builds hid it, and on a Mac, where no native build
		// exists yet, the panel was simply empty.
		boards, boardsErr := publishedBoards(ctx)
		list = append(list, boards...)
		// An empty non-nil list on failure, so a dead network is one failed
		// fetch rather than a fetch per frame.
		_, _ = st.Do(context.Background(), "firmware.published", publishedAnswer{
			builds: list, nativeDown: nativeErr != nil, boardsDown: boardsErr != nil,
		})
	}()
}

// publishedAnswer is what a fetch hands back: the builds, and which half of
// the catalogue could not be reached.
type publishedAnswer struct {
	builds                 []publishedBuild
	nativeDown, boardsDown bool
}
