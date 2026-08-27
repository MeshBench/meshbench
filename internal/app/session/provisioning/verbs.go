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
	st.Handle("provisioning.get", func(w *state.World, _ any) (any, error) {
		return s.Provisioning().Describe(), nil
	})

	st.Handle("provisioning.set", func(w *state.World, p any) (any, error) {
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

	// provisioning.apply sends the current settings to nodes already running,
	// which is the difference between changing what a future run does and
	// changing what this one is doing.
	st.Handle("provisioning.apply", func(w *state.World, _ any) (any, error) {
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
