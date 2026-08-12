# UI spikes

Three candidate toolkits, each drawing the same thing: the Plan view, with the
real 58 node Fife network loaded from `fixtures/fixture-fife-strict.json`.

Each spike answers the same four questions:

1. **Layout** - can it express a toolbar, a map that fills the space, side
   panels and a status bar without hand-placed pixels?
2. **Custom drawing** - can it draw the map, the links and the labels at a
   sensible frame rate?
3. **Tables** - a filterable node list with stable row identity.
4. **Real windows** - can the Inspector become a separate OS window rather than
   a docked tab? This is the specific thing ImGui makes awkward.

They are not applications. They render a static scene from real data, and they
are throwaway.

| spike | toolkit | build |
|---|---|---|
| `gio/` | gioui.org | `go run ./spikes/gio` |
| `cogent/` | cogentcore.org/core | `go run ./spikes/cogent` |
| `electron/` | Electron + HTML canvas | `cd spikes/electron && npm install && npm start` |

## Result

See `comparison.html` for the screenshots and the recommendation.

**Gio**, on the evidence here. It did what was asked on the first attempt, stays
in one process with the engine, and ships as a single 13 MB binary.

Docking turned out not to be the deciding question. Neither Gio nor Electron has
a docking framework, and expressing the Plan view as a flex tree took about
forty lines. A fixed layout per view, with a few panels able to become real
windows, is the balance - and it needs no docking framework.

| | Gio | Electron | Cogent Core |
|---|---|---|---|
| first working render | first try | first try | after ~45 minutes |
| build dependencies | Vulkan headers | 270 MB node_modules | none beyond Go |
| ships as | 13 MB binary | ~200 MB bundle | one binary |
| text and emoji | emoji missing | everything | wrapped per character |
| same process as the engine | yes | no, IPC | yes |
