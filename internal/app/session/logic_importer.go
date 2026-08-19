package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/provider"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// ImportFrom fetches a deployment and summarises what an import would produce.
//
// Describes rather than commits. Replacing the loaded network with a fetched
// one is a destructive act on somebody's work, and the useful half of an
// import panel is the part that tells you what you are about to get.
// ImportFrom describes what importing a deployment would do.
//
// region narrows it to a study area, as the commit path does: a description
// that counted the whole feed while the commit would keep a county is a
// description of a different import.
func ImportFrom(ctx context.Context, url string, region *scenario.Region) (*state.Import, error) {
	if url == "" {
		return nil, fmt.Errorf("no deployment URL")
	}
	cs := &provider.CoreScope{BaseURL: url}
	records, err := cs.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	res, err := scenario.Import(records, scenario.ImportOptions{
		Region:       region,
		DefaultBoard: "RAK4631",
		// The band this project works in. Stated rather than defaulted,
		// because no source publishes a modem configuration and a wrong one
		// makes every imported link wrong in the same direction. EU/UK
		// narrow, 4/8, which is what the shipped fixtures were built on -
		// and leaving the coding rate at zero is rejected rather than
		// defaulted, which is how this was caught.
		Radio:            scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62.5e3, SpreadFactor: 8, CodingRate: 4},
		MaxUncertaintyKm: 1,
	})
	if err != nil {
		return nil, err
	}
	return &state.Import{
		URL: url, Records: len(records), Nodes: len(res.Nodes),
		SkippedNoPosition: res.SkippedNoPosition,
		SkippedOutside:    res.SkippedOutside,
		Uncertain:         res.Uncertain, Participants: res.Participants,
	}, nil
}
