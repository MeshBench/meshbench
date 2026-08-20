// The Microsoft Global ML Building Footprints source: a worldwide dataset
// published as one gzipped GeoJSONL file per level-9 Bing quadkey, indexed by
// a CSV of links. Resolving a pull is quadkey arithmetic; the rest is
// downloads.
package session

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// msFile is one dataset file worth downloading, with the size the index
// promises so the pull can be priced before a byte moves.
type msFile struct {
	url   string
	bytes int64
}

// microsoftFiles resolves the patches to the dataset files that cover them,
// priced by the index's own size column. Refusal is by gigabytes, not file
// count: ninety small files are fine and three enormous ones are not.
func microsoftFiles(ctx context.Context, patches []llBox) ([]msFile, error) {
	want := map[string]bool{}
	for _, b := range patches {
		for _, k := range quadkeysFor(b.South, b.North, b.West, b.East) {
			want[k] = true
		}
	}
	resp, err := environGet(ctx, microsoftLinksURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dataset index answered %s", resp.Status)
	}
	rd := csv.NewReader(resp.Body)
	rd.FieldsPerRecord = -1
	seen := map[string]bool{}
	var files []msFile
	var total int64
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 3 || !want[rec[1]] || seen[rec[2]] {
			continue
		}
		seen[rec[2]] = true
		f := msFile{url: rec[2]}
		if len(rec) >= 4 {
			f.bytes, _ = strconv.ParseInt(strings.TrimSpace(rec[3]), 10, 64)
		}
		files = append(files, f)
		total += f.bytes
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("the dataset has no footprints for this map's area")
	}
	if total > microsoftMaxBytes {
		return nil, fmt.Errorf(
			"this map's dataset files total %.1f GB (limit %.0f GB); prepare "+
				"the region offline with tools/envgen instead",
			float64(total)/1e9, float64(microsoftMaxBytes)/1e9)
	}
	return files, nil
}

// patchIndex answers "is this point in any patch" in constant time - the
// filter runs once per building in files that mostly cover ground nobody
// asked about, so a linear scan over patches would turn minutes into hours.
type patchIndex struct {
	buckets map[[2]int][]llBox
}

const patchBucketDeg = 0.05

func newPatchIndex(patches []llBox) *patchIndex {
	idx := &patchIndex{buckets: map[[2]int][]llBox{}}
	for _, b := range patches {
		x0, x1 := int(math.Floor(b.West/patchBucketDeg)), int(math.Floor(b.East/patchBucketDeg))
		y0, y1 := int(math.Floor(b.South/patchBucketDeg)), int(math.Floor(b.North/patchBucketDeg))
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				idx.buckets[[2]int{x, y}] = append(idx.buckets[[2]int{x, y}], b)
			}
		}
	}
	return idx
}

func (idx *patchIndex) contains(lat, lon float64) bool {
	k := [2]int{int(math.Floor(lon / patchBucketDeg)), int(math.Floor(lat / patchBucketDeg))}
	for _, b := range idx.buckets[k] {
		if lat >= b.South && lat <= b.North && lon >= b.West && lon <= b.East {
			return true
		}
	}
	return false
}

// filterNDJSON keeps only the features whose first vertex lands in a patch,
// writing kept lines verbatim. Split from the download so the filter is
// testable without a network.
func filterNDJSON(r io.Reader, idx *patchIndex, out io.Writer) (kept int, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) < 2 {
			continue
		}
		var ft struct {
			Geometry struct {
				Coordinates [][][2]float64 `json:"coordinates"`
			} `json:"geometry"`
		}
		if json.Unmarshal(line, &ft) != nil ||
			len(ft.Geometry.Coordinates) == 0 || len(ft.Geometry.Coordinates[0]) == 0 {
			continue
		}
		v := ft.Geometry.Coordinates[0][0] // lon, lat as GeoJSON carries them
		if !idx.contains(v[1], v[0]) {
			continue
		}
		if _, err := out.Write(line); err != nil {
			return kept, err
		}
		if _, err := out.Write([]byte("\n")); err != nil {
			return kept, err
		}
		kept++
	}
	return kept, sc.Err()
}

// microsoftNDJSON downloads the files one at a time, keeps only what falls
// in the patches, and hands back the filtered stream. Each file is deleted
// as soon as it is filtered, so disk holds one dataset file at a time no
// matter how many the map spans.
func microsoftNDJSON(ctx context.Context, files []msFile, patches []llBox,
	progress func(done int)) (io.Reader, func(), error) {
	sweepStaleEnvironTemps()
	tmp, err := os.MkdirTemp("", "msim-environ-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	outPath := filepath.Join(tmp, "filtered.jsonl")
	out, err := os.Create(outPath)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	idx := newPatchIndex(patches)
	fail := func(e error) (io.Reader, func(), error) {
		_ = out.Close()
		cleanup()
		return nil, nil, e
	}
	for i, f := range files {
		path := filepath.Join(tmp, "part.gz")
		if err := downloadTo(ctx, f.url, path); err != nil {
			return fail(err)
		}
		gzf, err := os.Open(path)
		if err != nil {
			return fail(err)
		}
		gz, err := gzip.NewReader(gzf)
		if err != nil {
			_ = gzf.Close()
			return fail(fmt.Errorf("%s is not gzip: %w", f.url, err))
		}
		_, err = filterNDJSON(gz, idx, out)
		_ = gz.Close()
		_ = gzf.Close()
		_ = os.Remove(path)
		if err != nil {
			return fail(err)
		}
		if progress != nil {
			progress(i + 1)
		}
	}
	if err := out.Close(); err != nil {
		return fail(err)
	}
	rd, err := os.Open(outPath)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return rd, func() { _ = rd.Close(); cleanup() }, nil
}

func downloadTo(ctx context.Context, url, path string) error {
	resp, err := environGet(ctx, url)
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

// sweepStaleEnvironTemps clears extraction directories that earlier runs
// left behind. The cleanup closure handles every ordinary exit; a killed
// process cannot run it, and on Linux the temp directory is usually tmpfs -
// so three abandoned extractions were half a gigabyte of somebody's RAM.
// An hour of quiet is the safety margin: a directory younger than that may
// belong to another meshcoresim mid-download, and losing the race costs
// that download, not correctness.
func sweepStaleEnvironTemps() {
	stale, err := filepath.Glob(filepath.Join(os.TempDir(), "msim-environ-*"))
	if err != nil {
		return
	}
	for _, dir := range stale {
		if info, err := os.Stat(dir); err == nil &&
			time.Since(info.ModTime()) > time.Hour {
			_ = os.RemoveAll(dir)
		}
	}
}
