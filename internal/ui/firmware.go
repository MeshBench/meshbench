package ui

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/firmware"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// fwCatalogue is what the workbench knows about published firmware.
//
// Fetched once, in the background, and never on the frame thread: a combo box
// that blocks on an HTTP request freezes the window for as long as GitHub takes
// to answer, and the answer changes about once a night.
type fwCatalogue struct {
	mu     sync.Mutex
	images []firmware.NativeImage
	// cached marks role@version pairs already on disk. Two jobs: annotate the
	// dropdowns with what will start instantly, and keep them populated when
	// the listing is rate-limited or the machine is offline — a build on disk
	// is runnable regardless of what GitHub thinks right now.
	cached map[string]bool
	err    string
	state  int // 0 untouched, 1 loading, 2 done
}

// fwFetchTimeout bounds the catalogue request. Long enough for a slow link,
// short enough that a user on a dead network sees the dropdown say so rather
// than staying empty for a minute.
const fwFetchTimeout = 20 * time.Second

func (c *fwCatalogue) load() {
	c.mu.Lock()
	if c.state != 0 {
		c.mu.Unlock()
		return
	}
	c.state = 1
	c.mu.Unlock()

	go func() {
		cat := &firmware.NativeCatalogue{CacheDir: firmware.DefaultCacheDir()}

		// Disk first, immediately: the dropdowns fill with what can run right
		// now, before the network has said a word.
		local := cat.CachedImages()
		cached := map[string]bool{}
		for _, img := range local {
			cached[img.Role+"@"+img.Version] = true
		}
		c.mu.Lock()
		c.images, c.cached = local, cached
		c.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), fwFetchTimeout)
		defer cancel()
		images, err := cat.List(ctx)

		c.mu.Lock()
		defer c.mu.Unlock()
		c.state = 2
		if err != nil {
			// The listing failed but the disk did not. Downloaded builds stay
			// offered; the error is reported as a degradation, not a wall.
			if len(local) > 0 {
				c.err = "catalogue unreachable - showing downloaded builds only"
			} else {
				c.err = err.Error()
			}
			return
		}
		// The network knows more than the disk; the disk knows what is instant.
		// Merge, keeping every network entry and every local-only one.
		seen := map[string]bool{}
		for _, img := range images {
			seen[img.Role+"@"+img.Version+"@"+img.OS+"@"+img.Arch] = true
		}
		for _, img := range local {
			if !seen[img.Role+"@"+img.Version+"@"+img.OS+"@"+img.Arch] {
				images = append(images, img)
			}
		}
		c.images = images
	}()
}

// forThisMachine returns the roles and versions that will actually run here.
//
// Filtered rather than listed in full, because offering a darwin-arm64 build on
// a Linux machine is offering a choice that fails at the moment someone presses
// run, by which time they have stopped thinking about the dropdown.
func (c *fwCatalogue) forThisMachine() (roles, versions []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	seenRole, seenVer := map[string]bool{}, map[string]bool{}
	for _, img := range c.images {
		if !img.ForThisMachine() {
			continue
		}
		if !seenRole[img.Role] {
			seenRole[img.Role] = true
			roles = append(roles, img.Role)
		}
		if !seenVer[img.Version] {
			seenVer[img.Version] = true
			versions = append(versions, img.Version)
		}
	}
	sort.Strings(roles)
	// Newest first, and the two moving refs at the top: those are what someone
	// is usually after, and a list that opens on companion-v1.0.0a buries them.
	sort.Slice(versions, func(i, j int) bool {
		ri, rj := versionRank(versions[i]), versionRank(versions[j])
		if ri != rj {
			return ri < rj
		}
		return versions[i] > versions[j]
	})
	return roles, versions
}

func versionRank(v string) int {
	switch v {
	case "main":
		return 0
	case "dev":
		return 1
	default:
		return 2
	}
}

// versionsFor narrows the version list to the ones that published a given role.
//
// Upstream tags one role at a time, so repeater-v1.17.0 has no companion build
// in it. Showing every version under every role would offer combinations that do
// not exist.
func (c *fwCatalogue) versionsFor(role string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	seen := map[string]bool{}
	for _, img := range c.images {
		if img.Role != role || !img.ForThisMachine() || seen[img.Version] {
			continue
		}
		seen[img.Version] = true
		out = append(out, img.Version)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := versionRank(out[i]), versionRank(out[j])
		if ri != rj {
			return ri < rj
		}
		return out[i] > out[j]
	})
	return out
}

func (c *fwCatalogue) status() (loading bool, err string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state == 1, c.err
}

// markCached records a build that has just been fetched, so the row it came
// from stops offering to fetch it again.
func (c *fwCatalogue) markCached(role, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached == nil {
		c.cached = map[string]bool{}
	}
	c.cached[role+"@"+version] = true
}

// isCached reports whether a build is already on disk, ready to start with no
// download and no network.
func (c *fwCatalogue) isCached(role, version string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cached[role+"@"+version]
}

// drawFirmwarePicker chooses which MeshCore build a node runs.
//
// The point of the whole firmware pipeline lands here. A node is a node: what
// makes one a repeater rather than a companion is only which application is
// loaded onto it, so this is a dropdown and not a property of the node's type —
// and a role MeshCore ships next year appears in it without this file changing.
func (a *App) drawFirmwarePicker(n *scenario.Node) {
	if !n.Kind.RunsFirmware() {
		textDim("an SDR observer runs no firmware")
		return
	}
	a.fw.load()

	role := n.Firmware.Role
	if role == "" {
		role = n.Kind.Application()
	}
	version := n.Firmware.Version
	if version == "" {
		version = "main"
	}

	roles, _ := a.fw.forThisMachine()
	loading, err := a.fw.status()

	imgui.SetNextItemWidth(-70)
	if imgui.BeginCombo("role", role) {
		if len(roles) == 0 {
			textDim("nothing published for this machine")
		}
		for _, r := range roles {
			if imgui.SelectableBool(r) {
				n.Firmware.Role = r
				// The version may not exist for the new role, since upstream tags
				// one role at a time. Falling back to main is better than leaving
				// a combination that was never built.
				if len(a.fw.versionsFor(r)) > 0 && !containsStr(a.fw.versionsFor(r), version) {
					n.Firmware.Version = "main"
				}
				a.rebuildForFirmware()
			}
		}
		imgui.EndCombo()
	}

	imgui.SetNextItemWidth(-70)
	if imgui.BeginCombo("version", version) {
		versions := a.fw.versionsFor(role)
		if len(versions) == 0 {
			textDim("no builds of " + role)
		}
		for _, v := range versions {
			label := v
			if a.fw.isCached(role, v) {
				label = v + "  [downloaded]"
			}
			if imgui.SelectableBool(label) {
				n.Firmware.Version = v
				a.rebuildForFirmware()
			}
		}
		imgui.EndCombo()
	}

	switch {
	case loading:
		textDim("reading the firmware catalogue...")
	case err != "":
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		textWrap("catalogue unavailable: " + err)
		imgui.PopStyleColor()
	default:
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.55, 0.58, 0.65, 1))
		if a.fw.isCached(role, version) {
			textWrap(fmt.Sprintf("%s @ %s - downloaded, starts instantly", role, version))
		} else {
			textWrap(fmt.Sprintf("%s @ %s - downloaded on first run", role, version))
		}
		imgui.PopStyleColor()
	}
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// rebuildForFirmware drops the running simulation.
//
// Changing which firmware a node runs changes what the node is, and an engine
// holding a process started from the previous choice would keep answering with
// it. Cheaper to be obvious about it than to explain later why the version in
// the dropdown is not the version that replied.
func (a *App) rebuildForFirmware() {
	a.buildEngine()
	a.status = "firmware changed - press \"run real firmware\" to start the new build"
}

// drawFirmwareWindow is the library: what is published, what is on this
// machine already, and what every node is set to run.
//
// The per-node picker answers "what does this one run"; nobody could answer
// "what do I have, and what is my fleet on" without opening forty of them.
func (a *App) drawFirmwareWindow() {
	if !a.winFirmware {
		return
	}
	imgui.SetNextWindowSizeV(a.windowSize(96, 34), imgui.CondFirstUseEver)
	a.applyDockIntent("Firmware library")
	// Pinned to the main window unless it was deliberately popped out.
	//
	// Without this it takes a viewport of its own, and the compositor sizes
	// that to the display: an OS window filling the screen with a small panel
	// in one corner of it, which reads as a broken layout rather than a window
	// that escaped.
	a.applyWindowMode("Firmware library")
	open := a.winFirmware
	// MenuBar, or panelChrome has nowhere to draw and the window loses its
	// pop-out and dock verbs entirely - which is what made it a one-way trip
	// once it came back into the main window.
	if imgui.BeginV("Firmware library", &open, imgui.WindowFlagsMenuBar) {
		a.panelChrome("Firmware library")
		a.drawFirmwareLibraryBody()
	}
	imgui.End()
	a.winFirmware = open
}

// drawFirmwareLibraryBody is a catalogue, so it is a table.
//
// A Browser in the design language: rows you sort and pick from, not prose
// sections. One row per published build, with the fleet composition read
// straight off the "in use by" column - which is the answer to "did that
// swap take", and the question no per-node picker can answer.
func (a *App) drawFirmwareLibraryBody() {
	a.fw.load()
	loading, err := a.fw.status()
	if loading {
		textDim("reading the published catalogue...")
	}
	if err != "" {
		textColoured(colWarn, err)
	}

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

	roles, _ := a.fw.forThisMachine()
	if len(roles) == 0 && !loading {
		textDim("nothing published for this machine - check the network, or " +
			"import a local build with: msim firmware import")
	}

	if imgui.BeginTableV("##fwlib", 6,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY|
			imgui.TableFlagsSizingStretchProp, imgui.NewVec2(0, a.fwTableHeight(0.45)), 0) {
		imgui.TableSetupColumnV("role", imgui.TableColumnFlagsWidthStretch, 0, 0)
		imgui.TableSetupColumnV("version", imgui.TableColumnFlagsWidthStretch, 0, 0)
		imgui.TableSetupColumnV("board", imgui.TableColumnFlagsWidthStretch, 0, 0)
		// Fixed columns measured from their own widest content, not guessed in
		// pixels: "use everywhere" is 120 px in one font and clipped to "use
		// everywhe" in another.
		pad := imgui.CurrentStyle().FramePadding().X*2 + 8
		imgui.TableSetupColumnV("downloaded", imgui.TableColumnFlagsWidthFixed,
			imgui.CalcTextSize("on first use").X+pad, 0)
		imgui.TableSetupColumnV("in use by", imgui.TableColumnFlagsWidthFixed,
			imgui.CalcTextSize("000 nodes").X+pad, 0)
		imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthFixed,
			imgui.CalcTextSize("use for all companion_radio").X+pad*2, 0)
		imgui.TableHeadersRow()

		for _, role := range roles {
			for _, v := range a.fw.versionsFor(role) {
				imgui.TableNextRow()
				imgui.TableSetColumnIndex(0)
				imgui.Text(role)
				imgui.TableSetColumnIndex(1)
				imgui.Text(v)
				imgui.TableSetColumnIndex(2)
				// Everything in this catalogue is a host build today. Said
				// rather than left blank, because a blank cell reads as
				// unknown, and it is the column that will carry a board name
				// once published images are listed here too.
				textDim("native")
				imgui.TableSetColumnIndex(3)
				switch {
				case a.fw.isCached(role, v):
					textColoured(colOK, "yes")
				case a.fwDownloading(role, v):
					textDim("downloading...")
				default:
					// Explicit, as well as on first use. Someone about to work
					// without a network, or who wants a run to start on the
					// instant, is not served by a catalogue that can only be
					// browsed.
					if imgui.SmallButton("download##" + role + v) {
						a.downloadBuild(role, v)
					}
				}
				imgui.TableSetColumnIndex(4)
				if n := inUse[role+" "+v]; n > 0 {
					imgui.Text(fmt.Sprintf("%d nodes", n))
				} else {
					textDim("-")
				}
				imgui.TableSetColumnIndex(5)
				// Scoped to the role, and labelled as such. "use everywhere"
				// read as though it would turn companions into repeaters,
				// which is not what it did and not something anyone wants: a
				// node's application is what makes it a companion in the first
				// place.
				if imgui.SmallButton("use for all " + role + "##" + role + v) {
					n := 0
					for i := range a.Nodes {
						if a.Nodes[i].Kind.RunsFirmware() &&
							(a.Nodes[i].Firmware.Role == role || a.Nodes[i].Kind.Application() == role) {
							a.Nodes[i].Firmware.Role, a.Nodes[i].Firmware.Version = role, v
							n++
						}
					}
					a.rebuildForFirmware()
					a.status = fmt.Sprintf("%d %s nodes set to %s", n, role, v)
				}
			}
		}
		imgui.EndTable()
	}

	a.drawInstalledBuilds()

	imgui.SeparatorText("Storage")
	textDim("builds     " + firmware.DefaultCacheDir())
	textDim("node flash " + firmware.NodeWorkDir("<node>"))
	if a.confirmWipe {
		if dangerButton("wipe every node's memory - sure?", imgui.NewVec2(0, 0)) {
			a.confirmWipe = false
			a.buildEngine()
			if err := firmware.WipeNodeStorage(); err != nil {
				a.status = err.Error()
			} else {
				a.status = "node memories wiped - every node boots factory-fresh next time"
			}
		}
	} else if imgui.Button("wipe every node's memory") {
		a.confirmWipe = true
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Deletes every node's persisted identity, prefs and regions.\n" +
			"The cure for settings poisoned by an earlier bad run.")
	}
}
