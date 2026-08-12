// Loading a shipped network into the state layer.
package main

import (
	"encoding/json"
	"os"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

func loadFixture(path string) ([]state.Node, error) {
	var f struct {
		Nodes []struct {
			Name     string   `json:"Name"`
			Kind     string   `json:"Kind"`
			Regions  []string `json:"Regions"`
			Position struct {
				Lat float64 `json:"Lat"`
				Lon float64 `json:"Lon"`
			} `json:"Position"`
			HeightAGLm float64 `json:"HeightAGLm"`
			TxPowerDBm float64 `json:"TxPowerDBm"`
			Firmware   struct {
				Role    string `json:"Role"`
				Version string `json:"Version"`
			} `json:"Firmware"`
		} `json:"nodes"`
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	out := make([]state.Node, 0, len(f.Nodes))
	for i, n := range f.Nodes {
		out = append(out, state.Node{
			Name: n.Name, Kind: n.Kind, Lat: n.Position.Lat, Lon: n.Position.Lon,
			HeightM: n.HeightAGLm, TxDBm: n.TxPowerDBm, Regions: n.Regions,
			Firmware: n.Firmware.Version, Selected: i == 0,
			Heard: 0,
		})
	}
	return out, nil
}
