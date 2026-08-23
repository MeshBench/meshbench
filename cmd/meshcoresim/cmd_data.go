package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/mesh/energy"
	"github.com/MeshBench/meshbench/internal/mesh/firmware"
)

func runTerrain(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("terrain", flag.ExitOnError)
	store := terrainFlags(fs)
	south := fs.Float64("south", 0, "southern edge")
	north := fs.Float64("north", 0, "northern edge")
	west := fs.Float64("west", 0, "western edge")
	east := fs.Float64("east", 0, "eastern edge")
	estimate := fs.Bool("estimate", false, "report the download and stop")
	if err := parse(fs, args, "download elevation tiles for an area"); err != nil {
		return err
	}
	if err := requireAll(map[string]bool{
		"south": *south == 0, "north": *north == 0, "west": *west == 0, "east": *east == 0,
	}); err != nil {
		return err
	}
	if *north <= *south {
		return fmt.Errorf("north (%.4f) must be above south (%.4f)", *north, *south)
	}

	t, err := store()
	if err != nil {
		return err
	}
	e := t.Estimate(*south, *north, *west, *east)
	fmt.Printf("%d tiles; %d already cached, %d to fetch (about %d MB).\n",
		e.Tiles, e.Cached, e.ToFetch, e.BytesRough>>20)
	if *estimate {
		return nil
	}
	if e.ToFetch == 0 {
		return nil
	}
	if t.Offline {
		return fmt.Errorf("%d tiles are missing and downloads are off", e.ToFetch)
	}

	t.OnProgress = func(done, total int) {
		if done == total || done%10 == 0 {
			fmt.Fprintf(os.Stderr, "\r%d/%d", done, total)
		}
	}
	if err := t.Prefetch(ctx, *south, *north, *west, *east); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr)
	fmt.Println("Elevation data is derived from several national sources with their own")
	fmt.Println("attribution terms. That attribution must appear wherever these results are shown.")
	return nil
}

func runFirmware(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("firmware", flag.ExitOnError)
	defaultCache, _ := os.UserCacheDir()
	cache := fs.String("cache", filepath.Join(defaultCache, "meshcoresim", "firmware"),
		"where downloaded images live")
	offline := fs.Bool("offline", false, "list and use only what is already downloaded")
	board := fs.String("board", "", "filter by board, or name the board when importing")
	get := fs.String("get", "", "download an image by name, e.g. RAK_4631/repeater")
	importPath := fs.String("import", "", "import your own .uf2, .bin or .elf")
	role := fs.String("role", "repeater", "role, when importing")
	label := fs.String("label", "", "what to call an imported build; defaults to a timestamp")
	if err := parse(fs, args, "list, download or import MeshCore firmware"); err != nil {
		return err
	}

	c := &firmware.Catalogue{CacheDir: *cache, Offline: *offline}

	if *importPath != "" {
		img, err := c.Import(*importPath, *board, *role, firmware.ImportLabel(*label))
		if err != nil {
			return err
		}
		fmt.Printf("imported %s as %s (unverified — nothing published a digest for it)\n",
			*importPath, img.Name())
		return nil
	}

	images, err := c.List(ctx)
	if err != nil {
		return err
	}
	if *get != "" {
		for _, img := range images {
			if strings.EqualFold(img.Name(), *get) {
				path, err := c.Fetch(ctx, img)
				if err != nil {
					return err
				}
				fmt.Printf("%s %s -> %s\n", img.Name(), img.Version, path)
				if !img.Verified() {
					fmt.Println("WARNING: this release published no checksum, so the download is unverified.")
				}
				return nil
			}
		}
		return fmt.Errorf("no image named %q; run without -get to list them", *get)
	}

	shown := 0
	fmt.Printf("%-34s %-22s %9s  %s\n", "NAME", "VERSION", "SIZE", "VERIFIED")
	for _, img := range images {
		if *board != "" && !strings.EqualFold(img.Board, *board) {
			continue
		}
		v := "no"
		if img.Verified() {
			v = "yes"
		}
		fmt.Printf("%-34s %-22s %8dK  %s\n", img.Name(), img.Version, img.Size>>10, v)
		shown++
	}
	if shown == 0 {
		fmt.Println("nothing matched")
		if *board != "" {
			boards := firmware.Boards(images)
			sort.Strings(boards)
			fmt.Printf("boards in this catalogue: %s\n", strings.Join(boards, ", "))
		}
	}
	return nil
}

func runEnergy(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("energy", flag.ExitOnError)
	lat := fs.Float64("lat", 0, "latitude, north positive")
	lon := fs.Float64("lon", 0, "longitude, east positive")
	panelW := fs.Float64("panel", 0, "panel peak watts; 0 for no solar")
	tilt := fs.Float64("tilt", 50, "panel tilt from horizontal, degrees")
	batteryMAh := fs.Float64("battery", 3400, "battery capacity, mAh")
	txDBm := fs.Float64("tx", 22, "transmit power, dBm")
	alwaysOn := fs.Bool("always-on", true, "a repeater listens continuously")
	if err := parse(fs, args, "will a solar node survive the winter"); err != nil {
		return err
	}
	if err := requireAll(map[string]bool{"lat": *lat == 0}); err != nil {
		return err
	}

	site := energy.Site{
		Name: "site", LatDeg: *lat, LonDeg: *lon,
		Battery: energy.Battery{Chemistry: energy.LiIon, CapacityMAh: *batteryMAh, Cells: 1, CutoffV: 3.1},
		Panel: energy.Panel{PeakW: *panelW, TiltDeg: *tilt, AzimuthDeg: 180,
			SoilingFactor: 0.8, ChargeEfficiency: 0.95},
		Load:       energy.SX1262Load(),
		Duty:       energy.DutyFromAirtime(1, 0, 1000, *alwaysOn),
		TxPowerDBm: *txDBm,
		CloudByMonth: [12]float64{0.75, 0.72, 0.68, 0.62, 0.58, 0.58,
			0.60, 0.62, 0.65, 0.72, 0.78, 0.80},
		TempCByMonth: [12]float64{1, 1, 3, 5, 8, 11, 13, 13, 10, 7, 3, 1},
	}
	res, err := energy.SimulateYear(site)
	if err != nil {
		return err
	}

	fmt.Printf("%.0f W panel at %.0f degrees, %.0f mAh, latitude %.2f, %.0f dBm.\n\n",
		*panelW, *tilt, *batteryMAh, *lat, *txDBm)
	fmt.Printf("  worst charge  %5.0f%% on day %d\n", res.WorstSoC*100, res.WorstDay)
	fmt.Printf("  dead days     %5d\n", res.DeadDays)
	fmt.Printf("  autonomy      %5.1f days from full with no sun\n\n", res.AutonomyDays)
	switch {
	case res.DeadDays > 0:
		fmt.Printf("FAILS. It stops on %d days of the year.\n", res.DeadDays)
	case res.WorstSoC < 0.3:
		fmt.Println("MARGINAL — under 30% at the worst point, and nothing here allows for")
		fmt.Println("snow on the panel or a worse winter than average.")
	default:
		fmt.Println("Survives the year.")
	}
	fmt.Println("\nCloud cover is a monthly mean for a British winter, not measured data for")
	fmt.Println("this site. Receive current, not transmit power, is what usually decides this.")
	return nil
}
