package session

import (
	"context"
	"fmt"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/provider"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// ImportFrom fetches a deployment and summarises what an import would produce.
//
// Describes rather than commits. Replacing the loaded network with a fetched
// one is a destructive act on somebody's work, and the useful half of an
// import panel is the part that tells you what you are about to get.
func ImportFrom(ctx context.Context, url string) (*state.Import, error) {
	if url == "" {
		return nil, fmt.Errorf("no deployment URL")
	}
	cs := &provider.CoreScope{BaseURL: url}
	records, err := cs.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	res, err := scenario.Import(records, scenario.ImportOptions{
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
