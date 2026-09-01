// Package provisioning holds the provisioning settings verbs - reading them,
// changing what a future run provisions with, and re-provisioning the running
// nodes. Split out of internal/app/session; the Provisioning type and its logic
// stay in core because the experiment matrix shares them, so this reaches them
// through the accessors session exports and registers its verbs from init.
package provisioning

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerProvisioningSettings(st *state.Store, s *session.Sim) {
	st.HandleSpec("provisioning.get", state.Spec{
		What: "read the switches every node is told at boot, which is what " +
			"decides whether a mesh comes up named, positioned and in the same " +
			"conversation as the rest",
		Returns: []string{"set_name", "set_position", "set_clock",
			"region_from_area", "default_scope", "advert_hops", "advert_minutes",
			"stagger_ms", "flood_max_advert", "path_hash_mode",
			"comp_path_hash_mode", "loop_detect", "cad", "extra"},
		Answers: "The settings themselves, unwrapped. A zero or an empty string " +
			"is not a value sent to the firmware but a firmware default left " +
			"alone, and `path_hash_mode` and `comp_path_hash_mode` say the same " +
			"thing with -1.",
		Example: &state.Example{
			Params: map[string]any{}, What: "read what the next start will send",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		return s.Provisioning().Describe(), nil
	})

	st.HandleSpec("provisioning.set", state.Spec{
		What: "change what a future run tells its nodes, a switch at a time, " +
			"leaving every setting not named where it was",
		Params: []state.Param{
			{Name: "set_name", Type: state.ParamBool,
				What: "send each node its scenario name; absent leaves it alone, " +
					"and off leaves a node reporting as its board type"},
			{Name: "set_position", Type: state.ParamBool,
				What: "send each node its latitude and longitude; absent leaves " +
					"it alone, and off leaves a node advertising no position"},
			{Name: "set_clock", Type: state.ParamBool,
				What: "set every node to the run's own epoch; absent leaves it " +
					"alone, and off leaves clocks that reject traffic as replays"},
			{Name: "region_from_area", Type: state.ParamBool,
				What: "define a transport region named after the study area; " +
					"absent leaves it alone"},
			{Name: "default_scope", Type: state.ParamBool,
				What: "make that region the one nodes originate under; absent " +
					"leaves it alone, and off leaves them relaying but never " +
					"originating"},
			{Name: "advert_hops", Type: state.ParamNumber,
				What: "how far an advert may flood; absent leaves it alone and " +
					"zero says nothing to the firmware"},
			{Name: "advert_minutes", Type: state.ParamNumber,
				What: "how often a node says it is there; absent leaves it alone, " +
					"zero means never, and anything outside 60 to 240 is clamped " +
					"into that range because the firmware refuses the rest"},
			{Name: "stagger_ms", Type: state.ParamNumber,
				What: "milliseconds between node starts; absent leaves it alone"},
			{Name: "flood_max_advert", Type: state.ParamNumber,
				What: "how far an advert is relayed; absent leaves it alone and " +
					"zero leaves the firmware's own limit"},
			{Name: "path_hash_mode", Type: state.ParamNumber,
				What: "the repeaters' path-hash mode; absent leaves it alone and " +
					"negative says nothing to the firmware"},
			{Name: "comp_path_hash_mode", Type: state.ParamNumber,
				What: "the companions' own, which is a different question from " +
					"the repeaters'; absent leaves it alone and negative falls " +
					"back to path_hash_mode"},
			{Name: "loop_detect", Type: state.ParamString,
				What: "the firmware's loop-detect setting, sent only to nodes " +
					"that transmit; absent or empty leaves it alone"},
			{Name: "cad", Type: state.ParamString,
				What: "the firmware's CAD mode, sent only to nodes that transmit; " +
					"absent or empty leaves it alone"},
			{Name: "extra", Type: state.ParamString,
				What: "further console lines, one per line, sent after the rest " +
					"and unchecked; absent or empty leaves it alone"},
		},
		Returns: []string{"set_name", "set_position", "set_clock",
			"region_from_area", "default_scope", "advert_hops", "advert_minutes",
			"stagger_ms", "flood_max_advert", "path_hash_mode",
			"comp_path_hash_mode", "loop_detect", "cad", "extra"},
		Answers: "Every setting comes back, the same shape as " +
			"`provisioning.get`, so one call both changes and reads. Nothing " +
			"reaches a node that is already running: this is what the next start " +
			"sends, and `provisioning.apply` is what changes a mesh that is up.",
		Example: &state.Example{
			Params:   map[string]any{"advert_minutes": 60, "flood_max_advert": 32},
			What:     "make the mesh advertise, and let adverts cross a country",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		pr := s.Provisioning()
		for name, set := range map[string]func(bool){
			"set_name":         func(v bool) { pr.SetName = v },
			"set_position":     func(v bool) { pr.SetPosition = v },
			"set_clock":        func(v bool) { pr.SetClock = v },
			"region_from_area": func(v bool) { pr.RegionFromArea = v },
			"default_scope":    func(v bool) { pr.DefaultScope = v },
		} {
			if v, ok := session.BoolField(p, name); ok {
				set(v)
			}
		}
		if v, ok := session.NumField(p, "advert_hops"); ok {
			pr.AdvertHops = int(v)
		}
		if v, ok := session.NumField(p, "advert_minutes"); ok {
			pr.AdvertMinutes = int(v)
		}
		if v, ok := session.NumField(p, "stagger_ms"); ok {
			pr.StaggerMs = int(v)
		}
		if v, ok := session.NumField(p, "flood_max_advert"); ok {
			pr.FloodMaxAdvert = int(v)
		}
		if v, ok := session.NumField(p, "path_hash_mode"); ok {
			pr.PathHashMode = int(v)
		}
		if v, ok := session.NumField(p, "comp_path_hash_mode"); ok {
			pr.CompPathHashMode = int(v)
		}
		if v, ok := session.NamedField(p, "loop_detect"); ok {
			pr.LoopDetect = v
		}
		if v, ok := session.NamedField(p, "cad"); ok {
			pr.CadMode = v
		}
		if v, ok := session.NamedField(p, "extra"); ok {
			pr.Extra = v
		}
		w.Provisioning, w.ProvisioningNode = nil, ""
		w.Say("provisioning changed; the next start uses it")
		return pr.Describe(), nil
	})

	st.HandleSpec("provisioning.apply", state.Spec{
		What: "type the current settings into the nodes that are already " +
			"running, which is the difference between changing what a future " +
			"run does and changing what this one is doing",
		Returns: []string{"nodes"},
		Answers: "`nodes` counts the ones that had firmware up and were typed " +
			"at; a node not running is passed over rather than refused, so a " +
			"count below the size of the network is normal on a mesh that is " +
			"still coming up. The commands are queued at each node's serial " +
			"input, so the clock has to move before the firmware acts on them. " +
			"A node whose console will not take a line ends the whole call with " +
			"an error naming it, leaving the nodes after it untouched.",
		Example: &state.Example{
			Params: map[string]any{},
			What:   "push changed settings into a mesh that is up",
		},
	}, func(w *state.World, _ any) (any, error) {
		if s.Engine() == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		pr := s.Provisioning()
		sent := 0
		for _, n := range s.Nodes() {
			en, ok := s.Engine().NodeByName(n.Name)
			if !ok || en.Firmware == nil {
				continue
			}
			for _, line := range pr.CommandsFor(n) {
				if err := en.Firmware.Bridge.Type([]byte(line + "\r\n")); err != nil {
					return nil, fmt.Errorf("%s: %w", n.Name, err)
				}
			}
			sent++
		}
		w.Say(fmt.Sprintf("re-provisioned %d running nodes", sent))
		return map[string]any{"nodes": sent}, nil
	})
}

func init() { session.RegisterDomain(registerProvisioningSettings) }
