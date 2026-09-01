# Paused: this directory does not build

Nothing compiles, links or references anything here. The headers are the
platform layer of a host build of MeshCore's `simple_repeater` that was never
finished: the translation unit that would define their globals, `radio_init()`,
`HostRadio`'s two remaining virtuals and a `main()` does not exist, and neither
does a build script.

Why it stopped, what would have to be written to revive it, and the defects that
have been fixed in it since are in [`../README.md`](../README.md).
