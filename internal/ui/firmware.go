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
		imgui.TextDisabled("an SDR observer runs no firmware")
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
			imgui.TextDisabled("nothing published for this machine")
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
			imgui.TextDisabled("no builds of " + role)
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
		imgui.TextDisabled("reading the firmware catalogue...")
	case err != "":
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		imgui.TextWrapped("catalogue unavailable: " + err)
		imgui.PopStyleColor()
	default:
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.55, 0.58, 0.65, 1))
		if a.fw.isCached(role, version) {
			imgui.TextWrapped(fmt.Sprintf("%s @ %s — downloaded, starts instantly", role, version))
		} else {
			imgui.TextWrapped(fmt.Sprintf("%s @ %s — downloaded on first run", role, version))
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
	a.status = "firmware changed — press \"run real firmware\" to start the new build"
}
