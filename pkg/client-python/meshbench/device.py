"""A running board, as something a script can look at and prod.

Read what the display is showing, capture it as an image, press the buttons,
type at the keyboard, touch the panel. All of it works headless - the display
is the framebuffer the controller holds, not a picture of anybody's desktop -
which is the point: a board test that needs a screen in front of it does not
run in CI.
"""

from __future__ import annotations

import time
from datetime import timedelta
from typing import TYPE_CHECKING

from .types import Screen, Shot

if TYPE_CHECKING:
    from .workbench import Workbench

#: How long to wait for the screen to change before giving up, when a caller
#: names no timeout of its own.
DEFAULT_SCREEN_WAIT = timedelta(seconds=30)


class Device:
    """One running node's board, as a device to drive.

    A handle, not a copy: it holds a name and asks."""

    def __init__(self, wb: Workbench, name: str) -> None:
        self._wb = wb
        self.name = name

    def __repr__(self) -> str:  # pragma: no cover - for a REPL
        return f"<Device {self.name!r}>"

    # ---- looking ---------------------------------------------------------

    def screen(self) -> Screen:
        """What the display is showing, as numbers. Works headless - it reads
        the framebuffer the controller holds, not the desktop."""
        return Screen.parse(self._wb.call("board.screen", {"node": self.name}) or {})

    def screenshot(self) -> Shot:
        """Write the display to a PNG and return where it landed. The frame is
        exactly what the controller holds, at the size it holds it."""
        return Shot.parse(self._wb.call("board.screenshot", {"node": self.name}) or {})

    # ---- prodding --------------------------------------------------------

    def press(self, pin: int, down: bool = True) -> None:
        """Hold a button pin down, or release it. Held rather than clicked
        because the firmware cares: MeshCore wakes a sleeping display on a
        press and powers off on a long one, so time the release yourself - or
        use tap, which does not hold."""
        self._wb.call("board.press", {"node": self.name, "pin": pin, "down": down})

    def tap(self, pin: int) -> None:
        """Press a button and let go - the ordinary click."""
        self.press(pin, True)
        self.press(pin, False)

    def type(self, text: str) -> None:
        """Enter text at the board's own keyboard, a character at a time -
        which is what the keyboard sends and what the firmware polls for."""
        self._wb.call("board.key", {"node": self.name, "text": text})

    def touch(self, x: int, y: int, down: bool = True) -> None:
        """Put a finger on the panel at a point, or lift it off."""
        self._wb.call("board.touch", {"node": self.name, "x": x, "y": y, "down": down})

    def tap_at(self, x: int, y: int) -> None:
        """Touch a point and lift off - a tap on the panel."""
        self.touch(x, y, True)
        self.touch(x, y, False)

    # ---- waiting ---------------------------------------------------------

    def wait_screen(self, timeout: timedelta = DEFAULT_SCREEN_WAIT) -> Screen:
        """Wait until the display changes from what it shows now, and return the
        new frame; raise with what it was still showing if the timeout runs out.

        This is the honest way to check an input. Half duplex eats stimuli - a
        board handed a packet while transmitting never hears it - so a tap
        followed by an immediate screen read will intermittently read the frame
        from before the tap landed. Change is by digest, so a redraw that keeps
        the same number of lit pixels still counts.
        """
        before = self.screen()
        deadline = time.monotonic() + timeout.total_seconds()
        while time.monotonic() < deadline:
            time.sleep(0.05)
            now = self.screen()
            if now.digest != before.digest:
                return now
        raise TimeoutError(
            f"board {self.name}: the screen did not change within {timeout}"
        )
