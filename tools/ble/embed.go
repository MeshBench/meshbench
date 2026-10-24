// Package ble carries the BLE peripheral script, so a built workbench can run
// it without the source tree beside it.
package ble

import _ "embed"

// Peripheral is nus_peripheral.py.
//
//go:embed nus_peripheral.py
var Peripheral []byte
