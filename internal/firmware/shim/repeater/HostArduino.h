// The handful of Arduino functions MeshCore's helpers use that its own test
// mocks do not provide.
//
// Force-included ahead of everything else, because ArduinoHelpers.h calls them
// at namespace scope and there is nowhere later to put them.
#pragma once

#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// Seeded rather than entropic: a run has to be reproducible from its seed, and
// the identity a node generates comes through here.
inline void randomSeed(unsigned long seed) { srandom(seed); }

// Arduino defines these as macros and MeshCore calls them unqualified.
// Templates rather than macros: force-including a `min(a,b)` macro ahead of the
// standard library breaks <limits>, whose numeric_limits<T>::min() then looks
// like a macro call with one argument. That produced 368 errors inside libstdc++
// and none in any MeshCore file, which is a confusing place to start debugging.
template <class T, class U>
constexpr auto min(T a, U b) -> decltype(a < b ? a : b) {
  return a < b ? a : b;
}
template <class T, class U>
constexpr auto max(T a, U b) -> decltype(a > b ? a : b) {
  return a > b ? a : b;
}
template <class T, class L, class H>
constexpr auto constrain(T v, L lo, H hi) -> decltype(v < lo ? lo : (v > hi ? hi : v)) {
  return v < lo ? lo : (v > hi ? hi : v);
}

// Arduino's integer-to-string helpers, which MeshCore uses for its CLI replies.
inline char* ltoa(long value, char* out, int base) {
  if (base == 10) {
    sprintf(out, "%ld", value);
  } else if (base == 16) {
    sprintf(out, "%lx", value);
  } else {
    sprintf(out, "%ld", value);
  }
  return out;
}
inline char* itoa(int value, char* out, int base) { return ltoa(value, out, base); }
inline char* utoa(unsigned long value, char* out, int base) {
  sprintf(out, base == 16 ? "%lx" : "%lu", value);
  return out;
}

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

  // The full Arduino print family, not the subset today's MeshCore happens to
  // call.
  //
  // This shim has to keep compiling against MeshCore versions nobody has
  // written yet — the firmware pipeline syncs upstream and rebuilds without a
  // human — so implementing the platform contract is the job, and implementing
  // exactly what one release uses is how it breaks on the next one.
  size_t print(const char* s) { return s ? write((const uint8_t*)s, strlen(s)) : 0; }
  size_t print(char c) { return write((uint8_t)c); }
  size_t print(int v) { return printf("%d", v); }
  size_t print(unsigned int v) { return printf("%u", v); }
  size_t print(long v) { return printf("%ld", v); }
  size_t print(unsigned long v) { return printf("%lu", v); }
  size_t print(double v) { return printf("%.2f", v); }
  size_t print(float v) { return printf("%.2f", (double)v); }

  template <class T>
  size_t println(T v) {
    return print(v) + print("\r\n");
  }
  size_t println() { return print("\r\n"); }
  size_t printf(const char* fmt, ...) {
    char buf[512];
    va_list ap;
    va_start(ap, fmt);
    int n = vsnprintf(buf, sizeof buf, fmt, ap);
    va_end(ap);
    if (n <= 0) return 0;
    if (n > (int)sizeof buf - 1) n = (int)sizeof buf - 1;
    return write((const uint8_t*)buf, (size_t)n);
  }

  void begin(unsigned long) {}
  explicit operator bool() const { return true; }

 private:
  void (*out_)(const char*, size_t) = nullptr;
  int (*in_)() = nullptr;
};

extern HostSerial Serial;
