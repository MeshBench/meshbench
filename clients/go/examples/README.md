# Go examples

Runnable programs, one per directory, matching `clients/python/examples/`
one for one. The pair exist so neither client reads like a translation of the
other: the same thing is done in each, idiomatically, and where one is
awkward that is a fault in the client rather than in the language.

```
go run ./clients/go/examples/small-mesh-with-traffic
go run ./clients/go/examples/headless-regression fife-strict results.xml
```

Each needs `meshcoresim` on `PATH`, or `MESHBENCH_BINARY` naming one.

`go build ./...` compiles all of them, which is the point of their being
programs rather than snippets in a comment: an example that has stopped
compiling is a red build rather than something somebody finds by trying it.

| | what it does | needs |
|---|---|---|
| `blank-setup-with-a-board` | a T-Deck companion running wadamesh, its window open on Hardware | a display |
| `two-nodes-on-a-local-build` | a fixture trimmed to two, on a build from a MeshCore checkout, re-runnable | a checkout |
| `small-mesh-with-traffic` | two repeaters, two companions, a message every twenty seconds | — |
| `headless-regression` | the one CI runs: assertions, JUnit, non-zero on regression | — |
| `two-builds-in-one-scenario` | the A/B: two builds, two nodes, one seed | two builds |

There are also doc examples in `meshbench/example_test.go`. Those are for
`go doc`; these are for running.
