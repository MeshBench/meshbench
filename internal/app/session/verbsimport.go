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
		url := soleString(p)
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

	st.Handle("import.set", func(w *state.World, p any) (any, error) {
		im, _ := p.(*state.Import)
		w.Import = im
		if im != nil {
			w.Say(fmt.Sprintf(
				"%s: %d records, %d importable, %d with no position, %d placed loosely",
				im.URL, im.Records, im.Nodes, im.SkippedNoPosition, im.Uncertain))
		}
		return nil, nil
	})

	st.Handle("import.failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Say("import failed: " + msg)
		return nil, nil
	})

	// feed.stop: the live feed is a pull with a deadline rather than a socket
	// held open, so stopping it means not starting the next one. Said plainly
	// because a stop button that appears to do nothing is worse than no stop
	// button.
	st.Handle("feed.stop", func(w *state.World, _ any) (any, error) {
		s.feeding.Store(false)
		w.Say("live feed stopped; the traffic already pulled stays")
		return map[string]any{"stopped": true}, nil
	})

	st.Handle("feed.pull", func(w *state.World, p any) (any, error) {
		url := soleString(p)
		if m, ok := p.(map[string]any); ok {
			url, _ = m["url"].(string)
		}
		s.feeding.Store(true)
		if url == "" {
			return nil, fmt.Errorf("no deployment to pull from")
		}
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
			_, _ = st.Do(context.Background(), "feed.set", obs)
		}()
		return map[string]any{"url": url}, nil
	})

	st.Handle("feed.failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Say("no live feed: " + msg)
		return nil, nil
	})

	st.Handle("feed.set", func(w *state.World, p any) (any, error) {
		obs, _ := p.([]state.Observed)
		w.Observed = obs
		// Residuals fall out of the same data, so they are computed here
		// rather than behind a second button somebody has to know to press.
		w.Residuals = s.residualsOf(obs, w.Links, w.Nodes)
		w.Say(fmt.Sprintf("%d receptions; %d matched a link in this scenario",
			len(obs), w.Residuals.Matched))
		return map[string]any{"receptions": len(obs)}, nil
	})
}
