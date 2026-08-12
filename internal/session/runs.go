// Saving a run, so that two of them can be compared.
//
// A run record is deliberately small and self-describing: the numbers, the
// seed, and what was running when they were produced. The last of those is the
// point. Three arms of a study once returned identical numbers, and whether
// the firmware had actually differed could not be answered from the results -
// only from the provenance beside them.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// RunRecord is what lands on disk. Field names are the file format, so they
// are spelled for somebody reading the JSON rather than for Go.
type RunRecord struct {
	Name    string             `json:"name"`
	At      string             `json:"at"`
	Seed    uint64             `json:"seed"`
	Build   string             `json:"build"`
	Outcome string             `json:"outcome"`
	Metrics map[string]float64 `json:"metrics"`
}

func runsDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "meshcoresim", "runs")
	return dir, os.MkdirAll(dir, 0o755)
}

// SaveRun writes the current counters as a run.
func SaveRun(name string, s *state.Snapshot, build string) (string, error) {
	dir, err := runsDir()
	if err != nil {
		return "", err
	}
	sent, heard, delivered, redundant := 0, 0, 0, 0
	for _, v := range s.Scores {
		sent += v.Sent
		heard += v.Heard
		delivered += v.UniqueDelivery
		redundant += v.RedundantRelay
	}
	rec := RunRecord{
		Name: name, At: time.Now().UTC().Format(time.RFC3339),
		Seed: s.Seed, Build: build, Outcome: "saved",
		Metrics: map[string]float64{
			"transmissions": float64(sent),
			"receptions":    float64(heard),
			"delivered":     float64(delivered),
			"redundant":     float64(redundant),
			"nodes":         float64(len(s.Nodes)),
			"links":         float64(len(s.Links)),
			"seconds":       float64(s.NowMs) / 1000,
		},
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", err
	}
	// The name and the time, so two runs of one name do not overwrite each
	// other: a comparison needs both of them to still exist.
	path := filepath.Join(dir, name+"-"+time.Now().UTC().Format("20060102-150405")+".json")
	return path, os.WriteFile(path, b, 0o644)
}

// LoadRuns reads every saved run, newest first.
func LoadRuns() []RunRecord {
	dir, err := runsDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []RunRecord
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var r RunRecord
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
	return out
}

// MetricNames is every metric present in either run, in a stable order, so a
// comparison does not silently drop a metric one side happens not to have.
func MetricNames(a, b RunRecord) []string {
	seen := map[string]bool{}
	for k := range a.Metrics {
		seen[k] = true
	}
	for k := range b.Metrics {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// BuildOf is what was running when the numbers were produced.
//
// The firmware versions in play, not the simulator's own version: the
// simulator is the same binary for every arm of a study, and the thing that
// differed is what was Loaded into the nodes.
func BuildOf(s *Sim) string {
	seen := map[string]bool{}
	var out []string
	for _, n := range s.nodes {
		v := n.Firmware.Version
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return "no firmware"
	}
	r := out[0]
	for _, v := range out[1:] {
		r += " " + v
	}
	return r
}
