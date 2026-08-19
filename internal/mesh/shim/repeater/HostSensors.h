// No sensors on a host.
//
// The repeater application queries environment sensors and formats whatever
// they return into its advert. A host has none, so this answers empty — which
// is what a board with no sensors fitted does, and is therefore a case the
// firmware already handles.
#pragma once

#include <helpers/SensorManager.h>

class HostSensorManager : public SensorManager {
 public:
  bool begin() override { return false; }
  bool querySensors(uint8_t, CayenneLPP&) override { return false; }
  void loop() override {}
  int getNumSettings() const override { return 0; }
  const char* getSettingName(int) const override { return nullptr; }
  const char* getSettingValue(int) const override { return nullptr; }
  bool setSettingValue(const char*, const char*) override { return false; }
};
