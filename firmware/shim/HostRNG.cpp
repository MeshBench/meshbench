#include <RNG.h>
#include <cstdint>
RNGClass RNG;
RNGClass::RNGClass() {}
RNGClass::~RNGClass() {}
void RNGClass::begin(const char*) {}
void RNGClass::rand(uint8_t* data, size_t len) {
  static uint64_t s = 0x9E3779B97F4A7C15ULL;  // seeded: determinism is a requirement
  for (size_t i = 0; i < len; i++) { s = s*6364136223846793005ULL + 1442695040888963407ULL; data[i] = (uint8_t)(s>>33); }
}
bool RNGClass::available(size_t) const { return true; }
void RNGClass::stir(const uint8_t*, size_t, unsigned int) {}
void RNGClass::save() {}
void RNGClass::loop() {}
void RNGClass::destroy() {}
