#!/usr/bin/env python3
"""Expose a simulated MeshCore node as a real BLE peripheral.

Registers a Nordic UART Service with BlueZ over D-Bus and advertises it, so an
unmodified companion app (MeshCIM, the official clients) discovers and connects
to a simulated node exactly as it would to hardware.

UUIDs are MeshCore's own, taken from MeshCIM's ble_mesh_transport.dart rather
than assumed:

    service  6E400001-B5A3-F393-E0A9-E50E24DCCA9E
    rx       6E400002-...   central -> peripheral (write)
    tx       6E400003-...   peripheral -> central (notify)

Frames are bridged to the simulator over a local socket, so this process holds
no simulation state — it is a transport, and the clock-pinning rule in ADR-0008
stays the simulator's business.

    python3 nus_peripheral.py --name MSIM-GB7XYZ --bridge 127.0.0.1:7801
"""
import argparse
import socket
import sys

import dbus
import dbus.mainloop.glib
import dbus.service
from gi.repository import GLib

BLUEZ = "org.bluez"
GATT_MANAGER = "org.bluez.GattManager1"
LE_ADVERTISING_MANAGER = "org.bluez.LEAdvertisingManager1"
DBUS_OM = "org.freedesktop.DBus.ObjectManager"
DBUS_PROPS = "org.freedesktop.DBus.Properties"

SVC_UUID = "6e400001-b5a3-f393-e0a9-e50e24dcca9e"
RX_UUID = "6e400002-b5a3-f393-e0a9-e50e24dcca9e"
TX_UUID = "6e400003-b5a3-f393-e0a9-e50e24dcca9e"


class Application(dbus.service.Object):
    def __init__(self, bus, bridge):
        self.path = "/org/meshcoresim/app"
        self.services = [NUSService(bus, 0, bridge)]
        super().__init__(bus, self.path)

    def get_path(self):
        return dbus.ObjectPath(self.path)

    @dbus.service.method(DBUS_OM, out_signature="a{oa{sa{sv}}}")
    def GetManagedObjects(self):
        out = {}
        for s in self.services:
            out[s.get_path()] = s.props()
            for c in s.chars:
                out[c.get_path()] = c.props()
        return out


class NUSService(dbus.service.Object):
    def __init__(self, bus, index, bridge):
        self.path = f"/org/meshcoresim/app/service{index}"
        super().__init__(bus, self.path)
        self.chars = [TxChar(bus, 0, self), RxChar(bus, 1, self, bridge)]

    def get_path(self):
        return dbus.ObjectPath(self.path)

    def props(self):
        return {"org.bluez.GattService1": {
            "UUID": SVC_UUID, "Primary": dbus.Boolean(True),
        }}


class Characteristic(dbus.service.Object):
    def __init__(self, bus, index, service, uuid, flags):
        self.path = f"{service.path}/char{index}"
        self.uuid, self.flags, self.service = uuid, flags, service
        self.notifying = False
        super().__init__(bus, self.path)

    def get_path(self):
        return dbus.ObjectPath(self.path)

    def props(self):
        return {"org.bluez.GattCharacteristic1": {
            "Service": self.service.get_path(),
            "UUID": self.uuid,
            "Flags": self.flags,
        }}

    @dbus.service.method("org.bluez.GattCharacteristic1", in_signature="aya{sv}")
    def WriteValue(self, value, options):
        pass

    @dbus.service.method("org.bluez.GattCharacteristic1")
    def StartNotify(self):
        self.notifying = True

    @dbus.service.method("org.bluez.GattCharacteristic1")
    def StopNotify(self):
        self.notifying = False


class TxChar(Characteristic):
    """Peripheral -> central. The simulator's frames reach the app here."""

    def __init__(self, bus, index, service):
        super().__init__(bus, index, service, TX_UUID, ["notify"])

    def send(self, data: bytes):
        if not self.notifying:
            return
        self.PropertiesChanged("org.bluez.GattCharacteristic1",
                               {"Value": [dbus.Byte(b) for b in data]}, [])

    @dbus.service.signal(DBUS_PROPS, signature="sa{sv}as")
    def PropertiesChanged(self, interface, changed, invalidated):
        pass


class RxChar(Characteristic):
    """Central -> peripheral. App writes land here and go to the simulator."""

    def __init__(self, bus, index, service, bridge):
        super().__init__(bus, index, service, RX_UUID,
                         ["write", "write-without-response"])
        self.bridge = bridge

    @dbus.service.method("org.bluez.GattCharacteristic1", in_signature="aya{sv}")
    def WriteValue(self, value, options):
        frame = bytes(bytearray(value))
        if self.bridge:
            try:
                self.bridge.sendall(frame)
            except OSError as e:
                print(f"bridge write failed: {e}", file=sys.stderr)


class Advertisement(dbus.service.Object):
    def __init__(self, bus, index, name):
        self.path = f"/org/meshcoresim/adv{index}"
        self.name = name
        super().__init__(bus, self.path)

    def get_path(self):
        return dbus.ObjectPath(self.path)

    @dbus.service.method(DBUS_PROPS, in_signature="s", out_signature="a{sv}")
    def GetAll(self, interface):
        return {
            "Type": "peripheral",
            "ServiceUUIDs": dbus.Array([SVC_UUID], signature="s"),
            "LocalName": dbus.String(self.name),
            "Includes": dbus.Array(["tx-power"], signature="s"),
        }

    @dbus.service.method("org.bluez.LEAdvertisement1")
    def Release(self):
        pass


def find_adapter(bus):
    om = dbus.Interface(bus.get_object(BLUEZ, "/"), DBUS_OM)
    for path, ifaces in om.GetManagedObjects().items():
        if GATT_MANAGER in ifaces and LE_ADVERTISING_MANAGER in ifaces:
            return path
    return None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--name", default="MSIM-node")
    ap.add_argument("--bridge", default="", help="host:port of the simulator's node socket")
    ap.add_argument("--check", action="store_true", help="verify registration then exit")
    a = ap.parse_args()

    dbus.mainloop.glib.DBusGMainLoop(set_as_default=True)
    bus = dbus.SystemBus()
    adapter = find_adapter(bus)
    if not adapter:
        sys.exit("no BlueZ adapter with GattManager1 + LEAdvertisingManager1")
    print(f"adapter: {adapter}")

    sock = None
    if a.bridge:
        host, _, port = a.bridge.partition(":")
        sock = socket.create_connection((host, int(port)), timeout=5)

    app = Application(bus, sock)
    adv = Advertisement(bus, 0, a.name)

    gm = dbus.Interface(bus.get_object(BLUEZ, adapter), GATT_MANAGER)
    am = dbus.Interface(bus.get_object(BLUEZ, adapter), LE_ADVERTISING_MANAGER)

    loop = GLib.MainLoop()
    ok = {"gatt": False, "adv": False}

    def done(kind):
        ok[kind] = True
        print(f"{kind} registered")
        if a.check and all(ok.values()):
            print("REGISTERED OK")
            loop.quit()

    def fail(kind, e):
        print(f"{kind} registration failed: {e}", file=sys.stderr)
        loop.quit()

    gm.RegisterApplication(app.get_path(), {},
                           reply_handler=lambda: done("gatt"),
                           error_handler=lambda e: fail("gatt", e))
    am.RegisterAdvertisement(adv.get_path(), {},
                             reply_handler=lambda: done("adv"),
                             error_handler=lambda e: fail("adv", e))
    if a.check:
        GLib.timeout_add_seconds(10, loop.quit)
    print(f"advertising as {a.name!r} with the MeshCore companion service")
    loop.run()


if __name__ == "__main__":
    main()
