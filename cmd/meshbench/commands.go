package main

import "context"

// command is one verb of the binary: the name it is invoked by, the line the
// top-level usage lists it with, and the examples the CLI reference prints.
type command struct {
	name     string
	summary  string
	run      func(ctx context.Context, args []string) error
	examples []example
}

// example is a shell fragment that has been run, and what its output was worth
// saying about.
//
// Beside the commands rather than in the reference page because the page is
// generated: the flag tables come out of the binary's own help, and an example
// is the half no introspection can produce. tools/flagdoc checks that every
// flag an example names is one the command still declares, so an example
// cannot rot into documentation for a flag that has gone.
type example struct {
	shell string
	note  string
}

func commands() []command {
	return []command{
		{
			name:    "link",
			summary: "link budget between two points, both directions",
			run:     runLink,
			examples: []example{{
				shell: `meshbench link -from-lat 56.3980 -from-lon -3.4260 -from-height 20 -to-lat 56.3327 -to-lon -3.3239 -to-height 10 -offline`,
				note:  `A mast on the hill against a repeater in the glen, 9.6 km apart. Both directions are reported because reachability is asymmetric, and this pair happens to be balanced at +5.4 dB.`,
			}},
		},
		{
			name:    "profile",
			summary: "terrain profile and the worst obstruction on a path",
			run:     runProfile,
			examples: []example{{
				shell: `meshbench profile -from-lat 56.3980 -from-lon -3.4260 -from-height 20 -to-lat 56.0700 -to-lon -3.4530 -samples 400 -offline`,
				note:  `What stands in the way when a link budget comes back short. This one names the hill, 15.7 km along and 272 m into the path.`,
			}},
		},
		{
			name:    "coverage",
			summary: "coverage raster from one station, written as a PNG",
			run:     runCoverage,
			examples: []example{{
				shell: `meshbench coverage -lat 56.3980 -lon -3.4260 -height 20 -radius 15 -pixels 200 -o perth.png -offline`,
				note:  `A 30 km square around the mast, at 200 by 200 cells. One-way cells get their own colour: they are neither covered nor not.`,
			}},
		},
		{
			name:    "spectrum",
			summary: "what an SDR observer captures: waterfall PNG and audio",
			run:     runSpectrum,
			examples: []example{{
				shell: `meshbench spectrum -sf 10 -bandwidth 250 -rx -120 -o waterfall.png -wav chirp.wav`,
				note:  `An SF10 chirp 6 dB under the noise floor, as a picture and as a sound. A chirp through a narrow filter is a rising whistle.`,
			}},
		},
		{
			name:    "terrain",
			summary: "download elevation tiles for an area",
			run:     runTerrain,
			examples: []example{{
				shell: `meshbench terrain -south 56.0 -north 56.5 -west -3.6 -east -2.8 -estimate`,
				note:  `What the download would cost, before spending it. Drop -estimate to fetch. Tiles cache permanently, so -offline answers from them afterwards.`,
			}},
		},
		{
			name:    "boards",
			summary: "the hardware profiles this build knows about",
			run:     runBoards,
			examples: []example{{
				shell: `meshbench boards`,
				note:  `RADIATED is what leaves the antenna: chip power minus board loss plus the antenna it ships with. That is the number that decides range, and it is not the number on the box.`,
			}},
		},
		{
			name:    "firmware",
			summary: "list, download or import MeshCore firmware",
			run:     runFirmware,
			examples: []example{{
				shell: `meshbench firmware -offline`,
				note:  `What is already on this machine. Without -offline it lists the published catalogue; -get fetches one, -import takes a build of your own.`,
			}},
		},
		{
			name:    "energy",
			summary: "will a solar node survive the winter",
			run:     runEnergy,
			examples: []example{{
				shell: `meshbench energy -lat 56.34 -panel 10 -battery 6000 -tx 22`,
				note:  `A 10 W panel and a 6 Ah cell at Scottish latitude, over a year. Receive current, not transmit power, is what usually decides this.`,
			}},
		},
		{
			name:    "airtime",
			summary: "LoRa time on air, as the firmware computes it",
			run:     runAirtime,
			examples: []example{{
				shell: `meshbench airtime -sf 10 -bandwidth 250 -bytes 32`,
				note:  `259 ms, and 139 transmissions an hour at a 1% duty cycle. The same arithmetic the firmware's own getEstAirtimeFor() does.`,
			}},
		},
		{
			name:    "traffic",
			summary: "flood a message through a network and report what happened",
			run:     runTraffic,
			examples: []example{{
				shell: `printf '[{"Name":"Perth Hill","HasPosition":true,"Lat":56.398,"Lon":-3.426,"HeightAGLm":20,"Kind":"repeater"},{"Name":"Abernethy Repeater","HasPosition":true,"Lat":56.33271,"Lon":-3.32386,"HeightAGLm":10,"Kind":"repeater"},{"Name":"Glenrothes","HasPosition":true,"Lat":56.198,"Lon":-3.178,"HeightAGLm":10,"Kind":"repeater"},{"Name":"Kirkcaldy","HasPosition":true,"Lat":56.113,"Lon":-3.16,"HeightAGLm":8,"Kind":"repeater"}]\n' > fife.json
meshbench traffic -nodes fife.json -from "Perth Hill" -for 20000 -offline`,
				note: `One message into four nodes, with a cause for every node it did not reach. Add -firmware to run real MeshCore on each instead of injecting traffic.`,
			}},
		},
		{
			name:    "basemap",
			summary: "download map tiles for an area",
			run:     runBasemap,
			examples: []example{{
				shell: `meshbench basemap`,
				note:  `The layers, with the attribution each one requires. Naming one with -layer and an area downloads it.`,
			}, {
				shell: `meshbench basemap -layer carto-light -south 56.0 -north 56.5 -west -3.6 -east -2.8 -zoom 11 -estimate`,
				note:  `36 tiles, about 1 MB. Every layer here contacts a third party.`,
			}},
		},
		{
			name:    "dev",
			summary: "build a MeshCore checkout and give it to the workbench",
			run:     runDev,
			examples: []example{{
				shell: `meshbench dev -from ~/src/MeshCore -role simple_repeater -assign=false`,
				note:  `Builds the checkout into the firmware cache and stops there. Nothing is written into the MeshCore tree. Add -watch for a rebuild on every save, and drop -assign=false to put it on every node of that role.`,
			}},
		},
		{
			name:    "serve",
			summary: "run a mesh and expose a companion to your app",
			run:     runServe,
			examples: []example{{
				shell: `meshbench serve -fixture fixtures/fixture-fife-strict.json`,
				note:  `58 nodes on real firmware, one companion on a loopback port it prints. Point a client at that address; -serial gives a pty instead.`,
			}},
		},
		{
			name:    "test",
			summary: "run a fixture on real firmware and check its assertions",
			run:     runTest,
			examples: []example{{
				shell: `meshbench test -fixture fixtures/fixture-fife-strict.json -for 60000 -quiet`,
				note:  `The one a pipeline calls. Exit 0 if every assertion passed, 1 if any failed; -junit writes a report with one case per assertion.`,
			}},
		},
		{
			name:    "headless",
			summary: "run the verbs over the control socket, with no window",
			run:     runHeadless,
			examples: []example{{
				shell: `meshbench headless -fixture fife-strict -play -for 15s -control-socket /tmp/meshbench.sock`,
				note:  `The same session the window builds, with nothing attached to look at it. A client connects to that socket and drives it with the verbs.`,
			}},
		},
		{
			name:    "workbench",
			summary: "open the desktop workbench: build a scenario on a map and run it",
			run:     runWorkbench,
			examples: []example{{
				shell: `meshbench workbench -list-fixtures`,
				note:  `The networks built into this binary, without opening a window.`,
			}, {
				shell: `meshbench workbench -fixture fife-strict -panel Nodes -filter Abernethy -look 56.34,-3.32,11 -quit-after 20s`,
				note:  `One panel filling the window, filtered, over a fixed view, closing itself. That is the capture shape: every one of those flags exists so a screenshot does not need a hand on the mouse.`,
			}},
		},
	}
}
