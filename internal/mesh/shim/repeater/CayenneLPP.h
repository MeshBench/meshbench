// Cayenne LPP, enough of it for the repeater's telemetry advert.
//
// MeshCore's own test mock is smaller than the repeater needs. This encodes the
// same wire format — channel, type, then a big-endian value at the type's own
// scale — so a telemetry packet built here is one a real receiver would decode.
#pragma once

#include <stdint.h>
#include <string.h>

#include <vector>

#define LPP_TEMPERATURE 103
#define LPP_VOLTAGE 116
#define LPP_RELATIVE_HUMIDITY 104
#define LPP_BAROMETRIC_PRESSURE 115

class CayenneLPP {
 public:
  // A vector rather than a raw new[]: the buffer is the only thing this class
  // owns, and owning it by hand would mean writing a copy constructor and an
  // assignment operator that nobody has needed yet. Without them a copy shares
  // one allocation and the second destructor frees it twice, which is a crash
  // in whichever caller first passes one of these by value.
  explicit CayenneLPP(uint8_t size) : buf_(size) {}

  void reset() { len_ = 0; }
  uint8_t getSize() const { return len_; }
  uint8_t* getBuffer() { return buf_.data(); }

  // 0.01 V per count, big endian, as the specification defines it.
  uint8_t addVoltage(uint8_t channel, float v) {
    return add2(channel, LPP_VOLTAGE, (int16_t)(v * 100));
  }
  // 0.1 degrees per count.
  uint8_t addTemperature(uint8_t channel, float c) {
    return add2(channel, LPP_TEMPERATURE, (int16_t)(c * 10));
  }
  uint8_t addRelativeHumidity(uint8_t channel, float pct) {
    if ((size_t)len_ + 3 > buf_.size()) return 0;
    buf_[len_++] = channel;
    buf_[len_++] = LPP_RELATIVE_HUMIDITY;
    buf_[len_++] = (uint8_t)(pct * 2);
    return len_;
  }
  uint8_t addBarometricPressure(uint8_t channel, float hpa) {
    return add2(channel, LPP_BAROMETRIC_PRESSURE, (int16_t)(hpa * 10));
  }

 private:
  uint8_t add2(uint8_t channel, uint8_t type, int16_t value) {
    if ((size_t)len_ + 4 > buf_.size()) return 0;
    buf_[len_++] = channel;
    buf_[len_++] = type;
    buf_[len_++] = (uint8_t)(value >> 8);
    buf_[len_++] = (uint8_t)(value & 0xFF);
    return len_;
  }
  std::vector<uint8_t> buf_;
  uint8_t len_ = 0;
};
