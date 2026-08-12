#!/usr/bin/env python3
"""Build the comparison page with the screenshots embedded."""
import base64, os

D = os.path.expanduser("~/Documents/projects/meshcoresim/spikes/shots")


def img(name):
    with open(os.path.join(D, name), "rb") as f:
        return "data:image/jpeg;base64," + base64.b64encode(f.read()).decode()


HTML = """<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Three UI toolkits, one Plan view</title>
<style>
:root{--ground:#faf9f6;--card:#fff;--ink:#191d1c;--dim:#585f5d;--faint:#838a88;
      --rule:#dcd9d1;--accent:#12695a;--warn:#9a5a1e;--bad:#a33f2f;--good:#3f6b2c}
@media (prefers-color-scheme:dark){:root:not([data-theme="light"]){
  --ground:#0d1211;--card:#151b1a;--ink:#e6eae8;--dim:#98a3a0;--faint:#78827f;
  --rule:#232c2a;--accent:#5cbfa8;--warn:#dd9e69;--bad:#e08a76;--good:#a7c77e}}
*{box-sizing:border-box}
body{background:var(--ground);color:var(--ink);margin:0;padding:0 22px 90px;
     font:17px/1.62 Charter,"Bitstream Charter",Georgia,serif}
.wrap{max-width:1100px;margin:0 auto}.col{max-width:68ch}
code{font-family:ui-monospace,Menlo,monospace;font-size:.88em}
header{border-bottom:2px solid var(--ink);padding:48px 0 20px;margin-bottom:26px}
.eyebrow{font:12px ui-monospace,Menlo,monospace;letter-spacing:.14em;text-transform:uppercase;
         color:var(--faint);margin-bottom:18px}
h1{font:600 clamp(26px,4vw,40px)/1.12 ui-monospace,Menlo,monospace;letter-spacing:-.02em;
   margin:0 0 14px;max-width:26ch}
.stand{font-size:20px;line-height:1.5;color:var(--dim);max-width:62ch;margin:0}
h2{font:600 12px/1 ui-monospace,Menlo,monospace;letter-spacing:.16em;text-transform:uppercase;
   color:var(--accent);margin:52px 0 6px;padding-bottom:8px;border-bottom:1px solid var(--rule)}
h3{font:600 17px ui-monospace,Menlo,monospace;margin:28px 0 6px}
p{margin:0 0 16px}
figure{margin:22px 0;background:var(--card);border:1px solid var(--rule);padding:14px}
figure img{width:100%;display:block;border:1px solid var(--rule)}
figcaption{font-size:14px;color:var(--dim);margin-top:11px;padding-top:10px;
           border-top:1px solid var(--rule)}
table{border-collapse:collapse;width:100%;font:13.5px ui-monospace,Menlo,monospace;margin:18px 0}
th{text-align:left;font-size:11px;letter-spacing:.09em;text-transform:uppercase;color:var(--faint);
   padding:0 12px 8px 0;border-bottom:1px solid var(--rule)}
td{padding:8px 12px 8px 0;border-bottom:1px solid var(--rule);vertical-align:top}
.good{color:var(--good)}.bad{color:var(--bad)}.warn{color:var(--warn)}
.box{background:var(--card);border:1px solid var(--rule);border-left:4px solid var(--accent);
     padding:20px 22px;margin:24px 0}
.box.warn{border-left-color:var(--warn)}.box.bad{border-left-color:var(--bad)}
.box .lab{font:11px ui-monospace,Menlo,monospace;letter-spacing:.16em;text-transform:uppercase;
          color:var(--accent);display:block;margin-bottom:8px}
.box.warn .lab{color:var(--warn)}.box.bad .lab{color:var(--bad)}
.box p{margin:0;font-size:18px;max-width:60ch}
ul{max-width:68ch}li{margin-bottom:8px}
footer{margin-top:60px;padding-top:16px;border-top:2px solid var(--ink);
       font:12px ui-monospace,Menlo,monospace;color:var(--faint);line-height:1.7}
</style></head><body><div class="wrap">

<header>
  <div class="eyebrow">MeshBench &middot; UI spike &middot; branch ui-spikes &middot; 12 August 2026</div>
  <h1>Three toolkits, one Plan view</h1>
  <p class="stand">Each spike draws the same thing from the same data: the real
  58 node Fife network, 283 links, a filterable node list, an inspector, and one
  panel that can leave the layout and become a window of its own. They are
  throwaway, and they took roughly a day between them.</p>
</header>

<div class="col">
<h2>What was being tested</h2>
<p>Four questions, chosen because they are where ImGui costs the most in
MeshBench today:</p>
<ul>
<li><b>Layout.</b> A toolbar, a map that fills the space, side panels and a
status bar, without hand-placed pixels. The current UI has 137 <code>SameLine</code>
calls and 82 <code>SetNextItemWidth</code> calls doing that work by hand.</li>
<li><b>Custom drawing.</b> The map, its links and its labels.</li>
<li><b>Tables.</b> A filtered list with stable row identity.</li>
<li><b>Windows, not only docks.</b> The specific complaint: sometimes a panel
should be a window, and docking everything is the wrong default.</li>
</ul>
</div>

<h2>Gio</h2>
<figure><img alt="The Plan view in Gio" src="__GIO__">
<figcaption>Gio v0.10.2. Correct geography, links, colour by node kind, the node
list with its filter, and the inspector reading real fields. Note the transmit
power: 22 dBm, from the firmware fix earlier today.</figcaption></figure>
<div class="col">
<p>The closest to a finished thing on the first attempt. The layout is a flex
tree that reads like layout, custom drawing is a first-class operation rather
than an escape hatch, and a second window is <code>new(app.Window)</code> with
no framework in between.</p>
<p><b>The emoji question, settled.</b> The first attempt drew empty boxes,
because the theme used the Go fonts and those carry no emoji glyphs. A shaper
falls through its collection rune by rune, so appending a system emoji face is
the whole fix:</p>
<pre style="background:var(--card);border:1px solid var(--rule);padding:12px 14px;
overflow-x:auto;font-size:13px"><code>faces, _ := opentype.ParseCollection(notoColorEmoji)
th.Shaper = text.NewShaper(text.WithCollection(append(gofont.Collection(), faces...)))</code></pre>
<figure style="margin-top:16px"><img alt="Colour emoji in Gio" src="__GIOEMOJI__">
<figcaption>Colour bitmaps, not monochrome outlines. The two nodes still showing
replacement characters are that way in the source data: their CoreScope names
contain invalid bytes, which is a fact about the network rather than the
toolkit.</figcaption></figure>
<p><b>The remaining cost</b> is the build dependency: it would not compile until
the Vulkan development headers were installed, even though the Vulkan loader was
already present, and no build tag avoids it.</p>
</div>

<h2>Electron</h2>
<figure><img alt="The Plan view in Electron" src="__ELECTRON__">
<figcaption>Electron 33 with a canvas map. The same scene, plus the emoji in
node names that the Go toolkits drop.</figcaption></figure>
<div class="col">
<p>The fastest to write by a wide margin, and the best typography of the three
without any effort. Text wrapping, ellipsis, scrollbars, focus rings and emoji
all work because a browser engine has spent twenty years on them.</p>
<p><b>The costs are structural.</b> 270 MB of <code>node_modules</code> for a
spike. It is a second runtime beside the Go one, so the simulation would live
behind an IPC boundary rather than in the same process as the thing drawing it,
and the WebGPU compute path would have to be re-plumbed or duplicated. It also
contradicts ADR-0005 directly.</p>
</div>

<h2>Cogent Core</h2>
<figure><img alt="The Plan view in Cogent Core" src="__COGENT__">
<figcaption>Cogent Core v0.3.39. The map draws correctly. The side panel does
not: the node list is empty, the inspector wraps a character at a time, and the
link colours cycle instead of using the one colour they were given.</figcaption></figure>
<div class="col">
<p>The one I expected to recommend, because it already shares
<code>cogentcore/webgpu</code> with MeshBench, and the one that went worst.</p>
<p>Three things cost real time. Its canvas takes <b>normalised 0 to 1
coordinates</b> rather than pixels, so the first version drew everything off
screen. Setting <code>AppearanceSettings.Zoom</code> to make the type comparable
produced a window that rendered nothing at all. And it <b>persists window
geometry</b> per application name, so once a bad geometry was saved, every
later launch came up with an unusable window, no error, and an empty log. That
took three quarters of an hour to find, and it is the sort of thing that would
cost a user a support conversation rather than a bug report.</p>
<p>The broken panel is partly my inexperience with its styling model. The
disappearing window is not.</p>
</div>

<h2>Side by side</h2>
<table>
<tr><th>&nbsp;</th><th>Gio</th><th>Electron</th><th>Cogent Core</th></tr>
<tr><td>first working render</td><td class="good">first try</td><td class="good">first try</td><td class="bad">after ~45 min of debugging</td></tr>
<tr><td>build dependencies</td><td class="warn">Vulkan headers</td><td class="bad">270 MB node_modules</td><td class="good">none beyond Go</td></tr>
<tr><td>binary or bundle</td><td class="good">13 MB, one file</td><td class="bad">~200 MB bundle</td><td class="good">one file</td></tr>
<tr><td>layout</td><td class="good">flex tree</td><td class="good">CSS grid and flex</td><td class="warn">CSS-like, fought back</td></tr>
<tr><td>custom drawing</td><td class="good">native operation</td><td class="good">canvas</td><td class="warn">normalised coordinates</td></tr>
<tr><td>text and emoji</td><td class="good">colour emoji, one line</td><td class="good">everything</td><td class="bad">wrapped per character</td></tr>
<tr><td>second OS window</td><td class="good">one call</td><td class="good">window.open</td><td class="good">one call</td></tr>
<tr><td>same process as the engine</td><td class="good">yes</td><td class="bad">no, IPC</td><td class="good">yes</td></tr>
<tr><td>shares the WebGPU stack</td><td class="warn">separate</td><td class="bad">separate</td><td class="good">the same one</td></tr>
</table>

<div class="col">
<h2>Recommendation</h2>

<div class="box"><span class="lab">Gio, and not for the reason I expected</span>
<p>It is the only one of the three that did what was asked on the first attempt,
it stays in one process with the engine, and it ships as a single 13 MB binary
with no runtime beside it. The one thing it did worse than Electron, emoji, took
two lines to fix.</p></div>

<p>The docking question resolves more easily than expected. Neither Gio nor
Electron has a docking framework, and after building the layout in both, that
reads as an advantage rather than a gap. The Plan view is a toolbar, a map, and
two panels: expressing it as a flex tree took about forty lines and cannot drift
into the state the current UI reaches, where every panel is a dock and the
arrangement is a thing to be managed. The handful of panels that genuinely want
to move &mdash; the waterfall on a second monitor, an inspector beside the map
&mdash; are better served by a real window, which all three do in one call.</p>

<p>So: fixed layouts per view, with a small number of panels able to become
windows. That is the balance, and it needs no docking framework at all.</p>

<h3>What I would not do</h3>
<p><b>Electron</b>, despite it being the most pleasant to write. Splitting the
simulation from the drawing across an IPC boundary is a real architectural cost
for a tool whose whole job is rendering what a simulation is doing, and the
GPU compute path would need rebuilding. If MeshBench were a dashboard over a
service it would be the obvious answer. It is not one.</p>

<p><b>Cogent Core</b>, for now. The dependency alignment is genuinely
attractive and the project is moving quickly, but a framework that can save a
window geometry which prevents the application from ever appearing again, with
no diagnostic, is not one to move nineteen thousand lines onto this year. Worth
looking at again in six months.</p>

<h3>How I would sequence it</h3>
<ul>
<li>Fix the frame-thread coupling first, independently. Control verbs, headless
mode and screenshots all fight the renderer today, and that follows us to any
toolkit.</li>
<li>Port one view: <b>Bench</b>. It is tables, forms and charts with no map, so
it exercises the parts ImGui is worst at and none of the parts it is good at.</li>
<li>Keep the twelve files that draw the map, waterfall and timelines until last.
They are GPU work that any toolkit hosts, and they are the part ImGui does
adequately.</li>
<li>Both toolkits can coexist during the move: the panel registry already
abstracts a panel as a name and a function that fills it.</li>
</ul>

<h3>Honest caveats</h3>
<ul>
<li>A day of spiking is not a migration. The parts that hurt at scale &mdash;
thousands of table rows, a 60 fps waterfall, keyboard focus across a large
form &mdash; are not tested here.</li>
<li>The Cogent result is partly my inexperience with its styling model, and a
second attempt by somebody who knows it would look better than this.</li>
<li>None of this measures frame rate under real load. The waterfall at 60 fps is
the test that would actually decide it, and it is the next spike worth doing.</li>
</ul>
</div>

<footer>Three spikes on branch <code>ui-spikes</code>, each its own Go module or
npm package so none of them touch MeshBench's dependencies. Screenshots are of
the running applications on the development machine. Real data throughout: the
shipped Fife fixture, 58 nodes.</footer>
</div></body></html>
"""

out = HTML.replace("__GIOEMOJI__", img("gio-emoji.jpg")) \
          .replace("__GIO__", img("gio.jpg")) \
          .replace("__ELECTRON__", img("electron.jpg")) \
          .replace("__COGENT__", img("cogent.jpg"))
p = os.path.expanduser("~/Documents/projects/meshcoresim/spikes/comparison.html")
open(p, "w").write(out)
print("wrote", p, os.path.getsize(p) // 1024, "kB")
