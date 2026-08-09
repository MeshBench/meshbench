# BLE peripheral

Exposes a simulated node as a **real** BLE peripheral, so an unmodified
companion app discovers and connects to it exactly as it would to hardware.

```bash
python3 tools/ble/nus_peripheral.py --name MSIM-GB7XYZ --bridge 127.0.0.1:7801
```

Verified on elite against a real adapter:

```
adapter: /org/bluez/hci0
gatt registered
adv registered
REGISTERED OK
```

## UUIDs

Taken from MeshCIM's `ble_mesh_transport.dart`, not assumed — the app is the
authority on what it looks for.

| | |
|---|---|
| service | `6E400001-B5A3-F393-E0A9-E50E24DCCA9E` |
| rx | `6E400002-…` central → peripheral (write) |
| tx | `6E400003-…` peripheral → central (notify) |

## Design

This process holds **no simulation state**. Frames bridge to the simulator over
a local socket, so it is a transport and nothing more — the clock-pinning rule
in ADR-0008 (a real client forces 1× time) stays the simulator's business.

## Requirements

Linux with BlueZ and an adapter exposing `GattManager1` and
`LEAdvertisingManager1`. A physical adapter is ideal; where none exists,
`modprobe hci_vhci` provides a virtual controller (needs root).

One node per adapter: BlueZ advertises one peripheral per controller, so a
multi-node scenario needs either several adapters or the TCP transport
(MSIM-8), which has no such limit.
