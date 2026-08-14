# Windows that behave: where dialogs, answers and errors belong

Four complaints from using MeshBench on a Mac, and they are all the same
complaint underneath: **a pop-out window is a second-class window.** It can
draw a panel and nothing else - no prompt, no status line, no way to stay in
front - so anything it triggers happens somewhere the person is not looking.

Reported: clicking **import** in the Firmware window put "Import a build from"
in the *main* window, and the error that followed appeared in the *main*
window's status bar, behind the window being used.

## What a pop-out window can do today

`windows.popOut` (`internal/workbench/windows.go`) opens an `app.Window` whose
whole frame is:

    comp.Fill(...)
    layout.UniformInset(...) -> p.Draw(theme, gtx, snapshot)

That is the panel and nothing else. The main window's frame, by contrast,
draws the menu bar, the panel, **the status line** (`shell.go:378`) and
**`sh.Ask.Layout`** (`shell.go:200`) - the one prompt overlay the whole
application has. `Shell.Ask` is a single `Prompt` value, so every question,
from every window, is drawn by whichever window draws the shell.

That single fact explains three of the four reports.

## 1. Prompts open in the wrong window

**Cause.** One `Prompt` on the shell, drawn only by the main window. Nine call
sites post to it (`Ask.Open` / `Ask.Choose`), and none of them can say where
they were asked from.

**Fix.** A prompt belongs to a window.

- Give the pop-out frame the same overlay treatment the main window has: draw
  a `*shell.Prompt` after the panel.
- Give each window its own `Prompt` value, and let a panel reach *its own*
  window's prompt rather than the shell's. The panels already receive their
  chooser as a `choose func(title, opts, pick)` callback wired in `main.go` -
  that indirection is exactly the seam: wire each window's panels to that
  window's prompt instead of the shared one.
- The shell's `Ask` stays as the main window's prompt, so menu actions are
  unchanged.

**Test.** The control audit already presses every control; a prompt opened
from a popped-out panel should raise that window's prompt, not the shell's,
and that is assertable without a screen.

## 2. Errors and status appear where nobody is looking

**Cause.** `World.Status` is one string, drawn by the main window only. A verb
that fails says why through `ui.said`, which lands there.

**Fix, in two parts.**

- **Every window draws the status line.** It is one string in the snapshot;
  a pop-out showing it costs nothing and means the answer is wherever you are.
- **An action's result belongs to the window that started it.** The dispatcher
  in `main.go` (`do(verb, params)`) is where a failure becomes `ui.said`; it
  needs to know which window called it. Same shape as the prompt fix: each
  window's panels get a `do` that tags the window, and the failure is drawn
  there as well as in the shared line.

The second half matters more than it sounds: "firmware failed, so the run has
not started" is the sentence that explains why nothing happened, and it
appeared in a window that was behind the one being used.

## 3. Pop-out windows should stay above the main window

**This one is not free, and the code already knows why** - `windows.go` says
it: Gio can raise a window (`system.ActionRaise`) and has no always-on-top,
because under Wayland no client may ask; the compositor decides.

So it has to be done per platform, through the native handle Gio hands out in
`app.ViewEvent`:

| platform | how | cost |
|---|---|---|
| macOS | `NSWindow.setLevel:` to floating, via the view in `app.ViewEvent` | cgo + objc, small |
| Windows | `SetWindowPos(hwnd, HWND_TOPMOST, ...)` | syscall, small |
| X11 | `_NET_WM_STATE_ABOVE` via a client message | needs an X connection |
| Wayland | **not possible from the client** | a compositor rule, documented |

**Recommendation.** Do macOS and Windows, where it is a few lines each and
where Alex hit the problem. On X11 do it if the EWMH message is cheap;
on Wayland say so plainly in the docs rather than pretending. Make it a
setting - "keep panel windows in front" - defaulting on, because always-on-top
is a preference as often as it is a fix, and a window that cannot be put
behind anything is its own annoyance.

## 4. Anywhere it asks for a path, offer a browse button

**Cause.** There is no file dialog anywhere in MeshBench. `Ask.Open` gives a
text field and a hint - "path to a binary" - and expects a path to be typed or
pasted. Fine for a script, hostile in a window.

**Decision (Alex): use each platform's own dialog, not an in-app one.**

`ncruces/zenity` does exactly that and costs less than writing three of them:

| platform | what it drives |
|---|---|
| macOS | `osascript` -> the real Finder open/save panel |
| Windows | `IFileOpenDialog` through COM |
| Linux | xdg-desktop-portal, then zenity, then kdialog |

It is **MIT** (compatible with our GPL-3.0, and the licence window picks it up
automatically), actively maintained, and - the part that matters here - **no
cgo**, so it does not complicate the Windows cross-build or the macOS bundle.
The Linux portal path also works under Wayland, where an in-app browser would
have been the only other option.

**Where the button goes.** Five places ask for a path today: firmware import,
open a network, save as, the tile cache directory, and the fixture field. One
affordance used five times, not five dialogs:

- `Prompt.Open` gains a path variant - the same text field, plus a **browse**
  button that opens the platform dialog and fills the field. Typing still
  works, which keeps every script and the control audit working.
- The dialog is a blocking call on its own goroutine; the answer arrives back
  through the same callback the field uses, so nothing else changes.

**What this costs in testing.** A native dialog cannot be driven by the
control audit or captured by the screenshot flags - it is not our window. So
the audit keeps pressing the *field*, which still works, and the browse button
is covered by a test that it invokes the picker rather than by driving the
picker itself. That is the trade for dialogs people recognise, and it is the
right trade.

## Order

1. **Status in every window** (2a) - one line of layout, and it stops errors
   vanishing while the rest is built.
2. **Per-window prompts** (1) - the structural fix, and 2b falls out of the
   same seam.
3. **The browse button** (4) - the biggest piece of new UI, but self-contained.
4. **Always in front** (3), macOS and Windows first, as a setting.
