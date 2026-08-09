// The host "board" MeshCore's repeater application is built against.
//
// Every MeshCore variant provides a target.h declaring four globals and two
// functions; this is that same contract, implemented for a machine rather than
// a board. It exists so the repeater application — the real one, with its own
// forwarding policy, its own CLI and its own preferences — compiles and runs
// here unmodified.
//
// The alternative was reimplementing allowPacketForward, which is what this
// replaces. A simulator whose selling point is real firmware should not be
// deciding for itself which packets get relayed.
#pragma once

#include <Mesh.h>

#include "HostBoard.h"
#include "HostRadio.h"
#include "HostSensors.h"

extern HostBoard board;
extern HostRadio radio_driver;
extern HostRTCClock rtc_clock;
extern HostSensorManager sensors;

bool radio_init();
mesh::LocalIdentity radio_new_identity();
