// The Arduino platform, for a host.
//
// Ours rather than MeshCore's test mock, because the mock brings its own
// minimal Stream and two definitions of Print cannot coexist. Owning both
// headers keeps the contract in one place.
//
// millis() is the simulator's clock, not the wall clock. Every timing decision
// the firmware makes — CSMA, retransmit delay, duty-cycle refill — reads from
// here, so a node that consulted real time would run at real speed and could
// not be stepped, replayed or made deterministic.
#pragma once

#include <cmath>
#include <cstdint>
#include <cstdlib>
#include <cstring>

#include "Stream.h"

using std::isnan;

// Set by the bridge on every tick.
extern uint32_t g_sim_millis;

inline uint32_t millis() { return g_sim_millis; }
inline uint32_t micros() { return g_sim_millis * 1000; }

// A firmware that busy-waits would hang the simulation: simulated time only
// advances when the bridge says so, so a delay that spun until millis() moved
// would spin forever. Nothing in the repeater's main path calls it.
inline void delay(uint32_t) {}
inline void delayMicroseconds(uint32_t) {}
inline void yield() {}

inline void pinMode(int, int) {}
inline void digitalWrite(int, int) {}
inline int digitalRead(int) { return 0; }
inline int analogRead(int) { return 0; }

#define HIGH 1
#define LOW 0
#define INPUT 0
#define OUTPUT 1
#define INPUT_PULLUP 2
