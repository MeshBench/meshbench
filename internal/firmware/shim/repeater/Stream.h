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
  size_t print(int v) { return printf("%d", v); }
  size_t print(unsigned int v) { return printf("%u", v); }
  size_t print(long v) { return printf("%ld", v); }
  size_t print(unsigned long v) { return printf("%lu", v); }
  size_t print(float v) { return printf("%.2f", (double)v); }
  size_t print(double v) { return printf("%.2f", v); }

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
