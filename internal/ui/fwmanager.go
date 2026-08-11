package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/firmware"
)

// drawInstalledBuilds is what is actually on this machine.
//
// The catalogue above it lists what has been published; this lists what a node
// can run right now, which is a different question and the one that matters
// when a node will not start. A build that failed to download halfway, one left
// behind by a rename, and one in daily use are indistinguishable from outside
// the cache directory, and until this existed the answer lived in a shell.
//
// Native builds are executables for this machine, so their board column says so
// rather than being left blank: an empty cell reads as missing information
// where "native" is the information.
func (a *App) drawInstalledBuilds() {
	cache := firmware.DefaultCacheDir()
	installed := firmware.ListInstalled(cache)

	imgui.SeparatorText("Installed on this machine")
	if len(installed) == 0 {
		textDim("nothing downloaded yet - versions arrive on first use, or " +
			"import a local build below")
		return
	}

	// In use is per role and version, so a build about to be deleted can say
	// whether anything is relying on it.
	inUse := map[string]int{}
	for i := range a.Nodes {
		n := &a.Nodes[i]
		if !n.Kind.RunsFirmware() {
			continue
		}
		role := n.Firmware.Role
		if role == "" {
			role = n.Kind.Application()
		}
		version := n.Firmware.Version
		if version == "" {
			version = "main"
		}
		inUse[role+" "+version]++
	}

	pad := imgui.CurrentStyle().FramePadding().X*2 + 8
	if imgui.BeginTableV("##fwinstalled", 6,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY|
			imgui.TableFlagsSizingStretchProp, imgui.NewVec2(0, a.fwTableHeight(0.35)), 0) {
		imgui.TableSetupColumnV("role", imgui.TableColumnFlagsWidthStretch, 0, 0)
		imgui.TableSetupColumnV("version", imgui.TableColumnFlagsWidthStretch, 0, 0)
		imgui.TableSetupColumnV("board", imgui.TableColumnFlagsWidthStretch, 0, 0)
		imgui.TableSetupColumnV("size", imgui.TableColumnFlagsWidthFixed,
			imgui.CalcTextSize("000.0 MB").X+pad, 0)
		imgui.TableSetupColumnV("in use by", imgui.TableColumnFlagsWidthFixed,
			imgui.CalcTextSize("000 nodes").X+pad, 0)
		imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthFixed,
			imgui.CalcTextSize("delete").X+pad*2, 0)
		imgui.TableHeadersRow()

		for _, in := range installed {
			id := in.Role + in.Version + in.Board
			imgui.TableNextRow()

			imgui.TableSetColumnIndex(0)
			imgui.TextUnformatted(in.Role)

			imgui.TableSetColumnIndex(1)
			imgui.TextUnformatted(in.Version)

			imgui.TableSetColumnIndex(2)
			if in.Native {
				// Said rather than left blank: an empty cell reads as unknown,
				// and "native" is the answer, not the absence of one.
				textDim("native")
			} else {
				imgui.TextUnformatted(in.Board)
			}

			imgui.TableSetColumnIndex(3)
			textDim(humanBytes(in.Bytes))

			imgui.TableSetColumnIndex(4)
			if n := inUse[in.Role+" "+in.Version]; n > 0 {
				imgui.TextUnformatted(fmt.Sprintf("%d nodes", n))
			} else {
				textDim("-")
			}

			imgui.TableSetColumnIndex(5)
			// Deleting a build the scenario is using would leave those nodes
			// unable to start, with the failure arriving at play rather than
			// here, so it is confirmed rather than immediate.
			if a.confirmDeleteFW == id {
				if dangerButton("sure?##"+id, imgui.NewVec2(0, 0)) {
					a.confirmDeleteFW = ""
					if err := firmware.Remove(firmware.DefaultCacheDir(), in); err != nil {
						a.status = "delete failed: " + err.Error()
					} else {
						a.status = "deleted " + in.Label()
					}
				}
			} else if imgui.SmallButton("delete##" + id) {
				a.confirmDeleteFW = id
			}
		}
		imgui.EndTable()
	}

	if a.confirmDeleteFW != "" {
		textDim("a build in use by nodes will leave them unable to start")
	}
	textDim("builds  " + cache)
}

// fwTableHeight gives a table a share of whatever room the window has.
//
// Fixed pixel heights were the reason this window did not scale: two tables at
// 320 and 220 px leave a tall window mostly empty and a short one scrolling
// inside a scroll. A floor keeps a deliberately small window usable rather than
// showing one row and a scrollbar.
func (a *App) fwTableHeight(share float32) float32 {
	avail := imgui.ContentRegionAvail().Y
	h := avail * share
	if min := imgui.FrameHeight() * 5; h < min {
		return min
	}
	return h
}

// humanBytes keeps a size column narrow enough to stay beside the actions.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f kB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// download fetches a published build now, rather than on first use.
//
// Lazy download is right when someone presses play and waits a moment; it is
// wrong when they are about to go somewhere without a network, or want a run to
// start on the instant. Both are ordinary, and neither is served by a catalogue
// that can only be browsed.
//
// Off the frame thread, because the window must keep drawing while GitHub takes
// its time.
func (a *App) downloadBuild(role, version string) {
	key := role + "@" + version
	if a.fwDownloads == nil {
		a.fwDownloads = map[string]bool{}
	}
	if a.fwDownloads[key] {
		return
	}
	a.fwDownloads[key] = true
	a.status = "downloading " + role + " " + version

	go func() {
		cat := &firmware.NativeCatalogue{CacheDir: firmware.DefaultCacheDir()}
		ctx, cancel := context.WithTimeout(context.Background(), fwDownloadTimeout)
		defer cancel()
		_, err := cat.Ensure(ctx, role, version)

		a.fwDlMu.Lock()
		defer a.fwDlMu.Unlock()
		delete(a.fwDownloads, key)
		if err != nil {
			a.fwDownloadErr = "download failed: " + err.Error()
			return
		}
		a.fwDownloadErr = ""
		a.fw.markCached(role, version)
	}()
}

// fwDownloadTimeout bounds a fetch. A build is a few hundred kilobytes, so a
// minute is generous; leaving it unbounded means a dead network shows a button
// that says "downloading" for ever.
const fwDownloadTimeout = 60 * time.Second

// fetching reports whether a build is on its way, so the row can say so instead
// of offering a button that does nothing the second time it is pressed.
func (a *App) fwDownloading(role, version string) bool {
	a.fwDlMu.Lock()
	defer a.fwDlMu.Unlock()
	return a.fwDownloads[role+"@"+version]
}
