// Package benchble carries the BLE peripheral script, so a built workbench can run
// it without the source tree beside it.
package benchble

import _ "embed"

// Peripheral is nus_peripheral.py.
//
//go:embed nus_peripheral.py
var Peripheral []byte
