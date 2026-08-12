package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/firmware"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// drawFirmwareTable is every build, in one sortable, searchable list.
// selectedNode is the node the operator has picked, or nil.
//
// A board image is applied to one node rather than to a role, so the library
// needs to know which one — and needs to say "select a node first" rather than
// silently doing nothing when none is picked.
// boardImageFor finds a published image for a board, role and version.
//
// It answers from the catalogue the library has already loaded, so it says what
// is wrong rather than what is missing: an unknown board, a board nothing has
// been verified on, or a role that board does not publish are three different
// problems and each sends someone somewhere different. The E22 publishes a
// repeater and no companion, and "not found" would have read as a bad version.
func (a *App) boardImageFor(board, role, version string) (firmware.BoardImage, error) {
	if !scenario.EmulationSupported(board) {
		ok, blocked := scenario.EmulatableBoards()
		if why, isBlocked := blocked[board]; isBlocked {
			return firmware.BoardImage{}, fmt.Errorf("%s cannot be emulated: %s", board, why)
		}
		names := make([]string, 0, len(ok))
		for _, b := range ok {
			names = append(names, b.Name)
		}
		return firmware.BoardImage{}, fmt.Errorf(
			"no verified emulation wiring for %q - these have it: %s",
			board, strings.Join(names, ", "))
	}
	var roles []string
	for _, img := range a.fw.boardImages() {
		// Bootable means different things per family: a merged .bin for the
		// ESP32 boards, a .uf2 for the nRF52 ones, which are never merged
		// because their bootloader and SoftDevice are flashed separately.
		bootable := (img.Format == "bin" && img.Merged) || img.Format == "uf2"
		if !strings.EqualFold(img.Board, board) || !bootable {
			continue
		}
		if img.Version == version && img.Role == role {
			return img, nil
		}
		if img.Version == version {
			roles = append(roles, img.Role)
		}
	}
	if len(roles) > 0 {
		return firmware.BoardImage{}, fmt.Errorf(
			"%s publishes no %s at %s - it publishes: %s",
			board, role, version, strings.Join(roles, ", "))
	}
	return firmware.BoardImage{}, fmt.Errorf("no %s image for %s at %s", board, role, version)
}

func (a *App) selectedNode() *scenario.Node {
	if a.selected < 0 || a.selected >= len(a.Nodes) {
		return nil
	}
	return &a.Nodes[a.selected]
}

func (a *App) drawFirmwareTable() {
	all := a.fwRows()

	// Every published version of every supported board is thousands of rows,
	// so a filter is not a convenience here.
	imgui.SetNextItemWidth(imgui.CalcTextSize("companion-v1.17.0-faultyirq").X + 40)
	imgui.InputTextWithHint("##fwsearch", "search role, version or board",
		&a.fwSearch, 0, nil)
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Several words all have to match, so \"repeater 1.17\"\n" +
			"narrows further than either on its own.")
	}
	imgui.SameLine()
	imgui.Checkbox("on disk only", &a.fwOnDiskOnly)
	imgui.SameLine()
	if imgui.Checkbox("boards only", &a.fwBoardsOnly) && a.fwBoardsOnly {
		a.fwNativeOnly = false // the two are opposites; together they show nothing
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("The published images, run as emulated hardware.")
	}
	imgui.SameLine()
	if imgui.Checkbox("native only", &a.fwNativeOnly) && a.fwNativeOnly {
		a.fwBoardsOnly = false
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Host builds: MeshCore compiled for this machine.")
	}

	// The key. Two colours with no legend is a puzzle, and the difference
	// between them is the whole reason the column exists.
	textDim("key")
	imgui.SameLine()
	textColoured(colOK, "native")
	imgui.SameLine()
	textDim("host build")
	imgui.SameLine()
	textDim("  ·  ")
	imgui.SameLine()
	textColoured(colWarn, "board")
	imgui.SameLine()
	textDim("emulated hardware")

	var rows []fwRow
	for _, r := range all {
		if fwMatches(r, a.fwSearch, a.fwOnDiskOnly, a.fwBoardsOnly, a.fwNativeOnly) {
			rows = append(rows, r)
		}
	}
	if len(all) > 0 {
		imgui.SameLine()
		if len(rows) == len(all) {
			textDim(fmt.Sprintf("%d builds", len(all)))
		} else {
			// The total as well as the count: "12 of 3,893" says the filter is
			// working, where "12 builds" reads as a catalogue that is nearly
			// empty.
			textDim(fmt.Sprintf("%d of %d", len(rows), len(all)))
		}
	}
	if len(rows) == 0 {
		if len(all) > 0 {
			textDim("nothing matches that search")
		} else {
			textDim("nothing published for this machine and nothing downloaded - " +
				"check the network, or import a local build below")
		}
		return
	}

	pad := imgui.CurrentStyle().FramePadding().X*2 + 8
	if !imgui.BeginTableV("##fwall", 7,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY|
			imgui.TableFlagsScrollX|imgui.TableFlagsSortable|
			imgui.TableFlagsSortTristate|
			imgui.TableFlagsSizingFixedFit|imgui.TableFlagsResizable,
		imgui.NewVec2(0, a.fwTableHeight()), 0) {
		return
	}
	// Widths from the longest real content. Stretch alone let the action column
	// win the argument and left "companion_rad" and "companion-v1.17.0-f".
	imgui.TableSetupColumnV("role", imgui.TableColumnFlagsWidthFixed,
		imgui.CalcTextSize("simple_room_server").X+pad, 0)
	imgui.TableSetupColumnV("version", imgui.TableColumnFlagsWidthFixed,
		imgui.CalcTextSize("companion-v1.17.0-faultyirq").X+pad, 0)
	imgui.TableSetupColumnV("board", imgui.TableColumnFlagsWidthFixed,
		imgui.CalcTextSize("Generic_E22_sx1262").X+pad, 0)
	// Fixed columns measured from their own widest content, not guessed in
	// pixels: a label is one width in one font and clipped in another.
	imgui.TableSetupColumnV("on disk", imgui.TableColumnFlagsWidthFixed,
		imgui.CalcTextSize("downloading...").X+pad, 0)
	imgui.TableSetupColumnV("in use by", imgui.TableColumnFlagsWidthFixed,
		imgui.CalcTextSize("000 nodes").X+pad, 0)
	// The action column is not sortable: there is nothing in it to order by.
	// Measured from the buttons that actually appear, not from a role name.
	// "use for all companion_radio" was the widest string in the table and it
	// squeezed role and version into ellipses to make room for itself.
	// One action per column. Two buttons sharing a cell sit unevenly and shift
	// as rows gain or lose the second one, so each gets its own and they line
	// up down the table whether or not the row has both.
	imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthFixed|imgui.TableColumnFlagsNoSort,
		imgui.CalcTextSize("use for selected").X+pad*2, 0)
	imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthFixed|imgui.TableColumnFlagsNoSort,
		imgui.CalcTextSize("delete").X+pad*2, 0)
	imgui.TableSetupScrollFreeze(0, 1)
	imgui.TableHeadersRow()

	// Sorted every frame rather than only when imgui says the specification is
	// dirty: the rows are rebuilt each frame from the cache and the scenario,
	// so a sort applied once would be lost as soon as anything changed.
	col, ascending := fwSortDefault, true
	if spec := imgui.TableGetSortSpecs(); spec != nil && spec.SpecsCount() > 0 {
		s := spec.Specs()
		col = int(s.ColumnIndex())
		ascending = s.SortDirection() == imgui.SortDirectionAscending
	}
	fwSortRows(rows, col, ascending)

	for _, r := range rows {
		id := "##" + strings.ReplaceAll(r.key(), "\x00", "-")
		imgui.TableNextRow()

		imgui.TableSetColumnIndex(0)
		imgui.TextUnformatted(r.Role)

		imgui.TableSetColumnIndex(1)
		imgui.TextUnformatted(r.Version)

		imgui.TableSetColumnIndex(2)
		// Green for a host build, orange for a board image. The distinction
		// decides what a row can do - one runs in a scenario today, the other
		// needs an emulator - and it is worth seeing without reading.
		if r.Board == "" {
			textColoured(colOK, r.boardLabel())
		} else {
			textColoured(colWarn, r.boardLabel())
		}

		imgui.TableSetColumnIndex(3)
		switch {
		case r.OnDisk:
			textColoured(colOK, humanBytes(r.Bytes))
		case a.fwDownloading(r.Role, r.Version) ||
			(r.Board != "" && a.fwDownloading(r.Board+"/"+r.Role, r.Version)):
			textDim("downloading...")
		default:
			// Explicit, as well as on first use. Someone about to work without
			// a network is not served by a catalogue that can only be browsed.
			if imgui.SmallButton("download" + id) {
				if r.Board == "" {
					a.downloadBuild(r.Role, r.Version)
				} else {
					a.downloadImage(r.image)
				}
			}
		}

		imgui.TableSetColumnIndex(4)
		if r.InUse > 0 {
			imgui.TextUnformatted(fmt.Sprintf("%d nodes", r.InUse))
		} else {
			textDim("-")
		}

		imgui.TableSetColumnIndex(5)
		// A board image sets a role the same way a host build does. The only
		// difference is what it costs: every node it lands on becomes its own
		// emulator running in real time, which the tooltip says out loud with
		// the actual count rather than leaving it to be discovered at play.
		if r.Board == "" || r.OnDisk {
			if imgui.SmallButton("use for role" + id) {
				n := 0
				for i := range a.Nodes {
					if a.Nodes[i].Kind.RunsFirmware() &&
						(a.Nodes[i].Firmware.Role == r.Role || a.Nodes[i].Kind.Application() == r.Role) {
						a.Nodes[i].Firmware.Role, a.Nodes[i].Firmware.Version = r.Role, r.Version
						// The board selects the backend, so it is set on the way
						// to emulated hardware and cleared on the way back.
						// Without the clear, a node emulated once stays emulated
						// for ever, on a host build that never matches it.
						a.Nodes[i].Firmware.Board = r.Board
						n++
					}
				}
				a.rebuildForFirmware()
				if r.Board == "" {
					a.status = fmt.Sprintf("%d %s nodes set to %s", n, r.Role, r.Version)
				} else {
					a.status = fmt.Sprintf("%d %s nodes set to emulated %s %s",
						n, r.Role, r.Board, r.Version)
				}
			}
			if imgui.IsItemHovered() {
				// Scoped to the role, and said out loud. A node's application is
				// what makes it a companion in the first place, so nothing here
				// turns one kind of node into another.
				want := 0
				for i := range a.Nodes {
					if a.Nodes[i].Kind.RunsFirmware() &&
						(a.Nodes[i].Firmware.Role == r.Role || a.Nodes[i].Kind.Application() == r.Role) {
						want++
					}
				}
				tip := fmt.Sprintf("Set every %s node to %s.\nOther roles are left alone.",
					r.Role, r.Version)
				if r.Board != "" {
					tip += fmt.Sprintf("\n\n%d node(s) would run emulated %s - one emulator"+
						"\neach, in real time. A large fleet will not fit.", want, r.Board)
				}
				imgui.SetTooltip(tip)
			}
		}

		imgui.TableSetColumnIndex(6)
		if r.OnDisk {
			// Deleting a build the scenario is using leaves those nodes unable
			// to start, and the failure arrives at play rather than here, so it
			// is confirmed rather than immediate.
			if a.confirmDeleteFW == r.key() {
				if dangerButton("sure?"+id, imgui.NewVec2(0, 0)) {
					a.confirmDeleteFW = ""
					if err := firmware.Remove(firmware.DefaultCacheDir(), r.installed); err != nil {
						a.status = "delete failed: " + err.Error()
					} else {
						a.status = "deleted " + r.installed.Label()
					}
				}
			} else if imgui.SmallButton("delete" + id) {
				a.confirmDeleteFW = r.key()
			}
		}
	}
	imgui.EndTable()

	if a.confirmDeleteFW != "" {
		textDim("a build in use by nodes will leave them unable to start")
	}
}

// fwTableHeight leaves room for what sits under the table and gives it the rest.
//
// Fixed pixel heights were why this window did not scale: a tall window sat
// mostly empty and a short one scrolled inside a scroll. The reserve is the
// storage lines and the wipe button below; the floor keeps a deliberately small
// window usable rather than showing one row and a scrollbar.
func (a *App) fwTableHeight() float32 {
	h := imgui.ContentRegionAvail().Y - imgui.FrameHeight()*5
	if min := imgui.FrameHeight() * 6; h < min {
		return min
	}
	return h
}

// downloadBuild fetches a published build now, rather than on first use.
//
// Off the frame thread, because the window must keep drawing while GitHub takes
// its time.
func (a *App) downloadBuild(role, version string) {
	key := role + "@" + version
	a.fwDlMu.Lock()
	if a.fwDownloads == nil {
		a.fwDownloads = map[string]bool{}
	}
	if a.fwDownloads[key] {
		a.fwDlMu.Unlock()
		return
	}
	a.fwDownloads[key] = true
	a.fwDlMu.Unlock()
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

// downloadImage fetches a published board image.
//
// A separate path from the host builds because it is a different catalogue and
// a different kind of file: a flash image for particular hardware, not an
// executable for this machine.
func (a *App) downloadImage(img firmware.BoardImage) {
	key := img.Board + "/" + img.Role + "@" + img.Version
	a.fwDlMu.Lock()
	if a.fwDownloads == nil {
		a.fwDownloads = map[string]bool{}
	}
	if a.fwDownloads[key] {
		a.fwDlMu.Unlock()
		return
	}
	a.fwDownloads[key] = true
	a.fwDlMu.Unlock()
	a.status = "downloading " + img.Board + " " + img.Version

	go func() {
		bc := &firmware.BoardCatalogue{CacheDir: firmware.DefaultCacheDir()}
		ctx, cancel := context.WithTimeout(context.Background(), fwDownloadTimeout)
		defer cancel()
		_, err := bc.Ensure(ctx, img)

		a.fwDlMu.Lock()
		defer a.fwDlMu.Unlock()
		delete(a.fwDownloads, key)
		if err != nil {
			a.fwDownloadErr = "download failed: " + err.Error()
			return
		}
		a.fwDownloadErr = ""
		// Said, not left. The status line kept reading "downloading" after the
		// file had landed, so the only way to learn it had worked was to
		// dismiss the message and go and look at the row.
		a.fwDoneMsg = img.Name + " downloaded"
	}()
}

// fwDownloadTimeout bounds a fetch. A build is a few hundred kilobytes, so a
// minute is generous; unbounded means a dead network shows a button reading
// "downloading" for ever.
const fwDownloadTimeout = 60 * time.Second

// fwDownloading reports whether a build is on its way, so the row can say so
// instead of offering a button that does nothing the second time it is pressed.
func (a *App) fwDownloading(roleOrBoardRole, version string) bool {
	a.fwDlMu.Lock()
	defer a.fwDlMu.Unlock()
	return a.fwDownloads[roleOrBoardRole+"@"+version]
}

// humanBytes keeps the size column narrow enough to stay beside the actions.
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
