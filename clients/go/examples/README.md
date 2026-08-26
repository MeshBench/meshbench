# Go examples

Runnable programs, one per directory, matching `clients/python/examples/`
one for one. The pair exist so neither client reads like a translation of the
other: the same thing is done in each, idiomatically, and where one is
awkward that is a fault in the client rather than in the language.

```
go run ./clients/go/examples/small-mesh-with-traffic
go run ./clients/go/examples/headless-regression fife-strict results.xml
```

Each needs `meshbench` on `PATH`, or `MESHBENCH_BINARY` naming one.

`go build ./...` compiles all of them, which is the point of their being
programs rather than snippets in a comment: an example that has stopped
compiling is a red build rather than something somebody finds by trying it.

All of them open the workbench except `headless-regression`, which is the one
CI runs and deliberately has no display, no GPU and no toolkit. The rest are
things you sit and watch, so they show you what they are doing.

| | what it does | needs |
|---|---|---|
| `blank-setup-with-a-board` | a T-Deck companion running wadamesh, its window open on Hardware | a display |
| `two-nodes-on-a-local-build` | a fixture trimmed to two, on a build from a MeshCore checkout, re-runnable | a display, a checkout |
| `small-mesh-with-traffic` | two repeaters, two companions, a message every twenty seconds | a display |
| `headless-regression` | the one CI runs: assertions, JUnit, non-zero on regression | — |
| `two-builds-in-one-scenario` | the A/B: two builds, two nodes, one seed | a display, two builds |
| `live-import-and-advert` | a real mesh pulled live inside a study area, a node found by name, an advert sent | a display, the network |
| `replace-a-board-build` | build a repository, swap the image in, delete the old one, re-runnable | a display, a checkout |

The two marked re-runnable attach to the session already open at the default
address rather than starting another, so running one twice carries on from
where it was. Nothing names a socket: that is the default's job.

There are also doc examples in `meshbench/example_test.go`. Those are for
`go doc`; these are for running.
