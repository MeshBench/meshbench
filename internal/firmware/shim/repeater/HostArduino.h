// The handful of Arduino functions MeshCore's helpers use that its own test
// mocks do not provide.
//
// Force-included ahead of everything else, because ArduinoHelpers.h calls them
// at namespace scope and there is nowhere later to put them.
#pragma once

#include <stdint.h>
#include <stdlib.h>

// Seeded rather than entropic: a run has to be reproducible from its seed, and
// the identity a node generates comes through here.
inline void randomSeed(unsigned long seed) { srandom(seed); }
inline long random(long howbig) { return howbig <= 0 ? 0 : random() % howbig; }
inline long random(long howsmall, long howbig) {
  return howbig <= howsmall ? howsmall : howsmall + random(howbig - howsmall);
}
