package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/A13xB0/meshcoresim/internal/firmware"

	"github.com/AllenDang/cimgui-go/imgui"
)

// configState is the workbench's own settings, as opposed to a node's.
//
// Persisted, because a setting that has to be re-made every launch is a setting
// that gets forgotten and then blamed. What is written is configFile below —
// these fields are unexported and encoding/json cannot reach them, so the two
// are kept deliberately separate rather than tagged and hoped over.
type configState struct {
	// setNameOnStart issues `set name <node>` to every node as firmware comes
	// up, so the mesh identifies itself with the names on the map.
	setNameOnStart bool
	// setPositionOnStart tells each node where it is, which is what its adverts
	// carry and what a client shows on a map.
	setPositionOnStart bool
	// extraOnStart is run at every node after the built-in commands: a place
	// for whatever this particular study needs.
	extraOnStart string

	// setRegionOnStart defines a MeshCore transport region on every node, named
	// after the study area. This is the firmware's *own* region concept — the
	// routing scope in its region_map — and not the geographic boundary the
	// Boundary window draws, which only decides which nodes are in the study.
	setRegionOnStart bool
	// setDefaultScope makes that region this node's default scope, so the
	// traffic it originates is scoped rather than unscoped.
	setDefaultScope bool

	// floodMaxAdvert caps how far an advert is relayed. The firmware's own
	// default is 8, which on a national mesh stops adverts several hops short of
	// the edge — a node that never hears of another cannot route to it.
	setFloodMaxAdvert bool
	floodMaxAdvert    int32

	// autoWarm computes the link matrix in the background on every rebuild.
	autoWarm bool
	// controlEnabled opens the local control socket, which is what the MCP
	// server and any other agent drives the workbench through. On by default,
	// because that is how it has always behaved - but it is a door into this
	// window, so it is a door with a visible switch.
	controlEnabled bool

	// realFirmware makes play start MeshCore on every node. On by default:
	// running the real firmware is the point of this workbench, and it was a
	// second button people forgot to press.
	realFirmware bool

	// uiScale is the multiplier ctrl+/- last chose; zero means never chosen.
	uiScale float64
	// energyEnabled shows the solar/battery modelling. Off by default: it is a
	// planning specialism, and a panel most studies never open is clutter.
	energyEnabled bool
	// bootSpread staggers node start times, so adverts do not pile up on one
	// millisecond.
	bootSpread bool

	init bool
}

// defaultConfig turns on what an operator would otherwise turn on immediately.
func (a *App) ensureConfig() {
	if a.cfg.init {
		return
	}
	a.cfg.init = true
	a.cfg.setNameOnStart = true
	a.cfg.setPositionOnStart = true
	a.cfg.setFloodMaxAdvert = true
	// Off by default. A transport region changes how packets are scoped and
	// which ones a node will relay at all; turning that on for somebody without
	// being asked would change results they did not ask to change.
	a.cfg.setRegionOnStart = false
	a.cfg.setDefaultScope = false
	// 32, against the firmware's default of 8. Eight hops is generous on a
	// town-sized mesh and short on a national one: a node whose advert never
	// arrives is a node nobody can route to, and the cost of a larger cap is
	// airtime on adverts, which are small and infrequent. The firmware's own
	// ceiling is 64.
	a.cfg.floodMaxAdvert = 32
	a.cfg.autoWarm = true
	a.cfg.bootSpread = true
	a.cfg.energyEnabled = false
	a.cfg.controlEnabled = true
	a.cfg.realFirmware = true

	// A saved file wins over these defaults, and a missing one leaves them.
	a.loadConfig()
}

// configPath is where the workbench's own settings live.
func configPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "meshcoresim-config.json"
	}
	return filepath.Join(base, "meshcoresim", "config.json")
}

// configFile mirrors configState, because the fields it persists are
// unexported and encoding/json cannot reach them.
type configFile struct {
	SetName        bool    `json:"set_name_on_start"`
	SetPosition    bool    `json:"set_position_on_start"`
	SetFloodAdvert bool    `json:"set_flood_max_advert"`
	FloodMaxAdvert int32   `json:"flood_max_advert"`
	Extra          string  `json:"extra_on_start"`
	AutoWarm       bool    `json:"auto_warm"`
	EnergyEnabled  bool    `json:"energy_enabled"`
	ControlEnabled *bool   `json:"control_enabled,omitempty"`
	RealFirmware   *bool   `json:"real_firmware,omitempty"`
	UIScale        float64 `json:"ui_scale,omitempty"`
	BootSpread     bool    `json:"boot_spread"`
	Seed           uint64  `json:"seed"`
	Layer          string  `json:"basemap_layer"`
}

func (a *App) loadConfig() {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return // no file yet; the defaults stand
	}
	var f configFile
	if err := json.Unmarshal(b, &f); err != nil {
		// A corrupt file is not worth failing a launch over, but it is worth
		// saying: silently reverting to defaults looks like settings that do
		// not stick.
		a.status = "settings file unreadable, using defaults: " + err.Error()
		return
	}
	a.cfg.setNameOnStart = f.SetName
	a.cfg.setPositionOnStart = f.SetPosition
	a.cfg.setFloodMaxAdvert = f.SetFloodAdvert
	a.cfg.floodMaxAdvert = f.FloodMaxAdvert
	a.cfg.extraOnStart = f.Extra
	a.cfg.autoWarm = f.AutoWarm
	a.cfg.energyEnabled = f.EnergyEnabled
	// A pointer, so a config file written before this setting existed keeps
	// the default rather than reading as "off".
	if f.ControlEnabled != nil {
		a.cfg.controlEnabled = *f.ControlEnabled
	}
	if f.RealFirmware != nil {
		a.cfg.realFirmware = *f.RealFirmware
	}
	a.cfg.uiScale = f.UIScale
	a.cfg.bootSpread = f.BootSpread
	if f.Seed != 0 {
		a.seed = f.Seed
	}
	if f.Layer != "" {
		_ = a.SetLayer(f.Layer)
	}
}

// saveConfig writes the settings out.
//
// Called when one changes rather than on exit: a workbench that is killed — and
// this one is, routinely, along with its firmware children — would otherwise
// lose everything set during the session.
func (a *App) saveConfig() {
	f := configFile{
		SetName: a.cfg.setNameOnStart, SetPosition: a.cfg.setPositionOnStart,
		SetFloodAdvert: a.cfg.setFloodMaxAdvert, FloodMaxAdvert: a.cfg.floodMaxAdvert,
		Extra: a.cfg.extraOnStart, AutoWarm: a.cfg.autoWarm,
		EnergyEnabled:  a.cfg.energyEnabled,
		ControlEnabled: &a.cfg.controlEnabled,
		RealFirmware:   &a.cfg.realFirmware,
		UIScale:        a.cfg.uiScale,
		BootSpread:     a.cfg.bootSpread, Seed: a.seed, Layer: a.layerID,
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(configPath(), b, 0o644); err != nil {
		a.status = "could not save settings: " + err.Error()
	}
}

// The workbench's own settings, split in two because the old Configuration
// window mixed two unrelated kinds of decision: how this application behaves
// (Preferences) and what real nodes are told at their own CLIs when they boot
// (Provisioning). An operator changing one should not be reading warnings
// about the other — and every one of these is something an operator has a
// right to switch off and see the difference.

func (a *App) drawProvisionWindow() {
	if !a.winProvision {
		return
	}
	a.ensureConfig()
	imgui.SetNextWindowSizeV(a.windowSize(78, 26), imgui.CondFirstUseEver)
	open := a.winProvision
	if imgui.BeginV("Provisioning", &open, 0) {
		a.drawProvisionBody()
	}
	imgui.End()
	a.winProvision = open
}

func (a *App) drawPrefsWindow() {
	if !a.winPrefs {
		return
	}
	a.ensureConfig()
	imgui.SetNextWindowSizeV(a.windowSize(72, 20), imgui.CondFirstUseEver)
	open := a.winPrefs
	if imgui.BeginV("Preferences", &open, 0) {
		a.drawPrefsBody()
	}
	imgui.End()
	a.winPrefs = open
}

func (a *App) drawProvisionBody() {
	c := &a.cfg

	imgui.SeparatorText("When firmware starts")
	textWrap("Commands issued at every node's own CLI as it comes up. These are real " +
		"commands to real firmware - the same ones you would type on a hilltop.")

	changed := false
	changed = imgui.Checkbox("set name to the node's name on the map", &c.setNameOnStart) || changed
	if imgui.IsItemHovered() {
		imgui.SetTooltip("set name <node>\n\n" +
			"Without this every node advertises the firmware's built-in default,\n" +
			"so a mesh of three hundred repeaters is three hundred nodes with the\n" +
			"same name and nothing in a client can tell them apart.")
	}

	changed = imgui.Checkbox("set the node's position", &c.setPositionOnStart) || changed
	if imgui.IsItemHovered() {
		imgui.SetTooltip("set lat <lat> / set lon <lon>\n\n" +
			"What the node's adverts carry, and where a client will show it.")
	}

	changed = imgui.Checkbox("define a transport region from the study area", &c.setRegionOnStart) || changed
	if imgui.IsItemHovered() {
		imgui.SetTooltip("region put <area> / region allowf <area> / region save\n\n" +
			"MeshCore's own region concept: the routing scope in its region_map,\n" +
			"which decides how packets are scoped and which a node will relay.\n" +
			"Distinct from the geographic boundary, which only decides which\n" +
			"nodes are in the study - that one never reaches the firmware.\n\n" +
			"Off by default: this changes relaying, and so changes results.")
	}
	if c.setRegionOnStart {
		imgui.SameLine()
		if name := a.studyAreaName(); name != "" {
			textDim("-> region put " + regionToken(name))
		} else {
			imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
			imgui.Text("no study area chosen")
			imgui.PopStyleColor()
		}
		imgui.Indent()
		changed = imgui.Checkbox("and make it this node's default scope", &c.setDefaultScope) || changed
		if imgui.IsItemHovered() {
			imgui.SetTooltip("region default <name>   (MeshCore v1.15.0 and later)\n\n" +
				"Scopes the traffic this node originates - adverts, direct messages,\n" +
				"logins - to that region, rather than sending it unscoped.\n" +
				"Saves itself; a channel's own scope still overrides it.")
		}
		imgui.Unindent()
	}

	changed = imgui.Checkbox("cap advert hops", &c.setFloodMaxAdvert) || changed
	imgui.SameLine()
	imgui.SetNextItemWidth(90)
	if imgui.InputIntV("##fma", &c.floodMaxAdvert, 0, 0, 0) {
		// The firmware refuses anything above 64, and answers "Error, max 64"
		// rather than clamping. Clamped here so the reply is never an error the
		// operator has to go and read.
		if c.floodMaxAdvert < 1 {
			c.floodMaxAdvert = 1
		}
		if c.floodMaxAdvert > 64 {
			c.floodMaxAdvert = 64
		}
		changed = true
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("set flood.max.advert <n>\n\n" +
			"The firmware's own default is 8. On a national mesh that stops adverts\n" +
			"several hops short of the edge, and a node nobody has heard of cannot\n" +
			"be routed to. The firmware's ceiling is 64.")
	}

	textDim("then, at every node:")
	imgui.SetNextItemWidth(-1)
	if imgui.InputTextMultiline("##extra", &c.extraOnStart, imgui.NewVec2(0, 90), 0, nil) {
		changed = true
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("One CLI line per row. Anything the firmware understands:\n" +
			"set flood.max.advert 4\nset advert.interval 30")
	}

	// What this actually sends, before it sends it.
	//
	// The commands were only ever visible after the fact, in a console, on a
	// node somebody thought to open - so an import that came up unnamed
	// looked like a broken import rather than a setting that was off.
	imgui.SeparatorText("What each node will be told")
	if len(a.Nodes) == 0 {
		textDimWrap("no nodes yet - import or place some")
	} else {
		i := a.selected
		if i < 0 || i >= len(a.Nodes) {
			i = 0
		}
		cmds := a.startupCommands(i)
		textDim(a.Nodes[i].Name + ", at boot:")
		if len(cmds) == 0 {
			textColoured(colWarn, "nothing - every option above is off, so this node "+
				"comes up with the firmware's own defaults, including its default name")
		} else {
			pushMono()
			for _, c := range cmds {
				imgui.Text("  " + c)
			}
			popMono()
		}
		// The fleet-wide count, because one node's commands do not say
		// whether the other four hundred are covered.
		configured, bare := 0, 0
		for k := range a.Nodes {
			if len(a.startupCommands(k)) > 0 {
				configured++
			} else if a.Nodes[k].Kind.RunsFirmware() {
				bare++
			}
		}
		textDim(fmt.Sprintf("%d nodes get commands, %d would start unconfigured",
			configured, bare))
	}

	if a.eng != nil && a.eng.FirmwareCount() > 0 {
		imgui.Spacing()
		if imgui.Button("apply to the nodes already running") {
			n := a.applyStartupConfig()
			a.status = fmt.Sprintf("configured %d running nodes", n)
		}
		imgui.SameLine()
		textDim("these run automatically on the next start")
	}

	if changed {
		a.saveConfig()
	}
}

func (a *App) drawPrefsBody() {
	c := &a.cfg
	changed := false

	imgui.SeparatorText("Simulation")
	changed = imgui.Checkbox("stagger node start times", &c.bootSpread) || changed
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Real repeaters are powered on weeks apart. Started together they\n" +
			"share a timer phase and their adverts collide on the same millisecond\n" +
			"for ever - an artefact of the simulation, not of the network.\n\n" +
			"Deterministic: the same seed gives the same stagger.")
	}
	changed = imgui.Checkbox("compute the link matrix in the background", &c.autoWarm) || changed
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Off, the first message pays for every path at once, on the frame\n" +
			"thread. That is what made a large scenario freeze when you sent one.")
	}
	changed = imgui.Checkbox("energy modelling (solar / battery / survive-December)", &c.energyEnabled) || changed
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Shows the Energy panel and the per-node survivability question.\n" +
			"A planning specialism, hidden until wanted.")
	}
	if changed && !c.energyEnabled {
		// Hiding the feature closes its panel too - a window for a feature
		// that is switched off is a bug report waiting to be filed.
		if p := a.panelByName("Energy"); p != nil {
			p.open = false
		}
	}

	if changed {
		a.saveConfig()
	}

	imgui.SeparatorText("Agent control")
	on := c.controlEnabled
	if imgui.Checkbox("let agents drive this workbench (MCP)", &on) {
		a.setControlEnabled(on)
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Opens a local socket that the meshcoresim-mcp server - and any\n" +
			"other local client - uses to read and change this window: the view,\n" +
			"panels, the scenario, the run. Nothing leaves the machine, and the\n" +
			"socket exists only while this window does.")
	}
	if a.ctrl != nil {
		textDimWrap("listening at " + a.ctrl.Path())
	} else if c.controlEnabled {
		textDimWrap("enabled, but the socket could not be opened - see the status bar")
	} else {
		textDimWrap("off: MCP tools will report that no workbench is running")
	}

	imgui.SeparatorText("Where things are kept")
	textDim("scenarios   " + scenarioDir())
	textDim("boundaries  " + boundaryCacheDir())
	textDim("node flash  " + firmware.NodeWorkDir("<node>"))
}

// startupCommands is what a node is told as it comes up.
func (a *App) startupCommands(i int) []string {
	a.ensureConfig()
	n := a.Nodes[i]
	var out []string
	if a.cfg.setNameOnStart && n.Name != "" {
		// Truncated to what the firmware's own preference field holds. Sending
		// more is not an error the CLI reports — it simply stores the first
		// part, and a node then answers to a name nobody chose.
		// Truncated on a rune boundary, not a byte one. ScotMesh names carry
		// emoji - "West Lomond ⛰️🌤" - and cutting one in half sends the
		// firmware a partial UTF-8 sequence, which it stores as whatever the
		// bytes happen to mean.
		out = append(out, "set name "+truncateRunes(n.Name, maxNodeNameLen))
	}
	if a.cfg.setPositionOnStart && n.Kind.Transmits() {
		out = append(out,
			fmt.Sprintf("set lat %.6f", n.Position.Lat),
			fmt.Sprintf("set lon %.6f", n.Position.Lon))
	}
	out = append(out, a.regionCommands(i)...)
	if a.cfg.setFloodMaxAdvert && n.Kind.Transmits() {
		out = append(out, fmt.Sprintf("set flood.max.advert %d", a.cfg.floodMaxAdvert))
	}
	for _, line := range strings.Split(a.cfg.extraOnStart, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// maxNodeNameLen is MeshCore's own node_name field width.
const maxNodeNameLen = 32

// applyStartupConfig runs the startup commands at every node that has firmware.
func (a *App) applyStartupConfig() int {
	if a.eng == nil {
		return 0
	}
	done := 0
	for i := range a.Nodes {
		cmds := a.startupCommands(i)
		if len(cmds) == 0 {
			continue
		}
		name := a.Nodes[i].Name
		en, ok := a.eng.NodeByName(name)
		if !ok || en.Firmware == nil {
			continue
		}
		for _, cmd := range cmds {
			if err := a.typeAt(name, cmd); err != nil {
				a.status = err.Error()
				break
			}
		}
		done++
	}
	// The commands are queued at each node's serial input; time has to move for
	// the firmware to read and act on them.
	a.stepEngine(60)
	return done
}

// studyAreaName is the chosen boundary's name, if there is exactly one worth
// naming a transport region after.
func (a *App) studyAreaName() string {
	if len(a.bnd.chosen) == 0 {
		return ""
	}
	// The first area. Several areas are a union for filtering purposes, but a
	// transport region is one scope — inventing a combined name would produce
	// a region no other operator's node has ever heard of.
	return a.bnd.chosen[0].Name
}

// regionToken makes a name the firmware will accept.
//
// MeshCore's rule: lowercase alphanumeric and hyphens, at most 29 bytes. A
// space would also be read as the start of the next argument, so "New Zealand"
// would put a region called "New" with "Zealand" as its parent.
func regionToken(name string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return '-'
		}
	}, name)
	out = strings.Trim(out, "-")
	// 29 bytes is the firmware's own limit.
	if len(out) > 29 {
		out = out[:29]
	}
	return strings.ToLower(out)
}

// regionCommands is the region half of a node's provisioning, on its own so
// that "tell them at boot" and "tell them now" issue exactly the same lines.
//
// Nothing is inferred here: a node configured from its own observed traffic
// gets those regions, and one with none falls back to the study area's name
// only because the operator asked for that in Provisioning.
func (a *App) regionCommands(i int) []string {
	a.ensureConfig()
	if !a.cfg.setRegionOnStart {
		return nil
	}
	n := a.Nodes[i]
	var out []string
	// A node that was observed in real regions is configured with those,
	// whatever the study area is called: this came from the node's own traffic
	// and is a fact about it, where the study area is a name someone chose.
	if n.Kind.Transmits() && len(n.Regions) > 0 {
		for _, r := range n.Regions {
			token := regionToken(r)
			out = append(out, "region put "+token, "region allowf "+token)
		}
		out = append(out, "region save")
		if n.DefaultScope != "" {
			out = append(out, "region default "+regionToken(n.DefaultScope))
		}
	} else if n.Kind.Transmits() {
		if area := a.studyAreaName(); area != "" {
			token := regionToken(area)
			// The documented sequence, in this order:
			//
			//   region put <name>     defines it, with the wildcard as parent
			//   region allowf <name>  permits flood packets for it — a region
			//                         that exists but does not allow flooding
			//                         relays nothing, which looks like a broken
			//                         mesh rather than a configuration choice
			//   region save           persists it; without this the map is gone
			//                         at the next boot
			//
			// `region def` is the cursor-based form for building a whole tree in
			// one line. One flat region does not need it, and the explicit
			// commands say what they do.
			out = append(out,
				"region put "+token,
				"region allowf "+token,
				"region save")
			if a.cfg.setDefaultScope {
				// v1.15.0 and later: scopes this node's *own* flood traffic —
				// adverts, direct messages, logins — to the region. It performs
				// its own save, so no second one is needed.
				out = append(out, "region default "+token)
			}
		}
	}
	return out
}

// truncateRunes cuts a string to at most n bytes without splitting a rune.
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := 0
	for i := range s {
		if i > n {
			break
		}
		cut = i
	}
	return s[:cut]
}
