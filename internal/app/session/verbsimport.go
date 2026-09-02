// Bringing a real network in, and following it live: the CoreScope import and
// the observed-traffic feed.
package session

import (
	"context"
	"fmt"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerImportFeedVerbs(st *state.Store, s *Sim) {
	st.Handle("import.describe", func(w *state.World, p any) (any, error) {
		url := primaryString(p, "url")
		// Refused before the worker starts. A parameter this could not read as
		// a URL used to start a ninety second fetch of the empty string, answer
		// `{"url": ""}` as though it had been accepted, and deliver the failure
		// through import.failed - which the caller who asked never subscribes
		// to. The refusal is the answer to the call that was made.
		if url == "" {
			return nil, badParams("import.describe needs a url to fetch")
		}
		// The study area as it stands, read here on the store's goroutine
		// rather than in the worker, which must not reach into the session.
		region := s.importRegion(float64(w.MarginKm))
		if region != nil {
			w.Say(fmt.Sprintf("fetching %s, keeping what is in the study area", url))
		} else {
			w.Say("fetching " + url)
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			im, err := ImportFrom(ctx, url, region)
			if err != nil {
				_, _ = st.Do(context.Background(), "import.failed", err.Error())
				return
			}
			_, _ = st.Do(context.Background(), "import.set", im)
		}()
		return map[string]any{"url": url}, nil
	})

	st.HandleInternal("import.set", func(w *state.World, p any) (any, error) {
		im, ok := p.(*state.Import)
		if !ok {
			return nil, wrongCallback("import.set")
		}
		w.Import = im
		if im != nil {
			w.Say(fmt.Sprintf(
				"%s: %d records, %d importable, %d with no position, %d placed loosely",
				im.URL, im.Records, im.Nodes, im.SkippedNoPosition, im.Uncertain))
		}
		return nil, nil
	})

	st.HandleInternal("import.failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		// End the job as well as saying so. It used to only say so, and the
		// reading job then sat in the list for ever: anything waiting on it -
		// which is every scripted import - waited out its whole timeout for a
		// read that had already failed, with the reason visible only in the log.
		for i := range w.Jobs {
			if w.Jobs[i].ID == "infer" {
				w.Jobs[i].Finished, w.Jobs[i].Failed = true, true
				w.Jobs[i].What = "reading traffic failed: " + msg
			}
		}
		w.Say("import failed: " + msg)
		return nil, nil
	})

	// The flag this clears used to be written and never read anywhere, which
	// made it the stop button that genuinely did nothing: the pull it was
	// pressed to stop landed a minute later and the panel filled up with the
	// traffic somebody had just asked it not to fetch.
	st.Handle("feed.stop", func(w *state.World, _ any) (any, error) {
		was := s.feeding.Swap(false)
		if !was {
			w.Say("no live feed was running")
			return map[string]any{"stopped": false}, nil
		}
		w.Say("live feed stopped; the traffic already pulled stays")
		return map[string]any{"stopped": true}, nil
	})

	st.Handle("feed.pull", func(w *state.World, p any) (any, error) {
		url := primaryString(p, "url")
		if url == "" {
			return nil, fmt.Errorf("no deployment to pull from")
		}
		// Set only once the request is accepted, so a refused pull does not
		// leave the feed reading as running.
		s.feeding.Store(true)
		w.Say("pulling recent receptions from " + url)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			obs, err := PullObserved(ctx, url, time.Hour)
			if err != nil {
				// Its own message, not the import one: a deployment can
				// publish nodes and not receptions, and telling somebody the
				// import failed when the import worked sends them to the
				// wrong place.
				_, _ = st.Do(context.Background(), "feed.failed", err.Error())
				return
			}
			// Stopped while this pull was in the air. Landing it anyway is
			// what made feed.stop do nothing at all: a pull takes up to ninety
			// seconds, and the receptions somebody had just asked not to fetch
			// arrived and were counted.
			if !s.feeding.Load() {
				_, _ = st.Do(context.Background(), "feed.failed",
					"stopped before the pull came back; nothing was added")
				return
			}
			_, _ = st.Do(context.Background(), "feed.set", obs)
		}()
		return map[string]any{"url": url}, nil
	})

	st.HandleInternal("feed.failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Say("no live feed: " + msg)
		return nil, nil
	})

	st.HandleInternal("feed.set", func(w *state.World, p any) (any, error) {
		obs, ok := p.([]state.Observed)
		if !ok {
			return nil, wrongCallback("feed.set")
		}
		w.Observed = obs
		// Residuals fall out of the same data, so they are computed here
		// rather than behind a second button somebody has to know to press.
		w.Residuals = s.residualsOf(obs, w.Links, w.Nodes)
		w.Say(fmt.Sprintf("%d receptions; %d matched a link in this scenario",
			len(obs), w.Residuals.Matched))
		return map[string]any{"receptions": len(obs)}, nil
	})
}
