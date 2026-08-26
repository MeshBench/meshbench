// Bringing a real deployment in from a live feed.
//
// Four steps in a fixed order, and every one of them has been skipped by
// somebody at least once. The two that get missed are the last two, and
// missing them does not fail: the mesh comes up with regions inferred but
// never applied, which transmits everything, relays nothing, and reports no
// error at all. It reads as bad RF.
//
// So the steps are here individually, because sometimes you want to look at a
// preview before committing - and Pull runs all four, because the ordinary
// case is wanting the whole deployment and the ordinary mistake is stopping
// early.
package meshbench

import (
	"context"
	"fmt"
	"time"
)

// DefaultWindow is how far back to read traffic when working out what each
// node holds.
//
// A week, because that is what it takes for the quiet regions to say anything
// at all: on ScotMesh a small region is about sixty packets in seven days, and
// a shorter window drops it entirely rather than reporting it as thin.
const DefaultWindow = 7 * 24 * time.Hour

// ImportPreview is what a fetch found, before anything has been changed.
//
// SkippedNoPosition and Uncertain are the two worth reading before committing.
// A node with no position cannot be simulated at all, and an uncertain one is
// being placed to within kilometres - the answer it gives is that vague too,
// however confident the rest of the output looks.
type ImportPreview struct {
	Records           int `json:"records"`
	Nodes             int `json:"nodes"`
	SkippedNoPosition int `json:"skipped_no_position"`
	Uncertain         int `json:"uncertain"`
}

func (p ImportPreview) String() string {
	out := fmt.Sprintf("%d records, %d usable", p.Records, p.Nodes)
	if p.SkippedNoPosition > 0 {
		out += fmt.Sprintf(", %d with no position", p.SkippedNoPosition)
	}
	if p.Uncertain > 0 {
		out += fmt.Sprintf(", %d placed only roughly", p.Uncertain)
	}
	return out
}

// Live is a live feed, and the deployment it describes. Live in both senses.
type Live struct{ w *Workbench }

// Live reaches the import chain.
func (w *Workbench) Live() Live { return Live{w} }

// Pull fetches, commits, reads the traffic and applies what it implies.
//
// The whole chain, in the order that works. window is how far back into the
// feed's own history to read - the mesh's past, not your patience - and zero
// means DefaultWindow. wait is yours.
//
// Link measurement is still running when this returns on anything but a small
// mesh, so follow it with WaitIdle before starting a run.
func (l Live) Pull(ctx context.Context, url string, window, wait time.Duration) (ImportPreview, error) {
	preview, err := l.Fetch(ctx, url)
	if err != nil {
		return preview, err
	}
	if preview.Nodes == 0 {
		return preview, fmt.Errorf("%s described %d nodes, none usable", url, preview.Records)
	}
	if _, err := l.Commit(ctx, ""); err != nil {
		return preview, err
	}
	if err := l.Infer(ctx, window, wait); err != nil {
		return preview, err
	}
	_, err = l.ApplyRegions(ctx)
	return preview, err
}

// SetSource points at a feed without reading it, and reports the URL as the
// workbench tidied it.
func (l Live) SetSource(ctx context.Context, url string) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	err := l.w.CallInto(ctx, "import.set_source", map[string]any{"url": url}, &out)
	return out.URL, err
}

// Fetch reads the deployment and says what would change, changing nothing.
func (l Live) Fetch(ctx context.Context, url string) (ImportPreview, error) {
	var out ImportPreview
	if url != "" {
		if _, err := l.SetSource(ctx, url); err != nil {
			return out, err
		}
	}
	return out, l.w.CallInto(ctx, "import.fetch", nil, &out)
}

// Commit applies the fetched nodes to the scenario and says how many it now
// holds.
//
// "replace-all" is the default and is what the shipped fixtures were built
// with; "add" keeps what is already here and skips names that clash.
//
// Measuring the links afterwards is a job rather than part of this call - 676
// nodes is 228,000 terrain paths over real ground - so this returns while that
// is still running.
func (l Live) Commit(ctx context.Context, strategy Strategy) (int, error) {
	params := map[string]any{}
	if strategy != "" {
		params["strategy"] = strategy
	}
	var out struct {
		Nodes int `json:"nodes"`
	}
	return out.Nodes, l.w.CallInto(ctx, "import.commit", params, &out)
}

// Infer reads the feed's recent traffic to work out what each node holds.
//
// This is the step that decides whether anything relays. A node whose regions
// are unknown forwards nothing, and nothing says so.
//
// window is the feed's own past and zero means DefaultWindow; wait is how long
// you will sit here for it. A week of ScotMesh is around 150,000 packets and
// several minutes of paging.
func (l Live) Infer(ctx context.Context, window, wait time.Duration) error {
	if window <= 0 {
		window = DefaultWindow
	}
	if err := l.w.Do(ctx, "infer.run", map[string]any{"hours": window.Hours()}); err != nil {
		return err
	}
	return l.w.Job("infer").Wait(ctx, wait)
}

// ApplyRegions puts the inferred regions onto the nodes, and says how many
// took one.
//
// The forgotten step. Everything above can succeed and the mesh still be
// silent until this runs.
func (l Live) ApplyRegions(ctx context.Context) (int, error) {
	var out struct {
		Applied int `json:"applied"`
	}
	return out.Applied, l.w.CallInto(ctx, "infer.apply", nil, &out)
}
