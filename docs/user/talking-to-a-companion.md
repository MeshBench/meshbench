# Talking to a companion

Every companion in a scenario has a **Companion** tab in its node window, beside
Console and Settings. It speaks MeshCore's real binary protocol to the real
firmware — nothing here is simulated except the radio.

## Connecting

Press **Connect**. It claims the node's serial port exclusively, which is why it
is a button and not automatic: opening the tab must not steal the port from a
phone that is already attached. If something else holds it, the tab says what
and offers to take over.

On connect it configures the node from the scenario — name, radio, transmit
power, default scope, clock — because provisioning speaks the repeater CLI and a
companion build does not have one. Without that step a companion runs with the
firmware's default hex name, no radio and no scope.

## Sending

Pick a channel, pick a scope, type, send.

- **Channels** are read from the firmware, by index. A companion starts with
  only "Public"; adding `#sco` writes it into a free slot with the secret
  derived from its name, which is what makes a hashtag channel joinable by
  anyone who knows the name.
- **Scope** is the node's own transport configuration, set before the message
  goes. "Unscoped" is a real choice and often a revealing one.
- A sent message shows as **sending…** until the firmware acknowledges it, then
  **sent**. If it stays on sending, the node did not take it.

## Receiving

Messages arrive when the firmware pushes a notification and the client syncs —
event-driven, not polling. Nothing shown here is invented: an empty conversation
is an empty conversation.

## When it says "firmware rejected an argument as out of range"

Something was refused, and the message names the last command sent. The usual
causes:

* **transmit power** above what the build allows — the scenario asks for the
  board profile's, the build may top out lower. It is clamped now, and says so.
* **the clock** set to a time earlier than the node already believes. A fresh
  node starts from its build date, so an epoch before that is refused.
* **bandwidth** in the wrong unit. The firmware takes frequency in kHz and
  bandwidth in **Hz**, which catches everyone once.
