> **Working note, last true on 10 August 2026.** Kept for the thinking in it, not maintained as a description of the code. **3 of the 3 package paths it names no longer exist**, the seven-layer restructure of 19 August having moved them. Where this disagrees with the tree, the tree is right; the authority is the code in `internal/mesh/companion/` and `proto/`.

# The mini companion

A companion client inside the workbench, so that exercising a mesh does not
require a phone.

## Why

A companion node is the one thing in a scenario you cannot currently *use*.
Its repeaters can be configured, commanded and read; its companion has a
serial port and nothing to plug into it. Today, sending a real message means
attaching meshcore-cli or a phone app over TCP or a pty — which works, and is
the right tool for a fidelity test, and is far too much ceremony for "send
one message to #sco and see where it goes".

It is also the missing half of the live-traffic work: we can *watch* a real
network and replay its traffic, but we cannot *originate* traffic the way a
user does — through a companion, into a channel, under a scope.

## What it is not

Not a phone app, and not a replacement for one. It does the handful of things
an operator needs mid-experiment; anything beyond that is what the real
client is for, over the transports that already exist. Deliberately no
contact management beyond viewing, no key import/export, no telemetry.

## Where it lives

A **tab in a companion's node window**, beside Console / Settings / Stats /
Activity — not a dock panel. It belongs to one node, it is per-node state,
and a companion's window is where you already are when you want it. Node
windows pop out, so it goes to a second monitor with the rest of them.

Repeaters do not get the tab: they run `simple_repeater`, which speaks the
CLI, not the companion protocol.

## Connect is explicit

The tab does nothing until **Connect** is pressed, and says so. Connecting
claims the node's UART exclusively — the same claim a TCP or pty client takes
— so opening the tab must not steal the port from a phone that is already
attached. The button reads the current state:

- `Connect` — nothing holds the port.
- `In use by 127.0.0.1:53210 — take over?` — something else holds it; the
  operator can take it, and is told what they are displacing.
- `Disconnect` — we hold it. Releasing hands it straight back.

While connected the node's Console tab shows "the mini companion holds this
port", for the same reason the console already says that when a client is
attached.

## Wireframe

```
┌─ Dunkeld Companion ─────────────────────────── pop out ─┐
│ Console │ Settings │ Stats │ Activity │ Companion       │
├─────────────────────────────────────────────────────────┤
│  ● connected            [ Disconnect ]   fw v1.17.0     │
│  Dunkeld Companion · 3f9c…a21b · 869.525 MHz SF10 CR4/5  │
├─────────────────────────────────────────────────────────┤
│ ┌ Channels ───────────┐ ┌ #sco ───────────────────────┐ │
│ │ ▸ public       (0)  │ │ 12:04 ScotBot   Fife: Sunny │ │
│ │ ▸ #sco         (7)  │ │ 12:09 GM5JFC    testing 1 2 │ │
│ │ ▸ #fif         (1)  │ │ 12:11 me        received?   │ │
│ │ ▸ #weather     (3)  │ │        ↳ sent, 2 hops       │ │
│ │ ▸ private:GM0X  (2) │ │                             │ │
│ │ [ + add channel ]   │ │                             │ │
│ └─────────────────────┘ │                             │ │
│ ┌ Contacts ───────────┐ │                             │ │
│ │ GM5JFC      2 hops  │ │                             │ │
│ │ Ben Vrackie 1 hop   │ │                             │ │
│ │ [ advert ] [ sync ] │ │                             │ │
│ └─────────────────────┘ └─────────────────────────────┘ │
│  scope: [ #sco ▾ ]  ┌───────────────────────────────┐   │
│                     │ type a message…        [Send] │   │
│                     └───────────────────────────────┘   │
├─────────────────────────────────────────────────────────┤
│ Radio  869.525 MHz  250 kHz  SF10  CR4/5  22 dBm  [Set] │
└─────────────────────────────────────────────────────────┘
```

Three regions: what to talk on (left), the conversation (right), and how to
send (bottom). The scope selector sits with the send box because it is a
property of *this message*, not of the app.

## The protocol it speaks

MeshCore's companion binary protocol, over the byte-transparent serial link
that already exists (`internal/companion`). Frames are the firmware's own
`'<' / '>' + LE16 length` framing; nothing is re-framed. Commands, from
`examples/companion_radio/MyMesh.cpp`:

| need | command |
|---|---|
| handshake, self info | `CMD_APP_START` (1) → `RESP_CODE_SELF_INFO` (5) |
| message to a contact | `CMD_SEND_TXT_MSG` (2) |
| message to a channel | `CMD_SEND_CHANNEL_TXT_MSG` (3) — txt type, channel index, timestamp, text |
| contacts | `CMD_GET_CONTACTS` (4) → `CONTACTS_START` / `CONTACT` × n / `END_OF_CONTACTS` |
| advertise | `CMD_SEND_SELF_ADVERT` (7) |
| incoming | `CMD_SYNC_NEXT_MESSAGE` (10), driven by the push notification |
| radio | `CMD_SET_RADIO_PARAMS` (11): freq, bw (u32 each), sf, cr |
| power | `CMD_SET_RADIO_TX_POWER` (12) |
| name | `CMD_SET_ADVERT_NAME` (8) |
| position | `CMD_SET_ADVERT_LATLON` (14) |
| channels | `CMD_GET_CHANNEL` (31) / `CMD_SET_CHANNEL` (32) |
| time | `CMD_GET_DEVICE_TIME` (5) / `CMD_SET_DEVICE_TIME` (6) |

Channels are addressed **by index**, so the channel list is read with
`CMD_GET_CHANNEL` per slot and cached; adding one writes a slot with
`CMD_SET_CHANNEL`. A hashtag channel is the public-name form; a private
channel is a shared key in a slot. This is the firmware's model and the tab
does not invent a different one.

**Scope** is not part of the send frame: it is the node's own configuration,
held in prefs as `default_scope_name` + `default_scope_key`. It is set with
`CMD_SET_DEFAULT_FLOOD_SCOPE` (63), which takes a 31-byte padded name followed
by the 16-byte key; the bare command clears it to unscoped, and
`CMD_GET_DEFAULT_FLOOD_SCOPE` (64) reads it back as `RESP_CODE_DEFAULT_FLOOD_SCOPE`
(28). Both name **and** key are sent, because the firmware stores both and
matches on the key - a name alone scopes nothing.

This was wrong in the first version, which set the scope with the repeater CLI
`region default <name>`. **A companion build has no CLI** - only a serial rescue
mode - so the command went nowhere and every message went out unscoped while the
UI reported the scope applied. On a mesh that is entirely transport-scoped, as
ScotMesh is, that silently measures a different network from the one asked for.

## Companions are configured, like repeaters

The same mistake in a bigger place: provisioning is repeater CLI, so an imported
companion kept the firmware's default hex name, no radio and no scope, while the
fleet window reported it provisioned. `configureCompanion` applies the scenario's
name, radio, transmit power and default scope over the companion protocol, and
reads back what the node holds rather than assuming the commands landed. It runs
on connect and is the right-click *Provision* verb for a companion, and it needs
the port - so the tab must be connected first, rather than a second path fighting
it for the UART.

## What it must not do

- **No simulated replies.** Everything shown came out of the firmware. An
  empty conversation is an empty conversation.
- **No polling the port while disconnected.** Disconnected means the workbench
  is not touching that UART at all.
- **No silent reconfiguration.** Setting radio parameters or scope is an
  explicit act with a visible result, because both change what the run means.

## MCP

The same verbs, so an agent can do what the tab does — this is the gap that
stopped the ScotMesh experiment from sending anything:

| tool | does |
|---|---|
| `session_companion_connect` | claim the port, return self info |
| `session_companion_disconnect` | release it |
| `session_companion_channels` | list channels with indices |
| `session_companion_send` | text to a channel or contact, optional scope |
| `session_companion_messages` | what has arrived since a mark |
| `session_companion_advert` | send a self advert |
| `session_companion_contacts` | the contact list with hop counts |
| `session_companion_radio` | set frequency, bandwidth, SF, CR, power |

`session_companion_send` is the one that matters: "send a message from a
Fife companion to #sco" becomes one call, which is what the CAD comparison
needs to originate traffic the way a user does.

## Build order

1. **Codec** (`internal/companion/proto`): frame encode/decode for the
   commands above, table-driven, tested against captured frames. No UI.
2. **Session** (`internal/companion/client`): claim, handshake, request and
   response matching, an inbound queue. Tested against a real
   `companion_radio` build under `MESHBENCH_LIVE`.
3. **Tab**: the wireframe, reading only from the session.
4. **MCP verbs**, mirroring the tab exactly.

Each step is useful alone: the codec makes the protocol legible, the session
makes it drivable, and only then is there a UI worth looking at.

## What I would check before building

- Whether `CMD_GET_CHANNEL` enumerates empty slots or errors on them, which
  decides how the channel list is discovered.
- Whether the push notification for incoming messages arrives unsolicited or
  needs the sync command polled after a frame arrives.
- Whether setting the default scope through the CLI while the companion
  protocol holds the port is safe, or whether the claim has to be released
  first — this is the one place the design touches both interfaces, and it is
  the part most likely to be wrong.
