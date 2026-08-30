#pragma once
#include <cstdint>

// StepTick advances a simulated clock and runs the firmware's loop as it goes.
//
// One loop per millisecond of simulated time. Stepping rather than jumping is
// what keeps timeouts, retries and duty-cycle refill behaving as they do on
// hardware: a node that sees time move in 500 ms jumps takes different
// branches.
//
// A tick that advances no time still gets one loop, which is the whole reason
// for the second branch. Running it unconditionally gave the last millisecond
// of every advancing tick a second loop at the same clock value, so the
// firmware's own timers fired twice there, on every tick, for every node.
//
// Here rather than inline in the tick handler so the invariant can be tested
// without MeshCore's headers, which are not in this repository.
//
// Now is the clock's own type rather than a fixed width: MeshCore's
// MillisecondClock counts in unsigned long, and a tick arrives off the wire as
// a uint32_t.
template <typename Now, typename Loop>
void StepTick(Now& now, uint32_t at, Loop loop) {
  if (now < at) {
    while (now < at) {
      now++;
      loop();
    }
    return;
  }
  now = at;
  loop();
}
