package updates

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/update"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// downloadJob is the jobs-strip entry the fetch runs under. Tens of megabytes
// with nothing on screen is the thing three healthy waits were reported as
// crashes for.
const downloadJob = "update-download"

// downloadTimeout bounds the whole fetch, generously: a tarball on a slow line
// is a long wait and a legitimate one.
const downloadTimeout = 30 * time.Minute

func registerDownload(st *state.Store, s *session.Sim) {
	st.Handle("update.download", func(w *state.World, _ any) (any, error) {
		if version.Release() == "" {
			return nil, fmt.Errorf("this build is %s rather than a release, so "+
				"there is no newer release for it to take: a working copy is not "+
				"behind, it is unreleased", version.String())
		}
		if !available(w.Update) {
			return nil, fmt.Errorf("nothing to download: %s", whyNothing(w.Update))
		}
		dir, err := stageDir()
		if err != nil {
			return nil, err
		}
		u := w.Update
		startDownload(st, s, u, dir)
		return map[string]any{
			"downloading": u.Asset, "bytes": u.Bytes, "release": u.Latest,
			"into": dir,
		}, nil
	})

	// Where it landed, opened in whatever this desktop uses for a folder. The
	// point of the feature is not hunting for a tarball, and naming a path in
	// a status line is only most of the way there.
	st.Handle("update.reveal", func(w *state.World, _ any) (any, error) {
		if w.Update.Staged == "" {
			return nil, fmt.Errorf("nothing has been downloaded yet, so there " +
				"is no folder to open")
		}
		dir := filepath.Dir(w.Update.Staged)
		if why := openExternal(dir); why != "" {
			return nil, fmt.Errorf("could not open %s: %s", dir, why)
		}
		return map[string]any{"opened": dir}, nil
	})

	// The notes themselves are prose and will outgrow any panel, so they are
	// linked rather than embedded.
	st.Handle("update.notes", func(w *state.World, _ any) (any, error) {
		if w.Update.Notes == "" {
			return nil, fmt.Errorf("no release page is known yet: run " +
				"update.check first")
		}
		if why := openExternal(w.Update.Notes); why != "" {
			return nil, fmt.Errorf("could not open %s: %s", w.Update.Notes, why)
		}
		return map[string]any{"opened": w.Update.Notes}, nil
	})

	st.HandleInternal("update.staged", func(w *state.World, p any) (any, error) {
		path, _ := session.StringField(p, "path")
		w.Update.Staged = path
		w.Say(stagedWords(w.Update))
		return map[string]any{"staged": path}, nil
	})
}

// startDownload re-asks the feed, then fetches what it names.
//
// Re-asked rather than trusting what the check found, because a check can be
// hours old: what gets downloaded should be what is published now, and the
// asset URLs and the checksum file have to come from the same answer as each
// other or they are not about the same release.
func startDownload(st *state.Store, s *session.Sim, u state.Update, dir string) {
	feed := s.UpdateFeed()
	go func() {
		ctx, stop := context.WithTimeout(context.Background(), downloadTimeout)
		defer stop()
		what := "downloading MeshBench " + u.Latest + ", " + resource.SIBytes(u.Bytes)
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: downloadJob, What: what, Done: 0, Total: 1, Cancel: stop})
		path, err := fetch(ctx, feed, u, dir, func(done, total int64) {
			if total <= 0 {
				return
			}
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: downloadJob, What: what,
				Done: int(done >> 10), Total: int(total >> 10)})
		})
		done, release := session.Finishing(ctx)
		defer release()
		_, _ = st.Do(done, "job.done", downloadJob)
		if err != nil {
			if ctx.Err() != nil {
				_, _ = st.Do(done, "ui.said", "the update download was stopped; "+
					"nothing was changed")
				return
			}
			_, _ = st.Do(done, "ui.said", err.Error())
			return
		}
		_, _ = st.Do(done, "update.staged", map[string]any{"path": path})
	}()
}

// fetch is the download itself: ask, pick, verify, land.
func fetch(ctx context.Context, feed string, u state.Update, dir string,
	progress func(done, total int64)) (string, error) {

	c := update.Checker{Feed: feed}
	newest, err := c.Latest(ctx)
	if err != nil {
		return "", err
	}
	if !update.Newer(version.Release(), newest.Version) {
		return "", fmt.Errorf("update: the release page now says %s is the "+
			"newest, and this build is %s, so there is nothing to take",
			newest.Tag, version.Release())
	}
	// The API, for the assets and their digests. Asked here rather than reused
	// from the check because a check can be hours old, and what gets downloaded
	// should be what is published now.
	rel, err := c.Detail(ctx, newest.Tag)
	if err != nil {
		return "", err
	}
	a, why := update.AssetFor(rel, update.This(), runtime.GOOS, runtime.GOARCH, update.ThisVariant())
	if why != "" {
		return "", fmt.Errorf("update: %s", why)
	}
	st := update.Stage{Dir: dir, Redirected: c.Redirected()}
	return st.Get(ctx, rel, a, progress)
}

// stageDir is where a download lands: beside the other things this application
// fetches at runtime, and never beside the binary it might one day replace.
func stageDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("this machine has nowhere to keep a download: %w", err)
	}
	dir := filepath.Join(cache, "meshbench", "updates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// whyNothing is the refusal, and it distinguishes the four ways there can be
// nothing to download - which matter, because only two of them are worth doing
// anything about.
func whyNothing(u state.Update) string {
	switch {
	case u.Checked == "":
		return "nothing has asked the release page yet. Run update.check"
	case u.Err != "":
		return "the last check could not reach the release page: " + u.Err
	case u.Why != "":
		return u.Why
	case u.Latest == "":
		return "the last check found no published release"
	default:
		return "this build, " + version.Release() + ", is the newest release"
	}
}

// stagedWords is the line the status bar carries when a download lands.
//
// Short on purpose. The whole instruction - what to unpack, where to move it,
// and what it means for a pinned client - is content on the Setup row, where
// there is room to read it; a status bar four lines deep is a modal that has
// not admitted to being one.
func stagedWords(u state.Update) string {
	return "MeshBench " + u.Latest + " is downloaded and checked: " + u.Staged +
		". Nothing has been replaced; Setup, under This build, says how to swap it"
}

// clientWords is the consequence nobody expects until a script stops working.
//
// A client and the workbench it drives have to be the same release; the socket
// refuses the pair otherwise. So an update invalidates every pinned client on
// this machine, and being told that here is a great deal better than meeting it
// as a version-mismatch error in the middle of a run.
func clientWords(latest string) string {
	return "A client pinned to " + version.Release() + " will be refused by a " +
		latest + " workbench, so upgrade them together: pip install -U " +
		"meshbench==" + latest + ", npm install @meshbench/client@" + latest + "."
}
