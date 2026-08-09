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

// Arduino defines these as macros. MeshCore uses them unqualified.
#ifndef min
#define min(a, b) ((a) < (b) ? (a) : (b))
#endif
#ifndef max
#define max(a, b) ((a) > (b) ? (a) : (b))
#endif

inline long random(long howbig) { return howbig <= 0 ? 0 : random() % howbig; }
inline long random(long howsmall, long howbig) {
  return howbig <= howsmall ? howsmall : howsmall + random(howbig - howsmall);
}

// Serial is the repeater's console. On a board it is a UART; here it is the
// bridge, so the simulator can carry a node's CLI to whoever is looking at it.
//
// This is the whole point of linking the repeater application rather than
// reimplementing it: the CLI a user reaches when they click a repeater is
// MeshCore's own, with its own commands and its own replies.
#include <Stream.h>

class HostSerial : public Stream {
 public:
  // The simulator swaps these for the bridge's own queues.
  void attach(void (*out)(const char*, size_t), int (*in)()) {
    out_ = out;
    in_ = in;
  }

  int available() override { return in_ ? (in_() >= 0 ? 1 : 0) : 0; }
  int read() override { return in_ ? in_() : -1; }
  int peek() override { return -1; }
  void flush() override {}

  size_t write(uint8_t c) override {
    char ch = (char)c;
    if (out_) out_(&ch, 1);
    return 1;
  }
  size_t write(const uint8_t* buf, size_t len) override {
    if (out_) out_((const char*)buf, len);
    return len;
  }

  void begin(unsigned long) {}
  explicit operator bool() const { return true; }

 private:
  void (*out_)(const char*, size_t) = nullptr;
  int (*in_)() = nullptr;
};

extern HostSerial Serial;
