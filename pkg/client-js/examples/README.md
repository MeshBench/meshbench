# Node examples

Runnable programs, one per file, matching `pkg/client-go/examples/` and
`pkg/client-python/examples/` one for one. The three exist so no client reads
like a translation of another: the same thing is done in each, idiomatically,
and where one is awkward that is a fault in the client rather than in the
language.

```
node pkg/client-js/examples/small-mesh-with-traffic.mjs
node pkg/client-js/examples/headless-regression.mjs fife-strict results.xml
```

Each needs `meshbench` on `PATH`, or `MESHBENCH_BINARY` naming one. They start
their own workbench, so there is nothing to have running first - except the two
marked re-runnable, which attach to the session already open at the default
address rather than starting another, so running one twice carries on from
where it was.

`node --test` imports every one of them, which is why each exports `main` and
runs itself only when it is the file node was given. An example that has stopped
parsing, or that imports a helper this client no longer has, is a red test run
rather than something somebody finds by trying it.

All of them open the workbench except `headless-regression`, which is the one CI
runs and deliberately has no display, no GPU and no toolkit. The rest are things
you sit and watch, so they show you what they are doing.

| | what it does | needs |
|---|---|---|
| `blank-setup-with-a-board` | a T-Deck companion running wadamesh, its window open on Hardware | a display |
| `two-nodes-on-a-local-build` | a fixture trimmed to two, on a build from a MeshCore checkout, re-runnable | a display, a checkout |
| `small-mesh-with-traffic` | two repeaters, two companions, a message every twenty seconds | a display |
| `headless-regression` | the one CI runs: assertions, JUnit, non-zero on regression | - |
| `two-builds-in-one-scenario` | the A/B: two builds, two nodes, one seed | a display, two builds |
| `live-import-and-advert` | a real mesh pulled live inside a study area, a node found by name, an advert sent | a display, the network |
| `replace-a-board-build` | build a repository, swap the image in, delete the old one, re-runnable | a display, a checkout |

Nothing names a socket: that is the default's job.
