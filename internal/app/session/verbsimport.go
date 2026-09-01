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
	st.HandleSpec("import.describe", state.Spec{
		What: "count what a deployment would bring in, without setting it as " +
			"the import source or changing a node, so a URL can be weighed " +
			"before it is committed to",
		Params: []state.Param{
			{Name: "url", Type: state.ParamString, Primary: true,
				What: "the deployment to read, as a bare string or the one key " +
					"of an object; it is not checked here, so an empty or " +
					"unreachable one is reported by the read failing a moment " +
					"later rather than by this call being refused"},
		},
		Returns: []string{"url"},
		Answers: "It returns at once with the URL it started on. The counts " +
			"arrive later on the snapshot, as records, importable, no " +
			"position and placed loosely: the last of those is the nodes " +
			"whose published position is too loose to trust to a decibel, " +
			"which are kept and marked rather than dropped. An accepted study " +
			"area narrows it, which is read here rather than in the worker. " +
			"The read has ninety seconds.",
		Example: &state.Example{
			Params:   map[string]any{"url": "https://map.example.net"},
			What:     "count what a deployment holds before importing it",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleInternalSpec("import.set", state.Spec{
		What: "put the finished description on the snapshot and say what it " +
			"found, which is how import.describe answers at all",
		Answers: "Nothing: it writes the snapshot and returns nil.",
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleInternalSpec("import.failed", state.Spec{
		What: "say a read failed and end the traffic job with it, so a scripted " +
			"import fails at once instead of waiting out its whole timeout on a " +
			"job that will never finish",
		Answers: "Nothing: it marks the job failed and returns nil.",
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("feed.stop", state.Spec{
		What: "stop following a deployment's live traffic, which means not " +
			"starting the next pull and throwing away the one still in the air " +
			"rather than closing a connection",
		Returns: []string{"stopped"},
		Answers: "`stopped` is false when no feed was running, which is an " +
			"answer rather than a refusal. Receptions already pulled stay where " +
			"they are.",
		Example: &state.Example{
			Params: map[string]any{}, What: "stop following the live traffic",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		was := s.feeding.Swap(false)
		if !was {
			w.Say("no live feed was running")
			return map[string]any{"stopped": false}, nil
		}
		w.Say("live feed stopped; the traffic already pulled stays")
		return map[string]any{"stopped": true}, nil
	})

	st.HandleSpec("feed.pull", state.Spec{
		What: "fetch the last hour of a deployment's real receptions and put " +
			"them beside this scenario's links, which is what turns a simulation " +
			"into something with a measured answer to compare against",
		Params: []state.Param{
			{Name: "url", Type: state.ParamString, Required: true, Primary: true,
				What: "the deployment to pull from, as a bare string or under " +
					"this key; an empty one is refused, and so is an object " +
					"whose single key is anything else"},
		},
		Returns: []string{"url"},
		Answers: "It returns as soon as the pull is accepted, not when the " +
			"receptions land: they arrive later, and the residuals against this " +
			"scenario's links are computed with them rather than behind a second " +
			"call. A pull that comes back after feed.stop is thrown away. The " +
			"fetch has ninety seconds.",
		Example: &state.Example{
			Params:   map[string]any{"url": "https://map.example.net"},
			What:     "follow what a real deployment is hearing",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
		url := soleString(p)
		if m, ok := p.(map[string]any); ok {
			url, _ = m["url"].(string)
		}
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

	st.HandleInternalSpec("feed.failed", state.Spec{
		What: "say the live feed came back with nothing and why, kept separate " +
			"from the import's own failure because a deployment can publish " +
			"nodes and not receptions",
		Answers: "Nothing: it says so and returns nil.",
	}, func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Say("no live feed: " + msg)
		return nil, nil
	})

	st.HandleInternalSpec("feed.set", state.Spec{
		What: "take the receptions a pull came back with and compute the " +
			"residuals against this scenario's links in the same step, so " +
			"observed and predicted are never one button apart",
		Returns: []string{"receptions"},
		Answers: "`receptions` is everything pulled; how many of them matched a " +
			"link in this scenario is a smaller number, and it goes on the " +
			"snapshot rather than in this answer.",
	}, func(w *state.World, p any) (any, error) {
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
