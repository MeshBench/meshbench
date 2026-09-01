package main

import (
	"context"
	"errors"
	"os"

	"github.com/MeshBench/meshbench/internal/app/flagdump"
)

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string) error
	// examples are worked invocations, kept here rather than in a document
	// because a flag list generated from the code and an example written
	// somewhere else drift the moment a flag is renamed. tools/flagdoc checks
	// every flag named here still exists, which is the half of an example that
	// can be checked mechanically; the other half is that somebody ran it.
	examples []flagdump.Example
}

// The pair of Perthshire points most of these examples use: a mast on high
// ground north of the Ochils, and a repeater in the valley below it. Real
// places, and far enough apart that the answers are interesting rather than
// foregone.
const (
	hillLat, hillLon = "56.3980", "-3.4260"
	glenLat, glenLon = "56.3327", "-3.3239"
)

// nodesJSON writes the four-node export the traffic example floods through.
//
// Written by the example rather than shipped as a file because a provider
// export is the shape `traffic` takes, and inventing one in a line is the
// shortest honest way to have one to hand.
const nodesJSON = `printf '[{"Name":"Perth Hill","HasPosition":true,"Lat":56.398,"Lon":-3.426,` +
	`"HeightAGLm":20,"Kind":"repeater"},{"Name":"Abernethy Repeater","HasPosition":true,` +
	`"Lat":56.33271,"Lon":-3.32386,"HeightAGLm":10,"Kind":"repeater"},{"Name":"Glenrothes",` +
	`"HasPosition":true,"Lat":56.198,"Lon":-3.178,"HeightAGLm":10,"Kind":"repeater"},` +
	`{"Name":"Kirkcaldy","HasPosition":true,"Lat":56.113,"Lon":-3.16,"HeightAGLm":8,` +
	`"Kind":"repeater"}]\n' > fife.json`

func commands() []command {
	return []command{
		{"link", "link budget between two points, both directions", runLink, []flagdump.Example{{
			Line: "meshbench link -from-lat " + hillLat + " -from-lon " + hillLon +
				" -from-height 20 -to-lat " + glenLat + " -to-lon " + glenLon +
				" -to-height 10 -offline",
			Why: "A mast on the hill against a repeater in the glen, 9.6 km apart. " +
				"Both directions are reported because reachability is asymmetric, " +
				"and this pair happens to be balanced at +5.4 dB.",
		}}},
		{"profile", "terrain profile and the worst obstruction on a path", runProfile, []flagdump.Example{{
			Line: "meshbench profile -from-lat " + hillLat + " -from-lon " + hillLon +
				" -from-height 20 -to-lat 56.0700 -to-lon -3.4530 -samples 400 -offline",
			Why: "What stands in the way when a link budget comes back short. " +
				"This one names the hill, 15.7 km along and 272 m into the path.",
		}}},
		{"coverage", "coverage raster from one station, written as a PNG", runCoverage, []flagdump.Example{{
			Line: "meshbench coverage -lat " + hillLat + " -lon " + hillLon +
				" -height 20 -radius 15 -pixels 200 -o perth.png -offline",
			Why: "A 30 km square around the mast, at 200 by 200 cells. " +
				"One-way cells get their own colour: they are neither covered nor not.",
		}}},
		{"spectrum", "what an SDR observer captures: waterfall PNG and audio", runSpectrum, []flagdump.Example{{
			Line: "meshbench spectrum -sf 10 -bandwidth 250 -rx -120 -o waterfall.png -wav chirp.wav",
			Why: "An SF10 chirp 6 dB under the noise floor, as a picture and as a sound. " +
				"A chirp through a narrow filter is a rising whistle.",
		}}},
		{"terrain", "download elevation tiles for an area", runTerrain, []flagdump.Example{{
			Line: "meshbench terrain -south 56.0 -north 56.5 -west -3.6 -east -2.8 -estimate",
			Why: "What the download would cost, before spending it. Drop -estimate to fetch. " +
				"Tiles cache permanently, so -offline answers from them afterwards.",
		}}},
		{"boards", "the hardware profiles this build knows about", runBoards, []flagdump.Example{{
			Line: "meshbench boards",
			Why: "RADIATED is what leaves the antenna: chip power minus board loss plus " +
				"the antenna it ships with. That is the number that decides range, " +
				"and it is not the number on the box.",
		}}},
		{"firmware", "list, download or import MeshCore firmware", runFirmware, []flagdump.Example{{
			Line: "meshbench firmware -offline",
			Why: "What is already on this machine. Without -offline it lists the published " +
				"catalogue; -get fetches one, -import takes a build of your own.",
		}}},
		{"energy", "will a solar node survive the winter", runEnergy, []flagdump.Example{{
			Line: "meshbench energy -lat 56.34 -panel 10 -battery 6000 -tx 22",
			Why: "A 10 W panel and a 6 Ah cell at Scottish latitude, over a year. " +
				"Receive current, not transmit power, is what usually decides this.",
		}}},
		{"airtime", "LoRa time on air, as the firmware computes it", runAirtime, []flagdump.Example{{
			Line: "meshbench airtime -sf 10 -bandwidth 250 -bytes 32",
			Why: "259 ms, and 139 transmissions an hour at a 1% duty cycle. " +
				"The same arithmetic the firmware's own getEstAirtimeFor() does.",
		}}},
		{"traffic", "flood a message through a network and report what happened", runTraffic, []flagdump.Example{{
			Setup: nodesJSON,
			Line:  `meshbench traffic -nodes fife.json -from "Perth Hill" -for 20000 -offline`,
			Why: "One message into four nodes, with a cause for every node it did not reach. " +
				"Add -firmware to run real MeshCore on each instead of injecting traffic.",
		}}},
		{"basemap", "download map tiles for an area", runBasemap, []flagdump.Example{{
			Line: "meshbench basemap",
			Why: "The layers, with the attribution each one requires. Naming one with " +
				"-layer and an area downloads it.",
		}, {
			Line: "meshbench basemap -layer carto-light -south 56.0 -north 56.5 " +
				"-west -3.6 -east -2.8 -zoom 11 -estimate",
			Why: "36 tiles, about 1 MB. Every layer here contacts a third party.",
		}}},
		{"dev", "build a MeshCore checkout and give it to the workbench", runDev, []flagdump.Example{{
			Line: "meshbench dev -from ~/src/MeshCore -role simple_repeater -assign=false",
			Why: "Builds the checkout into the firmware cache and stops there. " +
				"Nothing is written into the MeshCore tree. Add -watch for a rebuild " +
				"on every save, and drop -assign=false to put it on every node of that role.",
		}}},
		{"serve", "run a mesh and expose a companion to your app", runServe, []flagdump.Example{{
			Line: "meshbench serve -fixture fixtures/fixture-fife-strict.json",
			Why: "58 nodes on real firmware, one companion on a loopback port it prints. " +
				"Point a client at that address; -serial gives a pty instead.",
		}}},
		{"test", "run a fixture on real firmware and check its assertions", runTest, []flagdump.Example{{
			Line: "meshbench test -fixture fixtures/fixture-fife-strict.json -for 60000 -quiet",
			Why: "The one a pipeline calls. Exit 0 if every assertion passed, 1 if any " +
				"failed; -junit writes a report with one case per assertion.",
		}}},
		{"headless", "run the verbs over the control socket, with no window", runHeadless, []flagdump.Example{{
			Line: "meshbench headless -fixture fife-strict -play -for 15s " +
				"-control-socket /tmp/meshbench.sock",
			Why: "The same session the window builds, with nothing attached to look at it. " +
				"A client connects to that socket and drives it with the verbs.",
		}}},
		{"workbench", "open the desktop workbench: build a scenario on a map and run it", runWorkbench, []flagdump.Example{{
			Line: "meshbench workbench -list-fixtures",
			Why:  "The networks built into this binary, without opening a window.",
		}, {
			Line: "meshbench workbench -fixture fife-strict -panel Nodes -filter Abernethy " +
				"-look 56.34,-3.32,11 -quit-after 20s",
			Why: "One panel filling the window, filtered, over a fixed view, closing itself. " +
				"That is the capture shape: every one of those flags exists so a screenshot " +
				"does not need a hand on the mouse.",
		}}},
	}
}

// describeSelf records what every command declares and prints it, which is
// where the CLI reference comes from.
//
// It runs each command with no arguments and stops it at parse(), rather than
// asking a table of flags kept somewhere for the purpose. A second table would
// be a second thing to keep true, and the reason this exists at all is that
// there was one.
func describeSelf(ctx context.Context) error {
	flagdump.Begin()
	for _, c := range commands() {
		flagdump.Note(c.name, c.summary, c.examples)
		if err := c.run(ctx, nil); err != nil && !errors.Is(err, flagdump.ErrRecorded) {
			return err
		}
	}
	return flagdump.Emit(os.Stdout)
}
