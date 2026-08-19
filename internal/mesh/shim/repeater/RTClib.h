// Minimal RTClib for the host build.
//
// MeshCore's repeater includes RTClib for one type: DateTime, used to format
// log timestamps. The real library drives I2C clock chips, none of which exist
// here.
#pragma once

#include <stdint.h>
#include <time.h>

class DateTime {
 public:
  DateTime(uint32_t epoch = 0) : t_(epoch) {}

  uint16_t year() const { return (uint16_t)(fields().tm_year + 1900); }
  uint8_t month() const { return (uint8_t)(fields().tm_mon + 1); }
  uint8_t day() const { return (uint8_t)fields().tm_mday; }
  uint8_t hour() const { return (uint8_t)fields().tm_hour; }
  uint8_t minute() const { return (uint8_t)fields().tm_min; }
  uint8_t second() const { return (uint8_t)fields().tm_sec; }
  uint32_t unixtime() const { return t_; }

 private:
  // Real calendar arithmetic rather than division by an average year. A log
  // line dated the 31st of February is the sort of detail that makes a reader
  // doubt everything else in the file.
  struct tm fields() const {
    time_t tt = (time_t)t_;
    struct tm out {};
    gmtime_r(&tt, &out);
    return out;
  }
  uint32_t t_;
};
