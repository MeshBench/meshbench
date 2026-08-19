// MeshCore's nRF52 build includes both Adafruit_LittleFS.h and
// InternalFileSystem.h. One shim, two names, so neither include order breaks.
#pragma once

#include "InternalFileSystem.h"
