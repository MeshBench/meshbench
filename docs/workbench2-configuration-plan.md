# Configuration, redesigned and settable

From Alex's mock (2026-08-14): a sidebar of sections, card grids of labelled
values with a caption saying why each matters, controls inline where the value
is. The current page is a read-only table with two controls bolted underneath;
none of the configuration is settable from it. Both halves change.

## Layout, per the mock

- Left sidebar: **Overview**, then SIMULATION (General, Nodes, Links,
  Environment, Time, Seed), then ADVANCED (Graphics, Events, System). A "best
  case profile" note at the bottom keeps the standing honesty line (no
  multipath, bare earth, ideal demodulator).
- Right: card sections. Each cell is icon-ish glyph, big value, small caption -
  the caption is the existing "why it matters" text, which the mock keeps.
- A status pill top right: **Ready to run** / warming / playing, from the same
  state the transport reads.
- New comp primitives needed: a card container (rounded, rule border - exists
  as RoundRect+Border), a stat cell (value + caption), a wrap grid (Flex with
  wrapping or manual row-fill), a toggle rendered as a switch (mock shows a
  pill toggle; comp.Check underneath, drawn differently), and a dropdown
  (first real one in the app - build it on the Prompt.Choose overlay pattern
  so it needs no new machinery).

## Everything becomes settable, each through its existing or new verb

| Setting | Verb | Notes |
|---|---|---|
| Seed | sim.seed | exists; rebuild is cheap now the matrix carries over |
| Study margin | boundary margin verb (check name) | exists in session |
| Speed (ms/tick) | sim.speed | exists |
| Run kind (real firmware) | sim.kind | exists |
| GPU on/off | gpu.set | exists, shown as the mock's toggle |
| Tile cache size | terrain.cache | exists; dropdown of 2/5/10/20 GB + free entry |
| **Tile cache location** | terrain.cache_dir (new) | see below |
| Advert minutes, provisioning | provisioning.set | link through to Provisioning |

## Moving the cache (the new verb)

`terrain.cache_dir {path}`:
1. Validate: create the directory, write-test it, refuse a path inside the old
   one.
2. Move the tiles as a job with progress ("moving 32,254 tiles"), rename when
   same filesystem, copy-then-delete when not - this is gigabytes and must not
   block the store; worker goroutine, job strip, same pattern as the warm.
3. Swap TileStore.CacheDir under its lock only after the move succeeds; the
   in-memory cache survives, so nothing re-decodes.
4. Persist the choice.

## Persistence (currently missing entirely)

Cache dir, cache GB, and the GPU choice do not survive a restart today. Add
`~/.config/meshcoresim/workbench2.json`, read at startup, written on change -
the same pattern workbench 1's loadConfig/saveConfig uses. Nothing else goes
in it yet: the scenario stays in the fixture, deliberately.

## Order

1. Persistence file + terrain.cache_dir verb (usable from the socket at once).
2. comp: stat cell, card, switch-drawn Check, dropdown-on-Choose.
3. The page itself, section by section, Overview first.
4. The control audit picks up every new control automatically - keep it green.
