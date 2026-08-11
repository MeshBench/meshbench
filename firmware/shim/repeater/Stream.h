// Arduino's Stream, in full.
//
// MeshCore ships a minimal Stream mock for its unit tests; the repeater
// application and several helpers use more of the interface than that mock has,
// and the missing pieces are the print family that every CLI reply goes
// through.
//
// Implemented as the platform contract rather than as the subset today's
// MeshCore calls, because the firmware pipeline rebuilds against upstream
// without a human and a shim that only covers this week's usage breaks on the
// next sync.
#pragma once

#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

#include <type_traits>

class Print {
 public:
  virtual ~Print() = default;

  virtual size_t write(uint8_t c) = 0;
  virtual size_t write(const uint8_t* buf, size_t len) {
    size_t n = 0;
    for (size_t i = 0; i < len; i++) n += write(buf[i]);
    return n;
  }
  size_t write(const char* s) { return s ? write((const uint8_t*)s, strlen(s)) : 0; }

  size_t print(const char* s) { return s ? write((const uint8_t*)s, strlen(s)) : 0; }
  size_t print(char c) { return write((uint8_t)c); }

  // One template rather than an overload per width. int32_t is int on some
  // platforms and long on others, so a fixed set of overloads is ambiguous on
  // one of them and missing on the other — and which one depends on the
  // architecture the pipeline happens to be building for.
  template <class T, class = decltype(T{} + 0)>
  size_t print(T v) {
    if constexpr (std::is_floating_point_v<T>) {
      return printf("%.2f", (double)v);
    } else if constexpr (std::is_signed_v<T>) {
      return printf("%lld", (long long)v);
    } else {
      return printf("%llu", (unsigned long long)v);
    }
  }

  // Arduino's two-argument form, print(value, base). MeshCore's config
  // serializer writes every integer with it.
  template <class T, class = decltype(T{} + 0)>
  size_t print(T v, int base) {
    if (base == 16) return printf("%llx", (unsigned long long)v);
    if (base == 8) return printf("%llo", (unsigned long long)v);
    if (base == 2) return printBinary((unsigned long long)v);
    return print(v);
  }

  template <class T>
  size_t println(T v) {
    return print(v) + print("\r\n");
  }
  template <class T, class = decltype(T{} + 0)>
  size_t println(T v, int base) {
    return print(v, base) + print("\r\n");
  }
  size_t println() { return print("\r\n"); }

 private:
  size_t printBinary(unsigned long long v) {
    char buf[65];
    int i = 64;
    buf[i] = 0;
    if (v == 0) buf[--i] = '0';
    while (v && i > 0) {
      buf[--i] = (v & 1) ? '1' : '0';
      v >>= 1;
    }
    return print(&buf[i]);
  }

 public:

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
};

class Stream : public Print {
 public:
  virtual int available() = 0;
  virtual int read() = 0;
  virtual int peek() = 0;
  virtual void flush() {}

  // readBytes over any buffer type. TransportKeyStore reads arrays of uint16_t
  // and of fixed-size byte arrays straight off a stream, which a uint8_t*-only
  // signature refuses.
  size_t readBytes(uint8_t* buf, size_t len) {
    size_t n = 0;
    while (n < len) {
      int c = read();
      if (c < 0) break;
      buf[n++] = (uint8_t)c;
    }
    return n;
  }
  template <class T>
  size_t readBytes(T* buf, size_t len) {
    return readBytes((uint8_t*)buf, len);
  }
  template <class T>
  int read(T* buf, size_t len) {
    return (int)readBytes((uint8_t*)buf, len);
  }
};
