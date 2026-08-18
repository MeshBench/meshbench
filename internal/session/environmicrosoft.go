// The Microsoft Global ML Building Footprints source: a worldwide dataset
// published as one gzipped GeoJSONL file per level-9 Bing quadkey, indexed by
// a CSV of links. Resolving a pull is quadkey arithmetic; the rest is
// downloads.
package session

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// quadkey is Bing's tile name: the slippy x,y interleaved, one digit a zoom.
func quadkey(x, y, z int) string {
	var b strings.Builder
	for i := z; i > 0; i-- {
		d := byte('0')
		mask := 1 << (i - 1)
		if x&mask != 0 {
			d++
		}
		if y&mask != 0 {
			d += 2
		}
		b.WriteByte(d)
	}
	return b.String()
}

func slippyXY(lat, lon float64, z int) (int, int) {
	n := float64(int(1) << z)
	x := int((lon + 180) / 360 * n)
	latRad := lat * math.Pi / 180
	y := int((1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * n)
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v >= int(n) {
			return int(n) - 1
		}
		return v
	}
	return clamp(x), clamp(y)
}

// quadkeysFor is every level-9 quadkey the box touches - the granularity
// Microsoft's dataset is published at.
func quadkeysFor(south, north, west, east float64) []string {
	const z = 9
	x0, y0 := slippyXY(north, west, z)
	x1, y1 := slippyXY(south, east, z)
	var keys []string
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			keys = append(keys, quadkey(x, y, z))
		}
	}
	return keys
}

// microsoftLinksURL lists the dataset's files; var so a test can point it at
// a fixture instead of the real blob store.
var microsoftLinksURL = "https://minedbuildings.z5.web.core.windows.net/global-buildings/dataset-links.csv"

// microsoftURLs resolves the patches to the dataset files that cover them.
func microsoftURLs(patches []llBox) ([]string, error) {
	want := map[string]bool{}
	for _, b := range patches {
		for _, k := range quadkeysFor(b.South, b.North, b.West, b.East) {
			want[k] = true
		}
	}
	resp, err := environClient.Get(microsoftLinksURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dataset index answered %s", resp.Status)
	}
	rd := csv.NewReader(resp.Body)
	rd.FieldsPerRecord = -1
	var urls []string
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) >= 3 && want[rec[1]] {
			urls = append(urls, rec[2])
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("the dataset has no footprints for this map's area")
	}
	if len(urls) > microsoftMaxFiles {
		return nil, fmt.Errorf(
			"this map spans %d dataset files (limit %d); prepare the region "+
				"offline with tools/envgen instead", len(urls), microsoftMaxFiles)
	}
	return urls, nil
}

// microsoftNDJSON downloads and concatenates the files as one stream. The
// closer owns the temp files, so a caller that stops reading leaks nothing.
func microsoftNDJSON(urls []string, progress func(done int)) (io.Reader, func(), error) {
	tmp, err := os.MkdirTemp("", "msim-environ-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	var readers []io.Reader
	var open []io.Closer
	for i, u := range urls {
		path := filepath.Join(tmp, fmt.Sprintf("part-%d.gz", i))
		if err := downloadTo(u, path); err != nil {
			cleanup()
			return nil, nil, err
		}
		f, err := os.Open(path)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			_ = f.Close()
			cleanup()
			return nil, nil, fmt.Errorf("%s is not gzip: %w", u, err)
		}
		readers = append(readers, gz, strings.NewReader("\n"))
		open = append(open, gz, f)
		if progress != nil {
			progress(i + 1)
		}
	}
	closeAll := func() {
		for _, c := range open {
			_ = c.Close()
		}
		cleanup()
	}
	return io.MultiReader(readers...), closeAll, nil
}

func downloadTo(url, path string) error {
	resp, err := environClient.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", url, resp.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
