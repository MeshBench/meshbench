// Shared scene: the real Fife network, loaded once and projected to pixels.
package main

import (
	"encoding/json"
	"math"
	"os"
	"sort"
)

type node struct {
	Name     string
	Kind     string
	Lat, Lon float64
	Height   float64
	TxDBm    float64
	Regions  []string
	X, Y     float32 // filled by project()
}

type link struct{ A, B int }

type scene struct {
	Nodes []node
	Links []link
	Name  string
}

// loadScene reads the shipped fixture, so every spike draws the same real
// network rather than a plausible-looking invention.
func loadScene(path string) *scene {
	var f struct {
		Name  string `json:"name"`
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
		} `json:"nodes"`
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return &scene{Name: "no fixture found"}
	}
	if json.Unmarshal(b, &f) != nil {
		return &scene{Name: "fixture unreadable"}
	}
	s := &scene{Name: f.Name}
	for _, n := range f.Nodes {
		if n.Position.Lat == 0 && n.Position.Lon == 0 {
			continue
		}
		s.Nodes = append(s.Nodes, node{
			Name: n.Name, Kind: n.Kind, Lat: n.Position.Lat, Lon: n.Position.Lon,
			Height: n.HeightAGLm, TxDBm: n.TxPowerDBm, Regions: n.Regions,
		})
	}
	sort.Slice(s.Nodes, func(i, j int) bool { return s.Nodes[i].Name < s.Nodes[j].Name })

	// Links by proximity: near enough to hear each other on this band, which is
	// what the real link matrix mostly comes out as anyway. Enough to make the
	// drawing load honest.
	for i := range s.Nodes {
		for j := i + 1; j < len(s.Nodes); j++ {
			if hav(s.Nodes[i], s.Nodes[j]) < 18 {
				s.Links = append(s.Links, link{i, j})
			}
		}
	}
	return s
}

func hav(a, b node) float64 {
	const r = 6371.0
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLon := (b.Lon - a.Lon) * math.Pi / 180
	la := a.Lat * math.Pi / 180
	lb := b.Lat * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la)*math.Cos(lb)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}

// project maps latitude and longitude into a pixel box, preserving aspect.
func (s *scene) project(w, h, pad float32) {
	if len(s.Nodes) == 0 {
		return
	}
	minLat, maxLat := 90.0, -90.0
	minLon, maxLon := 180.0, -180.0
	for _, n := range s.Nodes {
		minLat = math.Min(minLat, n.Lat)
		maxLat = math.Max(maxLat, n.Lat)
		minLon = math.Min(minLon, n.Lon)
		maxLon = math.Max(maxLon, n.Lon)
	}
	// Longitude degrees shrink with latitude; without this Scotland looks
	// stretched sideways.
	cos := math.Cos((minLat + maxLat) / 2 * math.Pi / 180)
	spanX := (maxLon - minLon) * cos
	spanY := maxLat - minLat
	if spanX <= 0 || spanY <= 0 {
		return
	}
	sx := float64(w-2*pad) / spanX
	sy := float64(h-2*pad) / spanY
	sc := math.Min(sx, sy)
	offX := (float64(w) - spanX*sc) / 2
	offY := (float64(h) - spanY*sc) / 2
	for i := range s.Nodes {
		n := &s.Nodes[i]
		n.X = float32(offX + (n.Lon-minLon)*cos*sc)
		n.Y = float32(offY + (maxLat-n.Lat)*sc)
	}
}

func kindColour(kind string) (r, g, b uint8) {
	switch kind {
	case "companion":
		return 0x5c, 0xbf, 0xa8
	case "room-server":
		return 0xdd, 0x9e, 0x69
	case "sdr-observer":
		return 0x9a, 0xa4, 0xb2
	case "emitter":
		return 0xe0, 0x8a, 0x76
	case "advanced-repeater":
		return 0x8f, 0xb3, 0xff
	default:
		return 0x6e, 0xa8, 0xfe
	}
}

// shortKind is what a reader wants in a table cell.
func shortKind(k string) string {
	switch k {
	case "simple-repeater":
		return "repeater"
	case "advanced-repeater":
		return "advanced"
	case "sdr-observer":
		return "observer"
	case "room-server":
		return "room server"
	}
	return k
}
