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
