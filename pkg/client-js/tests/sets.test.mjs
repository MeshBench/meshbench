// The generated sets.
//
// tools/clientgen writes these from internal/world/scenario and CI fails when
// they drift, so what is worth checking here is not the contents - the
// generator owns those - but that the shape a script relies on holds: every
// member is the string that goes on the wire, the objects cannot be edited by
// accident, and the "every one of them" arrays cannot disagree with the object
// they came from.

import { test } from "node:test";
import assert from "node:assert/strict";

import {
  Board, Boards, Class, Classes, DEFAULT_PRESET, Kind, Preset, Presets,
  Role, Roles, Strategy, Tab, Tabs, Transport,
} from "../meshbench.mjs";

const sets = { Kind, Board, Preset, Role, Class, Tab, Strategy, Transport };

test("every member is the string the workbench is keyed on", () => {
  for (const [name, set] of Object.entries(sets)) {
    const members = Object.entries(set);
    assert.ok(members.length > 0, `${name} is empty`);
    for (const [member, value] of members) {
      assert.equal(typeof value, "string", `${name}.${member} is not a string`);
      assert.ok(value.length > 0, `${name}.${member} is empty`);
    }
  }
});

test("the sets are frozen, so a typo cannot quietly add a member", () => {
  for (const [name, set] of Object.entries(sets)) {
    assert.ok(Object.isFrozen(set), `${name} is not frozen`);
  }
});

test("the every-one arrays cannot disagree with the set they came from", () => {
  for (const [all, set] of [[Boards, Board], [Presets, Preset], [Roles, Role],
    [Classes, Class], [Tabs, Tab]]) {
    assert.deepEqual([...all].sort(), Object.values(set).sort());
  }
});

// Roles are keyed on the application name, as MeshCore names its example
// directory. The published catalogue spells some of the same things differently
// - "repeater", "room-server" - and those belong to release assets: typing one
// at a verb pins nothing, and the run then refuses to start with no clue why.
test("the role names are the application names, not the catalogue's", () => {
  assert.equal(Role.SIMPLE_REPEATER, "simple_repeater");
  assert.equal(Role.COMPANION_RADIO, "companion_radio");
  assert.equal(Role.SIMPLE_ROOM_SERVER, "simple_room_server");
  // A node kind is not a role, and the two are spelled differently on purpose.
  assert.equal(Kind.SIMPLE_REPEATER, "simple-repeater");
  assert.equal(Kind.ROOM_SERVER, "room-server");
});

test("a preset a fresh scenario uses is one of the presets", () => {
  assert.ok(Presets.includes(DEFAULT_PRESET), `${DEFAULT_PRESET} is not in Presets`);
});

// The Python client spells its members identically, so a script moved between
// the two changes the dots and nothing else.
test("a board name is the profile name the simulator matches on", () => {
  assert.equal(Board.LILYGO_TDECK, "LilyGo_TDeck");
  assert.ok(Boards.length > 20, `only ${Boards.length} boards`);
});
