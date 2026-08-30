// Tests for StepTick, the one part of the tick handler that can be checked
// without MeshCore's headers.
//
//	g++ -std=c++17 -Wall -Wextra -o /tmp/tick_test tick_test.cpp && /tmp/tick_test
//
// Standalone on purpose: MeshCore is downloaded at runtime rather than vendored,
// so nothing else in this directory compiles without it, and an invariant that
// cannot be checked is one that drifts.
#include <cstdio>
#include <vector>

#include "tick.h"

namespace {

int failures = 0;

void expect(bool ok, const char* what) {
  if (!ok) {
    fprintf(stderr, "FAIL: %s\n", what);
    failures++;
  }
}

// A tick that advances time runs the loop once per millisecond crossed, and
// leaves the clock where it was told to.
void advancingTickLoopsOncePerMillisecond() {
  unsigned long now = 100;
  std::vector<unsigned long> at;
  StepTick(now, 105, [&] { at.push_back(now); });

  expect(now == 105, "the clock ends where the tick asked");
  expect(at.size() == 5, "five milliseconds means five loops");
  for (size_t i = 0; i < at.size(); i++) {
    expect(at[i] == 101 + i, "each loop sees its own millisecond");
  }
}

// The regression this file exists for. The last millisecond of an advancing
// tick used to get a second loop at the same clock value, so every timer the
// firmware runs fired twice there, on every tick.
void theFinalMillisecondIsNotRunTwice() {
  unsigned long now = 0;
  std::vector<unsigned long> at;
  StepTick(now, 3, [&] { at.push_back(now); });

  expect(at.size() == 3, "three milliseconds means three loops, not four");
  int atEnd = 0;
  for (unsigned long v : at) {
    if (v == 3) atEnd++;
  }
  expect(atEnd == 1, "the final millisecond is visited once");
}

// A tick that moves no time still gets one loop: the firmware is polled even
// when the clock is standing still.
void aStandingTickStillLoopsOnce() {
  unsigned long now = 42;
  int loops = 0;
  StepTick(now, 42, [&] { loops++; });

  expect(now == 42, "a standing tick leaves the clock alone");
  expect(loops == 1, "a standing tick loops exactly once");
}

// A tick asking for a time already past does not run the clock backwards over
// every millisecond between; it is one poll, like any other standing tick.
void aBackwardTickDoesNotUnwind() {
  unsigned long now = 500;
  int loops = 0;
  StepTick(now, 100, [&] { loops++; });

  expect(loops == 1, "a backward tick loops once rather than unwinding");
}

// One millisecond is the smallest advance, and the boundary the off-by-one
// lived on.
void oneMillisecondIsOneLoop() {
  unsigned long now = 7;
  std::vector<unsigned long> at;
  StepTick(now, 8, [&] { at.push_back(now); });

  expect(at.size() == 1, "one millisecond means one loop");
  expect(!at.empty() && at[0] == 8, "that loop sees the millisecond it crossed");
  expect(now == 8, "the clock ends on it");
}

}  // namespace

int main() {
  advancingTickLoopsOncePerMillisecond();
  theFinalMillisecondIsNotRunTwice();
  aStandingTickStillLoopsOnce();
  aBackwardTickDoesNotUnwind();
  oneMillisecondIsOneLoop();

  if (failures) {
    fprintf(stderr, "%d check(s) failed\n", failures);
    return 1;
  }
  printf("ok\n");
  return 0;
}
