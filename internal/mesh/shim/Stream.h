#pragma once
// Host shim for Arduino's Stream. Upstream's test/mocks/Stream.h lacks
// println(), which src/Identity.cpp uses — this is the complete surface the
// mesh stack actually touches.
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <string>

class Stream {
public:
  virtual ~Stream() {}
  virtual size_t write(uint8_t c) { buf_.push_back((char)c); return 1; }
  virtual size_t write(const uint8_t* p, size_t len) { buf_.append((const char*)p, len); return len; }
  size_t write(const char* p, size_t len) { return write((const uint8_t*)p, len); }
  virtual int available() { return 0; }
  virtual int read() { return -1; }
  virtual int peek() { return -1; }
  virtual void flush() {}

  // Blocking-ish read used by Identity::readFrom. Reads from an injectable
  // input buffer; returns short if the buffer runs out, which is what real
  // hardware does on timeout.
  virtual size_t readBytes(uint8_t* dst, size_t len) {
    size_t n = 0;
    while (n < len && in_pos_ < in_.size()) dst[n++] = (uint8_t)in_[in_pos_++];
    return n;
  }
  size_t readBytes(char* dst, size_t len) { return readBytes((uint8_t*)dst, len); }

  // Test hook: what the simulator feeds this stream.
  void feed(const std::string& data) { in_ += data; }

  size_t print(const char* s) { buf_ += s; return strlen(s); }
  size_t print(char c) { buf_ += c; return 1; }
  size_t print(int v) { char t[24]; int n = snprintf(t, sizeof t, "%d", v); buf_ += t; return n; }
  size_t print(unsigned v) { char t[24]; int n = snprintf(t, sizeof t, "%u", v); buf_ += t; return n; }
  size_t print(long v) { char t[32]; int n = snprintf(t, sizeof t, "%ld", v); buf_ += t; return n; }
  size_t print(unsigned long v) { char t[32]; int n = snprintf(t, sizeof t, "%lu", v); buf_ += t; return n; }
  size_t print(float v) { char t[32]; int n = snprintf(t, sizeof t, "%f", (double)v); buf_ += t; return n; }
  size_t print(double v) { char t[32]; int n = snprintf(t, sizeof t, "%f", v); buf_ += t; return n; }

  size_t println() { buf_ += '\n'; return 1; }
  template <typename T> size_t println(T v) { size_t n = print(v); return n + println(); }

  size_t printf(const char* fmt, ...);

  // Test/observation hook: what the firmware has written to this stream.
  const std::string& captured() const { return buf_; }
  void clearCaptured() { buf_.clear(); }

protected:
  std::string buf_;
  std::string in_;
  size_t in_pos_ = 0;
};

#include <cstdarg>
inline size_t Stream::printf(const char* fmt, ...) {
  char t[512];
  va_list ap; va_start(ap, fmt);
  int n = vsnprintf(t, sizeof t, fmt, ap);
  va_end(ap);
  if (n > 0) buf_.append(t, (size_t)n);
  return n > 0 ? (size_t)n : 0;
}
