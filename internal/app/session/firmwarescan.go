// What the catalogue offers, and asking it again.
//
// The fetch and the verb that repeats it live together because they are one
// question asked twice: the library asks it once on the way past, and the scan
// button asks it again when somebody has published something since.
package session

import (
	"context"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

func registerFirmwareScan(st *state.Store, s *Sim) {
	// firmware.rescan: ask the catalogue again, for a build published since
	// this session started.
	//
	// The catalogue is read once and kept, so without this a session that
	// started before a release could never be told about it. Forgetting the
	// last answer is what makes it a rescan; a fetch already in flight is left
	// alone, because two in flight would race to land.
	st.HandleSpec("firmware.rescan", state.Spec{
		What: "ask the catalogue what is published again, which is how a " +
			"build nobody has downloaded becomes offerable",
		Returns: []string{"scanning", "count"},
	}, func(w *state.World, _ any) (any, error) {
		if !s.fetchingPublished {
			s.publishedNet = nil
		}
		s.startPublishedFetch(st)
		s.fillLibrary(w)
		return map[string]any{
			"scanning": s.fetchingPublished, "count": len(w.Library),
		}, nil
	})

	// firmware.published: what the catalogue offers, landed from the fetch.
	st.HandleInternal("firmware.published", func(w *state.World, p any) (any, error) {
		list, ok := p.([]publishedBuild)
		if !ok {
			return nil, wrongCallback("firmware.published")
		}
		s.publishedNet = list
		s.fetchingPublished = false
		// Rebuild the rows here, or the fetch lands in a field nobody reads
		// again: the panel asks for the library once, the network answers a
		// few seconds later, and without this the answer sits unused until
		// somebody presses refresh. Which is what "the published builds never
		// appear" looked like.
		s.fillLibrary(w)
		return map[string]any{"published": len(list), "builds": len(w.Library)}, nil
	})
}

// startPublishedFetch asks the catalogue what exists, once, off the store's
// goroutine, and hands the answer back through firmware.published.
//
// Called from the store's goroutine, which is what makes the two flags a
// single-flight guard rather than a race.
func (s *Sim) startPublishedFetch(st *state.Store) {
	if s.publishedNet != nil || s.fetchingPublished {
		return
	}
	s.fetchingPublished = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		list := []publishedBuild{}
		// The native builds: MeshCore compiled for this machine, which is how
		// most nodes run.
		cat := &firmware.NativeCatalogue{CacheDir: firmware.DefaultCacheDir()}
		if images, err := cat.List(ctx); err == nil {
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
		list = append(list, publishedBoards(ctx)...)
		// An empty non-nil list on failure, so a dead network is one failed
		// fetch rather than a fetch per frame.
		_, _ = st.Do(context.Background(), "firmware.published", list)
	}()
}
