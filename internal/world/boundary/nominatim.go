// Package boundary finds named administrative areas and returns them as
// scenario boundaries.
//
// "Keep only Scotland and Ireland" starts with someone typing a name, not with
// someone locating a GeoJSON file. Nominatim answers names with polygons; this
// wraps it, simplifies the geometry to a sensible tolerance, and caches every
// answer on disk so a boundary is downloaded once and works offline after.
package boundary

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// DefaultBaseURL is OSM's public Nominatim.
//
// Its usage policy asks for an identifying agent and light use; a manual
// search per region choice is about as light as use gets, and every result is
// cached so nothing is ever asked twice.
const DefaultBaseURL = "https://nominatim.openstreetmap.org"

// polygonThresholdDeg simplifies the returned geometry. 0.002 degrees is
// roughly 200 m — far finer than any RF question asked of a national border,
// and it turns Scotland from ten megabytes of coastline into a few hundred
// kilobytes.
const polygonThresholdDeg = 0.002

// Found is one candidate answer to a name.
type Found struct {
	// Name is the short label; DisplayName the full disambiguating one —
	// "Scotland" versus "Scotland, United Kingdom".
	Name        string
	DisplayName string
	// Kind is what the place is: country, state, administrative...
	Kind string
	// Boundaries is the polygon set, already parsed.
	Boundaries []scenario.Boundary

	// Lat and Lon are the geocoder's own representative point for the place.
	//
	// Kept because the middle of a boundary is not where a place is: France's
	// outline takes in Guadeloupe, French Guiana and Réunion, so the middle
	// of its extent is open ocean off west Africa. A geocoder answers "where
	// is this" and that is the answer to use for pointing a camera.
	Lat, Lon float64
}

// Client searches and caches.
type Client struct {
	// BaseURL defaults to the public Nominatim.
	BaseURL string
	// CacheDir holds one GeoJSON file per previously chosen place.
	CacheDir string
	HTTP     *http.Client
}

// Search asks for places matching a name, polygons included.
//
// Only results that carry area come back: a search for "Perth" also matches
// bus stops and a city in Australia, and offering a point as a boundary would
// let someone filter a network down to nothing.
func (c *Client) Search(ctx context.Context, query string) ([]Found, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	u := fmt.Sprintf("%s/search?format=jsonv2&polygon_geojson=1&polygon_threshold=%g&limit=8&q=%s",
		base, polygonThresholdDeg, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	// Nominatim's policy requires an identifiable agent; anonymous requests
	// get blocked, which would present as "search never works".
	req.Header.Set("User-Agent", "meshbench/0.1 (github.com/MeshBench/meshbench)")

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("boundary: search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("boundary: search returned %s", resp.Status)
	}

	var rows []struct {
		Name        string          `json:"name"`
		DisplayName string          `json:"display_name"`
		Type        string          `json:"type"`
		GeoJSON     json.RawMessage `json:"geojson"`
		Lat         string          `json:"lat"`
		Lon         string          `json:"lon"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&rows); err != nil {
		return nil, fmt.Errorf("boundary: unparseable search result: %w", err)
	}

	var out []Found
	for _, r := range rows {
		if len(r.GeoJSON) == 0 {
			continue
		}
		bounds, err := scenario.ParseGeoJSON(r.GeoJSON, "")
		if err != nil || len(bounds) == 0 {
			continue // points and lines: real results, not usable as an area
		}
		name := r.Name
		if name == "" {
			name = r.DisplayName
		}
		for i := range bounds {
			bounds[i].Name = name
			bounds[i].Source = "osm"
		}
		f := Found{Name: name, DisplayName: r.DisplayName, Kind: r.Type, Boundaries: bounds}
		// Strings on the wire; a place whose point will not parse still has
		// its outline, so this is not a reason to drop the result.
		f.Lat, _ = strconv.ParseFloat(r.Lat, 64)
		f.Lon, _ = strconv.ParseFloat(r.Lon, 64)
		out = append(out, f)
		c.cache(name, r.GeoJSON)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("boundary: nothing with an area matches %q", query)
	}
	return out, nil
}

// Cached returns a previously downloaded boundary by name, for offline use.
func (c *Client) Cached(name string) ([]scenario.Boundary, bool) {
	if c.CacheDir == "" {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Join(c.CacheDir, slug(name)+".geojson"))
	if err != nil {
		return nil, false
	}
	bounds, err := scenario.ParseGeoJSON(data, "")
	if err != nil {
		return nil, false
	}
	for i := range bounds {
		bounds[i].Name = name
		bounds[i].Source = "osm"
	}
	return bounds, true
}

func (c *Client) cache(name string, geojson []byte) {
	if c.CacheDir == "" {
		return
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return
	}
	// Best effort: a failed cache write means a re-download later, not a
	// failed search now.
	_ = os.WriteFile(filepath.Join(c.CacheDir, slug(name)+".geojson"), geojson, 0o644)
}

func slug(name string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, name)
	return strings.Trim(out, "-")
}

// ReverseSearch finds the administrative areas containing a point.
//
// Nominatim's reverse endpoint answers with one place and its hierarchy; asking
// at a coarse zoom returns the country or region rather than a street, which is
// the level a study area is drawn at.
func (c *Client) ReverseSearch(ctx context.Context, lat, lon float64) ([]Found, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	// zoom=5 is country/region. A finer zoom returns a suburb, and a study
	// boundary drawn round a suburb excludes the network it was meant to hold.
	u := fmt.Sprintf("%s/reverse?format=jsonv2&polygon_geojson=1&polygon_threshold=%g&zoom=5&lat=%f&lon=%f",
		base, polygonThresholdDeg, lat, lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "meshbench/0.1 (github.com/MeshBench/meshbench)")

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("boundary: reverse lookup: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("boundary: reverse lookup returned %s", resp.Status)
	}

	var row struct {
		Name        string          `json:"name"`
		DisplayName string          `json:"display_name"`
		Type        string          `json:"type"`
		GeoJSON     json.RawMessage `json:"geojson"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&row); err != nil {
		return nil, fmt.Errorf("boundary: unparseable reverse result: %w", err)
	}
	if len(row.GeoJSON) == 0 {
		return nil, fmt.Errorf("boundary: nothing with an area covers %.4f, %.4f", lat, lon)
	}
	bounds, err := scenario.ParseGeoJSON(row.GeoJSON, "")
	if err != nil || len(bounds) == 0 {
		return nil, fmt.Errorf("boundary: the area at %.4f, %.4f has no polygon", lat, lon)
	}
	name := row.Name
	if name == "" {
		name = row.DisplayName
	}
	for i := range bounds {
		bounds[i].Name, bounds[i].Source = name, "osm"
	}
	c.cache(name, row.GeoJSON)
	return []Found{{Name: name, DisplayName: row.DisplayName, Kind: row.Type, Boundaries: bounds}}, nil
}
