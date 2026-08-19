// The host stand-in for a MeshCore board.
//
// Everything here answers with a value a real board would measure. Where there
// is no sensor to answer — battery voltage on a machine that is mains powered —
// it says so with a figure that cannot be mistaken for a measurement, rather
// than a plausible one that would quietly feed the energy model.
#pragma once

#include <Mesh.h>
#include <stdint.h>

class HostBoard : public mesh::MainBoard {
 public:
  void begin() {}

  uint8_t getStartupReason() const override { return 0; }

  // 4200 mV is a full Li-ion cell. A host has no battery, and reporting one
  // that is always full is the honest answer to "what is the battery doing" on
  // a machine plugged into the wall — internal/energy is where a real battery
  // is modelled, and it is not fed from here.
  uint16_t getBattMilliVolts() override { return 4200; }

  const char* getManufacturerName() const override { return "MeshcoreSim host"; }

  void reboot() override {}

  void powerOff() {}
  void sleep(int) {}
  void onBootComplete() {}

  // No thermal sensor. -128 is outside anything a working MCU reports, so a
  // reader sees an absent value rather than a cold day.
  float getMCUTemperature() { return -128.0f; }
};

// HostRTCClock is wall time, which is what a repeater's RTC provides once it
// has been set.
class HostRTCClock : public mesh::RTCClock {
 public:
  uint32_t getCurrentTime() override { return now_; }
  void setCurrentTime(uint32_t t) override { now_ = t; }

  // The simulator owns time, so it can move this forward without waiting.
  void advance(uint32_t seconds) { now_ += seconds; }

 private:
  uint32_t now_ = 1754700000;
};
